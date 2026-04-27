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

// TestUsageRepo_AggByDay 验证按天聚合返回完整 token 字段：
// prompt / completion / cache_read / cache_write_5m / cache_write_1h / reasoning + cost。
// 同时校验 userID 隔离与时间过滤。
func TestUsageRepo_AggByDay(t *testing.T) {
	d := newTestDB(t)
	r := NewUsageRepo(d)
	ctx := context.Background()

	now := time.Now()
	// user 1：今天 2 条同日数据，应当被聚合
	_ = d.Create(&UsageRow{
		UserID: 1, Model: "claude-opus-4-7", Capability: "chat",
		PromptTokens: 10, CompletionTokens: 100,
		CacheTokens: 50_000, CacheWriteTokens: 200, CacheWrite1hTokens: 500,
		ReasoningTokens: 30, Cost: 1_000,
		Status: "success", CreatedAt: now,
	}).Error
	_ = d.Create(&UsageRow{
		UserID: 1, Model: "claude-opus-4-7", Capability: "chat",
		PromptTokens: 5, CompletionTokens: 50,
		CacheTokens: 30_000, CacheWriteTokens: 100, CacheWrite1hTokens: 250,
		ReasoningTokens: 20, Cost: 500,
		Status: "success", CreatedAt: now,
	}).Error
	// user 2：同日 1 条，不应进入 user 1 的聚合
	_ = d.Create(&UsageRow{
		UserID: 2, Model: "claude-opus-4-7", Capability: "chat",
		PromptTokens: 999, CompletionTokens: 999,
		CacheTokens: 999, CacheWriteTokens: 999, CacheWrite1hTokens: 999,
		ReasoningTokens: 999, Cost: 999,
		Status: "success", CreatedAt: now,
	}).Error
	// user 1：早于 since 的 1 条，不应被聚合
	_ = d.Create(&UsageRow{
		UserID: 1, Model: "claude-opus-4-7", Capability: "chat",
		PromptTokens: 7777, CompletionTokens: 7777,
		CacheTokens: 7777, CacheWriteTokens: 7777, CacheWrite1hTokens: 7777,
		ReasoningTokens: 7777, Cost: 7777,
		Status: "success", CreatedAt: now.Add(-72 * time.Hour),
	}).Error

	since := now.Add(-24 * time.Hour)
	out, err := r.AggByDay(ctx, 1, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("应得 1 个分组，got %d: %+v", len(out), out)
	}
	got := out[0]
	if got.Requests != 2 {
		t.Errorf("requests want 2, got %d", got.Requests)
	}
	if got.PromptTokens != 15 {
		t.Errorf("prompt want 15 (10+5), got %d", got.PromptTokens)
	}
	if got.CompletionTokens != 150 {
		t.Errorf("completion want 150 (100+50), got %d", got.CompletionTokens)
	}
	if got.CacheTokens != 80_000 {
		t.Errorf("cache_read want 80000 (50000+30000), got %d", got.CacheTokens)
	}
	if got.CacheWriteTokens != 300 {
		t.Errorf("cache_write_5m want 300 (200+100), got %d", got.CacheWriteTokens)
	}
	if got.CacheWrite1hTokens != 750 {
		t.Errorf("cache_write_1h want 750 (500+250), got %d", got.CacheWrite1hTokens)
	}
	if got.ReasoningTokens != 50 {
		t.Errorf("reasoning want 50 (30+20), got %d", got.ReasoningTokens)
	}
	if got.Cost != 1_500 {
		t.Errorf("cost want 1500 (1000+500), got %d", got.Cost)
	}
}

// TestUsageRepo_AggByDay_AllZeros 验证早期 row 没有 cache 字段时聚合返回 0 而非 NULL。
func TestUsageRepo_AggByDay_AllZeros(t *testing.T) {
	d := newTestDB(t)
	r := NewUsageRepo(d)
	ctx := context.Background()

	now := time.Now()
	// 模拟"老数据"：仅 prompt/completion，cache 字段未设置
	_ = d.Create(&UsageRow{
		UserID: 1, Model: "m", Capability: "chat",
		PromptTokens: 10, CompletionTokens: 5, Cost: 100,
		Status: "success", CreatedAt: now,
	}).Error

	out, err := r.AggByDay(ctx, 1, now.Add(-1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 group, got %d", len(out))
	}
	g := out[0]
	if g.CacheTokens != 0 || g.CacheWriteTokens != 0 || g.CacheWrite1hTokens != 0 || g.ReasoningTokens != 0 {
		t.Errorf("零值缺失：cache_read=%d cache_w5m=%d cache_w1h=%d reasoning=%d",
			g.CacheTokens, g.CacheWriteTokens, g.CacheWrite1hTokens, g.ReasoningTokens)
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
