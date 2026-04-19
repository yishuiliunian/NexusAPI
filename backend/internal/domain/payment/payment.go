// Package payment 定义支付域的订单实体与仓储契约。
//
// 设计原则：
//   - 不绑定具体网关（Stripe/Creem/支付宝）。Gateway 字段保存网关名，
//     GatewayRef 存上游返回的 session/order id 供回查幂等。
//   - Amount 按 micro-unit（与 billing 统一），避免浮点误差。
//     网关实际收取的是 Cents（USD），由 app 层负责单位换算。
//   - 金额转配额的比率（Micro per Cent）由 app 层配置。
package payment

import (
	"context"
	"time"
)

// Status 订单生命周期。
type Status string

const (
	StatusPending  Status = "pending"  // 已创建 checkout，等待用户支付
	StatusPaid     Status = "paid"     // 网关回调确认已付款
	StatusRefunded Status = "refunded" // 已退款（暂未实现流程，保留字段）
	StatusExpired  Status = "expired"  // 超时未支付
	StatusCanceled Status = "canceled" // 用户主动取消
)

// Order 支付订单。
type Order struct {
	ID          string     `json:"id"`           // 本地订单 ID（uuid）
	UserID      uint64     `json:"user_id"`
	Amount      int64      `json:"amount"`       // 付款金额，micro（= 到账配额）
	AmountCents int64      `json:"amount_cents"` // 网关实际扣款，cents
	Currency    string     `json:"currency"`     // USD / CNY
	Gateway     string     `json:"gateway"`      // stripe / creem / alipay
	GatewayRef  string     `json:"gateway_ref"`  // 上游 session/order id（Stripe: cs_xxx）
	Mode        Mode       `json:"mode"`         // payment（一次性）/ subscription（订阅）
	PlanCode    string     `json:"plan_code"`    // 订阅模式时关联的本地 plan code；一次性支付为空
	Status      Status     `json:"status"`
	CheckoutURL string     `json:"checkout_url"` // 用户支付链接（由网关返回）
	PaidAt      *time.Time `json:"paid_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Mode 订单模式。
type Mode string

const (
	ModePayment      Mode = "payment"
	ModeSubscription Mode = "subscription"
)

// Repository 订单仓储。
type Repository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id string) (*Order, error)
	GetByGatewayRef(ctx context.Context, gateway, ref string) (*Order, error)
	ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*Order, int64, error)
	Update(ctx context.Context, o *Order) error
}
