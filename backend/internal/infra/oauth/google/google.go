// Package google 实现 app/oauth.Provider 契约。
//
// 使用 OpenID Connect 端点：
//   - Authorize: https://accounts.google.com/o/oauth2/v2/auth
//   - Token    : https://oauth2.googleapis.com/token
//   - UserInfo : https://openidconnect.googleapis.com/v1/userinfo
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	appOAuth "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
)

// Name provider 标识。
const Name = "google"

// Config Google 应用配置。
type Config struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
}

// Provider Google OIDC 实现。
type Provider struct {
	cfg Config
}

// New 构造。
func New(cfg Config) *Provider {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if cfg.UserInfoURL == "" {
		cfg.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
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
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("access_type", "online")
	return p.cfg.AuthorizeURL + "?" + q.Encode()
}

// Exchange code → token → userinfo。
func (p *Provider) Exchange(ctx context.Context, code, redirectURI string) (*appOAuth.Profile, error) {
	form := url.Values{}
	form.Set("client_id", p.cfg.ClientID)
	form.Set("client_secret", p.cfg.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google token: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("google token %d: %s", resp.StatusCode, raw)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("google token decode: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("google: empty access_token")
	}

	// UserInfo
	uReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.UserInfoURL, nil)
	uReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uResp, err := p.cfg.HTTPClient.Do(uReq)
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer uResp.Body.Close()
	if uResp.StatusCode >= 400 {
		body, _ := io.ReadAll(uResp.Body)
		return nil, fmt.Errorf("google userinfo %d: %s", uResp.StatusCode, body)
	}
	var u struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(uResp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("google userinfo decode: %w", err)
	}
	return &appOAuth.Profile{
		ExternalID: u.Sub,
		Email:      u.Email,
		Name:       u.Name,
	}, nil
}
