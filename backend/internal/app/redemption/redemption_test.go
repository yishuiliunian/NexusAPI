// redemption_test.go
package redemption

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	domainredemption "github.com/yishuiliunian/nexusapi/backend/internal/domain/redemption"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

type fakeStore struct {
	mu     sync.Mutex
	byCode map[string]*domainredemption.Voucher
}

func newFakeStore() *fakeStore { return &fakeStore{byCode: map[string]*domainredemption.Voucher{}} }
func (f *fakeStore) Create(_ context.Context, v *domainredemption.Voucher) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byCode[v.Code]; ok {
		return derrors.ErrAlreadyExists
	}
	v.ID = uint64(len(f.byCode) + 1)
	cp := *v
	f.byCode[v.Code] = &cp
	return nil
}
func (f *fakeStore) ClaimAtomic(_ context.Context, code string, userID uint64) (*domainredemption.Voucher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.byCode[code]
	if !ok {
		return nil, derrors.ErrNotFound
	}
	if v.UsedBy != nil {
		return nil, derrors.New(derrors.CodeAlreadyExists, "already used")
	}
	if v.ExpiresAt != nil && v.ExpiresAt.Before(time.Now()) {
		return nil, derrors.New(derrors.CodeInvalidArgument, "expired")
	}
	now := time.Now()
	v.UsedBy = &userID
	v.UsedAt = &now
	cp := *v
	return &cp, nil
}
func (f *fakeStore) List(_ context.Context, offset, limit int) ([]*domainredemption.Voucher, int64, error) {
	out := []*domainredemption.Voucher{}
	for _, v := range f.byCode {
		cp := *v
		out = append(out, &cp)
	}
	total := int64(len(out))
	if offset > len(out) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], total, nil
}

type fakeBilling struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeBilling) TopUp(_ context.Context, _ uint64, _ int64, _ billing.LedgerType, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}

// ---------- Tests ----------

func TestBatch_Validation(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeBilling{})
	if _, err := svc.Batch(context.Background(), 0, 100, nil, ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Error("count=0 应拒")
	}
	if _, err := svc.Batch(context.Background(), 10, 0, nil, ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Error("amount=0 应拒")
	}
	if _, err := svc.Batch(context.Background(), 2000, 100, nil, ""); !derrors.Is(err, derrors.CodeInvalidArgument) {
		t.Error("count>1000 应拒")
	}
}

func TestBatch_GeneratesUniqueCodes(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeBilling{})
	out, err := svc.Batch(context.Background(), 10, 100, nil, "promo")
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(out) != 10 {
		t.Errorf("got %d", len(out))
	}
	seen := map[string]bool{}
	for _, v := range out {
		if seen[v.Code] {
			t.Errorf("重复 code: %s", v.Code)
		}
		seen[v.Code] = true
		if len(v.Code) != 16 {
			t.Errorf("code 长度不对: %q", v.Code)
		}
	}
}

func TestRedeem_Success(t *testing.T) {
	store := newFakeStore()
	bill := &fakeBilling{}
	svc := NewService(store, bill)
	batch, _ := svc.Batch(context.Background(), 1, 10_000, nil, "")
	code := batch[0].Code

	amt, err := svc.Redeem(context.Background(), 42, code)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if amt != 10_000 {
		t.Errorf("amt: %d", amt)
	}
	if bill.calls != 1 {
		t.Errorf("TopUp 次数: %d", bill.calls)
	}
}

func TestRedeem_DuplicateFails(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, &fakeBilling{})
	batch, _ := svc.Batch(context.Background(), 1, 100, nil, "")

	_, err := svc.Redeem(context.Background(), 1, batch[0].Code)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Redeem(context.Background(), 2, batch[0].Code)
	if !derrors.Is(err, derrors.CodeAlreadyExists) {
		t.Errorf("want AlreadyExists, got %v", err)
	}
}

func TestRedeem_NotFound(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeBilling{})
	_, err := svc.Redeem(context.Background(), 1, "UNKNOWN12345678")
	if !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want NotFound, got %v", err)
	}
}

func TestRedeem_NormalizesCode(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, &fakeBilling{})
	batch, _ := svc.Batch(context.Background(), 1, 100, nil, "")
	code := batch[0].Code

	// 提供带空格 + 小写版本
	input := "  " + strings.ToLower(code) + "\n"
	_, err := svc.Redeem(context.Background(), 1, input)
	if err != nil {
		t.Errorf("规范化应成功, got %v", err)
	}
}
