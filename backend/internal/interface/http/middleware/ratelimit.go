// Package middleware 中的 ratelimit 部分：RPM 前置拦截。
//
// TPM 不在这里做前置（需要估算 token 会带来复杂度），由 relay handler 的
// finalize 阶段事后记账（Limiter.Add），管理台可读到当前窗口消耗。
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
	"github.com/yishuiliunian/nexusapi/backend/pkg/ratelimit"
)

// RateLimitConfig 传入中间件的配置。
type RateLimitConfig struct {
	Limiter    *ratelimit.Limiter
	DefaultRPM int // 0 = 不限
}

// RateLimit 按当前请求的 User + ApiKey 做两级 RPM 前置检查。
//
// 需前置 AuthApiKey 以便拿到 apikey 和 user。
//
// 两级桶独立生效（任一超限即 429）：
//   - 用户级：user.RPMLimit > 0 时检查 urpm:{userID}。0 = 不启用用户级。
//   - Key 级：优先 ApiKey.RPMLimit，否则 config.DefaultRPM。仍为 0 则不限。
//
// X-RateLimit-* header 使用 key 级为主（最常见场景）；用户级触发时 header
// 反映用户级阈值，并额外带 X-RateLimit-Scope=user 方便客户端区分。
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		k := CurrentApiKey(c)
		if k == nil {
			c.Next()
			return
		}

		// 1) 用户级优先。user 可能为 nil（走 session 路径通常不挂这个中间件）。
		u := CurrentUser(c)
		if u != nil && u.RPMLimit > 0 && cfg.Limiter != nil {
			bucket := "urpm:" + strconv.FormatUint(u.ID, 10)
			count, retry, err := cfg.Limiter.Check(c.Request.Context(), bucket, u.RPMLimit, time.Minute)
			c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(u.RPMLimit))
			c.Writer.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max0(u.RPMLimit-count)))
			c.Writer.Header().Set("X-RateLimit-Scope", "user")
			if err == ratelimit.ErrLimited {
				if retry > 0 {
					c.Writer.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
				}
				httperr.AbortStatus(c, http.StatusTooManyRequests,
					derrors.New(derrors.CodeRateLimited, "用户级请求频率超限"))
				return
			}
		}

		// 2) Key 级。
		limit := k.RPMLimit
		if limit == 0 {
			limit = cfg.DefaultRPM
		}
		if limit <= 0 || cfg.Limiter == nil {
			c.Next()
			return
		}
		bucket := "rpm:" + strconv.FormatUint(k.ID, 10)
		count, retry, err := cfg.Limiter.Check(c.Request.Context(), bucket, limit, time.Minute)
		c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Writer.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max0(limit-count)))
		// 仅在用户级未设置 Scope 时覆盖为 apikey。
		if c.Writer.Header().Get("X-RateLimit-Scope") == "" {
			c.Writer.Header().Set("X-RateLimit-Scope", "apikey")
		}
		if err == ratelimit.ErrLimited {
			if retry > 0 {
				c.Writer.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
			}
			httperr.AbortStatus(c, http.StatusTooManyRequests,
				derrors.New(derrors.CodeRateLimited, "请求频率超限"))
			return
		}
		c.Next()
	}
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
