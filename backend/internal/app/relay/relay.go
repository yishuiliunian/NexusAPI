// Package relay 提供 ChannelSelector 与熔断/亲和度辅助。
//
// 历史上这里还有 Runner（把 OpenAI 协议 DTO 发给上游），字节级透传架构下
// Runner 的全部方法均已删除，转发改由 pkg/proxy + internal/interface/http/passthrough
// 承担。本包现在只保留 Selector 及相关 Breaker/Affinity 协议。
//
// DIP：Selector 不直接依赖 infra/provider，通过 SyncLookup 函数类型注入。
package relay

import (
	"context"
	"math/rand"

	domainchannel "github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	domainrelay "github.com/yishuiliunian/nexusapi/backend/internal/domain/relay"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// SyncLookup 通过 provider 名称返回 SyncAdaptor（只用到 Name/Supports）。
// 不存在返回 nil；此处返回值只用来判断"这个 provider 名字是否注册过"。
type SyncLookup func(name string) domainrelay.SyncAdaptor

// Selector 基于模型 + 用户分组挑选渠道，支持熔断过滤与亲和度缓存。
type Selector struct {
	repo     domainchannel.Repository
	lookup   SyncLookup
	breaker  Breaker
	affinity Affinity
}

// NewSelector 构造。breaker/affinity 传 nil 时退化为 Noop。
func NewSelector(repo domainchannel.Repository, lookup SyncLookup) *Selector {
	return &Selector{
		repo:     repo,
		lookup:   lookup,
		breaker:  NoopBreaker{},
		affinity: NoopAffinity{},
	}
}

// WithBreaker 挂熔断器；返回同一实例便于链式。
func (s *Selector) WithBreaker(b Breaker) *Selector {
	if b != nil {
		s.breaker = b
	}
	return s
}

// WithAffinity 挂亲和度缓存。
func (s *Selector) WithAffinity(a Affinity) *Selector {
	if a != nil {
		s.affinity = a
	}
	return s
}

// Breaker 返回已注入的熔断器。
func (s *Selector) Breaker() Breaker { return s.breaker }

// Affinity 返回已注入的亲和度缓存。
func (s *Selector) Affinity() Affinity { return s.affinity }

// Candidates 返回符合条件的渠道列表（按 weight desc 排序）。
// 会排除处于熔断冷却期的渠道。
//
// 三层白名单逐层 AND 交集：Group ∧ User ∧ ApiKey。
// 任一参数为 0 视为"该层不限制"（与 channel 字段为空对称）。
func (s *Selector) Candidates(ctx context.Context, model string, groupID, userID, apiKeyID uint64) ([]*domainchannel.Channel, error) {
	all, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domainchannel.Channel, 0, len(all))
	for _, c := range all {
		if !c.Available() {
			continue
		}
		if !c.SupportsModel(model) {
			continue
		}
		if !c.AllowGroup(groupID) {
			continue
		}
		if !c.AllowUser(userID) {
			continue
		}
		if !c.AllowApiKey(apiKeyID) {
			continue
		}
		if s.lookup(c.Provider) == nil {
			continue
		}
		if s.breaker.IsOpen(ctx, c.ID) {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, derrors.New(derrors.CodeNotFound, "无可用渠道")
	}
	return out, nil
}

// PickAffine 优先返回亲和缓存命中的渠道（若仍在 candidates 中），
// 否则回退到加权随机。
func (s *Selector) PickAffine(ctx context.Context, userID uint64, model string, candidates []*domainchannel.Channel) *domainchannel.Channel {
	if len(candidates) == 0 {
		return nil
	}
	if chID, ok := s.affinity.Get(ctx, userID, model); ok {
		for _, c := range candidates {
			if c.ID == chID {
				return c
			}
		}
	}
	return s.Pick(candidates)
}

// Pick 按权重随机选一个。
func (s *Selector) Pick(candidates []*domainchannel.Channel) *domainchannel.Channel {
	if len(candidates) == 0 {
		return nil
	}
	total := 0
	for _, c := range candidates {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	r := rand.Intn(total) // nolint: gosec - non-crypto choice
	for _, c := range candidates {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		if r < w {
			return c
		}
		r -= w
	}
	return candidates[0]
}
