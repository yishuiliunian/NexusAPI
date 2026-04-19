// affinity.go —— 渠道亲和度缓存。
//
// 同一用户 + 模型在 TTL 内倾向于走同一个渠道，避免跨渠道 session 抖动
// （例如 openai 和 azure-openai 对会话历史的处理可能不一致）。
//
// Get 命中时，Selector.Pick 直接返回该渠道（前提是它仍在 candidates 内且未熔断）。
// 未命中或命中渠道已失效，Pick 正常回退到加权轮询。
package relay

import (
	"context"
	"sync"
	"time"
)

// Affinity 亲和度缓存契约。
type Affinity interface {
	Get(ctx context.Context, userID uint64, model string) (uint64, bool)
	Set(ctx context.Context, userID uint64, model string, channelID uint64)
}

// MemoryAffinity 单进程内存实现。
type MemoryAffinity struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]memAffItem
}

type memAffItem struct {
	channelID uint64
	expiresAt time.Time
}

// NewMemoryAffinity 构造。ttl <= 0 时禁用（Get 永远 miss，Set 为 no-op）。
func NewMemoryAffinity(ttl time.Duration) *MemoryAffinity {
	return &MemoryAffinity{ttl: ttl, m: map[string]memAffItem{}}
}

// NoopAffinity 禁用亲和度（Get 永远 miss）。
type NoopAffinity struct{}

func (NoopAffinity) Get(context.Context, uint64, string) (uint64, bool) { return 0, false }
func (NoopAffinity) Set(context.Context, uint64, string, uint64)        {}

// Get 实现。
func (a *MemoryAffinity) Get(_ context.Context, userID uint64, model string) (uint64, bool) {
	if a.ttl <= 0 {
		return 0, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	item, ok := a.m[affKey(userID, model)]
	if !ok {
		return 0, false
	}
	if time.Now().After(item.expiresAt) {
		delete(a.m, affKey(userID, model))
		return 0, false
	}
	return item.channelID, true
}

// Set 实现。
func (a *MemoryAffinity) Set(_ context.Context, userID uint64, model string, channelID uint64) {
	if a.ttl <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[affKey(userID, model)] = memAffItem{
		channelID: channelID,
		expiresAt: time.Now().Add(a.ttl),
	}
}

func affKey(userID uint64, model string) string {
	// key 格式不直接写进 Redis，仅本地 map 用；RedisAffinity 在 infra 层自定义。
	return model + "/" + strconvU64(userID)
}

// strconvU64 避免引入 strconv 包造成 affinity.go 导入增大。
func strconvU64(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 20)
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
