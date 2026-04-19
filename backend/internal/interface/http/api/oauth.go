// oauth.go —— GitHub / Google 登录回调与跳转。
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	oauthapp "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

const (
	oauthStateCookie = "nexus_oauth_state"
)

func (h *Handler) mountOAuth(public *gin.RouterGroup) {
	if h.OAuth == nil {
		return
	}
	public.GET("/auth/oauth/providers", h.oauthProviders)
	public.GET("/auth/oauth/:provider/authorize", h.oauthAuthorize)
	public.GET("/auth/oauth/:provider/callback", h.oauthCallback)
}

func (h *Handler) oauthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"providers": h.OAuth.Providers()})
}

// oauthAuthorize 重定向到 provider 的 authorize URL；同时把 state 放入 cookie 防 CSRF。
func (h *Handler) oauthAuthorize(c *gin.Context) {
	provider := c.Param("provider")
	redirectURI := h.OAuthRedirectURI(provider)
	authURL, state, err := h.OAuth.StartAuthorize(provider, redirectURI)
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	c.SetCookie(oauthStateCookie, state, 600, "/", "", false, true)
	c.Redirect(http.StatusFound, authURL)
}

// oauthCallback provider 回调处理。成功后写 session cookie 并跳回前端 dashboard。
func (h *Handler) oauthCallback(c *gin.Context) {
	provider := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		httperr.BadRequest(c, "缺少 code")
		return
	}
	want, err := c.Cookie(oauthStateCookie)
	if err != nil || want == "" || want != state {
		httperr.AbortStatus(c, http.StatusUnauthorized,
			derrors.New(derrors.CodeUnauthenticated, "state 不匹配"))
		return
	}
	c.SetCookie(oauthStateCookie, "", -1, "/", "", false, true)

	u, err := h.OAuth.HandleCallback(c.Request.Context(), provider, code, h.OAuthRedirectURI(provider))
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	// 建 session
	sess, err := h.Auth.NewSession(c.Request.Context(), u, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		httperr.Abort(c, err)
		return
	}
	ttlSec := int(h.Auth.SessionTTL.Seconds())
	c.SetCookie(middleware.CookieSess, sess.ID, ttlSec, "/", "", false, true)
	// 跳回前端；由前端决定落地路径
	target := h.OAuthPostLoginURL
	if target == "" {
		target = "/"
	}
	c.Redirect(http.StatusFound, target)
}

// 类型绑定锚点，避免未引用的 import 报错（当 Handler 字段未设置时）。
var _ = (*oauthapp.Service)(nil)
