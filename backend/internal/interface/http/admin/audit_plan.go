// audit_plan.go —— admin 审计日志查询 + 订阅套餐管理 + 订单 + 异步任务 handler。
package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	dsub2 "github.com/yishuiliunian/nexusapi/backend/internal/domain/subscription"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// ---------- 审计 ----------

func (h *Handler) listAudits(c *gin.Context) {
	offset, limit := parsePage(c)
	items, total, err := h.Audits.List(c.Request.Context(), offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// ---------- 订单 ----------

func (h *Handler) listAdminOrders(c *gin.Context) {
	offset, limit := parsePage(c)
	// 复用 ListByUser 的分页：传 userID=0 表示全量。因为 order repo 只提供 ListByUser，
	// 这里先简单按 userID 过滤（0 时走仓储 List 若提供）。当前实现：不支持全量时返回提示。
	// 为避免额外增加 Repository 方法，复用 GORM 直接查（通过 repo 原方法给出最近所有用户的订单）
	// 简化：ListByUser(0) 语义为全量——在 infra/db 改实现。
	items, total, err := h.Orders.ListByUser(c.Request.Context(), 0, offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// ---------- 异步任务 ----------

func (h *Handler) listAdminTasks(c *gin.Context) {
	offset, limit := parsePage(c)
	items, total, err := h.Tasks.ListAll(c.Request.Context(), offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// ---------- 订阅套餐 ----------

func (h *Handler) listPlans(c *gin.Context) {
	items, err := h.Subs.ListAllPlans(c.Request.Context())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

type planReq struct {
	ID            uint64  `json:"id"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	PriceCents    int64   `json:"price_cents"`
	Currency      string  `json:"currency"`
	PeriodDays    int     `json:"period_days"`
	IncludedQuota int64   `json:"included_quota"`
	GatewayRef    string  `json:"gateway_ref"`
	Enabled       bool    `json:"enabled"`
}

func (h *Handler) upsertPlan(c *gin.Context) {
	var req planReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	p := &dsub2.Plan{
		ID:            req.ID,
		Code:          req.Code,
		Name:          req.Name,
		PriceCents:    req.PriceCents,
		Currency:      req.Currency,
		PeriodDays:    req.PeriodDays,
		IncludedQuota: req.IncludedQuota,
		GatewayRef:    req.GatewayRef,
		Enabled:       req.Enabled,
	}
	if err := h.Subs.UpsertPlan(c.Request.Context(), p); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plan": p})
}

func (h *Handler) deletePlan(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.Subs.DeletePlan(c.Request.Context(), id); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type grantReq struct {
	UserID   uint64 `json:"user_id"`
	PlanCode string `json:"plan_code"`
}

// grantSubscription 管理员手动给 user 开通订阅（免 Stripe，本地发放）。
func (h *Handler) grantSubscription(c *gin.Context) {
	var req grantReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	sub, err := h.Subs.CreateLocal(c.Request.Context(), req.UserID, req.PlanCode)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": sub})
}
