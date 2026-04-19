// csrf.go —— double-submit cookie 式 CSRF 保护（面向 Session 鉴权的 /api/*）。
//
// 机制：
//   1. 登录/会话建立时，服务器同时设置两个 cookie：
//        - nexus_session（HttpOnly，鉴权用）
//        - nexus_csrf（非 HttpOnly，前端 JS 可读）
//   2. 前端在任何改变状态的请求（POST/PUT/DELETE/PATCH）上读 nexus_csrf 写入
//      X-CSRF-Token header
//   3. 服务器中间件校验 header == cookie；两者相等即可信（攻击页面无法读取或伪造
//      跨站 cookie）
//
// 为了保持契约最小：本中间件只拦截"登录态下的 mutating 请求"。GET / HEAD 不拦，
// 未登录请求（首次登录 / webhook）也放过。
package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"

	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// CSRF cookie 名与 header 名。
const (
	CookieCSRF = "nexus_csrf"
	HeaderCSRF = "X-CSRF-Token"
)

// NewCSRFToken 返回一个 22 字节 base64url 随机串。登录成功时写 cookie 用。
func NewCSRFToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

// SetCSRFCookie 在响应上设置 CSRF token cookie。
// path="/" secure=false (dev)；生产建议由前端 https + secure=true。
func SetCSRFCookie(c *gin.Context, token string, maxAgeSec int) {
	// 非 HttpOnly：JS 必须能读
	c.SetCookie(CookieCSRF, token, maxAgeSec, "/", "", false, false)
}

// CSRF 中间件：mutating method（POST/PUT/DELETE/PATCH）下要求
// X-CSRF-Token header == nexus_csrf cookie。
//
// 请仅挂到"有 session 的 /api/*"；不要挂 webhook 或无 session 的登录路径。
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		m := c.Request.Method
		if m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions {
			c.Next()
			return
		}
		// 允许无 session 请求通过（由后面的鉴权中间件决定；登录/注册等也归此类）
		if _, err := c.Cookie(CookieSess); err != nil {
			c.Next()
			return
		}
		cookie, err := c.Cookie(CookieCSRF)
		if err != nil || cookie == "" {
			httperr.AbortStatus(c, http.StatusForbidden,
				derrors.New(derrors.CodePermissionDenied, "missing CSRF cookie"))
			return
		}
		token := c.GetHeader(HeaderCSRF)
		if token == "" || token != cookie {
			httperr.AbortStatus(c, http.StatusForbidden,
				derrors.New(derrors.CodePermissionDenied, "CSRF token 不匹配"))
			return
		}
		c.Next()
	}
}
