package passthrough

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"

	"github.com/gin-gonic/gin"
)

// ---------- extractModel ----------

func TestExtractModel(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"model":"gpt-4o","messages":[]}`, "gpt-4o"},
		{`{"model":"claude-3-5-sonnet","max_tokens":1024}`, "claude-3-5-sonnet"},
		{`{"messages":[]}`, ""},                // 无 model
		{``, ""},                              // 空 body
		{`not json`, ""},                      // 非 JSON
		{`{"model":123}`, ""},                 // model 不是字符串
		{`{"nested":{"model":"x"}}`, ""},     // 只看顶层
	}
	for _, tc := range cases {
		got := extractModel([]byte(tc.body))
		if got != tc.want {
			t.Errorf("body=%q: got %q, want %q", tc.body, got, tc.want)
		}
	}
}

// ---------- shouldFailover ----------

func TestShouldFailover(t *testing.T) {
	// preflight 错误：可 failover
	if !shouldFailover(proxy.ErrSSEPreflight) {
		t.Error("ErrSSEPreflight should be failoverable")
	}
	// wrapped preflight 错误：可 failover
	wrapped := errors.New("preflight wrapper: " + proxy.ErrSSEPreflight.Error())
	_ = wrapped
	// 真正的 wrap
	wErr := &wrapErr{err: proxy.ErrSSEPreflight}
	if !shouldFailover(wErr) {
		t.Error("wrapped ErrSSEPreflight should be failoverable via Is")
	}
	// 上游 4xx/5xx：可 failover
	upErr := &proxy.ErrUpstreamStatus{Status: 429, Body: []byte("rate limit")}
	if !shouldFailover(upErr) {
		t.Error("upstream 4xx should be failoverable")
	}
	// 普通 error：不应 failover（流已 commit 或其它不可恢复错误）
	if shouldFailover(errors.New("client disconnect")) {
		t.Error("generic error should NOT be failoverable")
	}
	if shouldFailover(nil) {
		t.Error("nil error should NOT be failoverable")
	}
}

// wrapErr 简单测试 errors.Is 用途。
type wrapErr struct{ err error }

func (w *wrapErr) Error() string { return "wrap: " + w.err.Error() }
func (w *wrapErr) Unwrap() error { return w.err }

// ---------- writeFailoverError ----------

func TestWriteFailoverError_UpstreamStatusPassthrough(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/test", strings.NewReader(""))
	up := &proxy.ErrUpstreamStatus{
		Status: 429,
		Body:   []byte(`{"error":"rate_limited"}`),
	}
	writeFailoverError(c, up)
	if rec.Code != 429 {
		t.Errorf("status=%d, want 429", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("rate_limited")) {
		t.Errorf("body should be original upstream error: %q", rec.Body.String())
	}
}

func TestWriteFailoverError_NonUpstreamGives502(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/test", strings.NewReader(""))
	writeFailoverError(c, errors.New("all candidates exhausted"))
	if rec.Code != 502 {
		t.Errorf("status=%d, want 502", rec.Code)
	}
	// body 为 nexus 统一错误结构（顶层 code/message 平铺）。
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not json: %q (%v)", rec.Body.String(), err)
	}
	if code, _ := body["code"].(string); code != string(derrors.CodeUpstream) {
		t.Errorf("code=%v want upstream_error", body["code"])
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "candidates") {
		t.Errorf("message should contain original err: %q", msg)
	}
}

func TestWriteFailoverError_UpstreamNonJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/test", strings.NewReader(""))
	up := &proxy.ErrUpstreamStatus{
		Status: 503,
		Body:   []byte("upstream overload"),
	}
	writeFailoverError(c, up)
	if rec.Code != 503 {
		t.Errorf("status=%d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("non-JSON body should use text/plain, got %q", ct)
	}
}

// ---------- baseURLOf ----------

func TestBaseURLOf(t *testing.T) {
	// 完整写法：用 channel.BaseURL
	if got := baseURLOf(&domainchannel.Channel{BaseURL: "https://custom.example.com"}, "default"); got != "https://custom.example.com" {
		t.Errorf("channel base takes priority: got %q", got)
	}
	// channel.BaseURL 为空：回落到 Route.DefaultBaseURL
	if got := baseURLOf(&domainchannel.Channel{BaseURL: ""}, "https://fallback"); got != "https://fallback" {
		t.Errorf("fallback expected: got %q", got)
	}
}
