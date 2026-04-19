// Package subscription 定义订阅域：Plan + Subscription。
//
// 策略：仅支持 Stripe 原生订阅（mode=subscription）；我们本地只记录状态，
// 续费/扣款由 Stripe 自动调度，invoice.paid webhook 触发本地 TopUp。
// 这样避免在本地再实现 cron + 扣款链路。
//
// 本地兜底（Stripe 挂了）：worker 可每小时扫 NextResetAt <= now 的订阅，
// 发放配额并前滚 NextResetAt；此逻辑置于 cmd/worker。
package subscription

import (
	"context"
	"time"
)

// Plan 订阅套餐定义。
type Plan struct {
	ID            uint64    `json:"id"`
	Code          string    `json:"code"` // pro_monthly
	Name          string    `json:"name"`
	PriceCents    int64     `json:"price_cents"`
	Currency      string    `json:"currency"`
	PeriodDays    int       `json:"period_days"`    // 30 / 365
	IncludedQuota int64     `json:"included_quota"` // 每周期发放的 micro 配额
	GatewayRef    string    `json:"gateway_ref"`    // Stripe price id（price_xxx）
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Status 订阅生命周期。
type Status string

const (
	StatusActive   Status = "active"
	StatusCanceled Status = "canceled"
	StatusExpired  Status = "expired"
	StatusPastDue  Status = "past_due"
)

// Subscription 用户订阅实例。
type Subscription struct {
	ID               uint64     `json:"id"`
	UserID           uint64     `json:"user_id"`
	PlanCode         string     `json:"plan_code"`
	Status           Status     `json:"status"`
	PeriodQuota      int64      `json:"period_quota"` // 本周期应发配额
	GatewayRef       string     `json:"gateway_ref"`  // Stripe subscription id（sub_xxx）
	NextResetAt      time.Time  `json:"next_reset_at"`
	CurrentPeriodEnd *time.Time `json:"current_period_end"`
	CanceledAt       *time.Time `json:"canceled_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// PlanRepository 套餐仓储。
type PlanRepository interface {
	Upsert(ctx context.Context, p *Plan) error
	GetByCode(ctx context.Context, code string) (*Plan, error)
	ListEnabled(ctx context.Context) ([]*Plan, error)
	List(ctx context.Context) ([]*Plan, error)
	Delete(ctx context.Context, id uint64) error
}

// SubscriptionRepository 订阅仓储。
type SubscriptionRepository interface {
	Create(ctx context.Context, s *Subscription) error
	GetByID(ctx context.Context, id uint64) (*Subscription, error)
	GetByUser(ctx context.Context, userID uint64) (*Subscription, error)
	GetByGatewayRef(ctx context.Context, ref string) (*Subscription, error)
	Update(ctx context.Context, s *Subscription) error
	// ListDue 返回所有 NextResetAt <= now 的活跃订阅，用于 worker 兜底发放。
	ListDue(ctx context.Context, now time.Time, limit int) ([]*Subscription, error)
}
