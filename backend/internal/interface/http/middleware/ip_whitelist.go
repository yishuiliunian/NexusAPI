// ip_whitelist.go —— ApiKey 的 IP 白名单前置检查。
//
// 必须在 AuthApiKey 之后使用，以便拿到 CtxApiKey。
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// CheckApiKeyIP 校验当前请求 IP 是否在 ApiKey 的 IPWhitelist 中。
// 空白名单视为不限（默认开放）。
func CheckApiKeyIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		k := CurrentApiKey(c)
		if k == nil {
			c.Next()
			return
		}
		if !k.AllowIP(c.ClientIP()) {
			httperr.AbortStatus(c, http.StatusForbidden,
				derrors.New(derrors.CodePermissionDenied, "IP 不在白名单"))
			return
		}
		c.Next()
	}
}
