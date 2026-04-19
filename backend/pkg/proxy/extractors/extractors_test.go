package extractors

import (
	"testing"
)

// ---------- OpenAI ----------

func TestOpenAI_JSON(t *testing.T) {
	body := []byte(`{
		"id":"r1","model":"gpt-4o",
		"choices":[{"message":{"content":"hi"}}],
		"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,
		  "prompt_tokens_details":{"cached_tokens":3},
		  "completion_tokens_details":{"reasoning_tokens":2}}
	}`)
	u := OpenAI(body, false)
	if u == nil || u.PromptTokens != 10 || u.CompletionTokens != 5 || u.TotalTokens != 15 {
		t.Errorf("basic: %+v", u)
	}
	if u.CacheReadTokens != 3 {
		t.Errorf("cached_tokens: %d", u.CacheReadTokens)
	}
	if u.ReasoningTokens != 2 {
		t.Errorf("reasoning_tokens: %d", u.ReasoningTokens)
	}
}

func TestOpenAI_SSEWithFinalUsage(t *testing.T) {
	body := []byte(`
data: {"choices":[{"delta":{"content":"hi"}}]}

data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}}

data: [DONE]
`)
	u := OpenAI(body, true)
	if u == nil || u.TotalTokens != 11 {
		t.Errorf("sse: %+v", u)
	}
}

func TestOpenAI_SSENoUsage(t *testing.T) {
	body := []byte(`data: {"choices":[{"delta":{"content":"hi"}}]}

data: [DONE]
`)
	u := OpenAI(body, true)
	if u != nil {
		t.Errorf("should be nil without usage block, got %+v", u)
	}
}

// ---------- Claude ----------

func TestClaude_JSON(t *testing.T) {
	body := []byte(`{
		"id":"msg_1","model":"claude-3-5-sonnet",
		"content":[{"type":"text","text":"hi"}],
		"usage":{"input_tokens":100,"output_tokens":20,
		         "cache_creation_input_tokens":50,"cache_read_input_tokens":30}
	}`)
	u := Claude(body, false)
	if u == nil {
		t.Fatal("expected usage")
	}
	if u.PromptTokens != 100 || u.CompletionTokens != 20 {
		t.Errorf("tokens: %+v", u)
	}
	if u.CacheWriteTokens != 50 || u.CacheReadTokens != 30 {
		t.Errorf("cache: %+v", u)
	}
	if u.TotalTokens != 120 {
		t.Errorf("total: %d", u.TotalTokens)
	}
}

func TestClaude_SSE_MessageStartAndDelta(t *testing.T) {
	// Anthropic 的 SSE：message_start 里初始 usage，message_delta 里最终 output_tokens
	body := []byte(`event: message_start
data: {"type":"message_start","message":{"id":"m","model":"claude","usage":{"input_tokens":80,"output_tokens":1,"cache_read_input_tokens":40}}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}
`)
	u := Claude(body, true)
	if u == nil {
		t.Fatal("expected usage")
	}
	if u.PromptTokens != 80 {
		t.Errorf("prompt: %d", u.PromptTokens)
	}
	if u.CompletionTokens != 15 {
		t.Errorf("completion (should reflect delta): %d", u.CompletionTokens)
	}
	if u.CacheReadTokens != 40 {
		t.Errorf("cache_read: %d", u.CacheReadTokens)
	}
	if u.TotalTokens != 95 {
		t.Errorf("total: %d", u.TotalTokens)
	}
}

func TestClaude_SSE_OneHourCache(t *testing.T) {
	body := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":5,"ephemeral_1h_input_tokens":7}}}}

data: {"type":"message_delta","usage":{"output_tokens":3}}
`)
	u := Claude(body, true)
	if u == nil {
		t.Fatal("expected usage")
	}
	if u.CacheWriteTokens != 5 {
		t.Errorf("5m cache: %d", u.CacheWriteTokens)
	}
	if u.CacheWrite1hTokens != 7 {
		t.Errorf("1h cache: %d", u.CacheWrite1hTokens)
	}
}

// ---------- Gemini ----------

func TestGemini_JSON(t *testing.T) {
	body := []byte(`{
		"candidates":[{"content":{"parts":[{"text":"hi"}]}}],
		"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":4,"totalTokenCount":16,"cachedContentTokenCount":2}
	}`)
	u := Gemini(body, false)
	if u == nil || u.PromptTokens != 12 || u.CompletionTokens != 4 || u.TotalTokens != 16 {
		t.Errorf("basic: %+v", u)
	}
	if u.CacheReadTokens != 2 {
		t.Errorf("cache: %d", u.CacheReadTokens)
	}
}

func TestGemini_SSEStream(t *testing.T) {
	body := []byte(`data: {"candidates":[{"content":{"parts":[{"text":"a"}]}}]}

data: {"candidates":[{"content":{"parts":[{"text":"b"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}
`)
	u := Gemini(body, true)
	if u == nil || u.TotalTokens != 7 {
		t.Errorf("sse: %+v", u)
	}
}

func TestGemini_EmptyMetadata(t *testing.T) {
	body := []byte(`{"usageMetadata":{}}`)
	if Gemini(body, false) != nil {
		t.Error("empty metadata should return nil")
	}
}
