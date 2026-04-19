// Package middleware 汇集所有 HTTP 中间件。
package middleware

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/auth"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// Context keys。CtxReqID 保留对外可见，供 handler 写日志用；
// 实际值必须等于 httperr.RequestIDKey，以便 httperr 取到相同的 id。
const (
	CtxUser    = "nexus.user"
	CtxApiKey  = "nexus.apikey"
	CtxReqID   = httperr.RequestIDKey
	HeaderReq  = "X-Request-ID"
	CookieSess = "nexus_session"
)

// RequestID 生成或透传请求 ID。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderReq)
		if id == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			id = hex.EncodeToString(buf)
		}
		c.Set(CtxReqID, id)
		c.Writer.Header().Set(HeaderReq, id)
		c.Next()
	}
}

// AccessLog 记录每次请求。
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("dur", time.Since(start)),
			zap.String("req_id", c.GetString(CtxReqID)),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// Recover 捕获 panic，返回 500。
func Recover(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic", zap.Any("rec", rec), zap.String("path", c.Request.URL.Path))
				httperr.AbortCode(c, http.StatusInternalServerError,
					derrors.CodeInternal, "内部错误")
			}
		}()
		c.Next()
	}
}

// CORS 允许跨域。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		h.Set("Access-Control-Expose-Headers", "X-Request-ID")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// AuthSession 从 cookie 解析 session 并注入当前 user。
func AuthSession(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(CookieSess)
		if err != nil || cookie == "" {
			httperr.Unauthorized(c, "")
			return
		}
		u, err := svc.Authenticate(c.Request.Context(), cookie)
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		c.Set(CtxUser, u)
		c.Next()
	}
}

// AuthApiKey 解析客户端 Api Key。兼容三种常见传法：
//   - Authorization: Bearer sk-nexus-xxx    （OpenAI / Codex / 通用协议）
//   - x-api-key: sk-nexus-xxx               （Anthropic / Claude Code 默认）
//   - x-goog-api-key: sk-nexus-xxx          （Gemini 原生协议）
//
// 这样用户拿到 sk-nexus-xxx 后不需要改 coding agent 的 header 习惯即可直连。
// 同时校验关联用户的可用状态（banned 的用户即使 ApiKey 存活也拒绝）。
func AuthApiKey(svc *keysapp.Service, users user.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			httperr.Unauthorized(c, "缺少 Api Key（Authorization: Bearer / x-api-key / x-goog-api-key 任一）")
			return
		}
		k, err := svc.ResolveBearer(c.Request.Context(), token)
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		u, err := users.GetByID(c.Request.Context(), k.UserID)
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		if !u.Active() {
			httperr.Forbidden(c, "账号已禁用")
			return
		}
		c.Set(CtxApiKey, k)
		c.Set(CtxUser, u)
		c.Next()
	}
}

// extractBearerToken 从多种 header 中抽取 sk-nexus-* token。
// 优先级：Authorization > x-api-key > x-goog-api-key。
func extractBearerToken(c *gin.Context) string {
	if hdr := c.GetHeader("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		return strings.TrimPrefix(hdr, "Bearer ")
	}
	if v := c.GetHeader("x-api-key"); v != "" {
		return v
	}
	if v := c.GetHeader("x-goog-api-key"); v != "" {
		return v
	}
	// Gemini 把 key 放在 query param 里
	if v := c.Query("key"); v != "" {
		return v
	}
	return ""
}

// RequireAdmin 要求当前 user 必须是 admin。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			httperr.Unauthorized(c, "")
			return
		}
		if !u.IsAdmin() {
			httperr.Forbidden(c, "")
			return
		}
		c.Next()
	}
}

// CurrentUser 便捷取当前登录用户。
func CurrentUser(c *gin.Context) *user.User {
	v, ok := c.Get(CtxUser)
	if !ok {
		return nil
	}
	u, _ := v.(*user.User)
	return u
}

// CurrentApiKey 便捷取当前 Api Key。
func CurrentApiKey(c *gin.Context) *apikey.ApiKey {
	v, ok := c.Get(CtxApiKey)
	if !ok {
		return nil
	}
	k, _ := v.(*apikey.ApiKey)
	return k
}

// BodyCopy 缓存 request body 供 handler 重复读取。
func BodyCopy() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			data, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(data))
			c.Set("raw_body", data)
		}
		c.Next()
	}
}
