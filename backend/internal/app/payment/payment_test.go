// payment_test.go —— 支付 Service 编排：下单 + webhook 幂等。
package payment

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	dpayment "github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakeOrderRepo struct {
	mu     sync.Mutex
	byID   map[string]*dpayment.Order
	byRef  map[string]*dpayment.Order
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{byID: map[string]*dpayment.Order{}, byRef: map[string]*dpayment.Order{}}
}
func (f *fakeOrderRepo) Create(_ context.Context, o *dpayment.Order) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *o
	f.byID[o.ID] = &cp
	if o.GatewayRef != "" {
		f.byRef[o.Gateway+":"+o.GatewayRef] = &cp
	}
	return nil
}
func (f *fakeOrderRepo) GetByID(_ context.Context, id string) (*dpayment.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.byID[id]; ok {
		cp := *o
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeOrderRepo) GetByGatewayRef(_ context.Context, g, ref string) (*dpayment.Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.byRef[g+":"+ref]; ok {
		cp := *o
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeOrderRepo) ListByUser(_ context.Context, _ uint64, _, _ int) ([]*dpayment.Order, int64, error) {
	return nil, 0, nil
}
func (f *fakeOrderRepo) Update(_ context.Context, o *dpayment.Order) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.byID[o.ID]
	if !ok {
		return derrors.ErrNotFound
	}
	*stored = *o
	if o.GatewayRef != "" {
		f.byRef[o.Gateway+":"+o.GatewayRef] = stored
	}
	return nil
}

type fakeBilling struct {
	mu     sync.Mutex
	topUps []topUpCall
}
type topUpCall struct {
	UserID uint64
	Amount int64
	Type   billing.LedgerType
	RefID  string
}

func (f *fakeBilling) TopUp(_ context.Context, uid uint64, amt int64, typ billing.LedgerType, ref, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.topUps = append(f.topUps, topUpCall{uid, amt, typ, ref})
	return nil
}

// fakeGateway 不做真网关调用，在 CreateCheckout 里直接填 ID/URL。
type fakeGateway struct {
	name      string
	lastOrder *dpayment.Order
	eventFn   func() WebhookEvent
	sigValid  bool
}

func (g *fakeGateway) Name() string { return g.name }
func (g *fakeGateway) CreateCheckout(_ context.Context, o *dpayment.Order) error {
	o.GatewayRef = "sess_" + o.ID
	o.CheckoutURL = "https://fake/pay/" + o.ID
	g.lastOrder = o
	return nil
}
func (g *fakeGateway) ParseWebhook(_ context.Context, _ []byte, _ string) (WebhookEvent, error) {
	if !g.sigValid {
		return WebhookEvent{}, errors.New("bad sig")
	}
	if g.eventFn != nil {
		return g.eventFn(), nil
	}
	return WebhookEvent{Type: EventUnknown}, nil
}

// ---------- CreateTopUp ----------

func TestCreateTopUp_Success(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe"}
	svc := NewService(orders, bill, 10_000, gw)

	order, err := svc.CreateTopUp(context.Background(), 42, 2000, "usd", "stripe")
	if err != nil {
		t.Fatalf("create top-up: %v", err)
	}
	if order.Amount != 2000*10_000 {
		t.Errorf("Amount: %d", order.Amount)
	}
	if order.Currency != "USD" {
		t.Errorf("currency 未大写: %q", order.Currency)
	}
	if order.Gateway != "stripe" || order.Mode != dpayment.ModePayment {
		t.Errorf("%+v", order)
	}
	if order.CheckoutURL == "" {
		t.Error("checkout url 未回填")
	}
	// 必须已 persist
	if _, err := orders.GetByID(context.Background(), order.ID); err != nil {
		t.Errorf("未入库: %v", err)
	}
}

func TestCreateTopUp_InvalidAmount(t *testing.T) {
	svc := NewService(newFakeOrderRepo(), &fakeBilling{}, 10_000, &fakeGateway{name: "stripe"})
	_, err := svc.CreateTopUp(context.Background(), 1, 0, "USD", "stripe")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestCreateTopUp_UnknownGateway(t *testing.T) {
	svc := NewService(newFakeOrderRepo(), &fakeBilling{}, 10_000, &fakeGateway{name: "stripe"})
	_, err := svc.CreateTopUp(context.Background(), 1, 100, "USD", "unknown")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

// ---------- HandleWebhook: Checkout completed ----------

func TestHandleWebhook_CheckoutCompletedTopsUpUser(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe", sigValid: true}
	svc := NewService(orders, bill, 10_000, gw)

	order, _ := svc.CreateTopUp(context.Background(), 42, 500, "USD", "stripe")
	gw.eventFn = func() WebhookEvent {
		return WebhookEvent{Type: EventCheckoutCompleted, OrderID: order.ID, RefID: "cs_confirmed"}
	}

	if err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig"); err != nil {
		t.Fatalf("webhook: %v", err)
	}

	// 订单状态
	got, _ := orders.GetByID(context.Background(), order.ID)
	if got.Status != dpayment.StatusPaid {
		t.Errorf("订单未变 Paid: %+v", got)
	}
	if got.PaidAt == nil {
		t.Error("PaidAt 未设")
	}

	// TopUp 被调用一次
	if len(bill.topUps) != 1 {
		t.Fatalf("topUps: %+v", bill.topUps)
	}
	tu := bill.topUps[0]
	if tu.UserID != 42 || tu.Amount != 500*10_000 {
		t.Errorf("topup 参数不对: %+v", tu)
	}
	if tu.Type != billing.LedgerTopUp {
		t.Errorf("ledger type: %s", tu.Type)
	}
}

func TestHandleWebhook_CheckoutCompletedIdempotent(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe", sigValid: true}
	svc := NewService(orders, bill, 10_000, gw)

	order, _ := svc.CreateTopUp(context.Background(), 1, 100, "USD", "stripe")
	gw.eventFn = func() WebhookEvent {
		return WebhookEvent{Type: EventCheckoutCompleted, OrderID: order.ID}
	}

	_ = svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	_ = svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	_ = svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")

	if len(bill.topUps) != 1 {
		t.Errorf("webhook 重放应幂等，TopUp 只能触发 1 次, got %d", len(bill.topUps))
	}
}

func TestHandleWebhook_BadSignature(t *testing.T) {
	gw := &fakeGateway{name: "stripe", sigValid: false}
	svc := NewService(newFakeOrderRepo(), &fakeBilling{}, 10_000, gw)
	err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "bad")
	if !derrors.Is(err, derrors.CodeUnauthenticated) {
		t.Errorf("want Unauthenticated, got %v", err)
	}
}

func TestHandleWebhook_UnknownEventIgnored(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe", sigValid: true,
		eventFn: func() WebhookEvent { return WebhookEvent{Type: EventUnknown} }}
	svc := NewService(orders, bill, 10_000, gw)
	if err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig"); err != nil {
		t.Errorf("unknown 应忽略不报错: %v", err)
	}
	if len(bill.topUps) > 0 {
		t.Error("不该调 TopUp")
	}
}

func TestHandleWebhook_Expired(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe", sigValid: true}
	svc := NewService(orders, bill, 10_000, gw)
	order, _ := svc.CreateTopUp(context.Background(), 1, 100, "USD", "stripe")
	gw.eventFn = func() WebhookEvent {
		return WebhookEvent{Type: EventCheckoutExpired, OrderID: order.ID}
	}
	_ = svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig")
	got, _ := orders.GetByID(context.Background(), order.ID)
	if got.Status != dpayment.StatusExpired {
		t.Errorf("未标记 Expired: %+v", got.Status)
	}
}

// ---------- Subscription Checkout ----------

type fakePlanLookup struct {
	m map[string]string
}

func (f *fakePlanLookup) GatewayRefFor(_ context.Context, code string) (string, error) {
	if v, ok := f.m[code]; ok {
		return v, nil
	}
	return "", derrors.ErrNotFound
}

type fakeSubsHandler struct{ calls []invoiceCall }
type invoiceCall struct {
	UserID     uint64
	PlanCode   string
	GatewayRef string
}

func (f *fakeSubsHandler) HandleInvoicePaid(_ context.Context, uid uint64, plan, ref string) error {
	f.calls = append(f.calls, invoiceCall{uid, plan, ref})
	return nil
}

func TestCreateSubscription_Success(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe"}
	svc := NewService(orders, bill, 10_000, gw)
	svc.WithSubscriptions(&fakeSubsHandler{}, &fakePlanLookup{m: map[string]string{"pro": "price_xyz"}})

	order, err := svc.CreateSubscription(context.Background(), 7, "pro", "stripe")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if order.Mode != dpayment.ModeSubscription || order.PlanCode != "pro" {
		t.Errorf("%+v", order)
	}
	if order.CheckoutURL == "" {
		t.Error("checkout url 未回填")
	}
}

func TestCreateSubscription_MissingPlan(t *testing.T) {
	svc := NewService(newFakeOrderRepo(), &fakeBilling{}, 10_000, &fakeGateway{name: "stripe"})
	svc.WithSubscriptions(&fakeSubsHandler{}, &fakePlanLookup{m: map[string]string{}})
	_, err := svc.CreateSubscription(context.Background(), 1, "nonexistent", "stripe")
	if err == nil {
		t.Error("unknown plan 应报错")
	}
}

func TestHandleWebhook_InvoicePaid(t *testing.T) {
	orders := newFakeOrderRepo()
	bill := &fakeBilling{}
	gw := &fakeGateway{name: "stripe", sigValid: true}
	subs := &fakeSubsHandler{}
	svc := NewService(orders, bill, 10_000, gw)
	svc.WithSubscriptions(subs, &fakePlanLookup{})

	// 直接触发 invoice.paid，无关联本地 order
	gw.eventFn = func() WebhookEvent {
		return WebhookEvent{
			Type:           EventInvoicePaid,
			UserID:         99,
			PlanCode:       "pro",
			SubscriptionID: "sub_xxx",
		}
	}
	if err := svc.HandleWebhook(context.Background(), "stripe", []byte("{}"), "sig"); err != nil {
		t.Fatalf("%v", err)
	}
	if len(subs.calls) != 1 {
		t.Fatalf("subs.calls: %+v", subs.calls)
	}
	c := subs.calls[0]
	if c.UserID != 99 || c.PlanCode != "pro" || c.GatewayRef != "sub_xxx" {
		t.Errorf("call: %+v", c)
	}
}
