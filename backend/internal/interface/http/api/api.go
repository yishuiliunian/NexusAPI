// Package api 实现 /api/* 前端接口（基于 Session 鉴权）。
//
// 按职责拆分为四个文件：auth.go / user.go / billing.go / twofa.go。
// Handler 为同一个类型方便注入，路由也在这里统一注册，避免调用方多 handler 组装负担。
package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/auth"
	oauthapp "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
	paymentapp "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/redemption"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	verifyapp "github.com/yishuiliunian/nexusapi/backend/internal/app/verify"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
)

// Handler 聚合 /api/* 所需的各应用服务。
type Handler struct {
	Auth       *auth.Service
	ApiKey     *keysapp.Service
	Usages     billing.UsageRepository
	Ledgers    billing.LedgerRepository
	Redemption *redemption.Service
	Payments   *paymentapp.Service
	Orders     payment.Repository
	Subs       *subapp.Service
	Verify     *verifyapp.Service
	OAuth      *oauthapp.Service
	// OAuthRedirectURI 由调用方（router）提供，返回"公开 callback 路径"，
	// 通常是 {site.base_url}/api/auth/oauth/{provider}/callback。
	OAuthRedirectURI func(provider string) string
	// OAuthPostLoginURL 登录成功后跳转目标；默认 /。
	OAuthPostLoginURL string
	Users      user.Repository
}

// Register 挂载路由到 group（通常是 /api）。
func (h *Handler) Register(public, private *gin.RouterGroup) {
	// auth
	public.POST("/auth/register", h.register)
	public.POST("/auth/login", h.login)
	private.POST("/auth/logout", h.logout)

	// user
	private.GET("/user/me", h.me)
	private.PUT("/user/quota-alert", h.updateQuotaAlert)
	private.GET("/user/apikeys", h.listApiKeys)
	private.POST("/user/apikeys", h.createApiKey)
	private.DELETE("/user/apikeys/:id", h.deleteApiKey)
	private.GET("/user/usages", h.listUsages)
	private.GET("/user/ledgers", h.listLedgers)
	private.GET("/user/stats", h.userStats)

	// billing
	if h.Redemption != nil {
		private.POST("/billing/redeem", h.redeem)
	}
	if h.Payments != nil {
		private.POST("/billing/topup", h.topup)
		private.GET("/billing/gateways", h.gateways)
	}
	if h.Orders != nil {
		private.GET("/billing/orders", h.listOrders)
	}
	h.mountSubscription(private)
	h.mountVerify(public, private)
	h.mountOAuth(public)

	// 2fa
	if h.Users != nil {
		private.POST("/auth/2fa/setup", h.twoFASetup)
		private.POST("/auth/2fa/enable", h.twoFAEnable)
		private.POST("/auth/2fa/disable", h.twoFADisable)
	}
}

// parsePage 提取分页参数。所有 handler 文件共用。
func parsePage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size < 1 || size > 200 {
		size = 20
	}
	return (page - 1) * size, size
}
