// Package apikey 定义 API 密钥领域实体。
//
// ApiKey 是用户对外暴露的凭证，格式 sk-nexus-XXXXXX，中间 hash 存储。
// 每个 key 可配置独立配额、模型白名单、过期时间。
package apikey

import (
	"context"
	"time"
)

// Status 密钥状态。
type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// ApiKey 用户密钥实体。
//
// KeyHash 加 json:"-"，防止意外泄漏（明文只在创建时一次性返回）。
type ApiKey struct {
	ID             uint64     `json:"id"`
	UserID         uint64     `json:"user_id"`
	KeyPrefix      string     `json:"prefix"` // 前 12 位明文，用于展示
	KeySuffix      string     `json:"suffix"` // 后 4 位明文
	KeyHash        string     `json:"-"`       // 完整 key 的 SHA-256，用于反查，绝不对外
	Name           string     `json:"name"`
	ModelWhitelist []string   `json:"model_whitelist"`
	IPWhitelist    []string   `json:"ip_whitelist"`   // 空=不限
	QuotaLimit     int64      `json:"quota_limit"`
	UsedQuota      int64      `json:"used_quota"`
	RPMLimit       int        `json:"rpm_limit"`      // 每分钟请求上限；0=走系统默认
	TPMLimit       int        `json:"tpm_limit"`      // 每分钟 token 上限；0=走系统默认
	ExpiresAt      *time.Time `json:"expires_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	Status         Status     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Active 判断 key 是否可用。
func (k *ApiKey) Active() bool {
	if k.Status != StatusActive {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}

// AllowModel 判断 key 是否允许使用指定模型。
func (k *ApiKey) AllowModel(model string) bool {
	if len(k.ModelWhitelist) == 0 {
		return true
	}
	for _, m := range k.ModelWhitelist {
		if m == model {
			return true
		}
	}
	return false
}

// AllowIP 判断请求 IP 是否在白名单中；空白名单视为不限。
// 仅做精确匹配；如需 CIDR 支持，可在调用方预处理。
func (k *ApiKey) AllowIP(ip string) bool {
	if len(k.IPWhitelist) == 0 {
		return true
	}
	for _, allowed := range k.IPWhitelist {
		if allowed == ip {
			return true
		}
	}
	return false
}

// Repository API 密钥仓储接口。
//
// used_quota 累加由 billing.QuotaDelta 在事务内完成（通过 QuotaOp.ApiKeyID）。
type Repository interface {
	Create(ctx context.Context, k *ApiKey) error
	GetByID(ctx context.Context, id uint64) (*ApiKey, error)
	GetByHash(ctx context.Context, hash string) (*ApiKey, error)
	ListByUser(ctx context.Context, userID uint64) ([]*ApiKey, error)
	Update(ctx context.Context, k *ApiKey) error
	Delete(ctx context.Context, id uint64) error
	TouchLastUsed(ctx context.Context, id uint64, t time.Time) error
}
