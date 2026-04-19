// subscription_test.go —— 订阅 Service 核心路径。
package subscription

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	dsub "github.com/yishuiliunian/nexusapi/backend/internal/domain/subscription"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakePlans struct {
	byCode map[string]*dsub.Plan
	nextID uint64
}

func newFakePlans() *fakePlans { return &fakePlans{byCode: map[string]*dsub.Plan{}, nextID: 1} }
func (f *fakePlans) Upsert(_ context.Context, p *dsub.Plan) error {
	if p.ID == 0 {
		p.ID = f.nextID
		f.nextID++
	}
	cp := *p
	f.byCode[p.Code] = &cp
	return nil
}
func (f *fakePlans) GetByCode(_ context.Context, code string) (*dsub.Plan, error) {
	if p, ok := f.byCode[code]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakePlans) ListEnabled(_ context.Context) ([]*dsub.Plan, error) {
	out := []*dsub.Plan{}
	for _, p := range f.byCode {
		if p.Enabled {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakePlans) List(_ context.Context) ([]*dsub.Plan, error) {
	out := []*dsub.Plan{}
	for _, p := range f.byCode {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}
func (f *fakePlans) Delete(_ context.Context, id uint64) error {
	for k, p := range f.byCode {
		if p.ID == id {
			delete(f.byCode, k)
			return nil
		}
	}
	return derrors.ErrNotFound
}

type fakeSubs struct {
	mu     sync.Mutex
	byID   map[uint64]*dsub.Subscription
	byUser map[uint64]*dsub.Subscription
	byRef  map[string]*dsub.Subscription
	nextID uint64
}

func newFakeSubs() *fakeSubs {
	return &fakeSubs{byID: map[uint64]*dsub.Subscription{}, byUser: map[uint64]*dsub.Subscription{}, byRef: map[string]*dsub.Subscription{}, nextID: 1}
}
func (f *fakeSubs) Create(_ context.Context, s *dsub.Subscription) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s.ID = f.nextID
	f.nextID++
	cp := *s
	f.byID[s.ID] = &cp
	f.byUser[s.UserID] = &cp
	if s.GatewayRef != "" {
		f.byRef[s.GatewayRef] = &cp
	}
	return nil
}
func (f *fakeSubs) GetByID(_ context.Context, id uint64) (*dsub.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeSubs) GetByUser(_ context.Context, uid uint64) (*dsub.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.byUser[uid]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeSubs) GetByGatewayRef(_ context.Context, ref string) (*dsub.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.byRef[ref]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeSubs) Update(_ context.Context, s *dsub.Subscription) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.byID[s.ID]
	if !ok {
		return derrors.ErrNotFound
	}
	*stored = *s
	if s.GatewayRef != "" {
		f.byRef[s.GatewayRef] = stored
	}
	return nil
}
func (f *fakeSubs) ListDue(_ context.Context, now time.Time, limit int) ([]*dsub.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*dsub.Subscription{}
	for _, s := range f.byID {
		if s.Status == dsub.StatusActive && !s.NextResetAt.After(now) && len(out) < limit {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}

type fakeBill struct {
	mu    sync.Mutex
	calls []topUpCall
}
type topUpCall struct {
	UserID uint64
	Amount int64
	Type   billing.LedgerType
	RefID  string
	Note   string
}

func (f *fakeBill) TopUp(_ context.Context, uid uint64, amt int64, typ billing.LedgerType, ref, note string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, topUpCall{uid, amt, typ, ref, note})
	return nil
}

// ---------- Tests ----------

func seedPlan(t *testing.T, plans *fakePlans, code string, quota int64) *dsub.Plan {
	t.Helper()
	p := &dsub.Plan{Code: code, Name: code, PeriodDays: 30, IncludedQuota: quota, Enabled: true}
	_ = plans.Upsert(context.Background(), p)
	return p
}

func TestListPlans_OnlyEnabled(t *testing.T) {
	plans := newFakePlans()
	_ = plans.Upsert(context.Background(), &dsub.Plan{Code: "a", Name: "a", Enabled: true})
	_ = plans.Upsert(context.Background(), &dsub.Plan{Code: "b", Name: "b", Enabled: false})

	svc := NewService(plans, newFakeSubs(), &fakeBill{})
	list, _ := svc.ListPlans(context.Background())
	if len(list) != 1 || list[0].Code != "a" {
		t.Errorf("ListPlans: %+v", list)
	}
}

func TestUpsertPlan_Validation(t *testing.T) {
	svc := NewService(newFakePlans(), newFakeSubs(), &fakeBill{})
	if err := svc.UpsertPlan(context.Background(), &dsub.Plan{Code: "", IncludedQuota: 100, PeriodDays: 30}); err == nil {
		t.Error("空 code 应拒绝")
	}
	if err := svc.UpsertPlan(context.Background(), &dsub.Plan{Code: "a", IncludedQuota: 0, PeriodDays: 30}); err == nil {
		t.Error("0 quota 应拒绝")
	}
	// 有效 plan，currency 缺省应填 USD
	p := &dsub.Plan{Code: "a", IncludedQuota: 100, PeriodDays: 30}
	if err := svc.UpsertPlan(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if p.Currency != "USD" {
		t.Errorf("默认 currency: %q", p.Currency)
	}
}

func TestCreateLocal_NewSubAndTopUp(t *testing.T) {
	plans := newFakePlans()
	subs := newFakeSubs()
	bill := &fakeBill{}
	seedPlan(t, plans, "pro", 5_000_000)
	svc := NewService(plans, subs, bill)

	sub, err := svc.CreateLocal(context.Background(), 42, "pro")
	if err != nil {
		t.Fatalf("create local: %v", err)
	}
	if sub.UserID != 42 || sub.Status != dsub.StatusActive {
		t.Errorf("%+v", sub)
	}
	if !sub.NextResetAt.After(time.Now()) {
		t.Errorf("NextResetAt 应为未来, got %v", sub.NextResetAt)
	}

	// TopUp 被调用一次
	if len(bill.calls) != 1 || bill.calls[0].Amount != 5_000_000 {
		t.Errorf("topups: %+v", bill.calls)
	}
}

func TestCreateLocal_DisabledPlan(t *testing.T) {
	plans := newFakePlans()
	_ = plans.Upsert(context.Background(), &dsub.Plan{Code: "x", IncludedQuota: 100, PeriodDays: 30, Enabled: false})
	svc := NewService(plans, newFakeSubs(), &fakeBill{})
	_, err := svc.CreateLocal(context.Background(), 1, "x")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestCancel(t *testing.T) {
	plans := newFakePlans()
	subs := newFakeSubs()
	seedPlan(t, plans, "pro", 100)
	svc := NewService(plans, subs, &fakeBill{})
	_, _ = svc.CreateLocal(context.Background(), 7, "pro")

	if err := svc.Cancel(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Current(context.Background(), 7)
	if got.Status != dsub.StatusCanceled {
		t.Errorf("status: %q", got.Status)
	}
	if got.CanceledAt == nil {
		t.Error("CanceledAt 未设")
	}

	// 幂等：重复 cancel 不报错
	if err := svc.Cancel(context.Background(), 7); err != nil {
		t.Errorf("第二次 cancel 不应报错: %v", err)
	}
}

func TestHandleInvoicePaid_CreatesFirst(t *testing.T) {
	plans := newFakePlans()
	subs := newFakeSubs()
	bill := &fakeBill{}
	seedPlan(t, plans, "pro", 5_000_000)
	svc := NewService(plans, subs, bill)

	err := svc.HandleInvoicePaid(context.Background(), 42, "pro", "sub_xxx")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := subs.GetByGatewayRef(context.Background(), "sub_xxx")
	if got == nil || got.Status != dsub.StatusActive {
		t.Errorf("sub: %+v", got)
	}
	if len(bill.calls) != 1 {
		t.Errorf("topup: %+v", bill.calls)
	}
}

func TestHandleInvoicePaid_ExistingRenewal(t *testing.T) {
	plans := newFakePlans()
	subs := newFakeSubs()
	bill := &fakeBill{}
	p := seedPlan(t, plans, "pro", 1000)
	// 已有一条订阅
	_ = subs.Create(context.Background(), &dsub.Subscription{
		UserID: 42, PlanCode: p.Code, Status: dsub.StatusActive,
		GatewayRef: "sub_yyy", NextResetAt: time.Now().Add(-time.Hour),
	})
	svc := NewService(plans, subs, bill)

	_ = svc.HandleInvoicePaid(context.Background(), 42, "pro", "sub_yyy")

	got, _ := subs.GetByGatewayRef(context.Background(), "sub_yyy")
	if !got.NextResetAt.After(time.Now()) {
		t.Errorf("NextResetAt 未前滚: %v", got.NextResetAt)
	}
	// TopUp 被触发
	if len(bill.calls) != 1 {
		t.Errorf("topup: %+v", bill.calls)
	}
}

func TestApplyDueSubscriptions(t *testing.T) {
	plans := newFakePlans()
	subs := newFakeSubs()
	bill := &fakeBill{}
	p := seedPlan(t, plans, "pro", 5_000_000)

	// 即将到期
	_ = subs.Create(context.Background(), &dsub.Subscription{
		UserID: 1, PlanCode: p.Code, Status: dsub.StatusActive,
		NextResetAt: time.Now().Add(-1 * time.Minute),
	})
	// 未到期
	_ = subs.Create(context.Background(), &dsub.Subscription{
		UserID: 2, PlanCode: p.Code, Status: dsub.StatusActive,
		NextResetAt: time.Now().Add(1 * time.Hour),
	})
	// 已取消
	_ = subs.Create(context.Background(), &dsub.Subscription{
		UserID: 3, PlanCode: p.Code, Status: dsub.StatusCanceled,
		NextResetAt: time.Now().Add(-1 * time.Minute),
	})

	svc := NewService(plans, subs, bill)
	n, err := svc.ApplyDueSubscriptions(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("应处理 1 条, got %d", n)
	}
	if len(bill.calls) != 1 || bill.calls[0].UserID != 1 {
		t.Errorf("topups: %+v", bill.calls)
	}
}

func TestGatewayRefFor(t *testing.T) {
	plans := newFakePlans()
	_ = plans.Upsert(context.Background(), &dsub.Plan{Code: "pro", Enabled: true, GatewayRef: "price_xxx"})
	svc := NewService(plans, newFakeSubs(), &fakeBill{})
	ref, err := svc.GatewayRefFor(context.Background(), "pro")
	if err != nil || ref != "price_xxx" {
		t.Errorf("got %q %v", ref, err)
	}
}
