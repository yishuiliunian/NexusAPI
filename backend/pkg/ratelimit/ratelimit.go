// Package ratelimit 提供基于 Redis 的按 key 固定窗口限流。
//
// 为什么不用 token bucket：
//   - 固定窗口 INCR+EXPIRE 单次 RTT，实现最简
//   - /v1/* 的并发量（<数千 RPS）下精度足够
//   - 方便在管理台显示"本分钟剩余请求数"
//
// 使用约定：
//   - Limit = 0 表示不限
//   - Window 固定 60s（fixed minute）
//   - 超限返回 ErrLimited，调用方自行翻译成 429 + Retry-After
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrLimited 表示触发限流。
var ErrLimited = errors.New("rate limit exceeded")

// Limiter 固定窗口限流器。
//
// 实现要点：
//   - 原子 INCR：第一次 +1 且 PEXPIRE 70s（留缓冲避免 race 丢 TTL）
//   - 返回 (current, retryAfter, err)：current>limit 即拒绝
//   - 仅依赖 Redis，可横向扩展
type Limiter struct {
	rdb    *redis.Client
	prefix string
}

// New 构造。prefix 用于隔离同一 Redis 下的多实例（如 dev / prod）。
func New(rdb *redis.Client, prefix string) *Limiter {
	if prefix == "" {
		prefix = "rl"
	}
	return &Limiter{rdb: rdb, prefix: prefix}
}

// Check 对给定 bucket 做 +1 并检查上限。
//   - limit <= 0 时视为不限，直接返回 (0, 0, nil)
//   - 超限返回 ErrLimited + 到下一窗口的等待时间
//   - window 建议 60s
func (l *Limiter) Check(ctx context.Context, bucket string, limit int, window time.Duration) (int, time.Duration, error) {
	if limit <= 0 {
		return 0, 0, nil
	}
	if l.rdb == nil {
		return 0, 0, nil // 未配置 Redis 时退化为不限（开发/单副本兜底）
	}
	win := int64(window / time.Second)
	if win <= 0 {
		win = 60
	}
	now := time.Now().Unix()
	slot := now / win
	key := fmt.Sprintf("%s:%s:%d", l.prefix, bucket, slot)

	// 使用 pipeline 保证一次 RTT 完成 INCR + EXPIRE。
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window+10*time.Second) // 加 10s 冗余防 TTL 丢失
	if _, err := pipe.Exec(ctx); err != nil {
		// Redis 故障时选择"fail open"：不拦截流量，只日志记录
		return 0, 0, nil
	}
	count := int(incr.Val())
	if count > limit {
		retry := time.Duration((slot+1)*win-now) * time.Second
		return count, retry, ErrLimited
	}
	return count, 0, nil
}

// Add 向 bucket 追加 delta 并返回当前值（不拦截）。用于 TPM 事后记账。
func (l *Limiter) Add(ctx context.Context, bucket string, delta int, window time.Duration) (int, error) {
	if delta <= 0 || l.rdb == nil {
		return 0, nil
	}
	win := int64(window / time.Second)
	if win <= 0 {
		win = 60
	}
	now := time.Now().Unix()
	slot := now / win
	key := fmt.Sprintf("%s:%s:%d", l.prefix, bucket, slot)
	pipe := l.rdb.TxPipeline()
	incr := pipe.IncrBy(ctx, key, int64(delta))
	pipe.Expire(ctx, key, window+10*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, nil
	}
	return int(incr.Val()), nil
}

// Peek 返回当前窗口计数，不产生副作用。
func (l *Limiter) Peek(ctx context.Context, bucket string, window time.Duration) (int, error) {
	if l.rdb == nil {
		return 0, nil
	}
	win := int64(window / time.Second)
	if win <= 0 {
		win = 60
	}
	slot := time.Now().Unix() / win
	key := fmt.Sprintf("%s:%s:%d", l.prefix, bucket, slot)
	v, err := l.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return v, nil
}
