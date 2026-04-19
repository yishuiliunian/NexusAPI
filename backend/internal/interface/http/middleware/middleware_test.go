// middleware_test.go —— 核心中间件的行为单测。
//
// 采用 in-memory fake：
//   - miniredis 模拟 Redis（RateLimit）
//   - gin.CreateTestContext + httptest.NewRecorder 测响应
//   - 手动 Set(CtxUser/CtxApiKey) 模拟前置 middleware 已放行
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	"github.com/yishuiliunian/nexusapi/backend/pkg/ratelimit"
)

func init() { gin.SetMode(gin.TestMode) }

// newMiniRedis 启 in-memory redis 返回 client（t.Cleanup 自动关闭）。
func newMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	s := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: s.Addr()})
}

// ctxWith 构造一个带预设 key/user 的 gin.Context；req 带 path 和 IP。
func ctxWith(setup func(c *gin.Context)) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("POST", "/v1/x", strings.NewReader(""))
	c.Request.RemoteAddr = "127.0.0.1:12345"
	if setup != nil {
		setup(c)
	}
	return c, rec
}

// ---------- RateLimit ----------

func TestRateLimit_NoLimitIfLimiterMissing(t *testing.T) {
	cfg := RateLimitConfig{Limiter: nil, DefaultRPM: 10}
	c, rec := ctxWith(func(c *gin.Context) {
		c.Set(CtxApiKey, &apikey.ApiKey{ID: 1})
	})
	RateLimit(cfg)(c)
	if c.IsAborted() {
		t.Error("nil limiter 应放行")
	}
	_ = rec
}

func TestRateLimit_NoLimitIfBothZero(t *testing.T) {
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 0}
	c, _ := ctxWith(func(c *gin.Context) {
		c.Set(CtxApiKey, &apikey.ApiKey{ID: 1, RPMLimit: 0})
	})
	RateLimit(cfg)(c)
	if c.IsAborted() {
		t.Error("limit=0 应放行")
	}
}

func TestRateLimit_BlocksAfterThreshold(t *testing.T) {
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 3}

	fire := func() *httptest.ResponseRecorder {
		c, rec := ctxWith(func(c *gin.Context) {
			c.Set(CtxApiKey, &apikey.ApiKey{ID: 1, RPMLimit: 0})
		})
		RateLimit(cfg)(c)
		return rec
	}
	for i := 0; i < 3; i++ {
		rec := fire()
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("第 %d 次请求不应被拦截", i+1)
		}
	}
	rec := fire()
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("第 4 次应 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("应返回 Retry-After")
	}
}

func TestRateLimit_ApiKeyOverridesDefault(t *testing.T) {
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 100}
	k := &apikey.ApiKey{ID: 99, RPMLimit: 2} // ApiKey 自己的限制更严
	fire := func() *httptest.ResponseRecorder {
		c, rec := ctxWith(func(c *gin.Context) { c.Set(CtxApiKey, k) })
		RateLimit(cfg)(c)
		return rec
	}
	fire()
	fire()
	rec := fire() // 第 3 次应 429
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("ApiKey.RPMLimit 应 override Default，第 3 次 got %d", rec.Code)
	}
}

func TestRateLimit_HeadersEmitted(t *testing.T) {
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 5}
	c, rec := ctxWith(func(c *gin.Context) {
		c.Set(CtxApiKey, &apikey.ApiKey{ID: 1})
	})
	RateLimit(cfg)(c)
	if rec.Header().Get("X-RateLimit-Limit") != "5" {
		t.Errorf("Limit header: %q", rec.Header().Get("X-RateLimit-Limit"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "4" {
		t.Errorf("Remaining: %q", rec.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRateLimit_UserLevelBlocks(t *testing.T) {
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 0} // 关闭 key 级
	fire := func() *httptest.ResponseRecorder {
		c, rec := ctxWith(func(c *gin.Context) {
			c.Set(CtxApiKey, &apikey.ApiKey{ID: 1, RPMLimit: 0})
			c.Set(CtxUser, &user.User{ID: 7, RPMLimit: 2})
		})
		RateLimit(cfg)(c)
		return rec
	}
	fire()
	fire()
	rec := fire() // 第 3 次应 429（用户级）
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("期望 429，got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Scope") != "user" {
		t.Errorf("Scope 应为 user，got %q", rec.Header().Get("X-RateLimit-Scope"))
	}
}

func TestRateLimit_UserAndApiKeyIndependent(t *testing.T) {
	// 用户级允许 10，apikey 级允许 2。apikey 先触发。
	lim := ratelimit.New(newMiniRedis(t), "t")
	cfg := RateLimitConfig{Limiter: lim, DefaultRPM: 0}
	fire := func() *httptest.ResponseRecorder {
		c, rec := ctxWith(func(c *gin.Context) {
			c.Set(CtxApiKey, &apikey.ApiKey{ID: 1, RPMLimit: 2})
			c.Set(CtxUser, &user.User{ID: 7, RPMLimit: 10})
		})
		RateLimit(cfg)(c)
		return rec
	}
	fire()
	fire()
	rec := fire() // 第 3 次，apikey 级 429
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("期望 429，got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("应显示 apikey 级 limit=2，got %q", rec.Header().Get("X-RateLimit-Limit"))
	}
}

// ---------- CheckApiKeyIP ----------

func TestCheckApiKeyIP_NoWhitelistAllowsAny(t *testing.T) {
	k := &apikey.ApiKey{ID: 1, IPWhitelist: nil}
	c, _ := ctxWith(func(c *gin.Context) { c.Set(CtxApiKey, k) })
	CheckApiKeyIP()(c)
	if c.IsAborted() {
		t.Error("空白名单应放行")
	}
}

func TestCheckApiKeyIP_Match(t *testing.T) {
	k := &apikey.ApiKey{ID: 1, IPWhitelist: []string{"1.2.3.4", "127.0.0.1"}}
	c, _ := ctxWith(func(c *gin.Context) { c.Set(CtxApiKey, k) })
	CheckApiKeyIP()(c)
	if c.IsAborted() {
		t.Error("127.0.0.1 应在白名单中")
	}
}

func TestCheckApiKeyIP_Reject(t *testing.T) {
	k := &apikey.ApiKey{ID: 1, IPWhitelist: []string{"10.0.0.1"}}
	c, rec := ctxWith(func(c *gin.Context) { c.Set(CtxApiKey, k) })
	CheckApiKeyIP()(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

// ---------- CSRF ----------

func TestCSRF_SkipsGET(t *testing.T) {
	c, _ := ctxWith(func(c *gin.Context) {})
	c.Request.Method = "GET"
	CSRF()(c)
	if c.IsAborted() {
		t.Error("GET 不应拦截")
	}
}

func TestCSRF_NoSessionNoCheck(t *testing.T) {
	// mutating 请求但无 session cookie：让后续 auth middleware 决定，CSRF 放行。
	c, _ := ctxWith(nil)
	c.Request.Method = "POST"
	CSRF()(c)
	if c.IsAborted() {
		t.Error("无 session 的 POST 应放行")
	}
}

func TestCSRF_WithSessionRequiresMatchingToken(t *testing.T) {
	c, rec := ctxWith(nil)
	c.Request.Method = "POST"
	c.Request.AddCookie(&http.Cookie{Name: CookieSess, Value: "sess-123"})
	// 缺 CSRF cookie → 403
	CSRF()(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("missing CSRF cookie want 403, got %d", rec.Code)
	}
}

func TestCSRF_MatchAllows(t *testing.T) {
	c, _ := ctxWith(nil)
	c.Request.Method = "POST"
	c.Request.AddCookie(&http.Cookie{Name: CookieSess, Value: "sess"})
	c.Request.AddCookie(&http.Cookie{Name: CookieCSRF, Value: "abc123"})
	c.Request.Header.Set(HeaderCSRF, "abc123")
	CSRF()(c)
	if c.IsAborted() {
		t.Error("header == cookie 应放行")
	}
}

func TestCSRF_MismatchRejects(t *testing.T) {
	c, rec := ctxWith(nil)
	c.Request.Method = "POST"
	c.Request.AddCookie(&http.Cookie{Name: CookieSess, Value: "sess"})
	c.Request.AddCookie(&http.Cookie{Name: CookieCSRF, Value: "right"})
	c.Request.Header.Set(HeaderCSRF, "wrong")
	CSRF()(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("mismatch want 403, got %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "permission_denied" {
		t.Errorf("code: %v", body["code"])
	}
}

func TestSetCSRFCookie(t *testing.T) {
	c, rec := ctxWith(nil)
	tok := NewCSRFToken()
	SetCSRFCookie(c, tok, 3600)
	cookies := rec.Result().Cookies()
	found := false
	for _, ck := range cookies {
		if ck.Name == CookieCSRF {
			found = true
			if ck.HttpOnly {
				t.Error("CSRF cookie 必须 HttpOnly=false，前端要读")
			}
			if ck.Value == "" {
				t.Error("CSRF cookie 值为空")
			}
		}
	}
	if !found {
		t.Error("未设 CSRF cookie")
	}
}

func TestNewCSRFToken_Uniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok := NewCSRFToken()
		if tok == "" || len(tok) < 16 {
			t.Errorf("token too short: %q", tok)
		}
		if seen[tok] {
			t.Errorf("token 重复: %q", tok)
		}
		seen[tok] = true
	}
}

// ---------- RequireAdmin ----------

func TestRequireAdmin_NonAdmin(t *testing.T) {
	c, rec := ctxWith(func(c *gin.Context) {
		c.Set(CtxUser, &user.User{ID: 1, Role: user.RoleUser})
	})
	RequireAdmin()(c)
	if rec.Code != http.StatusForbidden {
		t.Errorf("non-admin want 403, got %d", rec.Code)
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	c, _ := ctxWith(func(c *gin.Context) {
		c.Set(CtxUser, &user.User{ID: 1, Role: user.RoleAdmin})
	})
	RequireAdmin()(c)
	if c.IsAborted() {
		t.Error("admin 应放行")
	}
}

func TestRequireAdmin_NoUser(t *testing.T) {
	c, rec := ctxWith(nil)
	RequireAdmin()(c)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("无 user 应 401, got %d", rec.Code)
	}
}

// ---------- RequestID ----------

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	c, rec := ctxWith(nil)
	RequestID()(c)
	id, ok := c.Get(CtxReqID)
	if !ok || id.(string) == "" {
		t.Error("未生成 request id")
	}
	if rec.Header().Get(HeaderReq) == "" {
		t.Error("响应头未写 request id")
	}
}

func TestRequestID_PropagatesFromHeader(t *testing.T) {
	c, rec := ctxWith(nil)
	c.Request.Header.Set(HeaderReq, "client-req-abc")
	RequestID()(c)
	if c.GetString(CtxReqID) != "client-req-abc" {
		t.Errorf("未透传: %q", c.GetString(CtxReqID))
	}
	if rec.Header().Get(HeaderReq) != "client-req-abc" {
		t.Error("响应头未回显")
	}
}

// ---------- Helper mustBool used in cleanup ----------

var _ = time.Now // ensure time imported even when unused by some test
