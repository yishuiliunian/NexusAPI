// Package oauth 提供通用 OAuth2 登录编排。
//
// 设计：每家 provider 只需实现 Provider 接口（Authorize URL + 代码换 token + 拉取用户资料）。
// Service.LoginOrSignup：
//   1. 根据回调 code 调 provider 拿到用户资料（id + email）
//   2. 若 OAuthBinding 已存在 → 直接返回该 user
//   3. 否则按 email 查 user：存在则补绑定；不存在则新建 user + binding
//
// 自动建号的 user.EmailVerified = true（第三方已保证邮箱可达）。
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	doauth "github.com/yishuiliunian/nexusapi/backend/internal/domain/oauth"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// Profile 第三方用户资料（由 provider 解析完上游响应后返回）。
type Profile struct {
	ExternalID string
	Email      string
	Name       string
}

// Provider 单家 OAuth2 提供方抽象。
type Provider interface {
	// Name 标识，用于 binding 与路由。
	Name() string
	// AuthorizeURL 生成跳转地址。state 由 Service 注入并核对。
	AuthorizeURL(state, redirectURI string) string
	// Exchange 用 code 换 token，并拉取用户资料。
	Exchange(ctx context.Context, code, redirectURI string) (*Profile, error)
}

// Service OAuth 登录编排。
type Service struct {
	providers map[string]Provider
	bindings  doauth.Repository
	users     user.Repository
}

// NewService 构造。
func NewService(bindings doauth.Repository, users user.Repository, providers ...Provider) *Service {
	m := make(map[string]Provider, len(providers))
	for _, p := range providers {
		if p != nil {
			m[p.Name()] = p
		}
	}
	return &Service{providers: m, bindings: bindings, users: users}
}

// Providers 返回已注册 provider 名（供前端展示登录按钮用）。
func (s *Service) Providers() []string {
	out := make([]string, 0, len(s.providers))
	for k := range s.providers {
		out = append(out, k)
	}
	return out
}

// StartAuthorize 生成 state + authorize url。state 需要由前端存储（cookie/session），
// callback 阶段与之比对防 CSRF。
func (s *Service) StartAuthorize(providerName, redirectURI string) (authURL, state string, err error) {
	p, ok := s.providers[providerName]
	if !ok {
		return "", "", derrors.New(derrors.CodeNotFound, "oauth provider 未配置: "+providerName)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", derrors.Wrap(derrors.CodeInternal, "state gen", err)
	}
	state = base64.RawURLEncoding.EncodeToString(buf)
	return p.AuthorizeURL(state, redirectURI), state, nil
}

// HandleCallback 处理 OAuth 回调，返回（或创建）本地 user 实体。
// redirectURI 必须与 StartAuthorize 一致，否则 provider 会拒绝。
func (s *Service) HandleCallback(ctx context.Context, providerName, code, redirectURI string) (*user.User, error) {
	p, ok := s.providers[providerName]
	if !ok {
		return nil, derrors.New(derrors.CodeNotFound, "oauth provider 未配置")
	}
	prof, err := p.Exchange(ctx, code, redirectURI)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeUpstream, "oauth exchange", err)
	}
	if prof.ExternalID == "" {
		return nil, derrors.New(derrors.CodeInternal, "provider 未返回 external id")
	}

	// 1) binding 已存在
	b, err := s.bindings.GetByProviderExternal(ctx, p.Name(), prof.ExternalID)
	if err == nil {
		return s.users.GetByID(ctx, b.UserID)
	}
	if !derrors.Is(err, derrors.CodeNotFound) {
		return nil, err
	}

	// 2) 按 email 关联；空 email 时只能建新号
	var u *user.User
	if prof.Email != "" {
		u, _ = s.users.GetByEmail(ctx, strings.ToLower(prof.Email))
	}
	if u == nil {
		u = &user.User{
			Email:         normalizeEmail(prof),
			EmailVerified: true,
			Role:          user.RoleUser,
			Status:        user.StatusActive,
		}
		if err := s.users.Create(ctx, u); err != nil {
			return nil, err
		}
	}
	// 3) 写 binding
	binding := &doauth.Binding{
		UserID:     u.ID,
		Provider:   p.Name(),
		ExternalID: prof.ExternalID,
		Email:      prof.Email,
	}
	if err := s.bindings.Create(ctx, binding); err != nil {
		return nil, err
	}
	return u, nil
}

// normalizeEmail 保证一定有 email；无则用 external id 合成占位。
func normalizeEmail(p *Profile) string {
	if p.Email != "" {
		return strings.ToLower(p.Email)
	}
	return fmt.Sprintf("%s-%s@oauth.nexusapi.local", url.PathEscape(p.ExternalID), time.Now().Format("20060102"))
}
