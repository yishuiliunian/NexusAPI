// Package proxy 实现字节级反向代理，用于中转 AI provider 原生协议（Anthropic/OpenAI/Gemini/...）。
//
// 设计要点（参考 CCProxy）：
//   - 不解析请求/响应 body，全量透传（仅读出用于 provider 识别 usage）
//   - URL 路径原样拼接到 Upstream.BaseURL 之后
//   - 客户端 auth header 替换为 Upstream.APIKey（按 AuthMode）
//   - SSE 流两阶段处理：
//       Phase 1 preflight：缓冲上游响应直到看到 commit 信号（比如 Claude 的
//       "content_block_delta"、OpenAI 的第一个有 content 的 delta），期间
//       若上游返回 5xx / EOF / UTF-8 乱码都可以回滚并让外层 failover
//       Phase 2 commit：把 preflight 缓冲一次性 flush 给客户端，之后字节 pipe
//   - Usage 由 per-provider 的 UsageExtractor 从尾部缓冲里摘取
//
// 代理不关心计费、限流、鉴权——那些由外层中间件负责。
package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AuthMode 决定上游 APIKey 放到哪个请求头。
type AuthMode string

const (
	// AuthBearer Authorization: Bearer <key>（OpenAI / 兼容协议）
	AuthBearer AuthMode = "bearer"
	// AuthXApiKey x-api-key: <key>（Anthropic / Midjourney proxy）
	AuthXApiKey AuthMode = "x-api-key"
	// AuthGoogleKey x-goog-api-key: <key>（Gemini v1beta）
	AuthGoogleKey AuthMode = "x-goog-api-key"
	// AuthQueryKey ?key=<key>（Gemini 备用）
	AuthQueryKey AuthMode = "query_key"
	// AuthBoth 同时设 Bearer + x-api-key（CCProxy 风格：上游按需取）
	AuthBoth AuthMode = "both"
)

// Upstream 描述一次转发的上游配置。
type Upstream struct {
	// BaseURL 根地址（如 https://api.anthropic.com）。
	// 请求 path（含路径前缀处理后）原样追加到其后。
	BaseURL string
	// APIKey 上游明文 key，由 AuthMode 决定放到哪个 header。
	APIKey string
	// AuthMode 见上文常量。
	AuthMode AuthMode
	// ExtraHeaders 上游要求的额外头（如 anthropic-version: 2023-06-01）。
	ExtraHeaders map[string]string
	// ModelMap 可选：客户端模型名 → 上游模型名的软映射。
	// 仅对请求体中的顶层 "model" 字段做替换；请求体不是 JSON 时跳过。
	ModelMap map[string]string
	// StripPathPrefix 从客户端请求 path 上去掉的前缀（如 "/anthropic"）。
	// 用于命名空间路径的映射：客户端 /anthropic/v1/messages → 上游 /v1/messages。
	StripPathPrefix string
}

// Usage 从响应末尾摘取的 token 消耗，交由外层 billing 使用。
type Usage struct {
	PromptTokens       int
	CompletionTokens   int
	CacheReadTokens    int // prompt_cache 命中
	CacheWriteTokens   int // prompt_cache 创建（Anthropic 5m）
	CacheWrite1hTokens int // Anthropic 1h TTL 缓存创建
	ReasoningTokens    int // OpenAI o1 / DeepSeek-R1
	TotalTokens        int
}

// UsageExtractor 从响应中提取 token 用量。
//
// isSSE=true 时 body 是累积的 SSE 全文（或尾部 N KB）；false 时是完整 JSON。
// 返回 nil 表示未能提取（非致命，外层按 0 处理）。
type UsageExtractor func(body []byte, isSSE bool) *Usage

// ErrUpstreamStatus 上游返回 4xx/5xx 状态码。
// preflight 阶段发生时，外层可以换 channel 重试。
type ErrUpstreamStatus struct {
	Status int
	Body   []byte // 响应体（可能截断）
}

func (e *ErrUpstreamStatus) Error() string {
	return fmt.Sprintf("upstream %d: %s", e.Status, truncate(string(e.Body), 512))
}

// ErrSSEPreflight preflight 阶段失败：上游 EOF / 乱码 / 未见 commit 信号。
// 外层可以安全重试（headers 尚未 commit 到客户端）。
var ErrSSEPreflight = errors.New("sse preflight failed")

// Result 转发结果。
type Result struct {
	Status      int   // 回给客户端的 HTTP 状态
	Usage       *Usage
	IsSSE       bool
	ClientBytes int64 // 发给客户端的字节总数
	UpstreamLat time.Duration
}

// Proxy 代理核心。
type Proxy struct {
	client           *http.Client
	preflightBytes   int
	tailBufferBytes  int
	commitTimeout    time.Duration // preflight 等待 commit 信号超时；默认 30s
}

// Config 构造参数。
type Config struct {
	Client           *http.Client
	PreflightBytes   int           // SSE preflight 缓冲上限，默认 256KB
	TailBufferBytes  int           // 尾部 tap 缓冲，默认 256KB（给 UsageExtractor 用）
	CommitTimeout    time.Duration // preflight 超时，默认 30s
}

// New 构造。
func New(cfg Config) *Proxy {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if cfg.PreflightBytes <= 0 {
		cfg.PreflightBytes = 256 * 1024
	}
	if cfg.TailBufferBytes <= 0 {
		cfg.TailBufferBytes = 256 * 1024
	}
	if cfg.CommitTimeout <= 0 {
		cfg.CommitTimeout = 30 * time.Second
	}
	return &Proxy{
		client:          cfg.Client,
		preflightBytes:  cfg.PreflightBytes,
		tailBufferBytes: cfg.TailBufferBytes,
		commitTimeout:   cfg.CommitTimeout,
	}
}

// Forward 执行一次转发。
//
//   - w / r 从 handler 直接传入；这里会接管 r.Body 和 w 的写。
//   - up 决定上游地址与鉴权。
//   - extractor 从响应内容里摘 usage（可为 nil，表示不需要计费 tap）。
//
// 返回 Result 含 Usage 供外层计费；err != nil 且 result == nil 表示
// preflight 前失败（可以安全 failover）；err != nil 且 result != nil
// 表示流已开始（不能再 failover，但可记录）。
func (p *Proxy) Forward(
	w http.ResponseWriter,
	r *http.Request,
	up *Upstream,
	extractor UsageExtractor,
) (*Result, error) {
	// 读 body（model 映射需要；无 body 时 rawBody=nil）
	rawBody, err := readBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	rawBody = applyModelMap(rawBody, up.ModelMap)

	// 构造上游 URL
	upstreamURL, err := rewriteURL(r, up)
	if err != nil {
		return nil, err
	}

	// 构造上游请求
	upReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	copyRequestHeaders(upReq, r)
	applyAuth(upReq, up)
	for k, v := range up.ExtraHeaders {
		upReq.Header.Set(k, v)
	}
	if len(rawBody) > 0 {
		upReq.ContentLength = int64(len(rawBody))
	}

	// 发请求
	start := time.Now()
	resp, err := p.client.Do(upReq)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}

	// 非 2xx：读 body 作为错误返回（尚未 commit headers，可以 failover）
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ErrUpstreamStatus{Status: resp.StatusCode, Body: body}
	}

	// 分支：SSE 还是 JSON
	ct := resp.Header.Get("Content-Type")
	isSSE := strings.HasPrefix(ct, "text/event-stream")

	if isSSE {
		return p.streamSSE(w, resp, extractor, latency)
	}
	return p.pipeDirect(w, resp, extractor, latency)
}

// pipeDirect 非流式响应：读完整 body，提取 usage 后写给客户端。
func (p *Proxy) pipeDirect(
	w http.ResponseWriter,
	resp *http.Response,
	extractor UsageExtractor,
	latency time.Duration,
) (*Result, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	writeResponseHeaders(w, resp)
	n, _ := w.Write(raw)

	var usage *Usage
	if extractor != nil {
		usage = extractor(raw, false)
	}
	return &Result{
		Status:      resp.StatusCode,
		Usage:       usage,
		IsSSE:       false,
		ClientBytes: int64(n),
		UpstreamLat: latency,
	}, nil
}

// ---------- helpers ----------

// rewriteURL 计算上游请求 URL：
//   BaseURL + (r.URL.Path 去掉 StripPathPrefix) + ? + RawQuery
func rewriteURL(r *http.Request, up *Upstream) (string, error) {
	if up.BaseURL == "" {
		return "", errors.New("upstream.BaseURL 为空")
	}
	base := strings.TrimRight(up.BaseURL, "/")
	path := r.URL.Path
	if up.StripPathPrefix != "" {
		path = strings.TrimPrefix(path, up.StripPathPrefix)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	u := base + path
	// QueryKey 模式：把 APIKey 附加到 query
	if up.AuthMode == AuthQueryKey {
		sep := "?"
		if strings.Contains(u, "?") || r.URL.RawQuery != "" {
			sep = "&"
		}
		if r.URL.RawQuery != "" {
			u += "?" + r.URL.RawQuery + "&key=" + up.APIKey
		} else {
			u += sep + "key=" + up.APIKey
		}
		return u, nil
	}
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	return u, nil
}

// copyRequestHeaders 把客户端 header 复制到上游请求，跳过 auth 和自动处理的项。
func copyRequestHeaders(dst, src *http.Request) {
	for k, vs := range src.Header {
		lk := strings.ToLower(k)
		switch lk {
		case "authorization", "x-api-key", "x-goog-api-key":
			continue // 由 applyAuth 替换
		case "host", "content-length":
			continue // net/http 自己管
		case "accept-encoding":
			continue // 让 Transport 决定压缩
		case "connection", "keep-alive", "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
			continue // hop-by-hop
		}
		for _, v := range vs {
			dst.Header.Add(k, v)
		}
	}
}

// applyAuth 按 AuthMode 替换上游请求的鉴权头。
func applyAuth(req *http.Request, up *Upstream) {
	key := up.APIKey
	switch up.AuthMode {
	case AuthBearer:
		req.Header.Set("Authorization", "Bearer "+key)
	case AuthXApiKey:
		req.Header.Set("x-api-key", key)
	case AuthGoogleKey:
		req.Header.Set("x-goog-api-key", key)
	case AuthBoth:
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("x-api-key", key)
	case AuthQueryKey:
		// 已在 rewriteURL 里处理
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

// writeResponseHeaders 把上游响应头/状态原样写回客户端。
func writeResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	dst := w.Header()
	for k, vs := range resp.Header {
		lk := strings.ToLower(k)
		// 跳过 hop-by-hop，和 Content-Encoding（body 已被 Transport 自动解压）
		switch lk {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailer", "transfer-encoding", "upgrade", "content-encoding":
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// applyModelMap 尝试把请求体顶层 "model" 字段按映射替换；非 JSON 或无 model 时原样返回。
func applyModelMap(body []byte, m map[string]string) []byte {
	if len(body) == 0 || len(m) == 0 {
		return body
	}
	// 避免引入完整 json 解析——简单的字节扫描即可（保留原字节顺序，替换点精确）
	// 为稳健起见仍走 json，但失败时回退原 body。
	return remapModelJSON(body, m)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// 编译期确保 Proxy 方法签名稳定。
var _ = context.TODO
