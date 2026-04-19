package extractors

import (
	"bytes"
	"encoding/json"

	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
)

// Claude 从 Anthropic Messages API 响应里提取 usage。
//
// JSON 模式：response.usage.{input_tokens, output_tokens,
//   cache_creation_input_tokens, cache_read_input_tokens}。
//
// SSE 模式（事件流）：
//   - message_start 事件：初始 usage（含 cache_read / cache_creation）
//   - message_delta 事件：最终 output_tokens（累计到结束时的总数）
//
// 策略：扫全部 data 行，累积 message_start 的字段，然后用 message_delta 里的
// output_tokens 覆盖（Anthropic 的 delta 已是累计值）。
func Claude(body []byte, isSSE bool) *proxy.Usage {
	if !isSSE {
		return parseClaudeJSON(body)
	}
	var out proxy.Usage
	matched := false
	for _, line := range iterateSSEData(body) {
		var evt struct {
			Type    string `json:"type"`
			Message *struct {
				Usage claudeUsage `json:"usage"`
			} `json:"message,omitempty"`
			Usage *claudeUsage `json:"usage,omitempty"`
		}
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}
		switch evt.Type {
		case "message_start":
			if evt.Message != nil {
				mergeClaudeUsage(&out, &evt.Message.Usage)
				matched = true
			}
		case "message_delta":
			if evt.Usage != nil {
				// message_delta 携带 output_tokens 的最终值
				if evt.Usage.OutputTokens > 0 {
					out.CompletionTokens = evt.Usage.OutputTokens
				}
				matched = true
			}
		}
	}
	if !matched {
		return nil
	}
	out.TotalTokens = out.PromptTokens + out.CompletionTokens
	return &out
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	// Anthropic 扩展：1h 缓存创建单独计
	CacheCreation *struct {
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation,omitempty"`
}

func parseClaudeJSON(data []byte) *proxy.Usage {
	var wrapper struct {
		Usage *claudeUsage `json:"usage"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil || wrapper.Usage == nil {
		return nil
	}
	out := &proxy.Usage{}
	mergeClaudeUsage(out, wrapper.Usage)
	out.TotalTokens = out.PromptTokens + out.CompletionTokens
	return out
}

func mergeClaudeUsage(dst *proxy.Usage, u *claudeUsage) {
	if u == nil {
		return
	}
	if u.InputTokens > 0 {
		dst.PromptTokens = u.InputTokens
	}
	if u.OutputTokens > 0 {
		dst.CompletionTokens = u.OutputTokens
	}
	if u.CacheReadInputTokens > 0 {
		dst.CacheReadTokens = u.CacheReadInputTokens
	}
	// Cache creation：细分字段优先（5m + 1h 分别计数），总和字段 fallback。
	//
	// Anthropic 规范：cache_creation_input_tokens = ephemeral_5m + ephemeral_1h。
	// 但某些上游（CCProxy 等中转层）返回时三者可能都相等，若盲目相加会**双算**，
	// 5m 和 1h 的 tokens 加起来 > total 就说明异常，保守按 total 全算 5m（较便宜）。
	if u.CacheCreation != nil {
		e5m := u.CacheCreation.Ephemeral5mInputTokens
		e1h := u.CacheCreation.Ephemeral1hInputTokens
		total := u.CacheCreationInputTokens
		if total > 0 && e5m+e1h > total {
			// 上游细分字段异常（5m+1h > total）→ 信任 total，保守按 5m 计费
			dst.CacheWriteTokens = total
			dst.CacheWrite1hTokens = 0
		} else {
			dst.CacheWriteTokens = e5m
			dst.CacheWrite1hTokens = e1h
			if total > 0 && e5m == 0 && e1h == 0 {
				// 有总和但细分都是 0 → 按 5m 计费（保守）
				dst.CacheWriteTokens = total
			}
		}
	} else if u.CacheCreationInputTokens > 0 {
		// 没细分对象，按 5m 计费（保守）
		dst.CacheWriteTokens = u.CacheCreationInputTokens
	}
}

// 避免 linter 提示
var _ = bytes.IndexByte
