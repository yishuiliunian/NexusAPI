// Package task 定义异步任务领域实体（Midjourney/Suno/视频生成等）。
//
// 与 relay 的同步/流式调用不同，任务需要：
//   1. 用户提交 → 后端同步返回 taskID
//   2. 后端 worker 定期轮询上游 → 更新 Task.Status/Progress/Result
//   3. 用户查询 taskID 看进度 / 拿结果 URL
package task

import (
	"context"
	"encoding/json"
	"time"
)

// Status 任务状态。
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

// Task 异步任务实体。
//
// 计费语义：Submit 阶段扣 Cost，若任务最终失败则走 Refund 退回。
// Refunded 字段用作幂等标记，避免重复退款；Cost 始终保留历史值。
type Task struct {
	ID         string `json:"id"` // uuid
	UserID     uint64 `json:"user_id"`
	ApiKeyID   uint64 `json:"api_key_id"`
	ChannelID  uint64 `json:"channel_id"`
	Provider   string `json:"provider"` // midjourney | suno | sora | kling | ...
	Action     string `json:"action"`   // imagine | blend | describe | music | text2video ...
	Model      string `json:"model"`
	Input      json.RawMessage `json:"input"`
	Status     Status          `json:"status"`
	Progress   int             `json:"progress"` // 0-100
	Result     json.RawMessage `json:"result"`
	ExternalID string          `json:"external_id"` // 上游 task id
	Cost       int64           `json:"cost"`        // micro，扣费金额（即使失败退款后仍保留）
	Refunded   bool            `json:"refunded"`    // 已退款幂等标记
	Error      string          `json:"error"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

// Repository 任务仓储。
type Repository interface {
	Create(ctx context.Context, t *Task) error
	GetByID(ctx context.Context, id string) (*Task, error)
	ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*Task, int64, error)
	ListAll(ctx context.Context, offset, limit int) ([]*Task, int64, error)
	Update(ctx context.Context, t *Task) error
	ListPending(ctx context.Context, limit int) ([]*Task, error)
}
