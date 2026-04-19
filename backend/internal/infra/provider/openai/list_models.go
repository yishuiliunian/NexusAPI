// list_models.go 实现 relay.ModelLister：从 OpenAI 兼容的 /v1/models 端点拉模型列表。
//
// 兼容以下情况：
//   - 标准 OpenAI：https://api.openai.com/v1 + Authorization: Bearer
//   - 所有 openaicompat 注册的供应商（deepseek/moonshot/qwen/...）
//
// Azure/Anthropic/Gemini 有各自端点，独立实现。

package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httpclient"
)

var httpClient = httpclient.New()

// defaultBaseURL 当 channel.BaseURL 为空时的兜底。仅 name=openai 用得上。
const defaultBaseURL = "https://api.openai.com/v1"

// ListModels 调用 GET {base}/models 拉模型 ID 列表。
func (a *Adaptor) ListModels(ctx context.Context, up relay.Upstream) ([]string, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	// base 可能已含 /v1，也可能没含。OpenAI 约定 /v1/models，所以统一确保路径。
	url := base + "/models"
	if !strings.Contains(base, "/v1") && !strings.Contains(base, "/compatible-mode") {
		url = base + "/v1/models"
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if up.Credentials != "" {
		req.Header.Set("Authorization", "Bearer "+up.Credentials)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	out := make([]string, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out, nil
}
