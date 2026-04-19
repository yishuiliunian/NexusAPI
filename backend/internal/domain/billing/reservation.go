package billing

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrReservationNotFound 预占 ID 找不到。通常说明已被 Settle/Refund 消费或 TTL 过期。
var ErrReservationNotFound = errors.New("reservation not found")

// Reservation 预占记录。
type Reservation struct {
	ID     string
	UserID uint64
	Amount int64
}

// ReservationStore 预占记录的持久化接口。
//
// 实现必须保证：
//   - Save：写入后 TTL 内可 Take
//   - Take：原子消费（并发 Settle + Refund 只有一次成功，另一次返回 ErrReservationNotFound）
type ReservationStore interface {
	Save(ctx context.Context, r Reservation, ttl time.Duration) error
	Take(ctx context.Context, id string) (*Reservation, error)
}

// ---------- 内存实现（单副本 / 测试用） ----------

type memReservation struct {
	r       Reservation
	expires time.Time
}

// MemoryReservations 单进程内存版本。
// 仅适合单副本部署或测试；水平扩展需用 Redis 版本。
type MemoryReservations struct {
	mu sync.Mutex
	m  map[string]memReservation
}

// NewMemoryReservations 构造。
func NewMemoryReservations() *MemoryReservations {
	return &MemoryReservations{m: make(map[string]memReservation)}
}

// Save 实现。
func (s *MemoryReservations) Save(ctx context.Context, r Reservation, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[r.ID] = memReservation{r: r, expires: time.Now().Add(ttl)}
	return nil
}

// Take 实现。
func (s *MemoryReservations) Take(ctx context.Context, id string) (*Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.m[id]
	if !ok {
		return nil, ErrReservationNotFound
	}
	delete(s.m, id)
	if time.Now().After(item.expires) {
		return nil, ErrReservationNotFound
	}
	return &item.r, nil
}
