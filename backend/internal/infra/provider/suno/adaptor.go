// Package suno 实现 Suno API 代理协议适配（TaskAdaptor）。
//
// 假定上游兼容 suno-api 格式：
//
//	POST {base}/submit/{action} → { task_id: "<externalID>" }
//	GET  {base}/fetch/{id}      → { status, audio_url, ... }
package suno

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

const Name = "suno"

type Adaptor struct{}

func init() { provider.RegisterTask(&Adaptor{}) }

func (a *Adaptor) Name() string { return Name }

func (a *Adaptor) Submit(ctx context.Context, up relay.Upstream, action string, input any) (string, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	if base == "" {
		return "", fmt.Errorf("suno: base_url required")
	}
	body, _ := json.Marshal(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/submit/"+action, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if up.Credentials != "" {
		req.Header.Set("Authorization", "Bearer "+up.Credentials)
	}
	resp, err := provider.HTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("suno submit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("suno upstream %d: %s", resp.StatusCode, string(raw))
	}
	var r struct {
		TaskID string `json:"task_id"`
		Data   struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("suno decode: %w", err)
	}
	if r.TaskID != "" {
		return r.TaskID, nil
	}
	return r.Data.TaskID, nil
}

func (a *Adaptor) Query(ctx context.Context, up relay.Upstream, externalID string) (*relay.TaskResult, error) {
	base := strings.TrimRight(up.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/fetch/"+externalID, nil)
	if err != nil {
		return nil, err
	}
	if up.Credentials != "" {
		req.Header.Set("Authorization", "Bearer "+up.Credentials)
	}
	resp, err := provider.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("suno query: %w", err)
	}
	defer resp.Body.Close()
	var r struct {
		Status   string `json:"status"`
		Progress int    `json:"progress"`
		AudioURL string `json:"audio_url"`
		VideoURL string `json:"video_url"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("suno decode: %w", err)
	}
	status := relay.TaskPending
	switch strings.ToLower(r.Status) {
	case "success", "complete", "completed":
		status = relay.TaskSuccess
	case "failed", "error":
		status = relay.TaskFailed
	case "running", "in_progress", "processing":
		status = relay.TaskRunning
	}
	return &relay.TaskResult{
		Status:   status,
		Progress: r.Progress,
		Result:   r,
		Error:    r.Error,
	}, nil
}
