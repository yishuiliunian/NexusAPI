package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/subscription"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- PlanRepo ----------

func TestPlanRepo_UpsertCreateAndUpdate(t *testing.T) {
	r := NewPlanRepo(newTestDB(t))
	ctx := context.Background()

	p := &subscription.Plan{
		Code:          "pro_monthly",
		Name:          "Pro Monthly",
		PriceCents:    1999,
		Currency:      "USD",
		PeriodDays:    30,
		IncludedQuota: 5_000_000,
		Enabled:       true,
	}
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 {
		t.Error("ID 未回填")
	}

	got, err := r.GetByCode(ctx, "pro_monthly")
	if err != nil {
		t.Fatal(err)
	}
	if got.PriceCents != 1999 {
		t.Errorf("got %+v", got)
	}

	// 再次 Upsert（带 ID）：应走 Save 路径更新而不是新建
	p.PriceCents = 2999
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	all, _ := r.List(ctx)
	if len(all) != 1 {
		t.Errorf("Upsert 未复用: %d records", len(all))
	}
	got, _ = r.GetByCode(ctx, "pro_monthly")
	if got.PriceCents != 2999 {
		t.Errorf("未更新: %d", got.PriceCents)
	}
}

func TestPlanRepo_ListEnabledFilters(t *testing.T) {
	r := NewPlanRepo(newTestDB(t))
	ctx := context.Background()

	_ = r.Upsert(ctx, &subscription.Plan{Code: "a", Name: "A", PeriodDays: 30, IncludedQuota: 100, Enabled: true, PriceCents: 100})
	_ = r.Upsert(ctx, &subscription.Plan{Code: "b", Name: "B", PeriodDays: 30, IncludedQuota: 100, Enabled: false, PriceCents: 200})

	enabled, _ := r.ListEnabled(ctx)
	if len(enabled) != 1 || enabled[0].Code != "a" {
		codes := []string{}
		for _, p := range enabled {
			codes = append(codes, p.Code+"(enabled="+boolStr(p.Enabled)+")")
		}
		t.Errorf("ListEnabled: %v", codes)
	}
	all, _ := r.List(ctx)
	if len(all) != 2 {
		t.Errorf("List 应含全部: %d", len(all))
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestPlanRepo_NotFound(t *testing.T) {
	r := NewPlanRepo(newTestDB(t))
	_, err := r.GetByCode(context.Background(), "nope")
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

// ---------- SubscriptionRepo ----------

func TestSubscriptionRepo_CreateAndGet(t *testing.T) {
	r := NewSubscriptionRepo(newTestDB(t))
	ctx := context.Background()

	future := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	s := &subscription.Subscription{
		UserID:           1,
		PlanCode:         "pro_monthly",
		Status:           subscription.StatusActive,
		PeriodQuota:      5_000_000,
		GatewayRef:       "sub_xxx",
		NextResetAt:      future,
		CurrentPeriodEnd: &future,
	}
	if err := r.Create(ctx, s); err != nil {
		t.Fatal(err)
	}
	if s.ID == 0 {
		t.Error("id 未回填")
	}
	got, err := r.GetByID(ctx, s.ID)
	if err != nil || got.PlanCode != "pro_monthly" {
		t.Errorf("%+v %v", got, err)
	}

	byUser, err := r.GetByUser(ctx, 1)
	if err != nil || byUser.ID != s.ID {
		t.Errorf("byUser: %+v %v", byUser, err)
	}

	byRef, err := r.GetByGatewayRef(ctx, "sub_xxx")
	if err != nil || byRef.ID != s.ID {
		t.Errorf("byRef: %+v %v", byRef, err)
	}
}

func TestSubscriptionRepo_ListDueFiltersStatus(t *testing.T) {
	r := NewSubscriptionRepo(newTestDB(t))
	ctx := context.Background()
	now := time.Now()

	// 即将到期 active
	_ = r.Create(ctx, &subscription.Subscription{
		UserID: 1, PlanCode: "pro", Status: subscription.StatusActive,
		NextResetAt: now.Add(-1 * time.Minute),
	})
	// 已过期但 canceled
	_ = r.Create(ctx, &subscription.Subscription{
		UserID: 2, PlanCode: "pro", Status: subscription.StatusCanceled,
		NextResetAt: now.Add(-1 * time.Minute),
	})
	// 未到期的 active
	_ = r.Create(ctx, &subscription.Subscription{
		UserID: 3, PlanCode: "pro", Status: subscription.StatusActive,
		NextResetAt: now.Add(1 * time.Hour),
	})

	due, err := r.ListDue(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].UserID != 1 {
		t.Errorf("due: %+v", due)
	}
}

func TestSubscriptionRepo_GetByUserNotFound(t *testing.T) {
	r := NewSubscriptionRepo(newTestDB(t))
	_, err := r.GetByUser(context.Background(), 999)
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}
