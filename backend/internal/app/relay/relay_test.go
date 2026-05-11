package relay

import (
	"context"
	"testing"
	"time"

	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	domainrelay "github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- fake Repository（完整实现 domainchannel.Repository） ----------

type fakeChannelRepo struct {
	active []*domainchannel.Channel
	err    error
}

func (f *fakeChannelRepo) Create(ctx context.Context, c *domainchannel.Channel) error { return nil }
func (f *fakeChannelRepo) GetByID(ctx context.Context, id uint64) (*domainchannel.Channel, error) {
	for _, c := range f.active {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, derrors.ErrNotFound
}
func (f *fakeChannelRepo) List(ctx context.Context, offset, limit int) ([]*domainchannel.Channel, int64, error) {
	return f.active, int64(len(f.active)), nil
}
func (f *fakeChannelRepo) ListActive(ctx context.Context) ([]*domainchannel.Channel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.active, nil
}
func (f *fakeChannelRepo) Update(ctx context.Context, c *domainchannel.Channel) error { return nil }
func (f *fakeChannelRepo) Delete(ctx context.Context, id uint64) error                { return nil }
func (f *fakeChannelRepo) UpdateHealth(ctx context.Context, id uint64, latencyMs int, testedAt time.Time) error {
	return nil
}

// stubAdaptor 最小实现，满足 SyncAdaptor 接口。
type stubAdaptor struct{ name string }

func (s *stubAdaptor) Name() string                       { return s.name }
func (s *stubAdaptor) Supports() []domainrelay.Capability { return nil }

func newSel(active []*domainchannel.Channel, providers ...string) *Selector {
	repo := &fakeChannelRepo{active: active}
	registry := map[string]domainrelay.SyncAdaptor{}
	for _, p := range providers {
		registry[p] = &stubAdaptor{name: p}
	}
	lookup := func(name string) domainrelay.SyncAdaptor { return registry[name] }
	return NewSelector(repo, lookup)
}

// ---------- Candidates ----------

func TestCandidates_FiltersDisabled(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusActive, Weight: 1},
		{ID: 2, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusDisabled, Weight: 1},
	}, "p")
	got, err := sel.Candidates(context.Background(), "m", 0, 0, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 {
		t.Errorf("should keep only active ID=1, got %+v", got)
	}
}

func TestCandidates_FiltersUnsupportedModel(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"other"}, Status: domainchannel.StatusActive},
	}, "p")
	if _, err := sel.Candidates(context.Background(), "m", 0, 0, 0); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("want not_found, got %v", err)
	}
}

func TestCandidates_FiltersGroup(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"m"}, GroupIDs: []uint64{2}, Status: domainchannel.StatusActive},
		{ID: 2, Provider: "p", Models: []string{"m"}, GroupIDs: nil, Status: domainchannel.StatusActive},
	}, "p")
	got, _ := sel.Candidates(context.Background(), "m", 3, 0, 0)
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("group filter failed, got %+v", got)
	}
}

func TestCandidates_FiltersMissingAdaptor(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "unknown", Models: []string{"m"}, Status: domainchannel.StatusActive},
	}) // 没有注册任何 provider
	if _, err := sel.Candidates(context.Background(), "m", 0, 0, 0); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("unregistered provider should filter out, want not_found, got %v", err)
	}
}

func TestCandidates_AllowsOpenGroup(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 9, Provider: "p", Models: []string{"m"}, GroupIDs: nil, Status: domainchannel.StatusActive},
	}, "p")
	got, err := sel.Candidates(context.Background(), "m", 42, 0, 0)
	if err != nil || len(got) != 1 {
		t.Errorf("nil GroupIDs means any group; got %+v, err=%v", got, err)
	}
}

func TestCandidates_FiltersUser_Whitelist(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"m"}, UserIDs: []uint64{100}, Status: domainchannel.StatusActive},
		{ID: 2, Provider: "p", Models: []string{"m"}, UserIDs: []uint64{200}, Status: domainchannel.StatusActive},
	}, "p")
	got, err := sel.Candidates(context.Background(), "m", 0, 100, 0)
	if err != nil || len(got) != 1 || got[0].ID != 1 {
		t.Errorf("UserIDs whitelist failed; got %+v err=%v", got, err)
	}
}

func TestCandidates_FiltersUser_EmptyMeansUnrestricted(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 9, Provider: "p", Models: []string{"m"}, UserIDs: nil, Status: domainchannel.StatusActive},
	}, "p")
	got, err := sel.Candidates(context.Background(), "m", 0, 12345, 0)
	if err != nil || len(got) != 1 {
		t.Errorf("nil UserIDs means any user; got %+v err=%v", got, err)
	}
}

func TestCandidates_FiltersApiKey_Whitelist(t *testing.T) {
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"m"}, ApiKeyIDs: []uint64{77}, Status: domainchannel.StatusActive},
		{ID: 2, Provider: "p", Models: []string{"m"}, ApiKeyIDs: nil, Status: domainchannel.StatusActive},
	}, "p")
	// apiKey=88 → 仅 ID=2（不限制）通过
	got, _ := sel.Candidates(context.Background(), "m", 0, 0, 88)
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("ApiKeyIDs whitelist failed; got %+v", got)
	}
	// apiKey=77 → 两条都通过
	got, _ = sel.Candidates(context.Background(), "m", 0, 0, 77)
	if len(got) != 2 {
		t.Errorf("ApiKeyIDs whitelist should accept matching key; got %+v", got)
	}
}

func TestCandidates_ThreeLayerAndIntersection(t *testing.T) {
	// 同一渠道同时设 Group=[3] 且 User=[100]。三层语义为 AND。
	sel := newSel([]*domainchannel.Channel{
		{ID: 1, Provider: "p", Models: []string{"m"},
			GroupIDs: []uint64{3}, UserIDs: []uint64{100},
			Status: domainchannel.StatusActive},
	}, "p")

	// 用户 100 但 group=4 → 被 Group 层过滤
	if _, err := sel.Candidates(context.Background(), "m", 4, 100, 0); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("group mismatch should be filtered out; got err=%v", err)
	}
	// group=3 但用户=200 → 被 User 层过滤
	if _, err := sel.Candidates(context.Background(), "m", 3, 200, 0); !derrors.Is(err, derrors.CodeNotFound) {
		t.Errorf("user mismatch should be filtered out; got err=%v", err)
	}
	// 三层都命中
	got, err := sel.Candidates(context.Background(), "m", 3, 100, 0)
	if err != nil || len(got) != 1 {
		t.Errorf("three-layer AND should pass when all match; got %+v err=%v", got, err)
	}
}

// ---------- Pick ----------

func TestPick_EmptyReturnsNil(t *testing.T) {
	sel := newSel(nil)
	if c := sel.Pick(nil); c != nil {
		t.Errorf("empty → nil, got %+v", c)
	}
}

func TestPick_SingleReturnsIt(t *testing.T) {
	sel := newSel(nil)
	ch := &domainchannel.Channel{ID: 1, Weight: 3}
	if got := sel.Pick([]*domainchannel.Channel{ch}); got != ch {
		t.Errorf("single candidate → that one, got %+v", got)
	}
}

func TestPick_ZeroWeightTreatedAsOne(t *testing.T) {
	sel := newSel(nil)
	chs := []*domainchannel.Channel{{ID: 1, Weight: 0}, {ID: 2, Weight: 0}}
	for i := 0; i < 20; i++ {
		if c := sel.Pick(chs); c == nil {
			t.Fatal("Pick returned nil with zero-weight candidates")
		}
	}
}

func TestPick_WeightedDistribution(t *testing.T) {
	sel := newSel(nil)
	chs := []*domainchannel.Channel{{ID: 1, Weight: 1}, {ID: 2, Weight: 9}}
	hits := map[uint64]int{}
	for i := 0; i < 2000; i++ {
		hits[sel.Pick(chs).ID]++
	}
	if hits[2] < hits[1]*3 {
		t.Errorf("higher weight should win much more often: %+v", hits)
	}
}

// ---------- MemoryBreaker ----------

func TestMemoryBreaker_OpensAfterThreshold(t *testing.T) {
	b := NewMemoryBreaker(BreakerConfig{Threshold: 3, Cooldown: 50 * time.Millisecond})
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		b.RecordFailure(ctx, 1)
		if b.IsOpen(ctx, 1) {
			t.Fatalf("should not open after %d failures", i+1)
		}
	}
	b.RecordFailure(ctx, 1)
	if !b.IsOpen(ctx, 1) {
		t.Fatal("should be open after threshold")
	}
}

func TestMemoryBreaker_CooldownExpires(t *testing.T) {
	b := NewMemoryBreaker(BreakerConfig{Threshold: 1, Cooldown: 10 * time.Millisecond})
	ctx := context.Background()
	b.RecordFailure(ctx, 5)
	if !b.IsOpen(ctx, 5) {
		t.Fatal("should open")
	}
	time.Sleep(15 * time.Millisecond)
	if b.IsOpen(ctx, 5) {
		t.Error("should auto-close after cooldown")
	}
}

func TestMemoryBreaker_SuccessResetsCount(t *testing.T) {
	b := NewMemoryBreaker(BreakerConfig{Threshold: 3, Cooldown: time.Second})
	ctx := context.Background()
	b.RecordFailure(ctx, 1)
	b.RecordFailure(ctx, 1)
	b.RecordSuccess(ctx, 1)
	b.RecordFailure(ctx, 1)
	if b.IsOpen(ctx, 1) {
		t.Error("success should reset counter")
	}
}

func TestBreaker_ZeroThresholdDisabled(t *testing.T) {
	b := NewMemoryBreaker(BreakerConfig{Threshold: 0})
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		b.RecordFailure(ctx, 1)
	}
	if b.IsOpen(ctx, 1) {
		t.Error("threshold=0 应视作禁用")
	}
}

// ---------- MemoryAffinity ----------

func TestMemoryAffinity_RoundTrip(t *testing.T) {
	a := NewMemoryAffinity(time.Minute)
	ctx := context.Background()
	a.Set(ctx, 1, "gpt-4", 42)
	if id, ok := a.Get(ctx, 1, "gpt-4"); !ok || id != 42 {
		t.Errorf("got (%d, %v), want (42, true)", id, ok)
	}
}

func TestMemoryAffinity_MissAnotherKey(t *testing.T) {
	a := NewMemoryAffinity(time.Minute)
	ctx := context.Background()
	a.Set(ctx, 1, "gpt-4", 42)
	if _, ok := a.Get(ctx, 1, "other-model"); ok {
		t.Error("other model should miss")
	}
	if _, ok := a.Get(ctx, 2, "gpt-4"); ok {
		t.Error("other user should miss")
	}
}

func TestMemoryAffinity_Expires(t *testing.T) {
	a := NewMemoryAffinity(10 * time.Millisecond)
	ctx := context.Background()
	a.Set(ctx, 1, "gpt-4", 42)
	time.Sleep(15 * time.Millisecond)
	if _, ok := a.Get(ctx, 1, "gpt-4"); ok {
		t.Error("should expire")
	}
}

func TestAffinity_ZeroTTLDisabled(t *testing.T) {
	a := NewMemoryAffinity(0)
	ctx := context.Background()
	a.Set(ctx, 1, "m", 99)
	if _, ok := a.Get(ctx, 1, "m"); ok {
		t.Error("ttl=0 应禁用")
	}
}

// ---------- Selector 与 breaker/affinity 的集成 ----------

func TestSelector_SkipsOpenBreaker(t *testing.T) {
	ch1 := &domainchannel.Channel{ID: 1, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusActive, Weight: 1}
	ch2 := &domainchannel.Channel{ID: 2, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusActive, Weight: 1}
	sel := newSel([]*domainchannel.Channel{ch1, ch2}, "p")

	b := NewMemoryBreaker(BreakerConfig{Threshold: 1, Cooldown: time.Second})
	sel.WithBreaker(b)
	b.RecordFailure(context.Background(), 1)

	got, err := sel.Candidates(context.Background(), "m", 0, 0, 0)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("should filter ch1, got %+v", got)
	}
}

func TestSelector_PickAffinePrefersCachedChannel(t *testing.T) {
	ch1 := &domainchannel.Channel{ID: 1, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusActive, Weight: 1}
	ch2 := &domainchannel.Channel{ID: 2, Provider: "p", Models: []string{"m"}, Status: domainchannel.StatusActive, Weight: 1}
	sel := newSel([]*domainchannel.Channel{ch1, ch2}, "p")
	a := NewMemoryAffinity(time.Minute)
	sel.WithAffinity(a)

	a.Set(context.Background(), 42, "m", 2)
	candidates := []*domainchannel.Channel{ch1, ch2}
	for i := 0; i < 20; i++ {
		got := sel.PickAffine(context.Background(), 42, "m", candidates)
		if got.ID != 2 {
			t.Fatalf("PickAffine should return cached ch2, got ch%d", got.ID)
		}
	}
}
