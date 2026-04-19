// Package verify 提供邮箱验证 + 密码重置的业务编排。
package verify

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	dverify "github.com/yishuiliunian/nexusapi/backend/internal/domain/verify"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
	"github.com/yishuiliunian/nexusapi/backend/pkg/mailer"
	"golang.org/x/crypto/bcrypt"
)

// Mailer 对外的发信接口（避免直接依赖 net/smtp）。
type Mailer interface {
	Send(to []string, subject, htmlBody string) error
	Enabled() bool
}

// Service 封装 token 生成 + 邮件发送 + 消费动作。
//
// BaseURL 用于拼接邮件中的验证链接（http://example.com/verify?token=...）。
// TTL 建议：邮箱验证 24h，密码重置 30min（越短越安全，但太短用户抱怨）。
type Service struct {
	tokens        dverify.Repository
	users         user.Repository
	mail          Mailer
	BaseURL       string
	VerifyTTL     time.Duration
	ResetTTL      time.Duration
}

// NewService 构造。
func NewService(tokens dverify.Repository, users user.Repository, mail Mailer, baseURL string) *Service {
	return &Service{
		tokens:    tokens,
		users:     users,
		mail:      mail,
		BaseURL:   strings.TrimRight(baseURL, "/"),
		VerifyTTL: 24 * time.Hour,
		ResetTTL:  30 * time.Minute,
	}
}

// MailEnabled 未配置 SMTP 时为 false。调用方可据此降级（如仅记日志）。
func (s *Service) MailEnabled() bool { return s.mail != nil && s.mail.Enabled() }

// NoopMailer 当 SMTP 未配置时作为占位；Send 总返回 ErrDisabled。
type NoopMailer struct{}

// Send 无操作。
func (NoopMailer) Send([]string, string, string) error { return mailer.ErrDisabled }

// Enabled 总是 false。
func (NoopMailer) Enabled() bool { return false }

// SendEmailVerification 为给定 user 创建 token 并发送验证邮件。
// 返回创建的 token；SMTP 未配置时 token 仍然写库（便于管理员手动发放）。
func (s *Service) SendEmailVerification(ctx context.Context, u *user.User) (*dverify.Token, error) {
	t, err := s.mint(ctx, u.ID, dverify.PurposeEmailVerify, s.VerifyTTL)
	if err != nil {
		return nil, err
	}
	if s.MailEnabled() {
		link := fmt.Sprintf("%s/verify-email?token=%s", s.BaseURL, t.ID)
		body := fmt.Sprintf(
			`<p>您好，</p><p>请点击以下链接完成 NexusAPI 邮箱验证（24 小时内有效）：</p><p><a href="%s">%s</a></p>`,
			link, link,
		)
		if err := s.mail.Send([]string{u.Email}, "验证您的 NexusAPI 邮箱", body); err != nil {
			return t, err
		}
	}
	return t, nil
}

// VerifyEmail 消费 token 并将 user.EmailVerified=true。
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	t, err := s.fetchAndConsume(ctx, token, dverify.PurposeEmailVerify)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, t.UserID)
	if err != nil {
		return err
	}
	if u.EmailVerified {
		return nil
	}
	u.EmailVerified = true
	return s.users.Update(ctx, u)
}

// SendPasswordReset 给 email 对应账号发送密码重置链接。
// 邮箱不存在时返回 nil（防枚举攻击）。
func (s *Service) SendPasswordReset(ctx context.Context, email string) error {
	u, err := s.users.GetByEmail(ctx, strings.ToLower(email))
	if derrors.Is(err, derrors.CodeNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	t, err := s.mint(ctx, u.ID, dverify.PurposePasswordReset, s.ResetTTL)
	if err != nil {
		return err
	}
	if !s.MailEnabled() {
		return nil
	}
	link := fmt.Sprintf("%s/reset-password?token=%s", s.BaseURL, t.ID)
	body := fmt.Sprintf(
		`<p>您申请重置 NexusAPI 密码。请在 30 分钟内打开以下链接设置新密码：</p><p><a href="%s">%s</a></p><p>如非本人操作请忽略此邮件。</p>`,
		link, link,
	)
	return s.mail.Send([]string{u.Email}, "NexusAPI 密码重置", body)
}

// ResetPassword 用 token + 新密码重置。token 消费成功且密码写入后生效。
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return derrors.New(derrors.CodeInvalidArgument, "密码至少 8 位")
	}
	t, err := s.fetchAndConsume(ctx, token, dverify.PurposePasswordReset)
	if err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, t.UserID)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return derrors.Wrap(derrors.CodeInternal, "hash password", err)
	}
	u.PasswordHash = string(hash)
	return s.users.Update(ctx, u)
}

// ---------- helpers ----------

func (s *Service) mint(ctx context.Context, userID uint64, p dverify.Purpose, ttl time.Duration) (*dverify.Token, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "rand", err)
	}
	t := &dverify.Token{
		ID:        base64.RawURLEncoding.EncodeToString(buf),
		UserID:    userID,
		Purpose:   p,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}
	if err := s.tokens.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) fetchAndConsume(ctx context.Context, id string, want dverify.Purpose) (*dverify.Token, error) {
	t, err := s.tokens.Get(ctx, id)
	if err != nil {
		return nil, derrors.New(derrors.CodeInvalidArgument, "token 无效")
	}
	if t.Purpose != want {
		return nil, derrors.New(derrors.CodeInvalidArgument, "token 用途不匹配")
	}
	if !t.Valid() {
		return nil, derrors.New(derrors.CodeInvalidArgument, "token 已失效")
	}
	ok, err := s.tokens.Consume(ctx, id, time.Now())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, derrors.New(derrors.CodeInvalidArgument, "token 已被使用")
	}
	return t, nil
}
