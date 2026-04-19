// Package pricing 从 LiteLLM 上游同步模型单价。
//
// LiteLLM 仓库维护一份公开的 model_prices_and_context_window.json，覆盖 OpenAI /
// Anthropic / Gemini / DeepSeek / Qwen 等上百模型，字段含 mode（能力）、
// input_cost_per_token、output_cost_per_token、cache_read_input_token_cost。
//
// 同步流程（人工触发，非周期任务）：
//   1. HTTP GET JSON（带超时 + 2 次重试）
//   2. 解析为 map[string]entry，过滤非模型键（如 sample_spec）
//   3. mode → Capability 映射
//   4. USD/token → micro CNY/1M token 换算（汇率可配）
//   5. 事务：DELETE WHERE capability <> 'task' → bulk INSERT
//
// 保留 capability = task 记录的原因：LiteLLM 不覆盖 Midjourney/Suno 等按次计费模型，
// 这些由管理员手工维护。
package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
)

// DefaultLiteLLMURL LiteLLM 社区价格清单（默认源）。
const DefaultLiteLLMURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// DefaultUSDToCNY USD → CNY 汇率缺省值。
const DefaultUSDToCNY = 7.2

// Writer 价格写入抽象。DB 层的 *db.ModelPriceRepo 直接满足。
type Writer interface {
	ReplaceNonTask(ctx context.Context, prices []*billing.ModelPrice) (inserted int, deleted int, err error)
}

// Syncer LiteLLM 价格同步器。
type Syncer struct {
	HTTP      *http.Client
	Repo      Writer
	URL       string  // 为空时用 DefaultLiteLLMURL
	USDToCNY  float64 // <=0 时用 DefaultUSDToCNY
}

// New 构造 Syncer。
func New(httpClient *http.Client, repo Writer, url string, rate float64) *Syncer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if url == "" {
		url = DefaultLiteLLMURL
	}
	if rate <= 0 {
		rate = DefaultUSDToCNY
	}
	return &Syncer{HTTP: httpClient, Repo: repo, URL: url, USDToCNY: rate}
}

// Result 同步结果。
type Result struct {
	Inserted int `json:"inserted"`
	Deleted  int `json:"deleted"`
	Skipped  int `json:"skipped"` // mode 未知 / 模型名非法等原因跳过
}

// Sync 拉 LiteLLM JSON → 换算 → 覆盖本地非 task 价格。
func (s *Syncer) Sync(ctx context.Context) (*Result, error) {
	data, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	prices, skipped := s.convert(data)
	inserted, deleted, err := s.Repo.ReplaceNonTask(ctx, prices)
	if err != nil {
		return nil, err
	}
	return &Result{Inserted: inserted, Deleted: deleted, Skipped: skipped}, nil
}

// fetch 下载 LiteLLM JSON。带 60s 超时 + 最多 2 次重试。
func (s *Syncer) fetch(ctx context.Context) (map[string]entry, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, s.URL, nil)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := s.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastErr = fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		defer resp.Body.Close()
		var payload map[string]entry
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, fmt.Errorf("decode json: %w", err)
		}
		return payload, nil
	}
	if lastErr == nil {
		lastErr = errors.New("fetch litellm: unknown failure")
	}
	return nil, lastErr
}

// entry LiteLLM JSON 中单条目。只映射本项目用得上的字段。
type entry struct {
	Mode                       string  `json:"mode"`
	InputCostPerToken          float64 `json:"input_cost_per_token"`
	OutputCostPerToken         float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost    float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
}

// modeMap LiteLLM mode → billing.Capability。
// 未列出的（audio_speech → tts, audio_transcription → stt 等）映射见函数。
func modeToCapability(mode string) (billing.Capability, bool) {
	switch mode {
	case "chat", "completion":
		return billing.CapChat, true
	case "embedding":
		return billing.CapEmbedding, true
	case "rerank":
		return billing.CapRerank, true
	case "image_generation":
		return billing.CapImage, true
	case "audio_speech":
		return billing.CapTTS, true
	case "audio_transcription":
		return billing.CapSTT, true
	case "responses":
		return billing.CapResponses, true
	case "moderation", "moderations":
		return billing.CapModeration, true
	}
	return "", false
}

// convert 将 LiteLLM 条目转为本项目 ModelPrice。非法 key / mode 未知 / 价格全零跳过。
func (s *Syncer) convert(data map[string]entry) ([]*billing.ModelPrice, int) {
	out := make([]*billing.ModelPrice, 0, len(data))
	skipped := 0
	for name, e := range data {
		// LiteLLM 会有 "sample_spec" 这类示例键；跳过。
		if name == "" || name == "sample_spec" {
			skipped++
			continue
		}
		cap, ok := modeToCapability(e.Mode)
		if !ok {
			skipped++
			continue
		}
		// 全零视为无效（某些占位 entry）
		if e.InputCostPerToken == 0 && e.OutputCostPerToken == 0 {
			skipped++
			continue
		}
		out = append(out, &billing.ModelPrice{
			Model:       name,
			Capability:  cap,
			InputPrice:  s.toMicroPer1M(e.InputCostPerToken),
			OutputPrice: s.toMicroPer1M(e.OutputCostPerToken),
			CachePrice:  s.toMicroPer1M(e.CacheReadInputTokenCost),
			Enabled:     true,
		})
	}
	return out, skipped
}

// toMicroPer1M USD/token → micro-CNY/1M-token。
//
// LiteLLM: USD per single token
// 我们：micro CNY per 1,000,000 tokens。1 CNY = 1,000,000 micro。
//
//	cny_per_token      = usd_per_token * rate
//	cny_per_1M_tokens  = cny_per_token * 1_000_000
//	micro_per_1M       = cny_per_1M_tokens * 1_000_000
//	                   = usd_per_token * rate * 1e12
//
// 例：gpt-4o input = $2.5/1M = $2.5e-6/token, rate=7.2
//     micro_per_1M = 2.5e-6 * 7.2 * 1e12 = 18,000,000 ≈ 18 CNY/1M。符合预期。
func (s *Syncer) toMicroPer1M(usdPerToken float64) int64 {
	if usdPerToken <= 0 {
		return 0
	}
	v := usdPerToken * s.USDToCNY * 1e12
	return int64(math.Round(v))
}
