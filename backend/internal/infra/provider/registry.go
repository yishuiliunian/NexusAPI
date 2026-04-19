// Package provider 维护 Sync 与 Task 两套 provider 注册表。
//
// 每个具体 provider 子包（openai/claude/gemini/midjourney/suno/...）在其 init() 中
// 调用 RegisterSync 或 RegisterTask。
package provider

import (
	"sort"
	"sync"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
)

var (
	mu         sync.RWMutex
	syncRegistry = map[string]relay.SyncAdaptor{}
	taskRegistry = map[string]relay.TaskAdaptor{}
)

// RegisterSync 注册一个同步/流式适配器。
func RegisterSync(a relay.SyncAdaptor) {
	mu.Lock()
	defer mu.Unlock()
	syncRegistry[a.Name()] = a
}

// RegisterTask 注册一个异步任务适配器。
func RegisterTask(a relay.TaskAdaptor) {
	mu.Lock()
	defer mu.Unlock()
	taskRegistry[a.Name()] = a
}

// Sync 按名称取同步适配器，不存在返回 nil。
func Sync(name string) relay.SyncAdaptor {
	mu.RLock()
	defer mu.RUnlock()
	return syncRegistry[name]
}

// Task 按名称取任务适配器，不存在返回 nil。
func Task(name string) relay.TaskAdaptor {
	mu.RLock()
	defer mu.RUnlock()
	return taskRegistry[name]
}

// Lister 返回实现了 ModelLister 的 sync 适配器。未注册或未实现返回 nil。
// 管理台"同步模型"功能据此判断能否拉取上游模型列表。
func Lister(name string) relay.ModelLister {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := syncRegistry[name]
	if !ok {
		return nil
	}
	l, ok := a.(relay.ModelLister)
	if !ok {
		return nil
	}
	return l
}

// Names 返回所有已注册供应商（Sync + Task 合集，去重，字典序）。
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	seen := map[string]struct{}{}
	for n := range syncRegistry {
		seen[n] = struct{}{}
	}
	for n := range taskRegistry {
		seen[n] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
