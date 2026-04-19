// list_models.go 实现 relay.ModelLister：Gemini /v1beta/models。
//
// 端点：GET https://generativelanguage.googleapis.com/v1beta/models?key={KEY}
// 响应：{models: [{name: "models/gemini-1.5-pro", displayName, ...}]}
// 注意：name 字段带 "models/" 前缀，需去除。

package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httpclient"
)

var httpClient = httpclient.New()

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// ListModels 拉 Gemini 可用模型列表。
func (a *Adaptor) ListModels(ctx context.Context, up relay.Upstream) ([]string, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	endpoint := base + "/v1beta/models"
	if strings.Contains(base, "/v1beta") || strings.Contains(base, "/v1") {
		endpoint = base + "/models"
	}

	q := url.Values{}
	q.Set("key", up.Credentials)
	q.Set("pageSize", "1000")
	endpoint = endpoint + "?" + q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
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
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	out := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}
