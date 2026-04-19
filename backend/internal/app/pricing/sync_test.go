package pricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
)

type fakeRepo struct {
	received []*billing.ModelPrice
}

func (r *fakeRepo) ReplaceNonTask(_ context.Context, prices []*billing.ModelPrice) (int, int, error) {
	r.received = prices
	return len(prices), 3, nil
}

func TestSyncer_Sync_Smoke(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sample_spec": map[string]any{"mode": "chat"},
			"gpt-4o": map[string]any{
				"mode":                        "chat",
				"input_cost_per_token":        2.5e-6,
				"output_cost_per_token":       10e-6,
				"cache_read_input_token_cost": 1.25e-6,
			},
			"text-embedding-3-small": map[string]any{
				"mode":                 "embedding",
				"input_cost_per_token": 0.02e-6,
			},
			"claude-sonnet-4": map[string]any{
				"mode":                  "chat",
				"input_cost_per_token":  3e-6,
				"output_cost_per_token": 15e-6,
			},
			"whisper-1": map[string]any{
				"mode":                 "audio_transcription",
				"input_cost_per_token": 0.006e-6,
			},
			"empty-prices": map[string]any{
				"mode":                  "chat",
				"input_cost_per_token":  0,
				"output_cost_per_token": 0,
			},
			"unknown-mode": map[string]any{
				"mode":                 "holographic",
				"input_cost_per_token": 1e-6,
			},
		})
	}))
	defer srv.Close()

	repo := &fakeRepo{}
	s := New(srv.Client(), repo, srv.URL)

	result, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 期望写入 4 条：gpt-4o、embedding、claude、whisper
	if result.Inserted != 4 {
		t.Fatalf("inserted=%d, want 4", result.Inserted)
	}
	if result.Skipped != 3 {
		t.Fatalf("skipped=%d, want 3", result.Skipped)
	}

	// gpt-4o input $2.5/1M → 2_500_000 micro USD per 1M tokens
	var gpt *billing.ModelPrice
	for _, p := range repo.received {
		if p.Model == "gpt-4o" {
			gpt = p
		}
	}
	if gpt == nil {
		t.Fatal("gpt-4o not inserted")
	}
	if gpt.Capability != billing.CapChat {
		t.Fatalf("gpt-4o capability = %q, want chat", gpt.Capability)
	}
	if gpt.InputPrice != 2_500_000 {
		t.Fatalf("gpt-4o input = %d, want 2_500_000 (= $2.5/1M)", gpt.InputPrice)
	}
	if gpt.OutputPrice != 10_000_000 {
		t.Fatalf("gpt-4o output = %d, want 10_000_000 (= $10/1M)", gpt.OutputPrice)
	}
	if gpt.CachePrice != 1_250_000 {
		t.Fatalf("gpt-4o cache = %d, want 1_250_000 (= $1.25/1M)", gpt.CachePrice)
	}
	if !gpt.Enabled {
		t.Fatal("gpt-4o should be enabled")
	}
}

func TestSyncer_Sync_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := New(srv.Client(), &fakeRepo{}, srv.URL)
	_, err := s.Sync(context.Background())
	if err == nil {
		t.Fatal("want error on 5xx, got nil")
	}
}

func TestToMicroPer1M(t *testing.T) {
	cases := []struct {
		usdPerToken float64
		want        int64
	}{
		{2.5e-6, 2_500_000}, // $2.5/1M
		{0.02e-6, 20_000},   // $0.02/1M
		{0, 0},
		{-1e-6, 0}, // 负数兜底
	}
	for _, c := range cases {
		got := toMicroPer1M(c.usdPerToken)
		if got != c.want {
			t.Errorf("toMicroPer1M(%g) = %d, want %d", c.usdPerToken, got, c.want)
		}
	}
}

func TestModeToCapability(t *testing.T) {
	cases := map[string]billing.Capability{
		"chat":                billing.CapChat,
		"completion":          billing.CapChat,
		"embedding":           billing.CapEmbedding,
		"rerank":              billing.CapRerank,
		"image_generation":    billing.CapImage,
		"audio_speech":        billing.CapTTS,
		"audio_transcription": billing.CapSTT,
		"responses":           billing.CapResponses,
		"moderation":          billing.CapModeration,
	}
	for mode, want := range cases {
		got, ok := modeToCapability(mode)
		if !ok || got != want {
			t.Errorf("modeToCapability(%q) = (%q, %v), want (%q, true)", mode, got, ok, want)
		}
	}
	if _, ok := modeToCapability("unknown"); ok {
		t.Error("unknown mode should return false")
	}
}
