package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	dbilling "github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes（仅测试用） ----------

type fakeDelta struct {
	balance int64
	ops     []dbilling.QuotaOp
	failOn  int // 第 N 次 Apply 返回错误；0 表示不失败
	calls   int
}

func (f *fakeDelta) Apply(ctx context.Context, op dbilling.QuotaOp) (int64, error) {
	f.calls++
	if f.failOn > 0 && f.calls == f.failOn {
		return 0, errors.New("fake apply failure")
	}
	if op.Amount < 0 && f.balance+op.Amount < 0 {
		return 0, derrors.ErrInsufficientQuota
	}
	f.balance += op.Amount
	f.ops = append(f.ops, op)
	return f.balance, nil
}

type fakeUsers struct {
	byID map[uint64]*user.User
}

func (f *fakeUsers) Create(ctx context.Context, u *user.User) error          { return nil }
func (f *fakeUsers) GetByID(ctx context.Context, id uint64) (*user.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) GetByEmail(ctx context.Context, e string) (*user.User, error) { return nil, derrors.ErrNotFound }
func (f *fakeUsers) Update(ctx context.Context, u *user.User) error               { return nil }
func (f *fakeUsers) List(ctx context.Context, o, l int) ([]*user.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUsers) SetQuota(ctx context.Context, id uint64, q int64) error { return nil }
func (f *fakeUsers) ListLowQuotaForAlert(ctx context.Context, cutoff time.Time, limit int) ([]*user.User, error) {
	return nil, nil
}

type fakeGroups struct {
	byID map[uint64]*user.Group
}

func (f *fakeGroups) Create(ctx context.Context, g *user.Group) error { return nil }
func (f *fakeGroups) GetByID(ctx context.Context, id uint64) (*user.Group, error) {
	if g, ok := f.byID[id]; ok {
		return g, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeGroups) GetByName(ctx context.Context, n string) (*user.Group, error) {
	return nil, derrors.ErrNotFound
}
func (f *fakeGroups) List(ctx context.Context) ([]*user.Group, error)  { return nil, nil }
func (f *fakeGroups) Update(ctx context.Context, g *user.Group) error  { return nil }
func (f *fakeGroups) Delete(ctx context.Context, id uint64) error      { return nil }

type fakePrices struct {
	m map[string]*dbilling.ModelPrice
}

func key(model string, cap dbilling.Capability) string { return model + "|" + string(cap) }

func (f *fakePrices) Upsert(ctx context.Context, p *dbilling.ModelPrice) error {
	if f.m == nil {
		f.m = map[string]*dbilling.ModelPrice{}
	}
	f.m[key(p.Model, p.Capability)] = p
	return nil
}
func (f *fakePrices) Get(ctx context.Context, model string, cap dbilling.Capability) (*dbilling.ModelPrice, error) {
	if p, ok := f.m[key(model, cap)]; ok {
		return p, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakePrices) List(ctx context.Context) ([]*dbilling.ModelPrice, error) { return nil, nil }
func (f *fakePrices) Delete(ctx context.Context, id uint64) error              { return nil }

// newEngine 构造测试专用 Engine（内存预占）。
func newEngine(delta *fakeDelta, users *fakeUsers, groups *fakeGroups, prices *fakePrices) *Engine {
	return NewEngine(delta, users, prices, groups, nil)
}

// ---------- Reserve ----------

func TestReserve_HappyPath(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	rid, err := e.Reserve(context.Background(), 1, 300)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rid == "" {
		t.Fatal("expected non-empty reservation id")
	}
	if d.balance != 700 {
		t.Errorf("balance=%d, want 700", d.balance)
	}
	if len(d.ops) != 1 || d.ops[0].Ledger.Type != dbilling.LedgerReserve {
		t.Errorf("expected single Reserve ledger, got %+v", d.ops)
	}
}

func TestReserve_InsufficientQuota(t *testing.T) {
	d := &fakeDelta{balance: 100}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	_, err := e.Reserve(context.Background(), 1, 500)
	if !derrors.Is(err, derrors.CodeInsufficientQuota) {
		t.Fatalf("want insufficient_quota, got %v", err)
	}
}

func TestReserve_ZeroEstimateCoercedToOne(t *testing.T) {
	d := &fakeDelta{balance: 100}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	if _, err := e.Reserve(context.Background(), 1, 0); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.balance != 99 {
		t.Errorf("balance=%d, want 99 (estimate coerced to 1)", d.balance)
	}
}

// ---------- Settle ----------

func TestSettle_ActualLessThanReserved(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	rid, _ := e.Reserve(context.Background(), 1, 500) // balance=500
	err := e.Settle(context.Background(), rid, 200, &dbilling.Usage{UserID: 1, ApiKeyID: 7, Model: "m"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// diff = 500-200 = 300 回补
	if d.balance != 800 {
		t.Errorf("balance=%d, want 800", d.balance)
	}
	last := d.ops[len(d.ops)-1]
	if last.Ledger.Type != dbilling.LedgerSettle || last.AddUsed != 200 || last.ApiKeyID != 7 {
		t.Errorf("settle op malformed: %+v", last)
	}
}

func TestSettle_ActualExceedsReserved(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	rid, _ := e.Reserve(context.Background(), 1, 100) // balance=900
	err := e.Settle(context.Background(), rid, 300, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// diff = 100-300 = -200 再扣
	if d.balance != 700 {
		t.Errorf("balance=%d, want 700", d.balance)
	}
}

func TestSettle_ReservationMissing(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	err := e.Settle(context.Background(), "no-such-id", 100, nil)
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Fatalf("want invalid_argument, got %v", err)
	}
}

func TestSettle_NegativeActualCoercedToZero(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	rid, _ := e.Reserve(context.Background(), 1, 200)
	if err := e.Settle(context.Background(), rid, -50, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// actual 被钳为 0，退回全部 200 → balance=1000
	if d.balance != 1000 {
		t.Errorf("balance=%d, want 1000", d.balance)
	}
}

// ---------- Refund ----------

func TestRefund_FullAmount(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	rid, _ := e.Reserve(context.Background(), 1, 400) // balance=600
	if err := e.Refund(context.Background(), rid); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.balance != 1000 {
		t.Errorf("balance=%d, want 1000", d.balance)
	}
	if d.ops[len(d.ops)-1].Ledger.Type != dbilling.LedgerRefund {
		t.Errorf("expected refund ledger")
	}
}

func TestRefund_IdempotentOnMissing(t *testing.T) {
	d := &fakeDelta{balance: 1000}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	if err := e.Refund(context.Background(), "gone"); err != nil {
		t.Fatalf("missing reservation should be no-op, got %v", err)
	}
	if d.balance != 1000 {
		t.Errorf("balance modified unexpectedly: %d", d.balance)
	}
}

// ---------- TopUp / Charge / Adjust ----------

func TestTopUp_RejectsNonPositive(t *testing.T) {
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	if err := e.TopUp(context.Background(), 1, 0, dbilling.LedgerTopUp, "", ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want invalid_argument, got %v", err)
	}
	if err := e.TopUp(context.Background(), 1, -5, dbilling.LedgerTopUp, "", ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want invalid_argument for negative, got %v", err)
	}
}

func TestCharge_ConvertsToNegative(t *testing.T) {
	d := &fakeDelta{balance: 500}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	if err := e.Charge(context.Background(), 1, 200, dbilling.LedgerTaskCharge, "ref", "note"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.balance != 300 {
		t.Errorf("balance=%d, want 300", d.balance)
	}
	op := d.ops[0]
	if op.Amount != -200 || op.Ledger.Type != dbilling.LedgerTaskCharge {
		t.Errorf("charge op malformed: amount=%d type=%s", op.Amount, op.Ledger.Type)
	}
}

func TestAdjust_AllowsPositiveAndNegative(t *testing.T) {
	d := &fakeDelta{balance: 500}
	e := newEngine(d, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	if err := e.Adjust(context.Background(), 1, 100, "bonus"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := e.Adjust(context.Background(), 1, -50, "penalty"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.balance != 550 {
		t.Errorf("balance=%d, want 550", d.balance)
	}
	if err := e.Adjust(context.Background(), 1, 0, ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("zero delta should error, got %v", err)
	}
}

// ---------- BuildContext ----------

func TestBuildContext_NilUser(t *testing.T) {
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	pc := e.BuildContext(context.Background(), nil)
	if pc.GroupPriceMultiplier != 1.0 || pc.UserID != 0 {
		t.Errorf("nil user should yield default ctx, got %+v", pc)
	}
}

func TestBuildContext_GroupMultiplier(t *testing.T) {
	groups := &fakeGroups{byID: map[uint64]*user.Group{
		9: {ID: 9, PriceMultiplier: 0.5},
	}}
	e := newEngine(&fakeDelta{}, &fakeUsers{}, groups, &fakePrices{})
	pc := e.BuildContext(context.Background(), &user.User{ID: 1, GroupID: 9})
	if pc.GroupPriceMultiplier != 0.5 {
		t.Errorf("want 0.5, got %v", pc.GroupPriceMultiplier)
	}
}

func TestBuildContext_BadMultiplierFallsBack(t *testing.T) {
	groups := &fakeGroups{byID: map[uint64]*user.Group{
		9: {ID: 9, PriceMultiplier: 0}, // invalid → 1.0
	}}
	e := newEngine(&fakeDelta{}, &fakeUsers{}, groups, &fakePrices{})
	pc := e.BuildContext(context.Background(), &user.User{ID: 1, GroupID: 9})
	if pc.GroupPriceMultiplier != 1.0 {
		t.Errorf("invalid multiplier should fall back to 1.0, got %v", pc.GroupPriceMultiplier)
	}
}

func TestBuildContextByID_UsesUserRepo(t *testing.T) {
	users := &fakeUsers{byID: map[uint64]*user.User{
		1: {ID: 1, GroupID: 9},
	}}
	groups := &fakeGroups{byID: map[uint64]*user.Group{
		9: {ID: 9, PriceMultiplier: 2.0},
	}}
	e := newEngine(&fakeDelta{}, users, groups, &fakePrices{})
	pc := e.BuildContextByID(context.Background(), 1)
	if pc.UserID != 1 || pc.GroupPriceMultiplier != 2.0 {
		t.Errorf("got %+v", pc)
	}
}

// ---------- Compute ----------

func TestCompute_BasicFormula(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapChat,
		InputPrice: 1_000_000, OutputPrice: 2_000_000,
		OutputMultiplier: 1, Enabled: true,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	pc := PricingContext{GroupPriceMultiplier: 1.0}
	ch := ChannelPricing{PriceMultiplier: 1.0}
	cost, err := e.Compute(context.Background(), pc, ch, &dbilling.Usage{
		Model: "m", Capability: dbilling.CapChat,
		PromptTokens: 1000, CompletionTokens: 500,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 1000 * 1_000_000 / 1_000_000 + 500 * 2_000_000 / 1_000_000 = 1000 + 1000 = 2000
	if cost != 2000 {
		t.Errorf("cost=%d, want 2000", cost)
	}
}

func TestCompute_MultiplierCompounding(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapChat,
		InputPrice: 1_000_000, OutputPrice: 0,
		OutputMultiplier: 1, Enabled: true,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	pc := PricingContext{GroupPriceMultiplier: 0.5}
	ch := ChannelPricing{PriceMultiplier: 2.0}
	cost, _ := e.Compute(context.Background(), pc, ch, &dbilling.Usage{
		Model: "m", Capability: dbilling.CapChat,
		PromptTokens: 1000,
	})
	// 1000 * 1_000_000 / 1_000_000 * 2.0 * 0.5 = 1000
	if cost != 1000 {
		t.Errorf("cost=%d, want 1000", cost)
	}
}

func TestCompute_TaskPriceAddedForTaskCap(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapTask,
		InputPrice: 0, OutputPrice: 0, OutputMultiplier: 1,
		TaskPrice: 5000, Enabled: true,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	pc := PricingContext{GroupPriceMultiplier: 2.0}
	ch := ChannelPricing{PriceMultiplier: 1.0}
	cost, _ := e.Compute(context.Background(), pc, ch, &dbilling.Usage{
		Model: "m", Capability: dbilling.CapTask,
	})
	// 0 + 5000 * 1.0 * 2.0 = 10000
	if cost != 10000 {
		t.Errorf("cost=%d, want 10000", cost)
	}
}

func TestCompute_DisabledPriceReturnsZero(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapChat,
		InputPrice: 1_000_000, Enabled: false,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	cost, _ := e.Compute(context.Background(), PricingContext{GroupPriceMultiplier: 1}, ChannelPricing{PriceMultiplier: 1}, &dbilling.Usage{
		Model: "m", Capability: dbilling.CapChat, PromptTokens: 1000,
	})
	if cost != 0 {
		t.Errorf("cost=%d, want 0 for disabled price", cost)
	}
}

func TestCompute_MissingPriceReturnsErrorInStrict(t *testing.T) {
	// 默认 strictPricing=true：缺价应返回 ErrNoPrice 而不是 (0, nil)。
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, &fakePrices{})
	cost, err := e.Compute(context.Background(), PricingContext{GroupPriceMultiplier: 1}, ChannelPricing{PriceMultiplier: 1}, &dbilling.Usage{
		Model: "unknown", Capability: dbilling.CapChat,
	})
	if err == nil {
		t.Fatalf("strict mode 缺价应返回错误，got cost=%d nil err", cost)
	}
	if cost != 0 {
		t.Errorf("错误时 cost 应为 0，got %d", cost)
	}
}

func TestCompute_MissingPriceReturnsZeroInLoose(t *testing.T) {
	// WithStrictPricing(false) 时回到旧行为：静默返回 (0, nil)。
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, &fakePrices{}).WithStrictPricing(false)
	cost, err := e.Compute(context.Background(), PricingContext{GroupPriceMultiplier: 1}, ChannelPricing{PriceMultiplier: 1}, &dbilling.Usage{
		Model: "unknown", Capability: dbilling.CapChat,
	})
	if err != nil || cost != 0 {
		t.Errorf("loose 模式缺价应 (0, nil)，got (%d, %v)", cost, err)
	}
}

func TestEnsurePriced(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapChat,
		InputPrice: 1_000_000, OutputPrice: 2_000_000, Enabled: true,
	})
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "disabled", Capability: dbilling.CapChat,
		InputPrice: 1_000_000, Enabled: false,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	if err := e.EnsurePriced(context.Background(), "m", dbilling.CapChat); err != nil {
		t.Errorf("已配置模型应通过: %v", err)
	}
	if err := e.EnsurePriced(context.Background(), "unknown", dbilling.CapChat); err == nil {
		t.Error("未配置模型应返回 ErrNoPrice")
	}
	if err := e.EnsurePriced(context.Background(), "disabled", dbilling.CapChat); err == nil {
		t.Error("已禁用模型应返回 ErrNoPrice")
	}
	// loose 模式所有情况都通过
	e2 := e.WithStrictPricing(false)
	if err := e2.EnsurePriced(context.Background(), "unknown", dbilling.CapChat); err != nil {
		t.Error("loose 模式应全部通过")
	}
}

func TestCompute_OutputMultiplierApplies(t *testing.T) {
	prices := &fakePrices{}
	_ = prices.Upsert(context.Background(), &dbilling.ModelPrice{
		Model: "m", Capability: dbilling.CapChat,
		InputPrice: 0, OutputPrice: 1_000_000,
		OutputMultiplier: 3, Enabled: true,
	})
	e := newEngine(&fakeDelta{}, &fakeUsers{}, &fakeGroups{}, prices)
	cost, _ := e.Compute(context.Background(), PricingContext{GroupPriceMultiplier: 1}, ChannelPricing{PriceMultiplier: 1}, &dbilling.Usage{
		Model: "m", Capability: dbilling.CapChat, CompletionTokens: 1000,
	})
	// 1000 * 1_000_000 * 3 / 1_000_000 = 3000
	if cost != 3000 {
		t.Errorf("cost=%d, want 3000", cost)
	}
}
