// Package webhook 汇集面向外部网关的回调入口（公开路径，仅靠签名自证）。
package webhook

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	paymentapp "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// Handler webhook 路由承载。
type Handler struct {
	Payments *paymentapp.Service
}

// Register 挂载路由。注意：不得前置 AuthSession/AuthApiKey，
// Stripe 的回调没有用户身份，仅以签名为信任根。
func (h *Handler) Register(g *gin.RouterGroup) {
	if h.Payments == nil {
		return
	}
	g.POST("/stripe", h.stripe)
}

func (h *Handler) stripe(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httperr.Abort(c, derrors.New(derrors.CodeInvalidArgument, "read body"))
		return
	}
	sig := c.GetHeader("Stripe-Signature")
	if err := h.Payments.HandleWebhook(c.Request.Context(), "stripe", body, sig); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
