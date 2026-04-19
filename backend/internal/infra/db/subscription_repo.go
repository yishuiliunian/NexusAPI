package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/subscription"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- PlanRepo ----------

// PlanRepo GORM 实现 subscription.PlanRepository。
type PlanRepo struct{ db *gorm.DB }

// NewPlanRepo 构造。
func NewPlanRepo(db *gorm.DB) *PlanRepo { return &PlanRepo{db: db} }

func toPlanRow(p *subscription.Plan) *PlanRow {
	return &PlanRow{
		ID:            p.ID,
		Code:          p.Code,
		Name:          p.Name,
		PriceCents:    p.PriceCents,
		Currency:      p.Currency,
		PeriodDays:    p.PeriodDays,
		IncludedQuota: p.IncludedQuota,
		GatewayRef:    p.GatewayRef,
		Enabled:       p.Enabled,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func fromPlanRow(r *PlanRow) *subscription.Plan {
	return &subscription.Plan{
		ID:            r.ID,
		Code:          r.Code,
		Name:          r.Name,
		PriceCents:    r.PriceCents,
		Currency:      r.Currency,
		PeriodDays:    r.PeriodDays,
		IncludedQuota: r.IncludedQuota,
		GatewayRef:    r.GatewayRef,
		Enabled:       r.Enabled,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func (r *PlanRepo) Upsert(ctx context.Context, p *subscription.Plan) error {
	row := toPlanRow(p)
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	row.UpdatedAt = time.Now()
	if row.ID == 0 {
		if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "create plan", err)
		}
		p.ID = row.ID
		return nil
	}
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update plan", err)
	}
	return nil
}

func (r *PlanRepo) GetByCode(ctx context.Context, code string) (*subscription.Plan, error) {
	var row PlanRow
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get plan", err)
	}
	return fromPlanRow(&row), nil
}

func (r *PlanRepo) ListEnabled(ctx context.Context) ([]*subscription.Plan, error) {
	var rows []PlanRow
	if err := r.db.WithContext(ctx).Where("enabled = ?", true).
		Order("price_cents ASC").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list plans", err)
	}
	out := make([]*subscription.Plan, 0, len(rows))
	for i := range rows {
		out = append(out, fromPlanRow(&rows[i]))
	}
	return out, nil
}

func (r *PlanRepo) List(ctx context.Context) ([]*subscription.Plan, error) {
	var rows []PlanRow
	if err := r.db.WithContext(ctx).Order("price_cents ASC").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list all plans", err)
	}
	out := make([]*subscription.Plan, 0, len(rows))
	for i := range rows {
		out = append(out, fromPlanRow(&rows[i]))
	}
	return out, nil
}

func (r *PlanRepo) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&PlanRow{}, id).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete plan", err)
	}
	return nil
}

// ---------- SubscriptionRepo ----------

type SubscriptionRepo struct{ db *gorm.DB }

func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo { return &SubscriptionRepo{db: db} }

func toSubRow(s *subscription.Subscription) *SubscriptionRow {
	return &SubscriptionRow{
		ID:               s.ID,
		UserID:           s.UserID,
		PlanCode:         s.PlanCode,
		Status:           string(s.Status),
		PeriodQuota:      s.PeriodQuota,
		GatewayRef:       s.GatewayRef,
		NextResetAt:      s.NextResetAt,
		CurrentPeriodEnd: s.CurrentPeriodEnd,
		CanceledAt:       s.CanceledAt,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}
}

func fromSubRow(r *SubscriptionRow) *subscription.Subscription {
	return &subscription.Subscription{
		ID:               r.ID,
		UserID:           r.UserID,
		PlanCode:         r.PlanCode,
		Status:           subscription.Status(r.Status),
		PeriodQuota:      r.PeriodQuota,
		GatewayRef:       r.GatewayRef,
		NextResetAt:      r.NextResetAt,
		CurrentPeriodEnd: r.CurrentPeriodEnd,
		CanceledAt:       r.CanceledAt,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func (r *SubscriptionRepo) Create(ctx context.Context, s *subscription.Subscription) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	s.UpdatedAt = time.Now()
	row := toSubRow(s)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create subscription", err)
	}
	s.ID = row.ID
	return nil
}

func (r *SubscriptionRepo) GetByID(ctx context.Context, id uint64) (*subscription.Subscription, error) {
	var row SubscriptionRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get subscription", err)
	}
	return fromSubRow(&row), nil
}

func (r *SubscriptionRepo) GetByUser(ctx context.Context, userID uint64) (*subscription.Subscription, error) {
	var row SubscriptionRow
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get sub by user", err)
	}
	return fromSubRow(&row), nil
}

func (r *SubscriptionRepo) GetByGatewayRef(ctx context.Context, ref string) (*subscription.Subscription, error) {
	var row SubscriptionRow
	err := r.db.WithContext(ctx).Where("gateway_ref = ?", ref).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get sub by gw ref", err)
	}
	return fromSubRow(&row), nil
}

func (r *SubscriptionRepo) Update(ctx context.Context, s *subscription.Subscription) error {
	s.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(toSubRow(s)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update sub", err)
	}
	return nil
}

func (r *SubscriptionRepo) ListDue(ctx context.Context, now time.Time, limit int) ([]*subscription.Subscription, error) {
	var rows []SubscriptionRow
	if err := r.db.WithContext(ctx).
		Where("status = ? AND next_reset_at <= ?", string(subscription.StatusActive), now).
		Limit(limit).Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list due subs", err)
	}
	out := make([]*subscription.Subscription, 0, len(rows))
	for i := range rows {
		out = append(out, fromSubRow(&rows[i]))
	}
	return out, nil
}
