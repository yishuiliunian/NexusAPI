// Package github 实现 app/oauth.Provider 契约。
//
// GitHub OAuth App：
//   - Authorize: https://github.com/login/oauth/authorize?client_id=...&scope=read:user user:email&state=...
//   - Token    : POST https://github.com/login/oauth/access_token
//   - User     : GET https://api.github.com/user  +  /user/emails（拉 primary email）
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	appOAuth "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
)

// Name provider 标识。
const Name = "github"

// Config 应用信息。
type Config struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
	// AuthorizeURL / TokenURL / APIBase 保留测试可覆盖默认值。
	AuthorizeURL string
	TokenURL     string
	APIBase      string
}

// Provider GitHub 实现。
type Provider struct {
	cfg Config
}

// New 构造。
func New(cfg Config) *Provider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = "https://github.com/login/oauth/authorize"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://github.com/login/oauth/access_token"
	}
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.github.com"
	}
	return &Provider{cfg: cfg}
}

// Name 实现。
func (p *Provider) Name() string { return Name }

// AuthorizeURL 拼接跳转地址。
func (p *Provider) AuthorizeURL(state, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", p.cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	return p.cfg.AuthorizeURL + "?" + q.Encode()
}

// Exchange code → access_token → user profile。
func (p *Provider) Exchange(ctx context.Context, code, redirectURI string) (*appOAuth.Profile, error) {
	form := url.Values{}
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github token %d: %s", resp.StatusCode, raw)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("github token decode: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("github: empty access_token (%s)", tok.Error)
	}

	// 拉取用户主资料
	userReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.APIBase+"/user", nil)
	userReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	userReq.Header.Set("Accept", "application/vnd.github+json")
	userResp, err := p.cfg.HTTPClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("github user: %w", err)
	}
	defer userResp.Body.Close()
	userRaw, _ := io.ReadAll(userResp.Body)
	if userResp.StatusCode >= 400 {
		return nil, fmt.Errorf("github user %d: %s", userResp.StatusCode, userRaw)
	}
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(userRaw, &u); err != nil {
		return nil, fmt.Errorf("github user decode: %w", err)
	}
	email := u.Email
	// 主资料中的 email 常为 null（用户隐藏了），退而从 /user/emails 拉 primary
	if email == "" {
		email = p.fetchPrimaryEmail(ctx, tok.AccessToken)
	}
	return &appOAuth.Profile{
		ExternalID: strconv.FormatInt(u.ID, 10),
		Email:      email,
		Name:       firstNonEmpty(u.Name, u.Login),
	}, nil
}

// fetchPrimaryEmail 尝试拉主邮箱；失败返回空串（调用方会合成占位 email）。
func (p *Provider) fetchPrimaryEmail(ctx context.Context, token string) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.APIBase+"/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	var list []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return ""
	}
	for _, e := range list {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	return ""
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
