package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

func TestAdaptor_ListModels(t *testing.T) {
	var gotQuery, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("key")
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "models/gemini-1.5-pro"},
				{"name": "models/gemini-2.0-flash"},
				{"name": ""},
			},
		})
	}))
	defer srv.Close()

	a := &Adaptor{}
	got, err := a.ListModels(context.Background(), relay.Upstream{
		BaseURL:     srv.URL,
		Credentials: "AIz-x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"gemini-1.5-pro", "gemini-2.0-flash"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	if gotQuery != "AIz-x" {
		t.Fatalf("key query = %q", gotQuery)
	}
	if gotPath != "/v1beta/models" {
		t.Fatalf("path = %q", gotPath)
	}
}
