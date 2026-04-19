// verify.go —— 邮箱验证 + 密码重置 handler。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	verifyapp "github.com/yishuiliunian/nexusapi/backend/internal/app/verify"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// mountVerify 挂载 /api/auth/{verify,resend,forgot,reset} 四个公开/半公开端点。
// Verify Service nil 时相关端点不挂载。
func (h *Handler) mountVerify(public, private *gin.RouterGroup) {
	if h.Verify == nil {
		return
	}
	public.POST("/auth/verify", h.verifyEmail)
	public.POST("/auth/forgot", h.forgotPassword)
	public.POST("/auth/reset", h.resetPassword)
	private.POST("/auth/resend", h.resendVerifyEmail)
}

type verifyEmailReq struct {
	Token string `json:"token" binding:"required"`
}

func (h *Handler) verifyEmail(c *gin.Context) {
	var req verifyEmailReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if err := h.Verify.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type forgotReq struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *Handler) forgotPassword(c *gin.Context) {
	var req forgotReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	// 总是返回 200，防邮箱枚举
	_ = h.Verify.SendPasswordReset(c.Request.Context(), req.Email)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type resetReq struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

func (h *Handler) resetPassword(c *gin.Context) {
	var req resetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	if err := h.Verify.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) resendVerifyEmail(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u.EmailVerified {
		httperr.BadRequest(c, "邮箱已验证")
		return
	}
	if _, err := h.Verify.SendEmailVerification(c.Request.Context(), u); err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// 类型绑定锚点。
var _ = (*verifyapp.Service)(nil)
