package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryReservations_SaveAndTake(t *testing.T) {
	s := NewMemoryReservations()
	ctx := context.Background()
	r := Reservation{ID: "r1", UserID: 1, Amount: 100}
	if err := s.Save(ctx, r, time.Minute); err != nil {
		t.Fatalf("save err: %v", err)
	}
	got, err := s.Take(ctx, "r1")
	if err != nil {
		t.Fatalf("take err: %v", err)
	}
	if got.Amount != 100 || got.UserID != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestMemoryReservations_TakeConsumes(t *testing.T) {
	s := NewMemoryReservations()
	ctx := context.Background()
	_ = s.Save(ctx, Reservation{ID: "r1", Amount: 50}, time.Minute)
	if _, err := s.Take(ctx, "r1"); err != nil {
		t.Fatalf("first take: %v", err)
	}
	if _, err := s.Take(ctx, "r1"); !errors.Is(err, ErrReservationNotFound) {
		t.Errorf("second take should return ErrReservationNotFound, got %v", err)
	}
}

func TestMemoryReservations_Expired(t *testing.T) {
	s := NewMemoryReservations()
	ctx := context.Background()
	_ = s.Save(ctx, Reservation{ID: "r1", Amount: 50}, time.Nanosecond)
	time.Sleep(2 * time.Millisecond)
	if _, err := s.Take(ctx, "r1"); !errors.Is(err, ErrReservationNotFound) {
		t.Errorf("expired should return ErrReservationNotFound, got %v", err)
	}
}

func TestMemoryReservations_MissingID(t *testing.T) {
	s := NewMemoryReservations()
	if _, err := s.Take(context.Background(), "no-such"); !errors.Is(err, ErrReservationNotFound) {
		t.Errorf("want ErrReservationNotFound, got %v", err)
	}
}

// 并发 Take 只有一个能成功，另一个必须拿到 ErrReservationNotFound。
func TestMemoryReservations_ConcurrentTakeOnlyOneWins(t *testing.T) {
	s := NewMemoryReservations()
	ctx := context.Background()
	_ = s.Save(ctx, Reservation{ID: "r1", Amount: 50}, time.Minute)

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := s.Take(ctx, "r1")
			results[idx] = err
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, e := range results {
		if e == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Errorf("exactly one Take should succeed, got %d success out of %v", successes, results)
	}
}
