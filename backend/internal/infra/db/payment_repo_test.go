package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

func TestPaymentRepo_CreateAndLifecycle(t *testing.T) {
	r := NewPaymentOrderRepo(newTestDB(t))
	ctx := context.Background()

	o := &payment.Order{
		ID:          "local-1",
		UserID:      42,
		Amount:      20_000_000,
		AmountCents: 2000,
		Currency:    "USD",
		Gateway:     "stripe",
		GatewayRef:  "cs_test_abc",
		Mode:        payment.ModePayment,
		Status:      payment.StatusPending,
		CheckoutURL: "https://pay.stripe.com/x",
	}
	if err := r.Create(ctx, o); err != nil {
		t.Fatal(err)
	}

	got, err := r.GetByID(ctx, "local-1")
	if err != nil || got.AmountCents != 2000 {
		t.Errorf("get by id: %+v %v", got, err)
	}
	if got.Mode != payment.ModePayment {
		t.Errorf("mode: %q", got.Mode)
	}

	byRef, err := r.GetByGatewayRef(ctx, "stripe", "cs_test_abc")
	if err != nil || byRef.ID != "local-1" {
		t.Errorf("get by ref: %+v %v", byRef, err)
	}

	// 更新到 paid
	now := time.Now()
	o.Status = payment.StatusPaid
	o.PaidAt = &now
	if err := r.Update(ctx, o); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(ctx, "local-1")
	if got.Status != payment.StatusPaid || got.PaidAt == nil {
		t.Errorf("paid 状态未更新: %+v", got)
	}
}

func TestPaymentRepo_SubscriptionMode(t *testing.T) {
	r := NewPaymentOrderRepo(newTestDB(t))
	ctx := context.Background()
	o := &payment.Order{
		ID: "sub-1", UserID: 1, Gateway: "stripe",
		Mode: payment.ModeSubscription, PlanCode: "pro_monthly",
		Status: payment.StatusPending,
	}
	_ = r.Create(ctx, o)
	got, _ := r.GetByID(ctx, "sub-1")
	if got.Mode != payment.ModeSubscription || got.PlanCode != "pro_monthly" {
		t.Errorf("sub fields: %+v", got)
	}
}

func TestPaymentRepo_ListByUserAllWhenUserZero(t *testing.T) {
	r := NewPaymentOrderRepo(newTestDB(t))
	ctx := context.Background()
	_ = r.Create(ctx, &payment.Order{ID: "u1a", UserID: 1, Gateway: "stripe", Mode: payment.ModePayment, Status: payment.StatusPending})
	_ = r.Create(ctx, &payment.Order{ID: "u2a", UserID: 2, Gateway: "stripe", Mode: payment.ModePayment, Status: payment.StatusPending})

	// userID != 0：按 user 过滤
	u1, total, _ := r.ListByUser(ctx, 1, 0, 10)
	if total != 1 || len(u1) != 1 {
		t.Errorf("u1: %d/%d", len(u1), total)
	}
	// userID == 0：全量（admin 用）
	all, total, _ := r.ListByUser(ctx, 0, 0, 10)
	if total != 2 || len(all) != 2 {
		t.Errorf("all: %d/%d", len(all), total)
	}
}

func TestPaymentRepo_NotFound(t *testing.T) {
	r := NewPaymentOrderRepo(newTestDB(t))
	if _, err := r.GetByID(context.Background(), "nope"); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
	if _, err := r.GetByGatewayRef(context.Background(), "stripe", "nope"); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}
