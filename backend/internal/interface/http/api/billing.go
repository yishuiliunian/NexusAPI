// billing.go 兑换码等面向终端用户的计费接口。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

type redeemReq struct {
	Code string `json:"code"`
}

func (h *Handler) redeem(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req redeemReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	amt, err := h.Redemption.Redeem(c.Request.Context(), u.ID, req.Code)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"amount": amt})
}

// ---------- 充值（Stripe / Creem / ...） ----------

type topupReq struct {
	AmountCents int64  `json:"amount_cents" binding:"required,min=100"`
	Currency    string `json:"currency"` // 默认 USD
	Gateway     string `json:"gateway" binding:"required"`
}

func (h *Handler) topup(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req topupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	order, err := h.Payments.CreateTopUp(c.Request.Context(),
		u.ID, req.AmountCents, req.Currency, req.Gateway)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"order_id":     order.ID,
		"checkout_url": order.CheckoutURL,
		"amount_cents": order.AmountCents,
		"currency":     order.Currency,
		"gateway":      order.Gateway,
	})
}

// gateways 返回可用网关名列表，前端展示充值方式按钮用。
func (h *Handler) gateways(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"gateways": h.Payments.Gateways()})
}

func (h *Handler) listOrders(c *gin.Context) {
	u := middleware.CurrentUser(c)
	offset, limit := parsePage(c)
	items, total, err := h.Orders.ListByUser(c.Request.Context(), u.ID, offset, limit)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}
