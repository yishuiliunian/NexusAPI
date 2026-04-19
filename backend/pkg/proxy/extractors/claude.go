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
	if u.CacheCreationInputTokens > 0 {
		dst.CacheWriteTokens = u.CacheCreationInputTokens
	}
	if u.CacheCreation != nil {
		if u.CacheCreation.Ephemeral5mInputTokens > 0 {
			dst.CacheWriteTokens = u.CacheCreation.Ephemeral5mInputTokens
		}
		if u.CacheCreation.Ephemeral1hInputTokens > 0 {
			dst.CacheWrite1hTokens = u.CacheCreation.Ephemeral1hInputTokens
		}
	}
}

// 避免 linter 提示
var _ = bytes.IndexByte
