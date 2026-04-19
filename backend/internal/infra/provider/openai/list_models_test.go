package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

func TestAdaptor_ListModels_Standard(t *testing.T) {
	var gotAuth string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{
				{"id": "gpt-4o"},
				{"id": "gpt-4o-mini"},
				{"id": ""}, // 应被过滤
			},
		})
	}))
	defer srv.Close()

	a := &Adaptor{ProviderName: "openai"}
	got, err := a.ListModels(context.Background(), relay.Upstream{
		BaseURL:     srv.URL + "/v1",
		Credentials: "sk-test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "gpt-4o" || got[1] != "gpt-4o-mini" {
		t.Fatalf("unexpected models: %v", got)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("path = %q, want /v1/models", gotPath)
	}
}

func TestAdaptor_ListModels_BaseURLWithoutV1(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	a := &Adaptor{}
	_, err := a.ListModels(context.Background(), relay.Upstream{
		BaseURL:     srv.URL,
		Credentials: "sk-x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("path = %q, want /v1/models", gotPath)
	}
}

func TestAdaptor_ListModels_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	a := &Adaptor{}
	_, err := a.ListModels(context.Background(), relay.Upstream{
		BaseURL:     srv.URL + "/v1",
		Credentials: "bad",
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
