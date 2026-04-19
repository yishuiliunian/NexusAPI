package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// PaymentOrderRepo GORM 实现 payment.Repository。
type PaymentOrderRepo struct{ db *gorm.DB }

// NewPaymentOrderRepo 构造。
func NewPaymentOrderRepo(db *gorm.DB) *PaymentOrderRepo { return &PaymentOrderRepo{db: db} }

func toPaymentRow(o *payment.Order) *PaymentOrderRow {
	mode := string(o.Mode)
	if mode == "" {
		mode = string(payment.ModePayment)
	}
	return &PaymentOrderRow{
		ID:          o.ID,
		UserID:      o.UserID,
		Amount:      o.Amount,
		AmountCents: o.AmountCents,
		Currency:    o.Currency,
		Gateway:     o.Gateway,
		GatewayRef:  o.GatewayRef,
		Mode:        mode,
		PlanCode:    o.PlanCode,
		Status:      string(o.Status),
		CheckoutURL: o.CheckoutURL,
		PaidAt:      o.PaidAt,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func fromPaymentRow(r *PaymentOrderRow) *payment.Order {
	mode := payment.Mode(r.Mode)
	if mode == "" {
		mode = payment.ModePayment
	}
	return &payment.Order{
		ID:          r.ID,
		UserID:      r.UserID,
		Amount:      r.Amount,
		AmountCents: r.AmountCents,
		Currency:    r.Currency,
		Gateway:     r.Gateway,
		GatewayRef:  r.GatewayRef,
		Mode:        mode,
		PlanCode:    r.PlanCode,
		Status:      payment.Status(r.Status),
		CheckoutURL: r.CheckoutURL,
		PaidAt:      r.PaidAt,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func (r *PaymentOrderRepo) Create(ctx context.Context, o *payment.Order) error {
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	o.UpdatedAt = time.Now()
	row := toPaymentRow(o)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create order", err)
	}
	return nil
}

func (r *PaymentOrderRepo) GetByID(ctx context.Context, id string) (*payment.Order, error) {
	var row PaymentOrderRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get order", err)
	}
	return fromPaymentRow(&row), nil
}

func (r *PaymentOrderRepo) GetByGatewayRef(ctx context.Context, gateway, ref string) (*payment.Order, error) {
	var row PaymentOrderRow
	err := r.db.WithContext(ctx).
		Where("gateway = ? AND gateway_ref = ?", gateway, ref).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get order by ref", err)
	}
	return fromPaymentRow(&row), nil
}

func (r *PaymentOrderRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*payment.Order, int64, error) {
	var rows []PaymentOrderRow
	var total int64
	q := r.db.WithContext(ctx).Model(&PaymentOrderRow{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count orders", err)
	}
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list orders", err)
	}
	out := make([]*payment.Order, 0, len(rows))
	for i := range rows {
		out = append(out, fromPaymentRow(&rows[i]))
	}
	return out, total, nil
}

func (r *PaymentOrderRepo) Update(ctx context.Context, o *payment.Order) error {
	o.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(toPaymentRow(o)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update order", err)
	}
	return nil
}
