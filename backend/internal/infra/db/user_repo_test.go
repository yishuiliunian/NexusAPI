package db

import (
	"context"
	"testing"
	"time"

	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
)

// ---------- UserRepo ----------

func TestUserRepo_CreateGetUpdate(t *testing.T) {
	r := NewUserRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	u := &user.User{
		Email:        "alice@example.com",
		PasswordHash: "hash",
		Role:         user.RoleUser,
		Status:       user.StatusActive,
		TwoFASecret:  "SECRET",
		Quota:        1_000_000,
	}
	if err := r.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("ID 未回填")
	}

	got, err := r.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Email != "alice@example.com" || got.TwoFASecret != "SECRET" {
		t.Errorf("got %+v", got)
	}

	byEmail, err := r.GetByEmail(ctx, "alice@example.com")
	if err != nil || byEmail.ID != u.ID {
		t.Errorf("get by email: %+v %v", byEmail, err)
	}

	u.Quota = 500_000
	u.EmailVerified = true
	if err := r.Update(ctx, u); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = r.GetByID(ctx, u.ID)
	if got.Quota != 500_000 || !got.EmailVerified {
		t.Errorf("updated state wrong: %+v", got)
	}
}

func TestUserRepo_GetByEmailNotFound(t *testing.T) {
	r := NewUserRepo(newTestDB(t), cryptoutil.Noop())
	_, err := r.GetByEmail(context.Background(), "nope@example.com")
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestUserRepo_EmailUniqueness(t *testing.T) {
	r := NewUserRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	u1 := &user.User{Email: "dup@x", PasswordHash: "h", Role: user.RoleUser, Status: user.StatusActive}
	if err := r.Create(ctx, u1); err != nil {
		t.Fatal(err)
	}
	u2 := &user.User{Email: "dup@x", PasswordHash: "h", Role: user.RoleUser, Status: user.StatusActive}
	if err := r.Create(ctx, u2); err == nil {
		t.Error("重复 email 应触发 unique 约束")
	}
}

func TestUserRepo_List(t *testing.T) {
	r := NewUserRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = r.Create(ctx, &user.User{
			Email:    string('a'+rune(i)) + "@x",
			Role:     user.RoleUser,
			Status:   user.StatusActive,
		})
	}
	out, total, err := r.List(ctx, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 || len(out) != 5 {
		t.Errorf("got total=%d len=%d", total, len(out))
	}
}

func TestUserRepo_LowQuotaAlert(t *testing.T) {
	r := NewUserRepo(newTestDB(t), cryptoutil.Noop())
	ctx := context.Background()

	// 触发阈值、未告警过
	u1 := &user.User{Email: "a@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 50, QuotaAlertAt: 100}
	_ = r.Create(ctx, u1)
	// 触发但最近告警过（sent_at 在冷却期内）
	recent := time.Now().Add(-1 * time.Hour)
	u2 := &user.User{Email: "b@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 50, QuotaAlertAt: 100, QuotaAlertSentAt: &recent}
	_ = r.Create(ctx, u2)
	// 未触发阈值
	u3 := &user.User{Email: "c@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 200, QuotaAlertAt: 100}
	_ = r.Create(ctx, u3)
	// 无告警设置
	u4 := &user.User{Email: "d@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 10, QuotaAlertAt: 0}
	_ = r.Create(ctx, u4)

	// cutoff = 2h 前：u2 的 sent_at (1h 前) 比 cutoff 更近 → 仍在冷却期 → 排除
	cutoff := time.Now().Add(-2 * time.Hour)
	due, err := r.ListLowQuotaForAlert(ctx, cutoff, 10)
	if err != nil {
		t.Fatalf("list due: %v", err)
	}
	if len(due) != 1 || due[0].Email != "a@x" {
		t.Errorf("got %d records, want 1 (a@x); got details: %+v", len(due), due)
	}

	// cutoff = 30min 前：u2 的 sent_at (1h 前) 已过冷却 → 应重新告警
	cutoff = time.Now().Add(-30 * time.Minute)
	due, _ = r.ListLowQuotaForAlert(ctx, cutoff, 10)
	if len(due) != 2 {
		t.Errorf("过冷却后应包含 a+b，got %d", len(due))
	}
}

// ---------- GroupRepo ----------

func TestGroupRepo_CRUD(t *testing.T) {
	r := NewGroupRepo(newTestDB(t))
	ctx := context.Background()

	g := &user.Group{Name: "vip", PriceMultiplier: 0.9}
	if err := r.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	byName, err := r.GetByName(ctx, "vip")
	if err != nil || byName.ID != g.ID {
		t.Errorf("get by name: %+v %v", byName, err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 1 {
		t.Errorf("list: %d %v", len(list), err)
	}
	g.PriceMultiplier = 0.5
	if err := r.Update(ctx, g); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, g.ID)
	if got.PriceMultiplier != 0.5 {
		t.Errorf("update failed")
	}
}

func TestGroupRepo_DeleteCascades(t *testing.T) {
	d := newTestDB(t)
	r := NewGroupRepo(d)
	ur := NewUserRepo(d, cryptoutil.Noop())
	ctx := context.Background()

	g := &user.Group{Name: "to-delete"}
	_ = r.Create(ctx, g)

	// 建一个该组下的用户
	u := &user.User{Email: "in@group", Role: user.RoleUser, Status: user.StatusActive, GroupID: g.ID}
	_ = ur.Create(ctx, u)

	// 挂一条 channel_groups 关联
	_ = d.Create(&ChannelGroupRow{ChannelID: 99, GroupID: g.ID}).Error

	if err := r.Delete(ctx, g.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// 级联：channel_groups 应清；users.group_id 应置 0
	var cgCount int64
	d.Model(&ChannelGroupRow{}).Where("group_id = ?", g.ID).Count(&cgCount)
	if cgCount != 0 {
		t.Errorf("channel_groups 未级联清理: %d", cgCount)
	}
	refreshed, _ := ur.GetByID(ctx, u.ID)
	if refreshed.GroupID != 0 {
		t.Errorf("users.group_id 未置 0: %d", refreshed.GroupID)
	}
}

// ---------- SessionRepo ----------

func TestSessionRepo_GetFiltersExpired(t *testing.T) {
	r := NewSessionRepo(newTestDB(t))
	ctx := context.Background()

	valid := &user.Session{ID: "ok", UserID: 1, ExpiresAt: time.Now().Add(time.Hour)}
	expired := &user.Session{ID: "stale", UserID: 1, ExpiresAt: time.Now().Add(-time.Minute)}
	_ = r.Create(ctx, valid)
	_ = r.Create(ctx, expired)

	if _, err := r.Get(ctx, "ok"); err != nil {
		t.Errorf("valid session should return: %v", err)
	}
	if _, err := r.Get(ctx, "stale"); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("expired session should be NotFound, got %v", err)
	}
}

func TestSessionRepo_DeleteByUser(t *testing.T) {
	r := NewSessionRepo(newTestDB(t))
	ctx := context.Background()
	_ = r.Create(ctx, &user.Session{ID: "a", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)})
	_ = r.Create(ctx, &user.Session{ID: "b", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)})
	_ = r.Create(ctx, &user.Session{ID: "c", UserID: 8, ExpiresAt: time.Now().Add(time.Hour)})

	if err := r.DeleteByUser(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "a"); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("a 应被删")
	}
	if _, err := r.Get(ctx, "c"); err != nil {
		t.Errorf("c 不该被删: %v", err)
	}
}
