// channel_sync.go：渠道同步模型 handler。
//
// POST /api/admin/channels/:id/sync-models
//   - 加载渠道（含解密凭证）
//   - 查 provider 是否实现 ModelLister
//   - 发 HTTP 请求拉上游 /v1/models 列表
//   - 覆盖 channel.Models 并持久化
//   - 返回 {models, count}

package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

func (h *Handler) syncChannelModels(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	ch, err := h.Channels.GetByID(c.Request.Context(), id)
	if err != nil {
		httperr.Abort(c, err)
		return
	}

	lister := h.Providers.Lister(ch.Provider)
	if lister == nil {
		httperr.BadRequest(c, "该 provider 暂不支持拉取模型列表: "+ch.Provider)
		return
	}

	models, err := lister.ListModels(c.Request.Context(), relay.Upstream{
		ID:          ch.ID,
		Provider:    ch.Provider,
		BaseURL:     ch.BaseURL,
		Credentials: ch.Credentials,
	})
	if err != nil {
		httperr.AbortCode(c, http.StatusBadGateway, derrors.CodeUpstream, "拉取上游模型失败: "+err.Error())
		return
	}

	ch.Models = models
	if err := h.Channels.Update(c.Request.Context(), ch); err != nil {
		httperr.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"count":  len(models),
	})
}
