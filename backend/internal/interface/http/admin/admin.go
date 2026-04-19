// Package admin 实现 /api/admin/* 管理员接口。
//
// 所有路由都经过 AuthSession + RequireAdmin。
package admin

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/app/pricing"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/audit"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/task"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// ProviderChecker 校验 provider 名称是否已注册。
// DIP：不直接依赖 infra/provider。
type ProviderChecker interface {
	// Exists 返回 provider 名称是否存在（sync 或 task 任一即可）。
	Exists(name string) bool
	// Names 返回所有已注册 provider（用于 /providers 列表）。
	Names() []string
	// Lister 返回 provider 对应的 ModelLister（未实现则 nil）。
	// 用于「同步模型」按钮检查能力可用性。
	Lister(name string) relay.ModelLister
}

// QuotaAdjuster 管理员调整用户配额的业务入口（由 billing.Engine.Adjust 实现）。
// 通过 Ledger 记录所有变更，避免直接 UPDATE users 导致余额与账本不一致。
type QuotaAdjuster interface {
	Adjust(ctx context.Context, userID uint64, delta int64, note string) error
}

// PricingSyncer 价格同步器抽象。由 app/pricing.Syncer 实现。
type PricingSyncer interface {
	Sync(ctx context.Context) (*pricing.Result, error)
}

// Handler 管理员接口。
type Handler struct {
	Users     user.Repository
	Groups    user.GroupRepository
	Channels  channel.Repository
	Prices    billing.ModelPriceRepository
	Usages    billing.UsageRepository
	Ledgers   billing.LedgerRepository
	Audits    audit.Repository
	Orders    payment.Repository
	Tasks     task.Repository
	Subs      *subapp.Service
	Providers ProviderChecker
	QuotaAdj  QuotaAdjuster
	Pricing   PricingSyncer
	// DB 直连，供激活码批量生成等暂无独立 repo 的场景使用。
	DB *gorm.DB
}

// 供 redemption.go 等文件复用。
func (h *Handler) db() *gorm.DB { return h.DB }

// Register 挂载路由（group 已经应用 AuthSession+RequireAdmin）。
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/providers", h.providers)

	g.GET("/users", h.listUsers)
	g.PUT("/users/:id/quota", h.updateUserQuota)
	g.PUT("/users/:id/status", h.updateUserStatus)
	g.PUT("/users/:id/rpm-limit", h.updateUserRPMLimit)

	g.GET("/groups", h.listGroups)
	g.POST("/groups", h.createGroup)
	g.PUT("/groups/:id", h.updateGroup)
	g.DELETE("/groups/:id", h.deleteGroup)

	g.GET("/channels", h.listChannels)
	g.POST("/channels", h.createChannel)
	g.GET("/channels/:id", h.getChannel)
	g.PUT("/channels/:id", h.updateChannel)
	g.DELETE("/channels/:id", h.deleteChannel)
	g.POST("/channels/:id/sync-models", h.syncChannelModels)

	g.GET("/models", h.listModels)
	g.PUT("/models", h.upsertModel)
	g.DELETE("/models/:id", h.deleteModel)
	g.POST("/models/sync-pricing", h.syncPricing)

	g.GET("/logs/usages", h.listUsages)
	g.GET("/stats", h.stats)

	// 激活码批量
	g.POST("/redemption-batches", h.createBatch)
	g.GET("/redemption-batches", h.listBatches)
	g.GET("/redemptions", h.listRedemptions)

	if h.Audits != nil {
		g.GET("/audits", h.listAudits)
	}
	if h.Orders != nil {
		g.GET("/orders", h.listAdminOrders)
	}
	if h.Tasks != nil {
		g.GET("/tasks", h.listAdminTasks)
	}
	if h.Subs != nil {
		g.GET("/plans", h.listPlans)
		g.PUT("/plans", h.upsertPlan)
		g.DELETE("/plans/:id", h.deletePlan)
		g.POST("/subscriptions/grant", h.grantSubscription)
	}
}

// ---------- providers ----------

func (h *Handler) providers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"providers": h.Providers.Names()})
}

// ---------- users ----------

func (h *Handler) listUsers(c *gin.Context) {
	offset, limit := parsePage(c)
	items, total, err := h.Users.List(c.Request.Context(), offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

type quotaReq struct {
	Quota int64 `json:"quota"`
}

func (h *Handler) updateUserQuota(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req quotaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	// 走 Engine.Adjust 写账本：先查当前 quota，再算差值
	u, err := h.Users.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	delta := req.Quota - u.Quota
	if delta == 0 {
		c.JSON(http.StatusOK, gin.H{"ok": true, "delta": 0})
		return
	}
	if h.QuotaAdj == nil {
		// 未注入则退化为直接 SetQuota（不推荐，仅开发期兜底）
		if err := h.Users.SetQuota(c.Request.Context(), id, req.Quota); err != nil {
			httperr.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "delta": delta, "warn": "未走账本"})
		return
	}
	if err := h.QuotaAdj.Adjust(c.Request.Context(), id, delta, "admin 调整配额"); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "delta": delta})
}

type statusReq struct {
	Status string `json:"status"`
}

func (h *Handler) updateUserStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req statusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	u, err := h.Users.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	u.Status = user.Status(req.Status)
	if err := h.Users.Update(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type rpmLimitReq struct {
	RPMLimit int `json:"rpm_limit"`
}

// updateUserRPMLimit 设置用户级每分钟请求数上限。0 = 不使用用户级限制。
func (h *Handler) updateUserRPMLimit(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req rpmLimitReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if req.RPMLimit < 0 {
		httperr.BadRequest(c, "rpm_limit 必须 >= 0")
		return
	}
	u, err := h.Users.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	u.RPMLimit = req.RPMLimit
	if err := h.Users.Update(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "rpm_limit": req.RPMLimit})
}

// ---------- groups ----------

func (h *Handler) listGroups(c *gin.Context) {
	items, err := h.Groups.List(c.Request.Context())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

type groupReq struct {
	Name            string  `json:"name"`
	PriceMultiplier float64 `json:"price_multiplier"`
}

func (h *Handler) createGroup(c *gin.Context) {
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	g := &user.Group{Name: req.Name, PriceMultiplier: req.PriceMultiplier}
	if err := h.Groups.Create(c.Request.Context(), g); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

func (h *Handler) updateGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	g, err := h.Groups.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	g.Name = req.Name
	g.PriceMultiplier = req.PriceMultiplier
	if err := h.Groups.Update(c.Request.Context(), g); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

func (h *Handler) deleteGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.Groups.Delete(c.Request.Context(), id); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- channels ----------

func (h *Handler) listChannels(c *gin.Context) {
	offset, limit := parsePage(c)
	items, total, err := h.Channels.List(c.Request.Context(), offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (h *Handler) getChannel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ch, err := h.Channels.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

type channelReq struct {
	Name            string   `json:"name"`
	Provider        string   `json:"provider"`
	BaseURL         string   `json:"base_url"`
	Credentials     string   `json:"credentials"`
	Models          []string `json:"models"`
	GroupIDs        []uint64 `json:"group_ids"`
	Weight          int      `json:"weight"`
	PriceMultiplier float64  `json:"price_multiplier"`
	Status          string   `json:"status"`
	Note            string   `json:"note"`
}

func (h *Handler) createChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if !h.Providers.Exists(req.Provider) {
		httperr.BadRequest(c, "未知 provider: "+req.Provider)
		return
	}
	ch := &channel.Channel{
		Name:            req.Name,
		Provider:        req.Provider,
		BaseURL:         req.BaseURL,
		Credentials:     req.Credentials,
		Models:          req.Models,
		GroupIDs:        req.GroupIDs,
		Weight:          defaultIntIfZero(req.Weight, 100),
		PriceMultiplier: defaultFloatIfZero(req.PriceMultiplier, 1.0),
		Status:          channel.Status(defaultStr(req.Status, "active")),
		Note:            req.Note,
	}
	if err := h.Channels.Create(c.Request.Context(), ch); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (h *Handler) updateChannel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	ch, err := h.Channels.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	ch.Name = req.Name
	ch.Provider = req.Provider
	ch.BaseURL = req.BaseURL
	if req.Credentials != "" {
		ch.Credentials = req.Credentials
	}
	ch.Models = req.Models
	ch.GroupIDs = req.GroupIDs
	ch.Weight = defaultIntIfZero(req.Weight, 100)
	ch.PriceMultiplier = defaultFloatIfZero(req.PriceMultiplier, 1.0)
	ch.Status = channel.Status(defaultStr(req.Status, "active"))
	ch.Note = req.Note
	if err := h.Channels.Update(c.Request.Context(), ch); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (h *Handler) deleteChannel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.Channels.Delete(c.Request.Context(), id); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- models ----------

func (h *Handler) listModels(c *gin.Context) {
	items, err := h.Prices.List(c.Request.Context())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

type modelReq struct {
	Model            string  `json:"model"`
	Capability       string  `json:"capability"`
	InputPrice       int64   `json:"input_price"`
	OutputPrice      int64   `json:"output_price"`
	CachePrice       int64   `json:"cache_price"`
	OutputMultiplier float64 `json:"output_multiplier"`
	TaskPrice        int64   `json:"task_price"`
	Enabled          bool    `json:"enabled"`
}

func (h *Handler) upsertModel(c *gin.Context) {
	var req modelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	p := &billing.ModelPrice{
		Model:            req.Model,
		Capability:       billing.Capability(defaultStr(req.Capability, "chat")),
		InputPrice:       req.InputPrice,
		OutputPrice:      req.OutputPrice,
		CachePrice:       req.CachePrice,
		OutputMultiplier: defaultFloatIfZero(req.OutputMultiplier, 1.0),
		TaskPrice:        req.TaskPrice,
		Enabled:          req.Enabled,
	}
	if err := h.Prices.Upsert(c.Request.Context(), p); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (h *Handler) deleteModel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.Prices.Delete(c.Request.Context(), id); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- logs ----------

func (h *Handler) listUsages(c *gin.Context) {
	offset, limit := parsePage(c)
	items, total, err := h.Usages.ListAll(c.Request.Context(), offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

// ---------- helpers ----------

func parsePage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size < 1 || size > 500 {
		size = 20
	}
	return (page - 1) * size, size
}

func defaultIntIfZero(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

func defaultFloatIfZero(v, d float64) float64 {
	if v == 0 {
		return d
	}
	return v
}

func defaultStr(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
