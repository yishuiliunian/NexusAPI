package suno

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

// ---------- Submit ----------

func TestSubmit_TopLevelTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer suno-key" {
			t.Errorf("auth=%q", auth)
		}
		if !strings.HasSuffix(r.URL.Path, "/submit/music") {
			t.Errorf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"task_id":"suno-1"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	ext, err := a.Submit(context.Background(), relay.Upstream{BaseURL: srv.URL, Credentials: "suno-key"}, "music", map[string]any{"prompt": "sad piano"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if ext != "suno-1" {
		t.Errorf("id=%q", ext)
	}
}

func TestSubmit_NestedTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"task_id":"nested-1"}}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	ext, err := a.Submit(context.Background(), relay.Upstream{BaseURL: srv.URL}, "music", nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if ext != "nested-1" {
		t.Errorf("id=%q", ext)
	}
}

func TestSubmit_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	a := &Adaptor{}
	_, err := a.Submit(context.Background(), relay.Upstream{BaseURL: srv.URL}, "music", nil)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("want 502 error, got %v", err)
	}
}

func TestSubmit_RequiresBaseURL(t *testing.T) {
	a := &Adaptor{}
	if _, err := a.Submit(context.Background(), relay.Upstream{}, "music", nil); err == nil {
		t.Error("missing base_url should error")
	}
}

// ---------- Query ----------

func TestQuery_StatusMapping(t *testing.T) {
	cases := map[string]relay.TaskStatus{
		"complete":    relay.TaskSuccess,
		"success":     relay.TaskSuccess,
		"completed":   relay.TaskSuccess,
		"failed":      relay.TaskFailed,
		"error":       relay.TaskFailed,
		"running":     relay.TaskRunning,
		"in_progress": relay.TaskRunning,
		"processing":  relay.TaskRunning,
		"queued":      relay.TaskPending,
	}
	for upstream, want := range cases {
		want := want // capture
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"` + upstream + `","progress":50,"audio_url":"https://x/1.mp3"}`))
		}))
		a := &Adaptor{}
		tr, err := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "t1")
		srv.Close()
		if err != nil {
			t.Errorf("upstream=%s err=%v", upstream, err)
			continue
		}
		if tr.Status != want {
			t.Errorf("upstream=%s: got %s, want %s", upstream, tr.Status, want)
		}
	}
}

func TestQuery_ErrorFieldPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"failed","error":"rate limit"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	tr, _ := a.Query(context.Background(), relay.Upstream{BaseURL: srv.URL}, "t")
	if tr.Error != "rate limit" {
		t.Errorf("error=%q", tr.Error)
	}
}

func TestName(t *testing.T) {
	if (&Adaptor{}).Name() != Name || Name != "suno" {
		t.Error("name mismatch")
	}
}
