// Package cache 基于 Redis 的 billing.ReservationStore 实现。
package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
)

// ReservationStore Redis 实现：
//   - Save: SET key value EX ttl
//   - Take: GETDEL（原子获取+删除，需 Redis 6.2+）
type ReservationStore struct {
	client *redis.Client
	prefix string
}

// NewReservationStore 构造。prefix 推荐传 "nexus:reserve:"，多环境共用 Redis 时区分。
func NewReservationStore(client *redis.Client, prefix string) *ReservationStore {
	if prefix == "" {
		prefix = "nexus:reserve:"
	}
	return &ReservationStore{client: client, prefix: prefix}
}

// Save 实现 billing.ReservationStore。
func (s *ReservationStore) Save(ctx context.Context, r billing.Reservation, ttl time.Duration) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.prefix+r.ID, b, ttl).Err()
}

// Take 实现：原子 GETDEL。
func (s *ReservationStore) Take(ctx context.Context, id string) (*billing.Reservation, error) {
	raw, err := s.client.GetDel(ctx, s.prefix+id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, billing.ErrReservationNotFound
		}
		return nil, err
	}
	var r billing.Reservation
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
