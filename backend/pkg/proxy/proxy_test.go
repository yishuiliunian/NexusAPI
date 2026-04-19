package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- Forward: JSON 透传 ----------

func TestForward_JSONPassthrough(t *testing.T) {
	var gotAuth, gotPath, gotQuery string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"r1","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer srv.Close()

	p := New(Config{})
	up := &Upstream{BaseURL: srv.URL, APIKey: "sk-test", AuthMode: AuthBearer}

	client := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions?a=1", strings.NewReader(`{"model":"gpt-4o","messages":[]}`))

	res, err := p.Forward(client, req, up, func(body []byte, isSSE bool) *Usage {
		// 简单 extractor：JSON 模式读顶层 usage
		var w struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(body, &w)
		return &Usage{
			PromptTokens:     w.Usage.PromptTokens,
			CompletionTokens: w.Usage.CompletionTokens,
			TotalTokens:      w.Usage.TotalTokens,
		}
	})
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header: %q", gotAuth)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path: %q", gotPath)
	}
	if gotQuery != "a=1" {
		t.Errorf("query: %q", gotQuery)
	}
	if !strings.Contains(string(gotBody), `"model":"gpt-4o"`) {
		t.Errorf("body: %q", gotBody)
	}
	if res.Status != 200 || res.IsSSE {
		t.Errorf("result: %+v", res)
	}
	if res.Usage == nil || res.Usage.TotalTokens != 15 {
		t.Errorf("usage: %+v", res.Usage)
	}
	if !strings.Contains(client.Body.String(), "r1") {
		t.Errorf("client body: %q", client.Body.String())
	}
}

// ---------- Auth 模式 ----------

func TestForward_AuthModes(t *testing.T) {
	cases := []struct {
		name string
		mode AuthMode
		want map[string]string // header name → value
	}{
		{"Bearer", AuthBearer, map[string]string{"Authorization": "Bearer KEY"}},
		{"x-api-key", AuthXApiKey, map[string]string{"X-Api-Key": "KEY"}},
		{"x-goog", AuthGoogleKey, map[string]string{"X-Goog-Api-Key": "KEY"}},
		{"both", AuthBoth, map[string]string{"Authorization": "Bearer KEY", "X-Api-Key": "KEY"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := http.Header{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			p := New(Config{})
			up := &Upstream{BaseURL: srv.URL, APIKey: "KEY", AuthMode: tc.mode}
			req := httptest.NewRequest("POST", "/v1/anything", strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			if _, err := p.Forward(rec, req, up, nil); err != nil {
				t.Fatalf("forward: %v", err)
			}
			for k, v := range tc.want {
				if got.Get(k) != v {
					t.Errorf("header %s: got %q want %q", k, got.Get(k), v)
				}
			}
		})
	}
}

// ---------- 客户端 auth 被剥除 ----------

func TestForward_StripsClientAuth(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := New(Config{})
	up := &Upstream{BaseURL: srv.URL, APIKey: "sk-real", AuthMode: AuthBearer}
	req := httptest.NewRequest("POST", "/v1/x", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer sk-client-fake")
	req.Header.Set("x-api-key", "client-fake")
	_, err := p.Forward(httptest.NewRecorder(), req, up, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer sk-real" {
		t.Errorf("client auth leaked or not replaced: %q", got.Get("Authorization"))
	}
	if got.Get("X-Api-Key") != "" {
		t.Errorf("x-api-key should be absent for Bearer mode, got %q", got.Get("X-Api-Key"))
	}
}

// ---------- StripPathPrefix ----------

func TestForward_StripPathPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := New(Config{})
	up := &Upstream{BaseURL: srv.URL, APIKey: "k", AuthMode: AuthXApiKey, StripPathPrefix: "/anthropic"}
	req := httptest.NewRequest("POST", "/anthropic/v1/messages", strings.NewReader(`{}`))
	_, err := p.Forward(httptest.NewRecorder(), req, up, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path=%q want /v1/messages", gotPath)
	}
}

// ---------- ModelMap ----------

func TestForward_ModelMap(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	p := New(Config{})
	up := &Upstream{
		BaseURL:  srv.URL,
		APIKey:   "k",
		AuthMode: AuthBearer,
		ModelMap: map[string]string{"claude-sonnet": "claude-3-5-sonnet-20241022"},
	}
	req := httptest.NewRequest("POST", "/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet","max_tokens":100}`))
	_, err := p.Forward(httptest.NewRecorder(), req, up, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotBody), `"claude-3-5-sonnet-20241022"`) {
		t.Errorf("model not remapped: %s", gotBody)
	}
}

// ---------- 上游 4xx: 可 failover ----------

func TestForward_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()
	p := New(Config{})
	up := &Upstream{BaseURL: srv.URL, APIKey: "k", AuthMode: AuthBearer}
	req := httptest.NewRequest("POST", "/v1/x", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	res, err := p.Forward(rec, req, up, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if res != nil {
		t.Errorf("res should be nil on preflight failure: %+v", res)
	}
	var upErr *ErrUpstreamStatus
	if !errors.As(err, &upErr) || upErr.Status != 429 {
		t.Errorf("want ErrUpstreamStatus 429, got %v", err)
	}
	// 客户端什么也没收到（状态仍是 200 默认）
	if rec.Code != 200 {
		t.Errorf("client WriteHeader should not have fired, got %d", rec.Code)
	}
}

// ---------- SSE: 健康流 ----------

func TestForward_SSEHealthyStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		f, _ := w.(http.Flusher)
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		if f != nil {
			f.Flush()
		}
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	p := New(Config{})
	up := &Upstream{BaseURL: srv.URL, APIKey: "k", AuthMode: AuthBearer}
	req := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o","stream":true}`))
	rec := httptest.NewRecorder()
	extractor := func(body []byte, isSSE bool) *Usage {
		// 简单找 total_tokens
		if !isSSE {
			return nil
		}
		var u Usage
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if strings.Contains(data, "total_tokens") {
				var w struct {
					Usage struct {
						PromptTokens     int `json:"prompt_tokens"`
						CompletionTokens int `json:"completion_tokens"`
						TotalTokens      int `json:"total_tokens"`
					} `json:"usage"`
				}
				if json.Unmarshal([]byte(data), &w) == nil && w.Usage.TotalTokens > 0 {
					u = Usage{
						PromptTokens:     w.Usage.PromptTokens,
						CompletionTokens: w.Usage.CompletionTokens,
						TotalTokens:      w.Usage.TotalTokens,
					}
				}
			}
		}
		return &u
	}
	res, err := p.Forward(rec, req, up, extractor)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if !res.IsSSE {
		t.Error("should be SSE")
	}
	if rec.Code != 200 {
		t.Errorf("status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hi") {
		t.Errorf("client body missing content: %q", rec.Body.String())
	}
	if res.Usage == nil || res.Usage.TotalTokens != 7 {
		t.Errorf("usage: %+v", res.Usage)
	}
}

// ---------- SSE: preflight EOF → 可 failover ----------

func TestForward_SSEPreflightEOF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		// 什么都不写就关连接
	}))
	defer srv.Close()
	p := New(Config{CommitTimeout: 2 * time.Second})
	up := &Upstream{BaseURL: srv.URL, APIKey: "k", AuthMode: AuthBearer}
	req := httptest.NewRequest("POST", "/v1/x", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	res, err := p.Forward(rec, req, up, nil)
	if !errors.Is(err, ErrSSEPreflight) {
		t.Errorf("want ErrSSEPreflight, got %v", err)
	}
	if res != nil {
		t.Errorf("res should be nil on preflight fail: %+v", res)
	}
	if rec.Code != 200 {
		t.Errorf("client should not have seen a WriteHeader: %d", rec.Code)
	}
}

// ---------- SSE: 纯错误事件 preflight 不 commit ----------

func TestForward_SSEOnlyErrorEventFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, "event: error\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"error\",\"error\":{\"message\":\"x\"}}\n\n")
	}))
	defer srv.Close()
	p := New(Config{CommitTimeout: 2 * time.Second})
	up := &Upstream{BaseURL: srv.URL, APIKey: "k", AuthMode: AuthBearer}
	req := httptest.NewRequest("POST", "/v1/x", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	_, err := p.Forward(rec, req, up, nil)
	if !errors.Is(err, ErrSSEPreflight) {
		t.Errorf("want ErrSSEPreflight, got %v", err)
	}
}

// ---------- shouldCommit 纯函数 ----------

func TestShouldCommit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"only done", "data: [DONE]\n", false},
		{"only error", `data: {"error":"x"}` + "\n", false},
		{"only error typed", `data: {"type":"error","error":{}}` + "\n", false},
		{"real payload", `data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n", true},
		{"partial line", `data: {"choices":[{"delta"`, false}, // 没换行
		{"blank lines", "\n\n\n", false},
		{"heartbeat then data", ": keepalive\ndata: {\"x\":1}\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldCommit([]byte(tc.in))
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// ---------- tailBuffer ----------

func TestTailBuffer_Capacity(t *testing.T) {
	tb := newTailBuffer(10)
	tb.Write([]byte("abcdef"))
	tb.Write([]byte("ghij"))
	tb.Write([]byte("KL"))
	got := string(tb.Bytes())
	if got != "cdefghijKL" {
		t.Errorf("got %q, want cdefghijKL", got)
	}
}

func TestTailBuffer_OverflowSingleWrite(t *testing.T) {
	tb := newTailBuffer(5)
	tb.Write([]byte("1234567890"))
	if string(tb.Bytes()) != "67890" {
		t.Errorf("got %q", tb.Bytes())
	}
}
