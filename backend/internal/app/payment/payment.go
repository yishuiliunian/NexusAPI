// Package payment 提供充值订单编排。
//
// 两个关键对象：
//   - Gateway —— 支付网关抽象（Stripe/Creem/支付宝/...）。
//     NexusAPI 本体不直接依赖任何网关 SDK；Gateway 实现位于 infra/payment/{name}。
//   - Service —— 业务编排：建单 → 调网关 → 写回 checkout url；
//     webhook 成功 → 幂等更新订单 → 通过 billing.Engine.TopUp 到账。
package payment

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	dpayment "github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// Gateway 支付网关抽象。
//
// NexusAPI 不绑定任何具体网关 SDK；每家实现放在 infra/payment/{name}。
type Gateway interface {
	// Name 网关标识，用于 Order.Gateway / 查重。
	Name() string

	// CreateCheckout 创建结账会话。Order.ID 作为 client_reference_id 传给网关，
	// 由网关生成 session id 写回 Order.GatewayRef 和 Order.CheckoutURL。
	CreateCheckout(ctx context.Context, o *dpayment.Order) error

	// ParseWebhook 校验 webhook 签名并解析出业务事件。
	//
	// 返回 (event, err)：err 不为 nil 表示签名无效或包体损坏——调用方必须拒绝。
	// 若 event.Type 不是本系统关心的类型，event.OrderID 为空，Service 应跳过。
	ParseWebhook(ctx context.Context, rawBody []byte, signature string) (WebhookEvent, error)
}

// WebhookEvent 网关回调抽象。
type WebhookEvent struct {
	Type    EventType
	OrderID string // 本地 Order.ID，即 client_reference_id
	RefID   string // 网关自身事件/订单 id，可用于排查
	// SubscriptionID 订阅类事件（invoice.paid / subscription.created/updated）带此字段；
	// 值为网关订阅 id（Stripe: sub_xxx）。
	SubscriptionID string
	// UserID / PlanCode 订阅事件的附加字段（通过 metadata 透传）。
	UserID   uint64
	PlanCode string
	Meta     map[string]any
}

// EventType 事件类型。
type EventType string

const (
	EventCheckoutCompleted EventType = "checkout.completed" // 一次性付款完成
	EventCheckoutExpired   EventType = "checkout.expired"
	EventRefunded          EventType = "refunded"
	EventInvoicePaid       EventType = "invoice.paid"       // 订阅续费成功
	EventSubscriptionEnded EventType = "subscription.ended" // 订阅取消/到期
	EventUnknown           EventType = "unknown"            // 非关心事件
)

// TopUpLedger 结算时写入的账本类型。
// 直接使用 billing.LedgerTopUp。

// Engine 依赖的 billing 接口（避免循环）。
type Engine interface {
	TopUp(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error
}

// SubscriptionHandler 订阅服务回调（订阅相关 webhook 事件时调用）。
// 避免 payment 直接依赖 app/subscription 包。
type SubscriptionHandler interface {
	HandleInvoicePaid(ctx context.Context, userID uint64, planCode, gatewayRef string) error
}

// PlanLookup 解析 plan code → Stripe price id（plan.GatewayRef）。
// 同样避免 import 环。
type PlanLookup interface {
	GatewayRefFor(ctx context.Context, code string) (string, error)
}

// Service 封装订单建单 + webhook 处理。
type Service struct {
	repo   dpayment.Repository
	engine Engine
	subs   SubscriptionHandler
	plans  PlanLookup
	gw     map[string]Gateway // by name
	// MicroPerCent 金额换算：每 cent 兑多少 micro 配额；默认 10_000（即 1 USD = 1_000_000 micro = 1 元等价）。
	MicroPerCent int64
}

// NewService 构造。gws 按 Name() 登记多网关；MicroPerCent 默认 10000。
func NewService(repo dpayment.Repository, engine Engine, micro int64, gws ...Gateway) *Service {
	if micro <= 0 {
		micro = 10_000
	}
	m := make(map[string]Gateway, len(gws))
	for _, g := range gws {
		if g != nil {
			m[g.Name()] = g
		}
	}
	return &Service{repo: repo, engine: engine, gw: m, MicroPerCent: micro}
}

// WithSubscriptions 注入订阅处理器（可选）。未注入时订阅类 webhook 事件被忽略。
func (s *Service) WithSubscriptions(h SubscriptionHandler, p PlanLookup) *Service {
	s.subs = h
	s.plans = p
	return s
}

// Gateways 返回已注册的网关名（运维/管理台展示用）。
func (s *Service) Gateways() []string {
	out := make([]string, 0, len(s.gw))
	for k := range s.gw {
		out = append(out, k)
	}
	return out
}

// CreateTopUp 创建一次充值订单并向网关申请 checkout URL。
//
// 调用方需在响应里把 CheckoutURL 返给前端，前端做 window.location 跳转。
// userID、amountCents、currency、gateway 四个参数必需。
func (s *Service) CreateTopUp(ctx context.Context, userID uint64, amountCents int64, currency, gatewayName string) (*dpayment.Order, error) {
	if amountCents <= 0 {
		return nil, derrors.New(derrors.CodeInvalidArgument, "金额必须大于 0")
	}
	gw, ok := s.gw[strings.ToLower(gatewayName)]
	if !ok {
		return nil, derrors.New(derrors.CodeInvalidArgument, "未配置此支付网关: "+gatewayName)
	}
	order := &dpayment.Order{
		ID:          uuid.NewString(),
		UserID:      userID,
		Amount:      amountCents * s.MicroPerCent,
		AmountCents: amountCents,
		Currency:    strings.ToUpper(currency),
		Gateway:     gw.Name(),
		Mode:        dpayment.ModePayment,
		Status:      dpayment.StatusPending,
		CreatedAt:   time.Now(),
	}
	if err := gw.CreateCheckout(ctx, order); err != nil {
		return nil, derrors.Wrap(derrors.CodeUpstream, "create checkout", err)
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// CreateSubscription 创建一个订阅结账会话。
// planCode 指向本地 Plan；Gateway.CreateCheckout 会读取 PlanLookup 得到 Stripe price id。
func (s *Service) CreateSubscription(ctx context.Context, userID uint64, planCode, gatewayName string) (*dpayment.Order, error) {
	if s.plans == nil {
		return nil, derrors.New(derrors.CodeInternal, "订阅未启用（PlanLookup 未注入）")
	}
	priceID, err := s.plans.GatewayRefFor(ctx, planCode)
	if err != nil {
		return nil, err
	}
	if priceID == "" {
		return nil, derrors.New(derrors.CodeInvalidArgument, "该套餐未配置 Stripe price id")
	}
	gw, ok := s.gw[strings.ToLower(gatewayName)]
	if !ok {
		return nil, derrors.New(derrors.CodeInvalidArgument, "未配置此支付网关: "+gatewayName)
	}
	order := &dpayment.Order{
		ID:         uuid.NewString(),
		UserID:     userID,
		Gateway:    gw.Name(),
		Mode:       dpayment.ModeSubscription,
		PlanCode:   planCode,
		Status:     dpayment.StatusPending,
		GatewayRef: priceID, // CreateCheckout 会把它读做 price id，再用 session id 覆盖
		CreatedAt:  time.Now(),
	}
	if err := gw.CreateCheckout(ctx, order); err != nil {
		return nil, derrors.Wrap(derrors.CodeUpstream, "create subscription checkout", err)
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// HandleWebhook 入口：网关名 + 原始包体 + 签名头。
//
// 实现步骤：
//  1. 找到对应 Gateway.ParseWebhook（会校验签名）
//  2. 若事件类型 == CheckoutCompleted，查订单，若 Pending 则原子改为 Paid + 调 TopUp
//  3. 已 Paid 视为重放，返回 nil（幂等）
//  4. Expired / 其他事件目前只更新状态
func (s *Service) HandleWebhook(ctx context.Context, gatewayName string, rawBody []byte, signature string) error {
	gw, ok := s.gw[strings.ToLower(gatewayName)]
	if !ok {
		return derrors.New(derrors.CodeNotFound, "未配置网关: "+gatewayName)
	}
	evt, err := gw.ParseWebhook(ctx, rawBody, signature)
	if err != nil {
		return derrors.Wrap(derrors.CodeUnauthenticated, "webhook 签名校验失败", err)
	}
	if evt.Type == EventUnknown {
		return nil // 忽略不相关事件
	}
	// InvoicePaid 可能没有 client_reference_id（续费事件）——直接走 subs.HandleInvoicePaid
	if evt.Type == EventInvoicePaid && evt.OrderID == "" {
		if s.subs == nil || evt.UserID == 0 || evt.PlanCode == "" || evt.SubscriptionID == "" {
			return nil
		}
		return s.subs.HandleInvoicePaid(ctx, evt.UserID, evt.PlanCode, evt.SubscriptionID)
	}
	if evt.OrderID == "" {
		return nil
	}
	order, err := s.repo.GetByID(ctx, evt.OrderID)
	if err != nil {
		return err
	}
	switch evt.Type {
	case EventCheckoutCompleted:
		// 订阅模式的 checkout.session.completed 只做标记，真正发配额由 invoice.paid 触发
		if order.Mode == dpayment.ModeSubscription {
			order.Status = dpayment.StatusPaid
			if order.GatewayRef == "" && evt.RefID != "" {
				order.GatewayRef = evt.RefID
			}
			return s.repo.Update(ctx, order)
		}
		if order.Status == dpayment.StatusPaid {
			return nil // 重放幂等
		}
		if order.Status != dpayment.StatusPending {
			return derrors.New(derrors.CodeInvalidArgument, "订单状态不允许支付完成: "+string(order.Status))
		}
		now := time.Now()
		order.Status = dpayment.StatusPaid
		order.PaidAt = &now
		if evt.RefID != "" && order.GatewayRef == "" {
			order.GatewayRef = evt.RefID
		}
		if err := s.repo.Update(ctx, order); err != nil {
			return err
		}
		return s.engine.TopUp(ctx, order.UserID, order.Amount,
			billing.LedgerTopUp, order.ID, "stripe 充值")
	case EventInvoicePaid:
		if s.subs == nil {
			return nil // 未启用订阅模块
		}
		userID := evt.UserID
		if userID == 0 {
			userID = order.UserID
		}
		planCode := evt.PlanCode
		if planCode == "" {
			planCode = order.PlanCode
		}
		gatewayRef := evt.SubscriptionID
		if gatewayRef == "" {
			gatewayRef = order.GatewayRef
		}
		return s.subs.HandleInvoicePaid(ctx, userID, planCode, gatewayRef)
	case EventCheckoutExpired:
		if order.Status == dpayment.StatusPending {
			order.Status = dpayment.StatusExpired
			return s.repo.Update(ctx, order)
		}
		return nil
	case EventRefunded:
		order.Status = dpayment.StatusRefunded
		return s.repo.Update(ctx, order)
	}
	return nil
}
