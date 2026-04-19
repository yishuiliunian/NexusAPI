// pricing_sync.go：从 LiteLLM 同步模型价格 handler。
//
// POST /api/admin/models/sync-pricing
//   - 委托给 app/pricing.Syncer
//   - 覆盖所有非 task 类价格，task 类手动维护
//   - 返回 {inserted, deleted, skipped}

package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"

	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

func (h *Handler) syncPricing(c *gin.Context) {
	if h.Pricing == nil {
		httperr.AbortCode(c, http.StatusServiceUnavailable, derrors.CodeInternal, "价格同步器未配置")
		return
	}
	result, err := h.Pricing.Sync(c.Request.Context())
	if err != nil {
		httperr.AbortCode(c, http.StatusBadGateway, derrors.CodeUpstream, "拉取 LiteLLM 失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
