package midjourney

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

// ---------- Submit ----------

func TestSubmit_Success(t *testing.T) {
	var seenAction, seenSecret string
	var seenBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAction = strings.TrimPrefix(r.URL.Path, "/submit/")
		seenSecret = r.Header.Get("mj-api-secret")
		seenBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"description":"ok","result":"ext-42"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	up := relay.Upstream{BaseURL: srv.URL, Credentials: "mj-secret"}
	ext, err := a.Submit(context.Background(), up, "imagine", map[string]any{"prompt": "cat"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if ext != "ext-42" {
		t.Errorf("ext id=%q", ext)
	}
	if seenAction != "imagine" {
		t.Errorf("action=%q", seenAction)
	}
	if seenSecret != "mj-secret" {
		t.Errorf("secret header missing: %q", seenSecret)
	}
	var body map[string]any
	_ = json.Unmarshal(seenBody, &body)
	if body["prompt"] != "cat" {
		t.Errorf("body forwarding failed: %v", body)
	}
}

func TestSubmit_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("upstream oops"))
	}))
	defer srv.Close()

	a := &Adaptor{}
	_, err := a.Submit(context.Background(), relay.Upstream{BaseURL: srv.URL}, "imagine", nil)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("want 500 error, got %v", err)
	}
}

func TestSubmit_EmptyResultIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"description":"quota"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	_, err := a.Submit(context.Background(), relay.Upstream{BaseURL: srv.URL}, "imagine", nil)
	if err == nil {
		t.Error("empty result should error")
	}
}

func TestSubmit_RequiresBaseURL(t *testing.T) {
	a := &Adaptor{}
	if _, err := a.Submit(context.Background(), relay.Upstream{}, "imagine", nil); err == nil {
		t.Error("missing base_url should error")
	}
}

// ---------- Query ----------

func TestQuery_SuccessMapsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/task/ext-1/fetch") {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"ext-1","status":"SUCCESS","progress":"100%","imageUrl":"https://x/1.png"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	tr, err := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "ext-1")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if tr.Status != relay.TaskSuccess || tr.Progress != 100 {
		t.Errorf("result=%+v", tr)
	}
}

func TestQuery_InProgressParsesPercent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"IN_PROGRESS","progress":"42%"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	tr, _ := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "x")
	if tr.Status != relay.TaskRunning || tr.Progress != 42 {
		t.Errorf("result=%+v", tr)
	}
}

func TestQuery_FailureCapturesReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"FAILURE","failReason":"nsfw"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	tr, _ := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "x")
	if tr.Status != relay.TaskFailed || tr.Error != "nsfw" {
		t.Errorf("result=%+v", tr)
	}
}

func TestQuery_UnknownStatusFallsToPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"WEIRD"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	tr, _ := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "x")
	if tr.Status != relay.TaskPending {
		t.Errorf("unknown status should fall to pending, got %s", tr.Status)
	}
}

func TestParseProgress(t *testing.T) {
	cases := map[string]int{
		"":       0,
		"0%":     0,
		"50%":    50,
		"100%":   100,
		"120%":   100, // 超过 100 要钳制
		"abc42%": 42,  // 非数字被跳过
	}
	for in, want := range cases {
		if got := parseProgress(in); got != want {
			t.Errorf("parseProgress(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestName(t *testing.T) {
	if (&Adaptor{}).Name() != Name || Name != "midjourney" {
		t.Error("name mismatch")
	}
}
