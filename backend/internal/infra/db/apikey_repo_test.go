package db

import (
	"context"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

func mkKey(t *testing.T, r *ApiKeyRepo, userID uint64, overrides ...func(k *apikey.ApiKey)) *apikey.ApiKey {
	t.Helper()
	k := &apikey.ApiKey{
		UserID:    userID,
		KeyPrefix: "sk-nexus-test",
		KeySuffix: "abcd",
		KeyHash:   "hash-" + time.Now().Format("20060102150405.000000000"),
		Name:      "test key",
		Status:    apikey.StatusActive,
	}
	for _, o := range overrides {
		o(k)
	}
	if err := r.Create(context.Background(), k); err != nil {
		t.Fatalf("create key: %v", err)
	}
	return k
}

func TestApiKeyRepo_CreateAndRetrieve(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	ctx := context.Background()

	k := mkKey(t, r, 1, func(k *apikey.ApiKey) {
		k.ModelWhitelist = []string{"gpt-4o", "claude-3-5-sonnet"}
		k.IPWhitelist = []string{"127.0.0.1"}
		k.RPMLimit = 60
		k.TPMLimit = 100000
		k.QuotaLimit = 1_000_000
	})
	if k.ID == 0 {
		t.Fatal("id 未回填")
	}

	got, err := r.GetByID(ctx, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ModelWhitelist) != 2 || got.ModelWhitelist[0] != "gpt-4o" {
		t.Errorf("ModelWhitelist 序列化失败: %+v", got.ModelWhitelist)
	}
	if len(got.IPWhitelist) != 1 || got.IPWhitelist[0] != "127.0.0.1" {
		t.Errorf("IPWhitelist 序列化失败: %+v", got.IPWhitelist)
	}
	if got.RPMLimit != 60 || got.TPMLimit != 100000 {
		t.Errorf("限流字段: RPM=%d TPM=%d", got.RPMLimit, got.TPMLimit)
	}

	byHash, err := r.GetByHash(ctx, k.KeyHash)
	if err != nil || byHash.ID != k.ID {
		t.Errorf("GetByHash: %+v %v", byHash, err)
	}
}

func TestApiKeyRepo_GetByHashNotFound(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	_, err := r.GetByHash(context.Background(), "not-exists")
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestApiKeyRepo_ListByUser(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	ctx := context.Background()
	mkKey(t, r, 1, func(k *apikey.ApiKey) { k.KeyHash = "h1"; k.Name = "k1" })
	mkKey(t, r, 1, func(k *apikey.ApiKey) { k.KeyHash = "h2"; k.Name = "k2" })
	mkKey(t, r, 2, func(k *apikey.ApiKey) { k.KeyHash = "h3"; k.Name = "other" })

	u1Keys, err := r.ListByUser(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(u1Keys) != 2 {
		t.Errorf("u1 应有 2 个 key, got %d", len(u1Keys))
	}
}

func TestApiKeyRepo_TouchLastUsed(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	ctx := context.Background()
	k := mkKey(t, r, 1)

	if k.LastUsedAt != nil {
		t.Error("新建 key LastUsedAt 应为 nil")
	}
	now := time.Now()
	if err := r.TouchLastUsed(ctx, k.ID, now); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByID(ctx, k.ID)
	if got.LastUsedAt == nil || got.LastUsedAt.Unix() != now.Unix() {
		t.Errorf("TouchLastUsed 失败: %+v", got.LastUsedAt)
	}
}

func TestApiKeyRepo_KeyHashUnique(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	ctx := context.Background()
	_ = mkKey(t, r, 1, func(k *apikey.ApiKey) { k.KeyHash = "dup" })
	k2 := &apikey.ApiKey{
		UserID: 2, KeyPrefix: "p", KeySuffix: "s", KeyHash: "dup",
		Name: "collide", Status: apikey.StatusActive,
	}
	if err := r.Create(ctx, k2); err == nil {
		t.Error("相同 hash 应触发 unique 约束")
	}
}

func TestApiKeyRepo_Delete(t *testing.T) {
	r := NewApiKeyRepo(newTestDB(t))
	ctx := context.Background()
	k := mkKey(t, r, 1)

	if err := r.Delete(ctx, k.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetByID(ctx, k.ID); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("删除后应 NotFound, got %v", err)
	}
}
