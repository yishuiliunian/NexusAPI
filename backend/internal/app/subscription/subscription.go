// Package subscription 提供订阅下单、续费发放与 Stripe webhook 处理。
//
// 与 payment 不同的是：订阅是重复发生的，Service 接受 Stripe 的
// invoice.paid webhook 作为"本周期已收款"的信号，同时通过 ApplyPeriod
// 给用户发配额。
//
// 本地兜底：ApplyDueSubscriptions(worker 定时调用）扫 NextResetAt <= now
// 的活跃订阅，按 PeriodDays 前滚并发放 IncludedQuota。这样即便 webhook
// 丢失、或使用离线 Plan（GatewayRef 为空、本地发放）也能保证配额到位。
package subscription

import (
	"context"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	dsub "github.com/yishuiliunian/nexusapi/backend/internal/domain/subscription"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// Engine 面向外部的 billing 接口（避免 import cycle）。
type Engine interface {
	TopUp(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error
}

// Service 订阅编排。
type Service struct {
	plans dsub.PlanRepository
	subs  dsub.SubscriptionRepository
	bill  Engine
}

// NewService 构造。
func NewService(plans dsub.PlanRepository, subs dsub.SubscriptionRepository, bill Engine) *Service {
	return &Service{plans: plans, subs: subs, bill: bill}
}

// ListPlans 返回启用的套餐（供前端展示）。
func (s *Service) ListPlans(ctx context.Context) ([]*dsub.Plan, error) {
	return s.plans.ListEnabled(ctx)
}

// ListAllPlans 管理台用，返回所有套餐（含停用）。
func (s *Service) ListAllPlans(ctx context.Context) ([]*dsub.Plan, error) {
	return s.plans.List(ctx)
}

// Current 查询当前用户最近一笔订阅（可能为 canceled/expired）。
// 不存在返回 ErrNotFound；调用方据此决定是否展示"订阅"按钮。
func (s *Service) Current(ctx context.Context, userID uint64) (*dsub.Subscription, error) {
	return s.subs.GetByUser(ctx, userID)
}

// UpsertPlan 管理台 CRUD 套餐。
func (s *Service) UpsertPlan(ctx context.Context, p *dsub.Plan) error {
	if p.Code == "" || p.IncludedQuota <= 0 || p.PeriodDays <= 0 {
		return derrors.New(derrors.CodeInvalidArgument, "plan 字段缺失")
	}
	if p.Currency == "" {
		p.Currency = "USD"
	}
	return s.plans.Upsert(ctx, p)
}

// DeletePlan 管理台删除。
func (s *Service) DeletePlan(ctx context.Context, id uint64) error {
	return s.plans.Delete(ctx, id)
}

// CreateLocal 立即给用户开一个"本地发放"的订阅（不走 Stripe），
// 用于赠送活动、内部测试、或用户用兑换码升级。
// 立刻发放一次 IncludedQuota，NextResetAt = now + PeriodDays。
func (s *Service) CreateLocal(ctx context.Context, userID uint64, planCode string) (*dsub.Subscription, error) {
	p, err := s.plans.GetByCode(ctx, planCode)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return nil, derrors.New(derrors.CodeInvalidArgument, "plan 已停用")
	}
	now := time.Now()
	next := now.Add(time.Duration(p.PeriodDays) * 24 * time.Hour)
	sub := &dsub.Subscription{
		UserID:           userID,
		PlanCode:         p.Code,
		Status:           dsub.StatusActive,
		PeriodQuota:      p.IncludedQuota,
		NextResetAt:      next,
		CurrentPeriodEnd: &next,
	}
	if err := s.subs.Create(ctx, sub); err != nil {
		return nil, err
	}
	if err := s.bill.TopUp(ctx, userID, p.IncludedQuota, billing.LedgerSubscribe, planCode, "订阅期初发放"); err != nil {
		return nil, err
	}
	return sub, nil
}

// Cancel 取消订阅：不立即停额，置 canceled，CurrentPeriodEnd 保留。
func (s *Service) Cancel(ctx context.Context, userID uint64) error {
	sub, err := s.subs.GetByUser(ctx, userID)
	if err != nil {
		return err
	}
	if sub.Status == dsub.StatusCanceled {
		return nil
	}
	sub.Status = dsub.StatusCanceled
	now := time.Now()
	sub.CanceledAt = &now
	return s.subs.Update(ctx, sub)
}

// HandleInvoicePaid Stripe webhook `invoice.paid` 触发入口。
//
// gatewayRef 为 Stripe subscription id（sub_xxx）。如果本地还没这条订阅
// （首次开通），会创建一条并立即发放 IncludedQuota。
// 如果已存在，则仅 TopUp 并前滚 NextResetAt。
func (s *Service) HandleInvoicePaid(ctx context.Context, userID uint64, planCode, gatewayRef string) error {
	p, err := s.plans.GetByCode(ctx, planCode)
	if err != nil {
		return err
	}
	now := time.Now()
	next := now.Add(time.Duration(p.PeriodDays) * 24 * time.Hour)

	sub, err := s.subs.GetByGatewayRef(ctx, gatewayRef)
	if derrors.Is(err, derrors.CodeNotFound) {
		sub = &dsub.Subscription{
			UserID:           userID,
			PlanCode:         p.Code,
			Status:           dsub.StatusActive,
			PeriodQuota:      p.IncludedQuota,
			GatewayRef:       gatewayRef,
			NextResetAt:      next,
			CurrentPeriodEnd: &next,
		}
		if err := s.subs.Create(ctx, sub); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		sub.Status = dsub.StatusActive
		sub.NextResetAt = next
		sub.CurrentPeriodEnd = &next
		sub.PlanCode = p.Code
		sub.PeriodQuota = p.IncludedQuota
		if err := s.subs.Update(ctx, sub); err != nil {
			return err
		}
	}
	return s.bill.TopUp(ctx, userID, p.IncludedQuota,
		billing.LedgerSubscribe, gatewayRef, "订阅续费发放")
}

// ApplyDueSubscriptions 后台兜底：扫 NextResetAt 到期的活跃订阅，发放配额。
// 调用方（worker）以合理频率触发（建议每小时一次）。返回处理条数。
func (s *Service) ApplyDueSubscriptions(ctx context.Context, now time.Time, limit int) (int, error) {
	due, err := s.subs.ListDue(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	for _, sub := range due {
		p, err := s.plans.GetByCode(ctx, sub.PlanCode)
		if err != nil {
			continue
		}
		if err := s.bill.TopUp(ctx, sub.UserID, p.IncludedQuota,
			billing.LedgerSubscribe, sub.GatewayRef, "订阅周期兜底发放"); err != nil {
			continue
		}
		next := now.Add(time.Duration(p.PeriodDays) * 24 * time.Hour)
		sub.NextResetAt = next
		sub.CurrentPeriodEnd = &next
		_ = s.subs.Update(ctx, sub)
	}
	return len(due), nil
}

// GatewayRefFor 实现 payment.PlanLookup：把本地 plan code 解析为网关 price id。
// 未找到或该 plan 无 gateway ref 时返回空字符串（调用方应视为"仅本地发放"）。
func (s *Service) GatewayRefFor(ctx context.Context, code string) (string, error) {
	p, err := s.plans.GetByCode(ctx, code)
	if err != nil {
		return "", err
	}
	return p.GatewayRef, nil
}
