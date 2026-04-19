// Package audit 定义审计日志领域。
//
// 每一条管理员有副作用的 HTTP 操作（change quota / ban user / upsert channel 等）
// 都应写入一条 AuditLog 供合规追溯。读是只追加，无删除接口。
package audit

import (
	"context"
	"time"
)

// Log 审计日志。
type Log struct {
	ID        uint64    `json:"id"`
	ActorID   uint64    `json:"actor_id"` // 操作发起者（管理员 user.id）
	Action    string    `json:"action"`   // channel.create / user.ban / model_price.update / ...
	Target    string    `json:"target"`   // 目标资源描述（例如 channel:42）
	Meta      []byte    `json:"meta"`     // JSON 补充信息
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}

// Repository 审计日志仓储。
type Repository interface {
	Create(ctx context.Context, l *Log) error
	List(ctx context.Context, offset, limit int) ([]*Log, int64, error)
	ListByActor(ctx context.Context, actorID uint64, offset, limit int) ([]*Log, int64, error)
}
