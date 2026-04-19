// Package relayhttp 实现 /v1/* 的本地端点（非透传）。
//
// 历史上这里承载所有 /v1/* 的协议转换中继；字节级透传已迁到 passthrough 包。
// 当前仅保留 GET /v1/models——这是本地聚合视图，不是透传，需要读 channels 表。
package relayhttp

import (
	"net/http"

	"github.com/gin-gonic/gin"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// Handler 承载 /v1/* 本地端点。字段保留原有形状以便外层组装不改。
type Handler struct {
	Selector  *relayapp.Selector
	Billing   *billingapp.Engine
	ApiKey    *keysapp.Service
	Channels  domainchannel.Repository
	RateLimit middleware.RateLimitConfig
}

// Register 挂载路由。
//
// 历史上这里曾承载所有 /v1/* 转发；字节级透传已迁到 passthrough 包。
// 本 handler 现在只保留一个本地端点：GET /models（跨渠道聚合）。
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/models", h.models)
}

// models 返回可用模型列表（聚合所有 active 渠道）。
func (h *Handler) models(c *gin.Context) {
	channels, err := h.Channels.ListActive(c.Request.Context())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	seen := map[string]bool{}
	type item struct {
		ID       string `json:"id"`
		Object   string `json:"object"`
		Provider string `json:"provider"`
	}
	out := []item{}
	for _, ch := range channels {
		for _, mid := range ch.Models {
			if seen[mid] {
				continue
			}
			seen[mid] = true
			out = append(out, item{ID: mid, Object: "model", Provider: ch.Provider})
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": out})
}
