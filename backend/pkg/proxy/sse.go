// sse.go —— SSE 两阶段转发实现。
//
// 为什么要"两阶段"：当 provider 选路后开始建连，若上游立即返回 5xx 或在
// 首字节前 EOF，我们希望能回滚到另一个 channel 再试。但一旦把 HTTP headers
// 写给客户端就不能反悔。于是：
//
//   Phase 1 (preflight)：缓冲上游响应，观察"值得 commit 的信号"
//       （Claude 的 content_block_delta / OpenAI 的 delta.content / Gemini
//        的 candidates 等），信号出现前上游若异常，整体回滚。
//   Phase 2 (commit + stream)：写 headers + flush 缓冲 + 继续 pipe。
//
// commit 信号策略：只要看到任意一个 `data:` 行且非 [DONE]、非 error，就视作
// 有效负载开始。这比硬编码 provider 特征稳定且够用。
package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// streamSSE preflight + commit 全流程。
func (p *Proxy) streamSSE(
	w http.ResponseWriter,
	resp *http.Response,
	extractor UsageExtractor,
	latency time.Duration,
) (*Result, error) {
	defer resp.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		// 无 Flusher 能力：退化为全量缓冲再一次性写
		return p.sseNoFlush(w, resp, extractor, latency)
	}

	// ---------- Phase 1: preflight ----------
	preflight := &bytes.Buffer{}
	buf := make([]byte, 8*1024)
	var committed bool
	deadline := time.Now().Add(p.commitTimeout)

	for preflight.Len() < p.preflightBytes && time.Now().Before(deadline) {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			preflight.Write(buf[:n])
			if shouldCommit(preflight.Bytes()) {
				committed = true
				break
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				// 上游在看到 data 前就关连接：preflight 失败，外层可 failover
				return nil, fmt.Errorf("%w: eof before commit signal (got %d bytes)", ErrSSEPreflight, preflight.Len())
			}
			return nil, fmt.Errorf("%w: %v", ErrSSEPreflight, readErr)
		}
	}
	if !committed {
		// preflight 超时/缓冲满仍无信号：也视作失败
		return nil, fmt.Errorf("%w: no commit signal in %d bytes / %s", ErrSSEPreflight, preflight.Len(), p.commitTimeout)
	}
	if hasCorruptedUTF8(preflight.Bytes()) {
		return nil, fmt.Errorf("%w: corrupted utf-8", ErrSSEPreflight)
	}

	// ---------- Phase 2: commit + stream ----------
	writeResponseHeaders(w, resp)

	// tail 缓冲：始终保留最后 tailBufferBytes 字节，用于结束后摘 usage
	tail := newTailBuffer(p.tailBufferBytes)
	// 先 flush preflight
	pfBytes := preflight.Bytes()
	tail.Write(pfBytes)
	if _, err := w.Write(pfBytes); err != nil {
		return newResultOnWriteErr(resp.StatusCode, latency, tail, extractor, err)
	}
	flusher.Flush()
	written := int64(len(pfBytes))

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			tail.Write(chunk)
			if _, werr := w.Write(chunk); werr != nil {
				// 客户端断了：停止但仍返回已提取的 usage
				return &Result{
					Status:      resp.StatusCode,
					Usage:       tryExtract(extractor, tail.Bytes(), true),
					IsSSE:       true,
					ClientBytes: written,
					UpstreamLat: latency,
				}, werr
			}
			flusher.Flush()
			written += int64(n)
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			// 读上游错误：流已经开始，不能回滚，记录即可
			return &Result{
				Status:      resp.StatusCode,
				Usage:       tryExtract(extractor, tail.Bytes(), true),
				IsSSE:       true,
				ClientBytes: written,
				UpstreamLat: latency,
			}, fmt.Errorf("upstream stream: %w", readErr)
		}
	}

	return &Result{
		Status:      resp.StatusCode,
		Usage:       tryExtract(extractor, tail.Bytes(), true),
		IsSSE:       true,
		ClientBytes: written,
		UpstreamLat: latency,
	}, nil
}

// sseNoFlush 用于 ResponseWriter 不支持 Flush 的测试/特殊环境。
func (p *Proxy) sseNoFlush(
	w http.ResponseWriter,
	resp *http.Response,
	extractor UsageExtractor,
	latency time.Duration,
) (*Result, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read sse: %w", err)
	}
	writeResponseHeaders(w, resp)
	n, _ := w.Write(raw)
	return &Result{
		Status:      resp.StatusCode,
		Usage:       tryExtract(extractor, raw, true),
		IsSSE:       true,
		ClientBytes: int64(n),
		UpstreamLat: latency,
	}, nil
}

// shouldCommit 检查是否看到足够"有内容的"data 行来认为流是健康的。
//
// 策略：任意一行 `data: <json>` 且不是 [DONE]、不是纯 error、不是 ping
// 即算 commit。解析 JSON 太重——用字节扫描足够稳健。
func shouldCommit(b []byte) bool {
	i := 0
	for i < len(b) {
		j := bytes.IndexByte(b[i:], '\n')
		if j < 0 {
			return false
		}
		line := bytes.TrimSpace(b[i : i+j])
		i += j + 1
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 {
			continue
		}
		s := string(data)
		if s == "[DONE]" {
			continue
		}
		// 纯错误事件：Anthropic "event: error" 会在 data 行里带 "error"
		// 但我们只忽略明显空/DONE/单独 error 事件，真实 payload 一概通过
		if strings.HasPrefix(s, `{"type":"error"`) || strings.HasPrefix(s, `{"error"`) {
			continue
		}
		// 到这里认为是有效数据
		return true
	}
	return false
}

// hasCorruptedUTF8 检测 U+FFFD 替换字符（上游网络腐败特征）。
// 合法请求体中不应出现此字符；检测到即怀疑字节在传输中被破坏。
func hasCorruptedUTF8(b []byte) bool {
	// 明确字节序列
	return bytes.Contains(b, []byte{0xEF, 0xBF, 0xBD})
}

// ---------- tail buffer ----------

// tailBuffer 固定容量的环形缓冲，保留最近 N 字节。
type tailBuffer struct {
	cap int
	buf []byte
}

func newTailBuffer(cap int) *tailBuffer {
	if cap <= 0 {
		cap = 64 * 1024
	}
	return &tailBuffer{cap: cap, buf: make([]byte, 0, cap+4096)}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	if len(p) >= t.cap {
		// 一次写超过容量，只保留末尾 cap 字节
		t.buf = append(t.buf[:0], p[len(p)-t.cap:]...)
		return len(p), nil
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.cap {
		drop := len(t.buf) - t.cap
		copy(t.buf, t.buf[drop:])
		t.buf = t.buf[:t.cap]
	}
	return len(p), nil
}

func (t *tailBuffer) Bytes() []byte { return t.buf }

func tryExtract(extractor UsageExtractor, data []byte, isSSE bool) *Usage {
	if extractor == nil {
		return nil
	}
	defer func() { _ = recover() }() // 容错：extractor panic 不应影响中转
	return extractor(data, isSSE)
}

func newResultOnWriteErr(status int, latency time.Duration, tail *tailBuffer, ex UsageExtractor, werr error) (*Result, error) {
	return &Result{
		Status:      status,
		Usage:       tryExtract(ex, tail.Bytes(), true),
		IsSSE:       true,
		ClientBytes: 0,
		UpstreamLat: latency,
	}, werr
}

// utilities used by proxy.go imports
var _ = errors.New
var _ = utf8.ValidString
