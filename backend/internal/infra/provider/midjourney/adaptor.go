// Package midjourney 实现 Midjourney proxy 协议适配（TaskAdaptor）。
//
// 只实现任务提交/查询，不参与同步调用。
//
// 假定上游是一个兼容 midjourney-proxy 的服务：
//
//	POST {base}/submit/{action}  → { result: "<externalID>", code: 1 }
//	GET  {base}/task/{id}/fetch  → { id, status, progress, imageUrl, ... }
package midjourney

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
)

const Name = "midjourney"

// Adaptor Midjourney 任务适配器。
type Adaptor struct{}

func init() { provider.RegisterTask(&Adaptor{}) }

// Name 实现 relay.TaskAdaptor。
func (a *Adaptor) Name() string { return Name }

// Submit 向上游 proxy 提交任务，返回上游 taskID。
func (a *Adaptor) Submit(ctx context.Context, up relay.Upstream, action string, input any) (string, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		return "", fmt.Errorf("midjourney: base_url required")
	}
	body, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/submit/"+action, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if up.Credentials != "" {
		req.Header.Set("mj-api-secret", up.Credentials)
	}
	resp, err := provider.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("midjourney submit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("midjourney upstream %d: %s", resp.StatusCode, string(raw))
	}
	var r submitResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("midjourney decode: %w", err)
	}
	if r.Result == "" {
		return "", fmt.Errorf("midjourney: empty result")
	}
	return r.Result, nil
}

// Query 轮询任务状态。
func (a *Adaptor) Query(ctx context.Context, up relay.Upstream, externalID string) (*relay.TaskResult, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("midjourney: base_url required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/task/"+externalID+"/fetch", nil)
	if err != nil {
		return nil, err
	}
	if up.Credentials != "" {
		req.Header.Set("mj-api-secret", up.Credentials)
	}
	resp, err := provider.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("midjourney query: %w", err)
	}
	defer resp.Body.Close()
	var r fetchResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("midjourney decode: %w", err)
	}
	status := relay.TaskPending
	progress := 0
	switch strings.ToUpper(r.Status) {
	case "SUCCESS":
		status = relay.TaskSuccess
		progress = 100
	case "FAILURE":
		status = relay.TaskFailed
	case "IN_PROGRESS", "SUBMITTED":
		status = relay.TaskRunning
		progress = parseProgress(r.Progress)
	}
	return &relay.TaskResult{
		Status:   status,
		Progress: progress,
		Result:   r,
		Error:    r.FailReason,
	}, nil
}

type submitResp struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
	Result      string `json:"result"`
}

type fetchResp struct {
	ID         string `json:"id"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	Progress   string `json:"progress"`
	ImageURL   string `json:"imageUrl"`
	FailReason string `json:"failReason"`
}

func parseProgress(s string) int {
	s = strings.TrimSuffix(s, "%")
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	if n > 100 {
		n = 100
	}
	return n
}
