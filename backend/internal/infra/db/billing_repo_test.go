package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- ModelPriceRepo ----------

func TestModelPriceRepo_UpsertAndGet(t *testing.T) {
	r := NewModelPriceRepo(newTestDB(t))
	ctx := context.Background()

	p := &billing.ModelPrice{
		Model:            "gpt-4o",
		Capability:       billing.CapChat,
		InputPrice:       5_000_000,
		OutputPrice:      15_000_000,
		CachePrice:       500_000,
		OutputMultiplier: 1.0,
		Enabled:          true,
	}
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 {
		t.Error("ID 未回填")
	}
	got, err := r.Get(ctx, "gpt-4o", billing.CapChat)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputPrice != 5_000_000 {
		t.Errorf("got %+v", got)
	}

	// Upsert 同 (model, capability)：应更新而非新增
	p.InputPrice = 6_000_000
	if err := r.Upsert(ctx, p); err != nil {
		t.Fatal(err)
	}
	all, _ := r.List(ctx)
	if len(all) != 1 {
		t.Errorf("Upsert 未复用记录，记录数 %d", len(all))
	}
	got, _ = r.Get(ctx, "gpt-4o", billing.CapChat)
	if got.InputPrice != 6_000_000 {
		t.Errorf("Upsert 未更新: %d", got.InputPrice)
	}
}

func TestModelPriceRepo_GetNotFound(t *testing.T) {
	r := NewModelPriceRepo(newTestDB(t))
	_, err := r.Get(context.Background(), "unknown", billing.CapChat)
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestModelPriceRepo_ModelCapUnique(t *testing.T) {
	d := newTestDB(t)
	r := NewModelPriceRepo(d)
	ctx := context.Background()
	_ = r.Upsert(ctx, &billing.ModelPrice{Model: "m", Capability: billing.CapChat, InputPrice: 100})
	// 直接插入同 (model, capability) 应违反唯一约束
	err := d.Create(&ModelPriceRow{Model: "m", Capability: string(billing.CapChat), InputPrice: 200}).Error
	if err == nil {
		t.Error("(model,capability) 唯一约束未生效")
	}
}

func TestModelPriceRepo_Delete(t *testing.T) {
	r := NewModelPriceRepo(newTestDB(t))
	ctx := context.Background()
	p := &billing.ModelPrice{Model: "m", Capability: billing.CapChat, InputPrice: 1, Enabled: true}
	_ = r.Upsert(ctx, p)
	if err := r.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := r.List(ctx)
	if len(list) != 0 {
		t.Errorf("Delete 未生效, still %d", len(list))
	}
}

// ---------- UsageRepo（只读）----------

func TestUsageRepo_ListByUser(t *testing.T) {
	d := newTestDB(t)
	r := NewUsageRepo(d)
	ctx := context.Background()

	// 直接写 row（QuotaDelta 在另一个测试文件测）
	for i := 0; i < 3; i++ {
		_ = d.Create(&UsageRow{
			UserID: 1, Model: "m", Capability: "chat",
			PromptTokens: 10, CompletionTokens: 5, Cost: 100,
			Status: "success", CreatedAt: time.Now(),
		}).Error
	}
	_ = d.Create(&UsageRow{UserID: 2, Model: "m", Capability: "chat", Status: "success", CreatedAt: time.Now()}).Error

	out, total, err := r.ListByUser(ctx, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(out) != 3 {
		t.Errorf("user 1 应 3 条, got %d total=%d", len(out), total)
	}
}

func TestUsageRepo_SumCostByUser(t *testing.T) {
	d := newTestDB(t)
	r := NewUsageRepo(d)
	ctx := context.Background()

	now := time.Now()
	_ = d.Create(&UsageRow{UserID: 1, Model: "m", Capability: "chat", Cost: 100, Status: "success", CreatedAt: now.Add(-1 * time.Hour)}).Error
	_ = d.Create(&UsageRow{UserID: 1, Model: "m", Capability: "chat", Cost: 200, Status: "success", CreatedAt: now.Add(-30 * time.Minute)}).Error
	_ = d.Create(&UsageRow{UserID: 1, Model: "m", Capability: "chat", Cost: 50, Status: "success", CreatedAt: now.Add(-48 * time.Hour)}).Error // 早于 since

	since := now.Add(-2 * time.Hour)
	sum, err := r.SumCostByUser(ctx, 1, since)
	if err != nil {
		t.Fatal(err)
	}
	if sum != 300 {
		t.Errorf("got %d, want 300 (100+200)", sum)
	}
}

// ---------- LedgerRepo（只读）----------

func TestLedgerRepo_ListByUser(t *testing.T) {
	d := newTestDB(t)
	r := NewLedgerRepo(d)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = d.Create(&LedgerRow{
			UserID: 1, Type: "topup", Amount: 1000, Balance: 1000 * int64(i+1),
			CreatedAt: time.Now(),
		}).Error
	}
	out, total, err := r.ListByUser(ctx, 1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(out) != 3 {
		t.Errorf("got %d/%d", len(out), total)
	}
}
