// list_models.go 实现 relay.ModelLister：Anthropic /v1/models。
//
// 端点：GET https://api.anthropic.com/v1/models
// 鉴权：x-api-key + anthropic-version
// 响应：{data: [{id, type="model", display_name, ...}]}

package claude

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

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
)

// ListModels 调用 GET {base}/v1/models 拉 Anthropic 模型列表。
func (a *Adaptor) ListModels(ctx context.Context, up relay.Upstream) ([]string, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	url := base + "/v1/models"
	if strings.HasSuffix(base, "/v1") {
		url = base + "/models"
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("x-api-key", up.Credentials)
	req.Header.Set("anthropic-version", apiVersion)
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
