// verify_test.go —— 邮箱验证 + 密码重置。
package verify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	dverify "github.com/yishuiliunian/nexusapi/backend/internal/domain/verify"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakeTokens struct {
	mu   sync.Mutex
	byID map[string]*dverify.Token
}

func newFakeTokens() *fakeTokens { return &fakeTokens{byID: map[string]*dverify.Token{}} }
func (f *fakeTokens) Create(_ context.Context, t *dverify.Token) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.byID[t.ID] = &cp
	return nil
}
func (f *fakeTokens) Get(_ context.Context, id string) (*dverify.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeTokens) Consume(_ context.Context, id string, now time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok || t.UsedAt != nil || now.After(t.ExpiresAt) {
		return false, nil
	}
	t.UsedAt = &now
	return true, nil
}
func (f *fakeTokens) DeleteExpired(_ context.Context, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for id, t := range f.byID {
		if now.After(t.ExpiresAt) {
			delete(f.byID, id)
			n++
		}
	}
	return n, nil
}

type fakeUsers struct {
	byID    map[uint64]*user.User
	byEmail map[string]*user.User
	nextID  uint64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[uint64]*user.User{}, byEmail: map[string]*user.User{}, nextID: 1}
}
func (f *fakeUsers) Create(_ context.Context, u *user.User) error {
	u.ID = f.nextID
	f.nextID++
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}
func (f *fakeUsers) GetByID(_ context.Context, id uint64) (*user.User, error) {
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) GetByEmail(_ context.Context, email string) (*user.User, error) {
	if u, ok := f.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *user.User) error {
	stored, ok := f.byID[u.ID]
	if !ok {
		return derrors.ErrNotFound
	}
	*stored = *u
	return nil
}
func (f *fakeUsers) List(_ context.Context, _, _ int) ([]*user.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUsers) SetQuota(_ context.Context, _ uint64, _ int64) error { return nil }
func (f *fakeUsers) ListLowQuotaForAlert(_ context.Context, _ time.Time, _ int) ([]*user.User, error) {
	return nil, nil
}

type captureMailer struct {
	enabled bool
	sent    []sentMail
}
type sentMail struct{ To []string; Subject, Body string }

func (c *captureMailer) Send(to []string, subj, body string) error {
	c.sent = append(c.sent, sentMail{to, subj, body})
	return nil
}
func (c *captureMailer) Enabled() bool { return c.enabled }

// ---------- Tests ----------

func TestSendEmailVerification_CreatesTokenAndSends(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	mail := &captureMailer{enabled: true}
	svc := NewService(tokens, users, mail, "https://app.example.com")

	u := &user.User{ID: 1, Email: "a@x.com"}
	tok, err := svc.SendEmailVerification(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if tok.ID == "" {
		t.Error("token ID 未生成")
	}
	if tok.Purpose != dverify.PurposeEmailVerify {
		t.Errorf("purpose: %q", tok.Purpose)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("mail not sent: %d", len(mail.sent))
	}
	got := mail.sent[0]
	if got.To[0] != "a@x.com" {
		t.Errorf("to: %v", got.To)
	}
	if got.Subject == "" || got.Body == "" {
		t.Error("subject/body 空")
	}
}

func TestSendEmailVerification_NoMailerStillMintsToken(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	svc := NewService(tokens, users, NoopMailer{}, "http://x")
	tok, err := svc.SendEmailVerification(context.Background(), &user.User{ID: 1, Email: "x@x"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if tok.ID == "" {
		t.Error("未 mint token")
	}
}

func TestVerifyEmail_Success(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	svc := NewService(tokens, users, NoopMailer{}, "http://x")
	u := &user.User{Email: "a@x"}
	_ = users.Create(context.Background(), u)
	tok, _ := svc.SendEmailVerification(context.Background(), u)

	if err := svc.VerifyEmail(context.Background(), tok.ID); err != nil {
		t.Fatalf("verify: %v", err)
	}
	got, _ := users.GetByID(context.Background(), u.ID)
	if !got.EmailVerified {
		t.Error("EmailVerified 未置 true")
	}
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	svc := NewService(newFakeTokens(), newFakeUsers(), NoopMailer{}, "http://x")
	err := svc.VerifyEmail(context.Background(), "no-such-token")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestVerifyEmail_WrongPurpose(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	svc := NewService(tokens, users, NoopMailer{}, "http://x")
	u := &user.User{Email: "x@x"}
	_ = users.Create(context.Background(), u)
	// mint 一个 password reset 类型的 token
	_, _ = svc.SendEmailVerification(context.Background(), u) // email verify 用途
	// 手动 insert 一个 purpose=reset 的 token，尝试用 VerifyEmail 消费
	resetTok := &dverify.Token{ID: "reset-x", UserID: u.ID, Purpose: dverify.PurposePasswordReset, ExpiresAt: time.Now().Add(time.Hour)}
	_ = tokens.Create(context.Background(), resetTok)

	err := svc.VerifyEmail(context.Background(), "reset-x")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("wrong purpose 应拒, got %v", err)
	}
}

func TestVerifyEmail_ExpiredToken(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	svc := NewService(tokens, users, NoopMailer{}, "http://x")
	expired := &dverify.Token{
		ID: "old", UserID: 1, Purpose: dverify.PurposeEmailVerify,
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	_ = tokens.Create(context.Background(), expired)
	err := svc.VerifyEmail(context.Background(), "old")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("expired should be InvalidArgument, got %v", err)
	}
}

func TestSendPasswordReset_NotFoundSilent(t *testing.T) {
	svc := NewService(newFakeTokens(), newFakeUsers(), NoopMailer{}, "http://x")
	// 不存在的 email 不应抛错（防枚举）
	if err := svc.SendPasswordReset(context.Background(), "none@x"); err != nil {
		t.Errorf("未注册的邮箱应静默返回, got %v", err)
	}
}

func TestResetPassword_Success(t *testing.T) {
	tokens := newFakeTokens()
	users := newFakeUsers()
	svc := NewService(tokens, users, NoopMailer{}, "http://x")
	u := &user.User{Email: "a@x", PasswordHash: "old-hash"}
	_ = users.Create(context.Background(), u)

	_ = svc.SendPasswordReset(context.Background(), "a@x")
	// 找到 service mint 的 token（purpose=reset）
	var resetTok string
	for id, t := range tokens.byID {
		if t.Purpose == dverify.PurposePasswordReset {
			resetTok = id
		}
	}
	if resetTok == "" {
		t.Fatal("service 未 mint reset token")
	}

	if err := svc.ResetPassword(context.Background(), resetTok, "NewStrongPass123"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	got, _ := users.GetByID(context.Background(), u.ID)
	if got.PasswordHash == "old-hash" || got.PasswordHash == "" {
		t.Errorf("密码未更新: %q", got.PasswordHash)
	}
}

func TestResetPassword_WeakPasswordRejected(t *testing.T) {
	svc := NewService(newFakeTokens(), newFakeUsers(), NoopMailer{}, "http://x")
	if err := svc.ResetPassword(context.Background(), "any", "short"); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}
