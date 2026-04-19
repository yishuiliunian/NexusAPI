// stats.go —— Dashboard 聚合统计接口。
//
// GET /api/user/stats        当前登录 user 的 7/30 天聚合
// GET /api/admin/stats       全局统计（仅 admin）
//
// 参数：
//   days=7          聚合时间窗口（默认 7，最大 90）
//
// 返回：
//   {
//     "summary":    {quota, used_quota, total_requests, total_cost, success_rate},
//     "by_day":     [{date, requests, prompt_tokens, completion_tokens, cost}],
//     "by_model":   [{model, requests, cost}],
//     "by_capability": [{capability, requests, cost}],
//     "by_status":  [{status, requests}]
//   }
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

func (h *Handler) userStats(c *gin.Context) {
	u := middleware.CurrentUser(c)
	days := parseDays(c)
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	ctx := c.Request.Context()

	byDay, err := h.Usages.AggByDay(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byModel, err := h.Usages.AggByModel(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byCap, err := h.Usages.AggByCapability(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byStatus, err := h.Usages.AggByStatus(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	totalReq, err := h.Usages.CountRequestsByUser(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	totalCost, err := h.Usages.SumCostByUser(ctx, u.ID, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}

	// 成功率
	var success, failed int64
	for _, s := range byStatus {
		if s.Status == "success" {
			success = s.Requests
		} else {
			failed += s.Requests
		}
	}
	var rate float64
	if total := success + failed; total > 0 {
		rate = float64(success) / float64(total)
	}

	// 防御：nil slice → []，前端 .length 不崩
	if byDay == nil {
		byDay = []billing.DailyAgg{}
	}
	if byModel == nil {
		byModel = []billing.ModelAgg{}
	}
	if byCap == nil {
		byCap = []billing.CapabilityAgg{}
	}
	if byStatus == nil {
		byStatus = []billing.StatusAgg{}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"quota":          u.Quota,
			"used_quota":     u.UsedQuota,
			"total_requests": totalReq,
			"total_cost":     totalCost,
			"success_rate":   rate,
			"since":          since.Format(time.RFC3339),
			"days":           days,
		},
		"by_day":        byDay,
		"by_model":      byModel,
		"by_capability": byCap,
		"by_status":     byStatus,
	})
}

func parseDays(c *gin.Context) int {
	d, _ := strconv.Atoi(c.Query("days"))
	if d < 1 {
		d = 7
	}
	if d > 90 {
		d = 90
	}
	return d
}