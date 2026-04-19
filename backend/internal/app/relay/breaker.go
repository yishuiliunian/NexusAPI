// breaker.go —— 渠道熔断器。
//
// 触发规则：连续失败次数达到 Threshold → 打开熔断，冷却期内 IsOpen 返回 true。
// Selector.Candidates 会排除处于 open 状态的渠道，避免病态上游拖垮整站。
//
// 实现分两份：
//   - MemoryBreaker（本包）—— 单副本/测试用
//   - RedisBreaker（infra/cache）—— 跨副本共享状态
package relay

import (
	"context"
	"sync"
	"time"
)

// Breaker 熔断器契约。
type Breaker interface {
	// RecordFailure 登记一次失败；若累积到阈值则开启熔断。
	RecordFailure(ctx context.Context, channelID uint64)
	// RecordSuccess 清零累积失败计数。
	RecordSuccess(ctx context.Context, channelID uint64)
	// IsOpen 是否当前处于熔断状态（冷却期内）。
	IsOpen(ctx context.Context, channelID uint64) bool
}

// BreakerConfig 阈值与冷却时长。
type BreakerConfig struct {
	Threshold int           // 连续失败阈值；<= 0 时禁用
	Cooldown  time.Duration // 冷却时长
}

// MemoryBreaker 单进程内存实现。
type MemoryBreaker struct {
	cfg  BreakerConfig
	mu   sync.Mutex
	fail map[uint64]int        // 失败计数
	open map[uint64]time.Time  // 开启时间
}

// NewMemoryBreaker 构造。Threshold <= 0 表示禁用熔断（所有调用返回 no-op）。
func NewMemoryBreaker(cfg BreakerConfig) *MemoryBreaker {
	return &MemoryBreaker{
		cfg:  cfg,
		fail: map[uint64]int{},
		open: map[uint64]time.Time{},
	}
}

// RecordFailure 实现。
func (b *MemoryBreaker) RecordFailure(_ context.Context, channelID uint64) {
	if b.cfg.Threshold <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fail[channelID]++
	if b.fail[channelID] >= b.cfg.Threshold {
		b.open[channelID] = time.Now().Add(b.cfg.Cooldown)
		b.fail[channelID] = 0 // 进入熔断后计数重置
	}
}

// RecordSuccess 实现。
func (b *MemoryBreaker) RecordSuccess(_ context.Context, channelID uint64) {
	if b.cfg.Threshold <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.fail, channelID)
}

// IsOpen 实现。
func (b *MemoryBreaker) IsOpen(_ context.Context, channelID uint64) bool {
	if b.cfg.Threshold <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.open[channelID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(b.open, channelID)
		return false
	}
	return true
}

// NoopBreaker 不做任何事；用于未启用熔断的测试/简化场景。
type NoopBreaker struct{}

func (NoopBreaker) RecordFailure(context.Context, uint64) {}
func (NoopBreaker) RecordSuccess(context.Context, uint64) {}
func (NoopBreaker) IsOpen(context.Context, uint64) bool   { return false }
