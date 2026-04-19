// auth_test.go —— 注册 / 登录 / 登出 / Session 鉴权的核心路径。
//
// 采用 in-memory fake repo，不依赖 DB。
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakeUsers struct {
	byID       map[uint64]*user.User
	byEmail    map[string]*user.User
	nextID     uint64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[uint64]*user.User{}, byEmail: map[string]*user.User{}, nextID: 1}
}

func (f *fakeUsers) Create(_ context.Context, u *user.User) error {
	if _, exists := f.byEmail[u.Email]; exists {
		return derrors.ErrAlreadyExists
	}
	u.ID = f.nextID
	f.nextID++
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
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
func (f *fakeUsers) SetQuota(_ context.Context, id uint64, q int64) error {
	if u, ok := f.byID[id]; ok {
		u.Quota = q
		return nil
	}
	return derrors.ErrNotFound
}
func (f *fakeUsers) ListLowQuotaForAlert(_ context.Context, _ time.Time, _ int) ([]*user.User, error) {
	return nil, nil
}

type fakeSessions struct {
	byID map[string]*user.Session
}

func newFakeSessions() *fakeSessions { return &fakeSessions{byID: map[string]*user.Session{}} }
func (f *fakeSessions) Create(_ context.Context, s *user.Session) error {
	cp := *s
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeSessions) Get(_ context.Context, id string) (*user.Session, error) {
	s, ok := f.byID[id]
	if !ok || time.Now().After(s.ExpiresAt) {
		return nil, derrors.ErrNotFound
	}
	cp := *s
	return &cp, nil
}
func (f *fakeSessions) Delete(_ context.Context, id string) error {
	delete(f.byID, id)
	return nil
}
func (f *fakeSessions) DeleteByUser(_ context.Context, userID uint64) error {
	for id, s := range f.byID {
		if s.UserID == userID {
			delete(f.byID, id)
		}
	}
	return nil
}

// ---------- Register ----------

func TestRegister_Success(t *testing.T) {
	users := newFakeUsers()
	svc := NewService(users, newFakeSessions(), time.Hour)

	u, err := svc.Register(context.Background(), "alice@x.com", "password123")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Email != "alice@x.com" {
		t.Errorf("email: %q", u.Email)
	}
	if u.Role != user.RoleUser {
		t.Errorf("role: %q", u.Role)
	}
	if u.Status != user.StatusActive {
		t.Errorf("status: %q", u.Status)
	}
	if u.PasswordHash == "" || u.PasswordHash == "password123" {
		t.Error("密码未 bcrypt")
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	_, err := svc.Register(context.Background(), "a@x", "short")
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestRegister_EmailInvalid(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	_, err := svc.Register(context.Background(), "", "password123")
	if err == nil {
		t.Error("空邮箱应拒绝")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "dup@x.com", "password123")
	_, err := svc.Register(ctx, "dup@x.com", "otherpass123")
	if err == nil {
		t.Error("重复邮箱应拒绝")
	}
}

func TestRegister_EmailLowercased(t *testing.T) {
	users := newFakeUsers()
	svc := NewService(users, newFakeSessions(), time.Hour)
	u, err := svc.Register(context.Background(), "  Alice@EXAMPLE.com  ", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "alice@example.com" {
		t.Errorf("邮箱未规范化: %q", u.Email)
	}
}

// ---------- Login ----------

func TestLogin_Success(t *testing.T) {
	users := newFakeUsers()
	sessions := newFakeSessions()
	svc := NewService(users, sessions, time.Hour)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "b@x.com", "password123")

	u, sess, err := svc.Login(ctx, "b@x.com", "password123", "1.2.3.4", "ua-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if u.Email != "b@x.com" {
		t.Errorf("user: %+v", u)
	}
	if sess.ID == "" || sess.IP != "1.2.3.4" || sess.UserAgent != "ua-test" {
		t.Errorf("session: %+v", sess)
	}
	if sess.ExpiresAt.Before(time.Now().Add(30 * time.Minute)) {
		t.Errorf("session 过期时间太近: %v", sess.ExpiresAt)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "b@x.com", "password123")

	_, _, err := svc.Login(ctx, "b@x.com", "wrong", "", "")
	if !derrors.Is(err, derrors.CodeUnauthenticated) {
		t.Errorf("want Unauthenticated, got %v", err)
	}
}

func TestLogin_NotFound(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	_, _, err := svc.Login(context.Background(), "none@x", "any", "", "")
	if !derrors.Is(err, derrors.CodeUnauthenticated) {
		t.Errorf("邮箱不存在应与密码错误一样返回 Unauthenticated，got %v", err)
	}
}

func TestLogin_BannedUser(t *testing.T) {
	users := newFakeUsers()
	svc := NewService(users, newFakeSessions(), time.Hour)
	ctx := context.Background()
	u, _ := svc.Register(ctx, "c@x.com", "password123")
	// Manually ban
	u.Status = user.StatusBanned
	_ = users.Update(ctx, u)

	_, _, err := svc.Login(ctx, "c@x.com", "password123", "", "")
	if !derrors.Is(err, derrors.CodePermissionDenied) {
		t.Errorf("banned 应 PermissionDenied, got %v", err)
	}
}

// ---------- Authenticate ----------

func TestAuthenticate_Success(t *testing.T) {
	users := newFakeUsers()
	sessions := newFakeSessions()
	svc := NewService(users, sessions, time.Hour)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "d@x", "password123")
	_, sess, _ := svc.Login(ctx, "d@x", "password123", "", "")

	u, err := svc.Authenticate(ctx, sess.ID)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if u.Email != "d@x" {
		t.Errorf("%+v", u)
	}
}

func TestAuthenticate_InvalidSession(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	_, err := svc.Authenticate(context.Background(), "nope")
	if !errors.Is(err, derrors.ErrUnauthenticated) {
		t.Errorf("want ErrUnauthenticated, got %v", err)
	}
}

func TestLogout_DeletesSession(t *testing.T) {
	users := newFakeUsers()
	sessions := newFakeSessions()
	svc := NewService(users, sessions, time.Hour)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "e@x", "password123")
	_, sess, _ := svc.Login(ctx, "e@x", "password123", "", "")

	if err := svc.Logout(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, sess.ID); err == nil {
		t.Error("logout 后 session 应不可用")
	}
}

// ---------- NewSession ----------

func TestNewSession_GeneratesUniqueIDs(t *testing.T) {
	svc := NewService(newFakeUsers(), newFakeSessions(), time.Hour)
	u := &user.User{ID: 1}
	s1, _ := svc.NewSession(context.Background(), u, "", "")
	s2, _ := svc.NewSession(context.Background(), u, "", "")
	if s1.ID == s2.ID || s1.ID == "" {
		t.Errorf("session id 不唯一: %q vs %q", s1.ID, s2.ID)
	}
}

// ---------- HashPassword ----------

func TestHashPassword_DifferentSalts(t *testing.T) {
	h1, _ := HashPassword("same")
	h2, _ := HashPassword("same")
	if h1 == h2 {
		t.Error("bcrypt 每次应随机盐")
	}
}
