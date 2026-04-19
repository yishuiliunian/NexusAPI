package extractors

import (
	"encoding/json"

	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
)

// Gemini 从 Google Gemini v1beta 响应里提取 usage。
//
// JSON 模式：response.usageMetadata。
// SSE 模式（alt=sse）：每个 data 行一个完整 generateContent 响应，通常最后一个
//   含 usageMetadata。我们取扫到的最后一个有效值。
func Gemini(body []byte, isSSE bool) *proxy.Usage {
	if !isSSE {
		return parseGeminiJSON(body)
	}
	var last *proxy.Usage
	for _, line := range iterateSSEData(body) {
		if u := parseGeminiJSON(line); u != nil {
			last = u
		}
	}
	return last
}

type geminiUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	// Gemini 2.5 之后可能返回 thoughtsTokenCount
	ThoughtsTokenCount int `json:"thoughtsTokenCount,omitempty"`
}

func parseGeminiJSON(data []byte) *proxy.Usage {
	var wrapper struct {
		UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil || wrapper.UsageMetadata == nil {
		return nil
	}
	m := wrapper.UsageMetadata
	if m.PromptTokenCount == 0 && m.CandidatesTokenCount == 0 && m.TotalTokenCount == 0 {
		return nil
	}
	return &proxy.Usage{
		PromptTokens:     m.PromptTokenCount,
		CompletionTokens: m.CandidatesTokenCount,
		TotalTokens:      m.TotalTokenCount,
		CacheReadTokens:  m.CachedContentTokenCount,
		ReasoningTokens:  m.ThoughtsTokenCount,
	}
}
