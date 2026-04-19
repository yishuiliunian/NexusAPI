// subscription.go —— /api/billing/plans / subscription / subscribe / unsubscribe。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// Subscriptions 惰性注入；nil 时相关路由不挂载。
func (h *Handler) mountSubscription(private *gin.RouterGroup) {
	if h.Subs == nil {
		return
	}
	private.GET("/billing/plans", h.listPlans)
	private.GET("/billing/subscription", h.currentSubscription)
	private.POST("/billing/subscribe", h.subscribe)
	private.POST("/billing/unsubscribe", h.unsubscribe)
}

type subscribeReq struct {
	PlanCode string `json:"plan_code" binding:"required"`
	// Mode 为 "local" 时绕过 Stripe（赠送/测试用途，需管理员配合），
	// 默认空等同 stripe。
	Mode string `json:"mode"`
}

func (h *Handler) listPlans(c *gin.Context) {
	plans, err := h.Subs.ListPlans(c.Request.Context())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": plans})
}

func (h *Handler) currentSubscription(c *gin.Context) {
	u := middleware.CurrentUser(c)
	sub, err := h.Subs.Current(c.Request.Context(), u.ID)
	if err != nil {
		if derrors.Is(err, derrors.CodeNotFound) {
			c.JSON(http.StatusOK, gin.H{"subscription": nil})
			return
		}
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}

func (h *Handler) subscribe(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req subscribeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	switch req.Mode {
	case "local":
		sub, err := h.Subs.CreateLocal(c.Request.Context(), u.ID, req.PlanCode)
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"subscription": sub})
	case "", "stripe":
		// 走 Stripe Checkout（mode=subscription）
		if h.Payments == nil {
			httperr.Abort(c, derrors.New(derrors.CodeInvalidArgument, "支付网关未启用"))
			return
		}
		order, err := h.Payments.CreateSubscription(c.Request.Context(), u.ID, req.PlanCode, "stripe")
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"order_id":     order.ID,
			"checkout_url": order.CheckoutURL,
		})
	default:
		httperr.BadRequest(c, "unknown mode: "+req.Mode)
	}
}

func (h *Handler) unsubscribe(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if err := h.Subs.Cancel(c.Request.Context(), u.ID); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// 静态保证 Subs 字段存在于 Handler（下面编辑 Handler 结构体时加上）。
var _ = (*subapp.Service)(nil)
