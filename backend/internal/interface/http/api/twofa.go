// twofa.go 两步验证相关 handler。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/app/twofa"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

type twoFAEnableReq struct {
	Code string `json:"code"`
}

func (h *Handler) twoFASetup(c *gin.Context) {
	u := middleware.CurrentUser(c)
	secret, err := twofa.GenerateSecret()
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	u.TwoFASecret = secret
	if err := h.Users.Update(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"secret": secret,
		"url":    twofa.OtpauthURL(secret, u.Email, "NexusAPI"),
	})
}

func (h *Handler) twoFAEnable(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var req twoFAEnableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if u.TwoFASecret == "" {
		httperr.BadRequest(c, "未初始化 2FA，先 /setup")
		return
	}
	if !twofa.Verify(u.TwoFASecret, req.Code) {
		httperr.AbortStatus(c, http.StatusUnauthorized,
			derrors.New(derrors.CodeUnauthenticated, "2FA 码错误"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) twoFADisable(c *gin.Context) {
	u := middleware.CurrentUser(c)
	u.TwoFASecret = ""
	if err := h.Users.Update(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
