package alert

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fakes ----------

type fakeUsers struct {
	due     []*user.User
	updates []*user.User
}

func (f *fakeUsers) Create(_ context.Context, _ *user.User) error { return nil }
func (f *fakeUsers) GetByID(_ context.Context, _ uint64) (*user.User, error) {
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) GetByEmail(_ context.Context, _ string) (*user.User, error) {
	return nil, derrors.ErrNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *user.User) error {
	f.updates = append(f.updates, u)
	return nil
}
func (f *fakeUsers) List(_ context.Context, _, _ int) ([]*user.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUsers) SetQuota(_ context.Context, _ uint64, _ int64) error { return nil }
func (f *fakeUsers) ListLowQuotaForAlert(_ context.Context, _ time.Time, _ int) ([]*user.User, error) {
	return f.due, nil
}

type captureMailer struct {
	enabled bool
	sends   []string
	fail    bool
}

func (c *captureMailer) Send(to []string, subj, body string) error {
	if c.fail {
		return derrors.ErrNotFound
	}
	c.sends = append(c.sends, strings.Join(to, ","))
	return nil
}
func (c *captureMailer) Enabled() bool { return c.enabled }

// ---------- Tests ----------

func TestCheckAndNotify_DisabledMailerSkips(t *testing.T) {
	users := &fakeUsers{due: []*user.User{{Email: "a@x"}}}
	svc := NewService(users, &captureMailer{enabled: false})
	n, err := svc.CheckAndNotify(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("禁用 mailer 应跳过, got %d", n)
	}
	if len(users.updates) != 0 {
		t.Error("不应更新 user")
	}
}

func TestCheckAndNotify_SendsAndUpdatesSentAt(t *testing.T) {
	users := &fakeUsers{due: []*user.User{
		{ID: 1, Email: "a@x", Quota: 50, QuotaAlertAt: 100},
		{ID: 2, Email: "b@x", Quota: 30, QuotaAlertAt: 100},
	}}
	mail := &captureMailer{enabled: true}
	svc := NewService(users, mail)

	n, err := svc.CheckAndNotify(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("notify count=%d, want 2", n)
	}
	if len(mail.sends) != 2 {
		t.Errorf("发信数: %d", len(mail.sends))
	}
	// user.Update 被触发，QuotaAlertSentAt 非 nil
	for _, u := range users.updates {
		if u.QuotaAlertSentAt == nil {
			t.Errorf("sent_at 未更新: %+v", u)
		}
	}
}

func TestCheckAndNotify_SendFailureDoesNotMark(t *testing.T) {
	users := &fakeUsers{due: []*user.User{{ID: 1, Email: "a@x", Quota: 10, QuotaAlertAt: 100}}}
	mail := &captureMailer{enabled: true, fail: true}
	svc := NewService(users, mail)

	n, err := svc.CheckAndNotify(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("发信失败不应计数")
	}
	if len(users.updates) != 0 {
		t.Error("发信失败不应 mark sent_at（下次仍会重试）")
	}
}
