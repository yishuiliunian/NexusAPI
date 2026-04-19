// auth.go 认证相关 handler：register / login / logout。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

type authReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

func (h *Handler) register(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	u, err := h.Auth.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "email": u.Email})
}

func (h *Handler) login(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}
	u, s, err := h.Auth.Login(c.Request.Context(), req.Email, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	ttlSec := int(h.Auth.SessionTTL.Seconds())
	c.SetCookie(middleware.CookieSess, s.ID, ttlSec, "/", "", false, true)
	middleware.SetCSRFCookie(c, middleware.NewCSRFToken(), ttlSec)
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "email": u.Email, "role": u.Role})
}

func (h *Handler) logout(c *gin.Context) {
	cookie, _ := c.Cookie(middleware.CookieSess)
	if cookie != "" {
		_ = h.Auth.Logout(c.Request.Context(), cookie)
	}
	c.SetCookie(middleware.CookieSess, "", -1, "/", "", false, true)
	c.SetCookie(middleware.CookieCSRF, "", -1, "/", "", false, false)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
