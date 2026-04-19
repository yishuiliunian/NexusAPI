// Package auth 提供认证服务：注册/登录/会话/密码校验。
//
// 设计：
//   - 密码用 bcrypt（cost=10）
//   - Session 32 字节随机 token，base64url 编码，存 sessions 表
//   - 提供 middleware 用的 Authenticate 方法
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// Service 认证服务。
type Service struct {
	users    user.Repository
	sessions user.SessionRepository
	// SessionTTL 会话有效期。
	SessionTTL time.Duration
}

// NewService 构造。ttl 为会话有效期（0 或负数回退到 30 天）。
func NewService(users user.Repository, sessions user.SessionRepository, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &Service{users: users, sessions: sessions, SessionTTL: ttl}
}

// Register 注册新用户。
func (s *Service) Register(ctx context.Context, email, password string) (*user.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, derrors.New(derrors.CodeInvalidArgument, "邮箱格式无效")
	}
	if len(password) < 8 {
		return nil, derrors.New(derrors.CodeInvalidArgument, "密码至少 8 位")
	}

	if existing, err := s.users.GetByEmail(ctx, email); err == nil && existing != nil {
		return nil, derrors.ErrAlreadyExists
	} else if err != nil && !derrors.Is(err, derrors.CodeNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "bcrypt", err)
	}

	u := &user.User{
		Email:        email,
		PasswordHash: string(hash),
		Role:         user.RoleUser,
		Status:       user.StatusActive,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Login 校验邮箱密码，签发 session。
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (*user.User, *user.Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if derrors.Is(err, derrors.CodeNotFound) {
			return nil, nil, derrors.New(derrors.CodeUnauthenticated, "邮箱或密码错误")
		}
		return nil, nil, err
	}
	if !u.Active() {
		return nil, nil, derrors.New(derrors.CodePermissionDenied, "账号已禁用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, nil, derrors.New(derrors.CodeUnauthenticated, "邮箱或密码错误")
	}

	sess, err := s.newSession(u.ID, ip, ua)
	if err != nil {
		return nil, nil, err
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, nil, err
	}
	return u, sess, nil
}

// Logout 删除 session。
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// NewSession 为已通过其他方式鉴权过的 user 建立会话（OAuth 登录用）。
// 调用方需保证 u != nil 且已通过可信身份校验。
func (s *Service) NewSession(ctx context.Context, u *user.User, ip, ua string) (*user.Session, error) {
	sess, err := s.newSession(u.ID, ip, ua)
	if err != nil {
		return nil, err
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Authenticate 通过 session token 返回用户。
func (s *Service) Authenticate(ctx context.Context, sessionID string) (*user.User, error) {
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, derrors.ErrUnauthenticated
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active() {
		return nil, derrors.New(derrors.CodePermissionDenied, "账号已禁用")
	}
	return u, nil
}

// newSession 生成随机 session。
func (s *Service) newSession(userID uint64, ip, ua string) (*user.Session, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "rand", err)
	}
	return &user.Session{
		ID:        base64.RawURLEncoding.EncodeToString(buf),
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.SessionTTL),
		IP:        ip,
		UserAgent: ua,
		CreatedAt: time.Now(),
	}, nil
}

// HashPassword 给外部（种子数据、测试）用的密码 hash 辅助。
func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt: %w", err)
	}
	return string(h), nil
}
