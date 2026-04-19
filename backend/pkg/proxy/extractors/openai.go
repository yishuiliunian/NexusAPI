// Package extractors 提供 per-provider 的 UsageExtractor 实现。
//
// 每个 extractor 只关心"从响应末尾的 N KB 里识别 token 用量"，不做协议转换。
package extractors

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
)

// OpenAI 从 OpenAI 及其兼容协议（DeepSeek/Qwen/Moonshot/...）响应里提取 usage。
//
// JSON 模式（非流式）：顶层或 `usage` 对象里取 prompt_tokens / completion_tokens。
// SSE 模式：需要调用方 `stream_options.include_usage=true` 才会在末尾 chunk 里
// 带 `usage` 字段。扫描末尾 data 行的 JSON 提取。
func OpenAI(body []byte, isSSE bool) *proxy.Usage {
	if !isSSE {
		return parseOpenAIJSON(body)
	}
	// SSE：找最后一个含 "usage" 的 data 行
	for _, line := range iterateSSEData(body) {
		if bytes.Contains(line, []byte(`"usage"`)) {
			if u := parseOpenAIJSON(line); u != nil {
				return u
			}
		}
	}
	return nil
}

type openaiUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

func parseOpenAIJSON(data []byte) *proxy.Usage {
	// 优先解析包含 "usage" 的顶层对象
	var wrapper struct {
		Usage *openaiUsage `json:"usage"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Usage != nil {
		return openaiUsageToProxy(wrapper.Usage)
	}
	// 也支持直接传 usage 对象
	var u openaiUsage
	if err := json.Unmarshal(data, &u); err == nil && (u.PromptTokens > 0 || u.CompletionTokens > 0) {
		return openaiUsageToProxy(&u)
	}
	return nil
}

func openaiUsageToProxy(u *openaiUsage) *proxy.Usage {
	out := &proxy.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		out.CacheReadTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return out
}

// iterateSSEData 返回 body 中所有 `data: <json>` 行的 JSON 载荷（已 TrimPrefix + TrimSpace）。
// 非 JSON 行（如 "event: ..."）直接跳过。
func iterateSSEData(body []byte) [][]byte {
	var out [][]byte
	for _, raw := range bytes.Split(body, []byte("\n")) {
		line := bytes.TrimSpace(raw)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
			continue
		}
		// JSON 首字符必须是 { 或 [
		if !(bytes.HasPrefix(data, []byte("{")) || bytes.HasPrefix(data, []byte("["))) {
			continue
		}
		out = append(out, data)
	}
	return out
}

// 保留 strings 引用以避免 linter 警告
var _ = strings.TrimSpace
