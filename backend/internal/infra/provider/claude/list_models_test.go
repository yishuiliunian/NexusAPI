package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

func TestAdaptor_ListModels(t *testing.T) {
	var gotKey, gotVersion, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-4-5-20250929", "type": "model"},
				{"id": "claude-opus-4-5-20250520", "type": "model"},
			},
		})
	}))
	defer srv.Close()

	a := &Adaptor{}
	got, err := a.ListModels(context.Background(), relay.Upstream{
		BaseURL:     srv.URL,
		Credentials: "sk-ant-x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 models, got %d: %v", len(got), got)
	}
	if gotKey != "sk-ant-x" {
		t.Fatalf("x-api-key = %q", gotKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("version = %q", gotVersion)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("path = %q", gotPath)
	}
}
