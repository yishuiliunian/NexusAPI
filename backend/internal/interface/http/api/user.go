// user.go 用户信息、ApiKey CRUD、Usage/Ledger 查询。
package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

func (h *Handler) me(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		httperr.Unauthorized(c, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":              u.ID,
		"email":           u.Email,
		"email_verified":  u.EmailVerified,
		"role":            u.Role,
		"quota":           u.Quota,
		"used_quota":      u.UsedQuota,
		"quota_alert_at":  u.QuotaAlertAt,
		"status":          u.Status,
	})
}

type quotaAlertReq struct {
	QuotaAlertAt int64 `json:"quota_alert_at"` // 0 = 关闭
}

// updateQuotaAlert 用户设置"余额低于阈值告警"触发值（micro-unit）。
// 重置后 QuotaAlertSentAt 置 nil，保证下次触达立刻发送而不是陷在冷却期。
func (h *Handler) updateQuotaAlert(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req quotaAlertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if req.QuotaAlertAt < 0 {
		httperr.BadRequest(c, "阈值不能为负")
		return
	}
	u.QuotaAlertAt = req.QuotaAlertAt
	u.QuotaAlertSentAt = nil
	if err := h.Users.Update(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "quota_alert_at": u.QuotaAlertAt})
}

type createKeyReq struct {
	Name           string   `json:"name"`
	ModelWhitelist []string `json:"model_whitelist"`
	QuotaLimit     int64    `json:"quota_limit"`
}

func (h *Handler) listApiKeys(c *gin.Context) {
	u := middleware.CurrentUser(c)
	keys, err := h.ApiKey.ListByUser(c.Request.Context(), u.ID)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	out := make([]gin.H, 0, len(keys))
	for _, k := range keys {
		out = append(out, gin.H{
			"id":              k.ID,
			"name":            k.Name,
			"prefix":          k.KeyPrefix,
			"suffix":          k.KeySuffix,
			"model_whitelist": k.ModelWhitelist,
			"quota_limit":     k.QuotaLimit,
			"used_quota":      k.UsedQuota,
			"status":          k.Status,
			"expires_at":      k.ExpiresAt,
			"last_used_at":    k.LastUsedAt,
			"created_at":      k.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": len(out)})
}

func (h *Handler) createApiKey(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req createKeyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	res, err := h.ApiKey.Create(c.Request.Context(), u.ID, strings.TrimSpace(req.Name),
		req.ModelWhitelist, req.QuotaLimit, nil)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":     res.Key.ID,
		"name":   res.Key.Name,
		"secret": res.Secret,
		"prefix": res.Key.KeyPrefix,
		"suffix": res.Key.KeySuffix,
	})
}

func (h *Handler) deleteApiKey(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		httperr.BadRequest(c, "invalid id")
		return
	}
	if err := h.ApiKey.Delete(c.Request.Context(), u.ID, id); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listUsages(c *gin.Context) {
	u := middleware.CurrentUser(c)
	offset, limit := parsePage(c)
	items, total, err := h.Usages.ListByUser(c.Request.Context(), u.ID, offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (h *Handler) listLedgers(c *gin.Context) {
	u := middleware.CurrentUser(c)
	offset, limit := parsePage(c)
	items, total, err := h.Ledgers.ListByUser(c.Request.Context(), u.ID, offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}
