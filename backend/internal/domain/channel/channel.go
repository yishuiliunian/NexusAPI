// Package channel 定义上游渠道领域实体。
//
// 渠道是对接某一上游供应商的一组配置：供应商名、base URL、凭证、
// 支持的模型、可用分组、权重、价格倍率。
// ChannelSelector 基于这些信息为请求选路。
package channel

import (
	"context"
	"time"
)

// Status 渠道状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusTesting  Status = "testing"
)

// Channel 上游渠道实体。
//
// Credentials 加 json:"-" 防止管理台误序列化泄漏。
type Channel struct {
	ID              uint64     `json:"id"`
	Name            string     `json:"name"`
	Provider        string     `json:"provider"`
	BaseURL         string     `json:"base_url"`
	Credentials     string     `json:"-"` // 仅在 handler 里显式暴露；默认不进任何 JSON
	Models          []string   `json:"models"`
	GroupIDs        []uint64   `json:"group_ids"`
	Weight          int        `json:"weight"`
	PriceMultiplier float64    `json:"price_multiplier"`
	Status          Status     `json:"status"`
	TestedAt        *time.Time `json:"tested_at"`
	LatencyMs       int        `json:"latency_ms"`
	Note            string     `json:"note"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Available 判断渠道是否可选。
func (c *Channel) Available() bool { return c.Status == StatusActive }

// SupportsModel 判断渠道是否支持指定模型。
func (c *Channel) SupportsModel(model string) bool {
	for _, m := range c.Models {
		if m == model {
			return true
		}
	}
	return false
}

// AllowGroup 判断渠道是否对指定分组开放。空 GroupIDs 表示全分组可用。
func (c *Channel) AllowGroup(groupID uint64) bool {
	if len(c.GroupIDs) == 0 {
		return true
	}
	for _, g := range c.GroupIDs {
		if g == groupID {
			return true
		}
	}
	return false
}

// Repository 渠道仓储接口。
type Repository interface {
	Create(ctx context.Context, c *Channel) error
	GetByID(ctx context.Context, id uint64) (*Channel, error)
	List(ctx context.Context, offset, limit int) ([]*Channel, int64, error)
	ListActive(ctx context.Context) ([]*Channel, error)
	Update(ctx context.Context, c *Channel) error
	Delete(ctx context.Context, id uint64) error
	UpdateHealth(ctx context.Context, id uint64, latencyMs int, testedAt time.Time) error
}
