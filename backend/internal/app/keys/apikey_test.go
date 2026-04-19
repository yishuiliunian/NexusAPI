// apikey_test.go —— ApiKey 服务的核心路径。
package keys

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

type fakeRepo struct {
	mu       sync.Mutex
	byID     map[uint64]*apikey.ApiKey
	byHash   map[string]*apikey.ApiKey
	nextID   uint64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byID: map[uint64]*apikey.ApiKey{}, byHash: map[string]*apikey.ApiKey{}, nextID: 1}
}

func (f *fakeRepo) Create(_ context.Context, k *apikey.ApiKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byHash[k.KeyHash]; ok {
		return derrors.ErrAlreadyExists
	}
	k.ID = f.nextID
	f.nextID++
	cp := *k
	f.byID[k.ID] = &cp
	f.byHash[k.KeyHash] = &cp
	return nil
}
func (f *fakeRepo) GetByID(_ context.Context, id uint64) (*apikey.ApiKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, ok := f.byID[id]; ok {
		cp := *k
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeRepo) GetByHash(_ context.Context, h string) (*apikey.ApiKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, ok := f.byHash[h]; ok {
		cp := *k
		return &cp, nil
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeRepo) ListByUser(_ context.Context, uid uint64) ([]*apikey.ApiKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*apikey.ApiKey{}
	for _, k := range f.byID {
		if k.UserID == uid {
			cp := *k
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeRepo) Update(_ context.Context, k *apikey.ApiKey) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.byID[k.ID]
	if !ok {
		return derrors.ErrNotFound
	}
	*stored = *k
	return nil
}
func (f *fakeRepo) Delete(_ context.Context, id uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, ok := f.byID[id]; ok {
		delete(f.byHash, k.KeyHash)
		delete(f.byID, id)
	}
	return nil
}
func (f *fakeRepo) TouchLastUsed(_ context.Context, id uint64, t time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, ok := f.byID[id]; ok {
		k.LastUsedAt = &t
	}
	return nil
}

// ---------- Create ----------

func TestCreate_GeneratesSecretAndHash(t *testing.T) {
	svc := NewService(newFakeRepo())
	res, err := svc.Create(context.Background(), 42, "test", nil, 1_000_000, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if res.Key.ID == 0 {
		t.Error("ID 未填")
	}
	if !strings.HasPrefix(res.Secret, "sk-nexus-") {
		t.Errorf("secret 格式不对: %q", res.Secret)
	}
	if len(res.Secret) < 40 {
		t.Errorf("secret 过短: %q", res.Secret)
	}
	if res.Key.KeyHash == "" {
		t.Error("hash 未设")
	}
}

func TestCreate_EmptyNameGetsDefault(t *testing.T) {
	svc := NewService(newFakeRepo())
	res, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)
	if res.Key.Name == "" {
		t.Error("空名应有默认值")
	}
}

func TestCreate_TwoKeysHaveDifferentSecrets(t *testing.T) {
	svc := NewService(newFakeRepo())
	r1, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)
	r2, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)
	if r1.Secret == r2.Secret {
		t.Error("两次生成的 secret 相同")
	}
	if r1.Key.KeyHash == r2.Key.KeyHash {
		t.Error("两次生成的 hash 相同")
	}
}

// ---------- ResolveBearer ----------

func TestResolveBearer_ValidKey(t *testing.T) {
	svc := NewService(newFakeRepo())
	res, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)

	k, err := svc.ResolveBearer(context.Background(), res.Secret)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if k.ID != res.Key.ID {
		t.Errorf("ID 不匹配: %d vs %d", k.ID, res.Key.ID)
	}
}

func TestResolveBearer_UnknownKey(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.ResolveBearer(context.Background(), "sk-nexus-unknown")
	if !derrors.Is(err, derrors.CodeUnauthenticated) {
		t.Errorf("want Unauthenticated, got %v", err)
	}
}

func TestResolveBearer_BadPrefix(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.ResolveBearer(context.Background(), "Bearer abc")
	if !derrors.Is(err, derrors.CodeUnauthenticated) {
		t.Errorf("非 sk-nexus 前缀应拒绝")
	}
}

func TestResolveBearer_DisabledKey(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)
	res.Key.Status = apikey.StatusDisabled
	_ = repo.Update(context.Background(), res.Key)

	_, err := svc.ResolveBearer(context.Background(), res.Secret)
	if err == nil {
		t.Error("disabled key 不应通过")
	}
}

func TestResolveBearer_ExpiredKey(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)
	past := time.Now().Add(-time.Hour)
	res.Key.ExpiresAt = &past
	_ = repo.Update(context.Background(), res.Key)

	_, err := svc.ResolveBearer(context.Background(), res.Secret)
	if err == nil {
		t.Error("过期 key 不应通过")
	}
}

// ---------- List / Delete ----------

func TestListByUser(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()
	_, _ = svc.Create(ctx, 1, "a", nil, 0, nil)
	_, _ = svc.Create(ctx, 1, "b", nil, 0, nil)
	_, _ = svc.Create(ctx, 2, "other", nil, 0, nil)

	keys, err := svc.ListByUser(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("got %d", len(keys))
	}
}

func TestDelete_OwnerOnly(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()
	res, _ := svc.Create(ctx, 1, "", nil, 0, nil)

	// 他人尝试删除应拒绝
	if err := svc.Delete(ctx, 999, res.Key.ID); err == nil {
		t.Error("非所有人应不能删除")
	}
	// 所有人删除成功
	if err := svc.Delete(ctx, 1, res.Key.ID); err != nil {
		t.Fatal(err)
	}
}

// ---------- TouchUsed ----------

func TestTouchUsed(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	res, _ := svc.Create(context.Background(), 1, "", nil, 0, nil)

	if err := svc.TouchUsed(context.Background(), res.Key.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(context.Background(), res.Key.ID)
	if got.LastUsedAt == nil {
		t.Error("LastUsedAt 未更新")
	}
}
