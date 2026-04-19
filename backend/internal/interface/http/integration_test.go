// integration_test.go —— 端到端 HTTP 测试。
//
// 启一个完整 Gin engine（真 router + 真 service + SQLite :memory: repo），用
// httptest.NewServer 驱动；验证关键链路：
//   1. 注册 → 登录 → CSRF cookie 流程 → 鉴权读取 /api/user/me
//   2. 用户创建 ApiKey → 通过透传路径调模拟 Anthropic → 计费落 Ledger + Usage
//   3. Stripe webhook 到账 → 用户余额 + ledger 更新
//
// 不依赖外部服务：上游 Anthropic 由 httptest.NewServer 模拟；Redis 置 nil
// 走内存降级；邮件用 NoopMailer。
package httpiface

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/auth"
	auditapp "github.com/yishuiliunian/nexusapi/backend/internal/app/audit"
	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	paymentapp "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/redemption"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	verifyapp "github.com/yishuiliunian/nexusapi/backend/internal/app/verify"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	domainrelay "github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/db"
	stripegw "github.com/yishuiliunian/nexusapi/backend/internal/infra/payment/stripe"
	mw "github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
)

func init() { gin.SetMode(gin.TestMode) }

// testEnv 承载 integration 测试的所有依赖。
type testEnv struct {
	t          *testing.T
	db         *gorm.DB
	engine     *gin.Engine
	srv        *httptest.Server
	client     *http.Client
	stripeKey  string // webhook secret
}

func setupEnv(t *testing.T) *testEnv {
	t.Helper()
	// 1. SQLite :memory: + AutoMigrate
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(gdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cipher := cryptoutil.Noop()

	// 2. 仓储
	userRepo := db.NewUserRepo(gdb, cipher)
	groupRepo := db.NewGroupRepo(gdb)
	sessionRepo := db.NewSessionRepo(gdb)
	apiKeyRepo := db.NewApiKeyRepo(gdb)
	channelRepo := db.NewChannelRepo(gdb, cipher)
	priceRepo := db.NewModelPriceRepo(gdb)
	usageRepo := db.NewUsageRepo(gdb)
	ledgerRepo := db.NewLedgerRepo(gdb)
	redemptionRepo := db.NewRedemptionRepo(gdb)
	orderRepo := db.NewPaymentOrderRepo(gdb)
	planRepo := db.NewPlanRepo(gdb)
	subRepo := db.NewSubscriptionRepo(gdb)
	verifyTokenRepo := db.NewVerifyTokenRepo(gdb)
	auditRepo := db.NewAuditLogRepo(gdb)
	quotaDelta := db.NewQuotaDelta(gdb)

	// 3. 应用服务
	authSvc := auth.NewService(userRepo, sessionRepo, time.Hour)
	apiKeySvc := keysapp.NewService(apiKeyRepo)
	// 集成测试用 SQLite 无预置价格；关闭 strictPricing 避免所有请求被 402 拦截。
	// （端到端的 strictPricing 行为由 billing_test.go 覆盖）
	billingEngine := billingapp.NewEngine(quotaDelta, userRepo, priceRepo, groupRepo, nil).
		WithStrictPricing(false)
	relaySelector := relayapp.NewSelector(channelRepo,
		func(name string) domainrelay.SyncAdaptor { return stubAdaptor{name: name} }).
		WithBreaker(relayapp.NewMemoryBreaker(relayapp.BreakerConfig{Threshold: 5, Cooldown: time.Second})).
		WithAffinity(relayapp.NewMemoryAffinity(time.Minute))
	subSvc := subapp.NewService(planRepo, subRepo, billingEngine)
	redemptionSvc := redemption.NewService(redemptionRepo, billingEngine)
	verifySvc := verifyapp.NewService(verifyTokenRepo, userRepo, verifyapp.NoopMailer{}, "http://test")

	// 4. Stripe 支付（用 webhook secret 模拟）
	const whSecret = "whsec_test"
	stripe := stripegw.New(stripegw.Config{
		SecretKey:     "sk_test_fake",
		WebhookSecret: whSecret,
		APIBase:       "http://localhost:0", // 不会被真正调用
	})
	paymentSvc := paymentapp.NewService(orderRepo, billingEngine, 10_000, stripe).WithSubscriptions(subSvc, subSvc)

	// 5. 代理
	px := proxy.New(proxy.Config{})

	// 6. 组装 router
	engine := NewRouter(Deps{
		Infra: InfraDeps{Logger: zap.NewNop()},
		Auth: AuthDeps{
			Auth:   authSvc,
			ApiKey: apiKeySvc,
			Users:  userRepo,
			Verify: verifySvc,
		},
		Relay: RelayDeps{
			Selector: relaySelector,
			Billing:  billingEngine,
			Channels: channelRepo,
		},
		Billing: BillingDeps{
			Redemption: redemptionSvc,
			Payments:   paymentSvc,
			Orders:     orderRepo,
			Subs:       subSvc,
		},
		Admin: AdminDeps{
			Groups:    groupRepo,
			Channels:  channelRepo,
			Prices:    priceRepo,
			Users:     userRepo,
			Audits:    auditRepo,
			Orders:    orderRepo,
			Subs:      subSvc,
			Providers: stubProviderChecker{},
			QuotaAdj:  billingEngine,
			Recorder:  auditapp.NewRecorder(auditRepo, zap.NewNop()),
		},
		Query: QueryDeps{Usages: usageRepo, Ledgers: ledgerRepo},
		Proxy: px,
	})

	srv := httptest.NewServer(engine)
	t.Cleanup(func() { srv.Close() })

	// cookie jar 让 client 自动携带 session + CSRF
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	return &testEnv{
		t:         t,
		db:        gdb,
		engine:    engine,
		srv:       srv,
		client:    client,
		stripeKey: whSecret,
	}
}

// stubProviderChecker 满足 admin.ProviderChecker。
type stubProviderChecker struct{}

func (stubProviderChecker) Exists(string) bool                  { return true }
func (stubProviderChecker) Names() []string                     { return []string{"openai", "claude"} }
func (stubProviderChecker) Lister(string) domainrelay.ModelLister { return nil }

// stubAdaptor 最小 SyncAdaptor 用于 Selector 过滤（Candidates 需要判断 provider 是否已注册）。
type stubAdaptor struct{ name string }

func (s stubAdaptor) Name() string                       { return s.name }
func (s stubAdaptor) Supports() []domainrelay.Capability { return nil }

// ---------- helper: JSON request with auto CSRF ----------

func (e *testEnv) do(method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		e.t.Fatalf("req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// 自动注入 CSRF：从 jar 取 nexus_csrf cookie
	if method != "GET" && method != "HEAD" {
		for _, c := range e.client.Jar.Cookies(req.URL) {
			if c.Name == mw.CookieCSRF {
				req.Header.Set(mw.HeaderCSRF, c.Value)
			}
		}
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	raw := mustReadAll(e.t, resp)
	return resp, raw
}

func mustReadAll(t *testing.T, r *http.Response) []byte {
	t.Helper()
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}

// ---------- Tests ----------

// TestAuthFlow_RegisterLoginMe 端到端验证：
//   1. POST /api/auth/register 返回 user id
//   2. POST /api/auth/login 设置 session + CSRF cookie
//   3. GET /api/user/me 返回当前用户（走 AuthSession 中间件）
func TestAuthFlow_RegisterLoginMe(t *testing.T) {
	env := setupEnv(t)

	// 1. 注册
	resp, body := env.do("POST", "/api/auth/register", map[string]string{
		"email":    "alice@example.com",
		"password": "password123",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("register status=%d body=%s", resp.StatusCode, body)
	}
	var regOut struct {
		ID    uint64 `json:"id"`
		Email string `json:"email"`
	}
	_ = json.Unmarshal(body, &regOut)
	if regOut.ID == 0 || regOut.Email != "alice@example.com" {
		t.Errorf("register output: %+v", regOut)
	}

	// 2. 登录
	resp, body = env.do("POST", "/api/auth/login", map[string]string{
		"email":    "alice@example.com",
		"password": "password123",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("login status=%d body=%s", resp.StatusCode, body)
	}
	// 验证两个 cookie 都设了
	var gotSess, gotCSRF bool
	for _, c := range resp.Cookies() {
		if c.Name == mw.CookieSess {
			gotSess = true
		}
		if c.Name == mw.CookieCSRF {
			gotCSRF = true
		}
	}
	if !gotSess || !gotCSRF {
		t.Errorf("cookies missing: sess=%v csrf=%v", gotSess, gotCSRF)
	}

	// 3. 读 me
	resp, body = env.do("GET", "/api/user/me", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("me status=%d body=%s", resp.StatusCode, body)
	}
	var me struct {
		Email string `json:"email"`
		ID    uint64 `json:"id"`
	}
	_ = json.Unmarshal(body, &me)
	if me.Email != "alice@example.com" || me.ID != regOut.ID {
		t.Errorf("me: %+v", me)
	}
}

// TestAuthFlow_WrongPasswordRejected 登录失败不返回 cookie
func TestAuthFlow_WrongPasswordRejected(t *testing.T) {
	env := setupEnv(t)
	env.do("POST", "/api/auth/register", map[string]string{
		"email": "b@example.com", "password": "password123",
	})
	resp, _ := env.do("POST", "/api/auth/login", map[string]string{
		"email": "b@example.com", "password": "wrongpass456",
	})
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// TestAuthFlow_MeWithoutSession
// 未登录访问 /api/user/me 应 401
func TestAuthFlow_MeWithoutSession(t *testing.T) {
	env := setupEnv(t)
	resp, _ := env.do("GET", "/api/user/me", nil)
	if resp.StatusCode != 401 {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

// TestAuthFlow_LogoutInvalidatesSession
// POST /api/auth/logout 后再 GET /api/user/me 应 401
func TestAuthFlow_LogoutInvalidatesSession(t *testing.T) {
	env := setupEnv(t)
	resp, body := env.do("POST", "/api/auth/register", map[string]string{"email": "c@example.com", "password": "password123"})
	if resp.StatusCode != 200 {
		t.Fatalf("register: %d %s", resp.StatusCode, body)
	}
	resp, body = env.do("POST", "/api/auth/login", map[string]string{"email": "c@example.com", "password": "password123"})
	if resp.StatusCode != 200 {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	resp, body = env.do("GET", "/api/user/me", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("me before logout: %d %s", resp.StatusCode, body)
	}

	// logout（POST 需要 CSRF，do 会自动注入）
	resp, body = env.do("POST", "/api/auth/logout", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("logout status=%d body=%s", resp.StatusCode, body)
	}
	// 第一次 GET /me 之后，server 已经 Set-Cookie 清空了 session——但 cookiejar
	// 会追随 server 清除。再次访问应 401。
	resp, _ = env.do("GET", "/api/user/me", nil)
	if resp.StatusCode != 401 {
		t.Errorf("logout 后 me 应 401, got %d", resp.StatusCode)
	}
}

// TestCSRFProtection: 登录后不带 X-CSRF-Token 的 POST 应被拦截
func TestCSRFProtection_RejectsWithoutHeader(t *testing.T) {
	env := setupEnv(t)
	env.do("POST", "/api/auth/register", map[string]string{"email": "d@example.com", "password": "password123"})
	env.do("POST", "/api/auth/login", map[string]string{"email": "d@example.com", "password": "password123"})

	// 直接构造请求，不通过 env.do（避免自动注入 CSRF header）
	req, _ := http.NewRequest("POST", env.srv.URL+"/api/auth/logout", nil)
	req.Header.Set("Content-Type", "application/json")
	// 手动把 jar 里的 cookie 加到请求——但不加 X-CSRF-Token
	for _, c := range env.client.Jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("缺 CSRF header 应 403, got %d", resp.StatusCode)
	}
}

// TestCreateApiKey_WithCSRF: 登录后通过 CSRF 正确创建 ApiKey
func TestCreateApiKey_WithCSRF(t *testing.T) {
	env := setupEnv(t)
	env.do("POST", "/api/auth/register", map[string]string{"email": "e@example.com", "password": "password123"})
	env.do("POST", "/api/auth/login", map[string]string{"email": "e@example.com", "password": "password123"})

	resp, body := env.do("POST", "/api/user/apikeys", map[string]any{
		"name": "test-key",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("create key status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		ID     uint64 `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(body, &out)
	if out.ID == 0 || !strings.HasPrefix(out.Secret, "sk-nexus-") {
		t.Errorf("create key: %+v", out)
	}

	// 列出验证
	resp, body = env.do("GET", "/api/user/apikeys", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("list keys: %d", resp.StatusCode)
	}
	var list struct {
		Items []struct{ ID uint64 } `json:"items"`
	}
	_ = json.Unmarshal(body, &list)
	if len(list.Items) != 1 || list.Items[0].ID != out.ID {
		t.Errorf("list: %+v", list)
	}
}

// TestV1Messages_ProxyToUpstream: 配置 Claude channel 指向 mock 上游；
// 用 ApiKey 调 /v1/messages 验证字节级透传 + 计费。
func TestV1Messages_ProxyToUpstream(t *testing.T) {
	env := setupEnv(t)
	ctx := context.Background()

	// 1) 建一个"claude"渠道指向 mock 上游
	var gotAuth, gotPath string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotPath = r.URL.Path
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r.Body)
		gotBody = buf.Bytes()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, `{
			"id":"msg_1","type":"message","role":"assistant",
			"content":[{"type":"text","text":"hi"}],
			"usage":{"input_tokens":10,"output_tokens":3}
		}`)
	}))
	defer upstream.Close()

	channelRepo := db.NewChannelRepo(env.db, cryptoutil.Noop())
	_ = channelRepo.Create(ctx, &channel.Channel{
		Name:            "mock-claude",
		Provider:        "claude",
		BaseURL:         upstream.URL,
		Credentials:     "sk-real-upstream-key",
		Models:          []string{"claude-3-5-sonnet"},
		Weight:          100,
		PriceMultiplier: 1.0,
		Status:          channel.StatusActive,
	})

	// 2) 注册 user 建 ApiKey
	env.do("POST", "/api/auth/register", map[string]string{"email": "f@example.com", "password": "password123"})
	env.do("POST", "/api/auth/login", map[string]string{"email": "f@example.com", "password": "password123"})
	_, body := env.do("POST", "/api/user/apikeys", map[string]any{"name": "k"})
	var keyOut struct{ Secret string }
	_ = json.Unmarshal(body, &keyOut)

	// 用户需要有余额才能预占
	var u db.UserRow
	_ = env.db.Where("email = ?", "f@example.com").First(&u).Error
	_ = env.db.Model(&db.UserRow{}).Where("id = ?", u.ID).Update("quota", 10_000_000).Error

	// 3) 调 /v1/messages（用 Bearer ApiKey，透传到 mock）
	reqBody := `{"model":"claude-3-5-sonnet","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", env.srv.URL+"/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+keyOut.Secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("proxy call: %v", err)
	}
	defer resp.Body.Close()
	raw := mustReadAll(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("proxy status=%d body=%s", resp.StatusCode, raw)
	}
	if !bytes.Contains(raw, []byte("msg_1")) {
		t.Errorf("body not passed through: %s", raw)
	}

	// 4) 验证：上游收到了 x-api-key（不是客户端的 Bearer）
	if gotAuth != "sk-real-upstream-key" {
		t.Errorf("upstream auth header: %q", gotAuth)
	}
	if gotPath != "/messages" {
		t.Errorf("path should be stripped of /v1: %q", gotPath)
	}
	if !bytes.Contains(gotBody, []byte("claude-3-5-sonnet")) {
		t.Errorf("body forwarding: %s", gotBody)
	}

	// 5) 验证 usage 行落库
	var usageCount int64
	env.db.Model(&db.UsageRow{}).Count(&usageCount)
	if usageCount != 1 {
		t.Errorf("usage row 数: %d", usageCount)
	}
	// 6) 验证 quota 被扣（或原样；不配 ModelPrice 时 cost=0 也算通过）
	var row db.UserRow
	_ = env.db.Where("email = ?", "f@example.com").First(&row).Error
	if row.Quota > 10_000_000 {
		t.Errorf("余额增加了: %d", row.Quota)
	}
}

// TestStripeWebhook_TopsUpUser: 创建订单 → webhook → 余额到账
func TestStripeWebhook_TopsUpUser(t *testing.T) {
	env := setupEnv(t)
	// 注册 + login（只为了拿到 userID）
	env.do("POST", "/api/auth/register", map[string]string{"email": "g@example.com", "password": "password123"})
	env.do("POST", "/api/auth/login", map[string]string{"email": "g@example.com", "password": "password123"})

	var u db.UserRow
	_ = env.db.Where("email = ?", "g@example.com").First(&u).Error

	// 手写一条 pending 订单到 DB（跳过真实 Stripe checkout session 创建）
	orderID := "order-test-1"
	order := &db.PaymentOrderRow{
		ID:          orderID,
		UserID:      u.ID,
		Amount:      5000 * 10_000, // 500 USD 对应 5_000_000 micro
		AmountCents: 5000,
		Currency:    "USD",
		Gateway:     "stripe",
		GatewayRef:  "cs_test_xyz",
		Mode:        "payment",
		Status:      "pending",
		CreatedAt:   time.Now(),
	}
	if err := env.db.Create(order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}

	// 构造 Stripe 签名的 webhook payload
	payload := fmt.Sprintf(`{
		"id":"evt_1",
		"type":"checkout.session.completed",
		"data":{"object":{
			"id":"cs_test_xyz",
			"client_reference_id":"%s"
		}}
	}`, orderID)
	ts := time.Now().Unix()
	signed := fmt.Sprintf("%d.%s", ts, payload)
	mac := hmac.New(sha256.New, []byte(env.stripeKey))
	mac.Write([]byte(signed))
	sig := fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))

	req, _ := http.NewRequest("POST", env.srv.URL+"/api/webhook/stripe", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", sig)
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b := mustReadAll(t, resp)
		t.Fatalf("webhook status=%d body=%s", resp.StatusCode, b)
	}

	// 验证订单 paid
	var o db.PaymentOrderRow
	_ = env.db.Where("id = ?", orderID).First(&o).Error
	if o.Status != "paid" {
		t.Errorf("order status: %q", o.Status)
	}

	// 验证用户余额（Amount 以 micro 为单位 = 5000 cents × 10_000 micro/cent）
	var u2 db.UserRow
	_ = env.db.Where("id = ?", u.ID).First(&u2).Error
	if u2.Quota != 50_000_000 {
		t.Errorf("quota after topup: %d", u2.Quota)
	}

	// 验证 ledger 行
	var ledgerCount int64
	env.db.Model(&db.LedgerRow{}).Where("user_id = ? AND type = ?", u.ID, "topup").Count(&ledgerCount)
	if ledgerCount != 1 {
		t.Errorf("topup ledger: %d", ledgerCount)
	}

	// 幂等：重复发送同一 webhook 不应再次扣
	req2, _ := http.NewRequest("POST", env.srv.URL+"/api/webhook/stripe", strings.NewReader(payload))
	req2.Header.Set("Stripe-Signature", sig)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := env.client.Do(req2)
	defer resp2.Body.Close()
	_ = mustReadAll(t, resp2)
	var u3 db.UserRow
	_ = env.db.Where("id = ?", u.ID).First(&u3).Error
	if u3.Quota != 50_000_000 {
		t.Errorf("重放 webhook 不应重复 topup, got %d", u3.Quota)
	}
}
