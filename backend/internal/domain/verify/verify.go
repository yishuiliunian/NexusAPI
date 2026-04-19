// Package verify 一次性验证 token 的领域定义。
//
// 两类用途：
//   - 邮箱验证（注册后激活）
//   - 密码重置（忘记密码时发送）
//
// 设计上不区分类型，由 Purpose 字段决定；这样共享同一张表 / 同一仓储。
// Token 使用 32 字节随机并 base64url 编码，熵 ~256 bit。
package verify

import (
	"context"
	"time"
)

// Purpose 用途标记。
type Purpose string

const (
	PurposeEmailVerify  Purpose = "email_verify"
	PurposePasswordReset Purpose = "password_reset"
)

// Token 一次性 token。
type Token struct {
	ID        string    // token 字符串，主键
	UserID    uint64
	Purpose   Purpose
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// Valid 判断是否仍可使用（未过期且未消费）。
func (t *Token) Valid() bool {
	return t.UsedAt == nil && time.Now().Before(t.ExpiresAt)
}

// Repository token 仓储。
type Repository interface {
	Create(ctx context.Context, t *Token) error
	Get(ctx context.Context, id string) (*Token, error)
	// Consume 把 token 标记为已使用。成功返回 true；
	// 若 token 已使用/过期/不存在返回 false（幂等友好）。
	Consume(ctx context.Context, id string, now time.Time) (bool, error)
	// DeleteExpired 清理；worker 周期调用。
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}
