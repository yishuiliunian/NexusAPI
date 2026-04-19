package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
)

func newClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: s.Addr()}), s
}

func TestReservationStore_SaveAndTake(t *testing.T) {
	c, _ := newClient(t)
	s := NewReservationStore(c, "test:")
	ctx := context.Background()

	r := billing.Reservation{ID: "r1", UserID: 1, Amount: 500}
	if err := s.Save(ctx, r, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := s.Take(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "r1" || got.Amount != 500 {
		t.Errorf("got %+v", got)
	}
}

func TestReservationStore_TakeIsAtomic(t *testing.T) {
	c, _ := newClient(t)
	s := NewReservationStore(c, "test:")
	ctx := context.Background()

	r := billing.Reservation{ID: "atomic", UserID: 1, Amount: 100}
	_ = s.Save(ctx, r, time.Minute)

	// 并发 Take：只有一次成功
	const workers = 5
	var wg sync.WaitGroup
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := s.Take(ctx, "atomic")
			results[idx] = err
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("exactly 1 Take should win, got %d", successes)
	}
}

func TestReservationStore_TakeNotFound(t *testing.T) {
	c, _ := newClient(t)
	s := NewReservationStore(c, "test:")
	_, err := s.Take(context.Background(), "nope")
	if !errors.Is(err, billing.ErrReservationNotFound) {
		t.Errorf("want ErrReservationNotFound, got %v", err)
	}
}

func TestReservationStore_TTLExpiry(t *testing.T) {
	c, srv := newClient(t)
	s := NewReservationStore(c, "test:")
	ctx := context.Background()

	_ = s.Save(ctx, billing.Reservation{ID: "expire", UserID: 1, Amount: 10}, 100*time.Millisecond)
	// miniredis 支持手动推进时钟
	srv.FastForward(200 * time.Millisecond)

	_, err := s.Take(ctx, "expire")
	if !errors.Is(err, billing.ErrReservationNotFound) {
		t.Errorf("过期 reservation 应 NotFound, got %v", err)
	}
}

func TestReservationStore_PrefixIsolation(t *testing.T) {
	c, _ := newClient(t)
	s1 := NewReservationStore(c, "env1:")
	s2 := NewReservationStore(c, "env2:")
	ctx := context.Background()

	_ = s1.Save(ctx, billing.Reservation{ID: "same", Amount: 1}, time.Minute)
	// s2 用不同 prefix，取不到
	_, err := s2.Take(ctx, "same")
	if !errors.Is(err, billing.ErrReservationNotFound) {
		t.Error("prefix 隔离失效")
	}
}
