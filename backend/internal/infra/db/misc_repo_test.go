package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/audit"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/oauth"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/redemption"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/task"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/verify"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- OAuthBindingRepo ----------

func TestOAuthBindingRepo_CRUD(t *testing.T) {
	r := NewOAuthBindingRepo(newTestDB(t))
	ctx := context.Background()

	b := &oauth.Binding{
		UserID:     1,
		Provider:   "github",
		ExternalID: "42",
		Email:      "alice@x",
	}
	if err := r.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	if b.ID == 0 {
		t.Error("id 未回填")
	}

	got, err := r.GetByProviderExternal(ctx, "github", "42")
	if err != nil || got.UserID != 1 {
		t.Errorf("get: %+v %v", got, err)
	}

	list, err := r.ListByUser(ctx, 1)
	if err != nil || len(list) != 1 {
		t.Errorf("list: %d %v", len(list), err)
	}

	if err := r.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByProviderExternal(ctx, "github", "42"); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("删除后应 NotFound")
	}
}

func TestOAuthBindingRepo_ProviderExternalUnique(t *testing.T) {
	r := NewOAuthBindingRepo(newTestDB(t))
	ctx := context.Background()
	_ = r.Create(ctx, &oauth.Binding{UserID: 1, Provider: "github", ExternalID: "42"})
	err := r.Create(ctx, &oauth.Binding{UserID: 2, Provider: "github", ExternalID: "42"})
	if err == nil {
		t.Error("相同 (provider, external_id) 应触发 unique 约束")
	}
}

// ---------- AuditLogRepo ----------

func TestAuditLogRepo_CreateAndList(t *testing.T) {
	r := NewAuditLogRepo(newTestDB(t))
	ctx := context.Background()

	_ = r.Create(ctx, &audit.Log{ActorID: 1, Action: "user.ban", Target: "user:5", IP: "1.2.3.4"})
	_ = r.Create(ctx, &audit.Log{ActorID: 1, Action: "channel.create", Target: "channel:7"})
	_ = r.Create(ctx, &audit.Log{ActorID: 2, Action: "plan.upsert", Target: "plan:pro"})

	list, total, err := r.List(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(list) != 3 {
		t.Errorf("list: %d/%d", len(list), total)
	}

	byActor, total, _ := r.ListByActor(ctx, 1, 0, 10)
	if total != 2 || len(byActor) != 2 {
		t.Errorf("by actor 1: %d/%d", len(byActor), total)
	}
}

// ---------- RedemptionRepo ----------

func TestRedemptionRepo_ClaimAtomicSuccess(t *testing.T) {
	r := NewRedemptionRepo(newTestDB(t))
	ctx := context.Background()

	_ = r.Create(ctx, &redemption.Voucher{Code: "CODE123", Amount: 10_000})

	v, err := r.ClaimAtomic(ctx, "CODE123", 42)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if v.UsedBy == nil || *v.UsedBy != 42 {
		t.Errorf("UsedBy: %+v", v.UsedBy)
	}

	// 重复消费应失败（AlreadyExists）
	_, err = r.ClaimAtomic(ctx, "CODE123", 99)
	if !derrors.Is(err, derrors.CodeAlreadyExists) {
		t.Errorf("want AlreadyExists, got %v", err)
	}
}

func TestRedemptionRepo_ClaimExpired(t *testing.T) {
	r := NewRedemptionRepo(newTestDB(t))
	ctx := context.Background()
	expired := time.Now().Add(-time.Hour)
	_ = r.Create(ctx, &redemption.Voucher{Code: "OLD", Amount: 100, ExpiresAt: &expired})

	_, err := r.ClaimAtomic(ctx, "OLD", 1)
	if !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Errorf("want InvalidArgument, got %v", err)
	}
}

func TestRedemptionRepo_ClaimNotFound(t *testing.T) {
	r := NewRedemptionRepo(newTestDB(t))
	_, err := r.ClaimAtomic(context.Background(), "NOPE", 1)
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

// ---------- TaskRepo ----------

func TestTaskRepo_CreateAndListByUser(t *testing.T) {
	r := NewTaskRepo(newTestDB(t))
	ctx := context.Background()

	tk := &task.Task{
		ID: "task-1", UserID: 1, ApiKeyID: 5,
		Provider: "midjourney", Action: "imagine", Model: "mj",
		Status: task.StatusPending, CreatedAt: time.Now(),
	}
	if err := r.Create(ctx, tk); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetByID(ctx, "task-1")
	if err != nil || got.Provider != "midjourney" {
		t.Errorf("get: %+v %v", got, err)
	}

	// 另一个用户
	_ = r.Create(ctx, &task.Task{ID: "t2", UserID: 2, Provider: "suno", Action: "music", Status: task.StatusPending, CreatedAt: time.Now()})

	u1, total, _ := r.ListByUser(ctx, 1, 0, 10)
	if total != 1 || len(u1) != 1 {
		t.Errorf("u1 tasks: %d/%d", len(u1), total)
	}
}

func TestTaskRepo_ListPendingAndAll(t *testing.T) {
	r := NewTaskRepo(newTestDB(t))
	ctx := context.Background()

	_ = r.Create(ctx, &task.Task{ID: "p1", UserID: 1, Provider: "m", Action: "a", Status: task.StatusPending, CreatedAt: time.Now()})
	_ = r.Create(ctx, &task.Task{ID: "r1", UserID: 1, Provider: "m", Action: "a", Status: task.StatusRunning, CreatedAt: time.Now()})
	_ = r.Create(ctx, &task.Task{ID: "s1", UserID: 1, Provider: "m", Action: "a", Status: task.StatusSuccess, CreatedAt: time.Now()})

	pending, err := r.ListPending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("pending+running 应 2, got %d", len(pending))
	}

	all, total, _ := r.ListAll(ctx, 0, 10)
	if total != 3 || len(all) != 3 {
		t.Errorf("all: %d/%d", len(all), total)
	}
}

func TestTaskRepo_Update(t *testing.T) {
	r := NewTaskRepo(newTestDB(t))
	ctx := context.Background()
	tk := &task.Task{ID: "u1", UserID: 1, Provider: "m", Action: "a", Status: task.StatusPending, CreatedAt: time.Now()}
	_ = r.Create(ctx, tk)

	tk.Status = task.StatusSuccess
	tk.Progress = 100
	tk.Cost = 5_000
	if err := r.Update(ctx, tk); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, "u1")
	if got.Status != task.StatusSuccess || got.Progress != 100 || got.Cost != 5_000 {
		t.Errorf("update: %+v", got)
	}
}

// ---------- VerifyTokenRepo ----------

func TestVerifyTokenRepo_ConsumeFlow(t *testing.T) {
	r := NewVerifyTokenRepo(newTestDB(t))
	ctx := context.Background()

	tok := &verify.Token{
		ID:        "t1",
		UserID:    42,
		Purpose:   verify.PurposeEmailVerify,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := r.Create(ctx, tok); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, "t1")
	if err != nil || got.UserID != 42 {
		t.Errorf("get: %+v %v", got, err)
	}

	// 首次消费成功
	ok, err := r.Consume(ctx, "t1", time.Now())
	if err != nil || !ok {
		t.Errorf("first consume: ok=%v err=%v", ok, err)
	}

	// 重复消费失败（已 used_at）
	ok, _ = r.Consume(ctx, "t1", time.Now())
	if ok {
		t.Error("重复消费不应成功")
	}
}

func TestVerifyTokenRepo_ExpiredToken(t *testing.T) {
	r := NewVerifyTokenRepo(newTestDB(t))
	ctx := context.Background()

	expired := &verify.Token{ID: "exp", UserID: 1, Purpose: verify.PurposeEmailVerify, ExpiresAt: time.Now().Add(-time.Minute)}
	_ = r.Create(ctx, expired)

	// 过期 token Consume 应失败
	ok, _ := r.Consume(ctx, "exp", time.Now())
	if ok {
		t.Error("过期 token 不应 consume")
	}
}

func TestVerifyTokenRepo_DeleteExpired(t *testing.T) {
	r := NewVerifyTokenRepo(newTestDB(t))
	ctx := context.Background()

	_ = r.Create(ctx, &verify.Token{ID: "live", UserID: 1, Purpose: verify.PurposeEmailVerify, ExpiresAt: time.Now().Add(time.Hour)})
	_ = r.Create(ctx, &verify.Token{ID: "dead", UserID: 1, Purpose: verify.PurposeEmailVerify, ExpiresAt: time.Now().Add(-time.Hour)})

	n, err := r.DeleteExpired(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("应删 1 条, got %d", n)
	}
}
