// quota_delta_test.go —— 事务计费的核心路径。
package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

func TestQuotaDelta_ApplyPositive(t *testing.T) {
	d := newTestDB(t)
	ur := NewUserRepo(d, cryptoutil.Noop())
	u := &user.User{Email: "x@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 100}
	_ = ur.Create(context.Background(), u)

	qd := NewQuotaDelta(d)
	bal, err := qd.Apply(context.Background(), billing.QuotaOp{
		UserID: u.ID,
		Amount: 500,
		Ledger: &billing.Ledger{
			UserID: u.ID, Type: billing.LedgerTopUp, Amount: 500,
			Note: "test top-up", CreatedAt: time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if bal != 600 {
		t.Errorf("balance=%d want 600", bal)
	}

	// 验证 user.Quota + ledger 行都更新
	got, _ := ur.GetByID(context.Background(), u.ID)
	if got.Quota != 600 {
		t.Errorf("user.Quota=%d", got.Quota)
	}
	var ledgerCount int64
	d.Model(&LedgerRow{}).Where("user_id = ?", u.ID).Count(&ledgerCount)
	if ledgerCount != 1 {
		t.Errorf("ledger row 数 %d", ledgerCount)
	}
}

func TestQuotaDelta_ApplyNegative(t *testing.T) {
	d := newTestDB(t)
	ur := NewUserRepo(d, cryptoutil.Noop())
	u := &user.User{Email: "x@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 1000}
	_ = ur.Create(context.Background(), u)

	qd := NewQuotaDelta(d)
	_, err := qd.Apply(context.Background(), billing.QuotaOp{
		UserID: u.ID, Amount: -300,
		AddUsed: 300,
		Ledger:  &billing.Ledger{UserID: u.ID, Type: billing.LedgerSettle, Amount: -300, CreatedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := ur.GetByID(context.Background(), u.ID)
	if got.Quota != 700 {
		t.Errorf("quota=%d", got.Quota)
	}
	if got.UsedQuota != 300 {
		t.Errorf("used=%d", got.UsedQuota)
	}
}

func TestQuotaDelta_InsufficientQuota(t *testing.T) {
	d := newTestDB(t)
	ur := NewUserRepo(d, cryptoutil.Noop())
	u := &user.User{Email: "x@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 50}
	_ = ur.Create(context.Background(), u)

	qd := NewQuotaDelta(d)
	_, err := qd.Apply(context.Background(), billing.QuotaOp{
		UserID: u.ID, Amount: -100,
		Ledger: &billing.Ledger{UserID: u.ID, Type: billing.LedgerReserve, Amount: -100, CreatedAt: time.Now()},
	})
	if !derrors.Is(err, derrors.CodeInsufficientQuota) {
		t.Errorf("应返回 InsufficientQuota, got %v", err)
	}
	// 余额不应变
	got, _ := ur.GetByID(context.Background(), u.ID)
	if got.Quota != 50 {
		t.Errorf("余额被错误修改: %d", got.Quota)
	}
	// Ledger 也不应有行（回滚了）
	var count int64
	d.Model(&LedgerRow{}).Count(&count)
	if count != 0 {
		t.Errorf("ledger 不应插入, got %d", count)
	}
}

func TestQuotaDelta_WithUsageAndApiKey(t *testing.T) {
	d := newTestDB(t)
	ur := NewUserRepo(d, cryptoutil.Noop())
	u := &user.User{Email: "x@x", Role: user.RoleUser, Status: user.StatusActive, Quota: 1000}
	_ = ur.Create(context.Background(), u)

	// 建一个 ApiKey 供 used_quota 累加目标
	ar := NewApiKeyRepo(d)
	k := mkKey(t, ar, u.ID)

	qd := NewQuotaDelta(d)
	_, err := qd.Apply(context.Background(), billing.QuotaOp{
		UserID:   u.ID,
		Amount:   -200,
		AddUsed:  200,
		ApiKeyID: k.ID,
		Usage: &billing.Usage{
			UserID: u.ID, ApiKeyID: k.ID,
			Model: "gpt-4o", Capability: billing.CapChat,
			PromptTokens: 100, CompletionTokens: 50, Cost: 200,
			Status: billing.StatusSuccess, CreatedAt: time.Now(),
		},
		Ledger: &billing.Ledger{UserID: u.ID, Type: billing.LedgerSettle, Amount: -200, CreatedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Usage 行
	var usageCount int64
	d.Model(&UsageRow{}).Count(&usageCount)
	if usageCount != 1 {
		t.Errorf("usage 未插入")
	}

	// api_keys.used_quota 累加
	got, _ := ar.GetByID(context.Background(), k.ID)
	if got.UsedQuota != 200 {
		t.Errorf("api key used_quota=%d", got.UsedQuota)
	}
}

// mkKey 已在 apikey_repo_test.go 定义。
var _ = apikey.StatusActive
