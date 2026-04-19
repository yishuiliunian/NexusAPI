// stats.go —— Admin 全局统计。
//
// GET /api/admin/stats      — 最近 N 天的全局聚合
//
// 返回：
//   {
//     "summary":  {total_requests, total_cost, active_users, success_rate},
//     "by_day":   [{date, requests, prompt_tokens, completion_tokens, cost}],
//     "by_model": [...],
//     "by_capability": [...],
//     "by_status":[...],
//     "top_users":[{user_id, email, requests, cost}]
//   }
package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

func (h *Handler) stats(c *gin.Context) {
	days := parseDays(c)
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	ctx := c.Request.Context()

	byDay, err := h.Usages.AggByDay(ctx, 0, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byModel, err := h.Usages.AggByModel(ctx, 0, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byCap, err := h.Usages.AggByCapability(ctx, 0, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	byStatus, err := h.Usages.AggByStatus(ctx, 0, since)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	topUsers, err := h.Usages.TopUsersByCost(ctx, since, 10)
	if err != nil {
		httperr.Abort(c, err)
		return
	}

	// 汇总
	var totalReq, totalCost, success, failed int64
	for _, d := range byDay {
		totalReq += d.Requests
		totalCost += d.Cost
	}
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
	if topUsers == nil {
		topUsers = []billing.TopUserAgg{}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"total_requests": totalReq,
			"total_cost":     totalCost,
			"active_users":   len(topUsers),
			"success_rate":   rate,
			"since":          since.Format(time.RFC3339),
			"days":           days,
		},
		"by_day":        byDay,
		"by_model":      byModel,
		"by_capability": byCap,
		"by_status":     byStatus,
		"top_users":     topUsers,
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