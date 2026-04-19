// Package oauth 定义 OAuth 绑定的领域实体与仓储契约。
//
// Binding：用户与第三方身份的映射（github / google / discord / oidc）。
// 同一用户可绑定多个 Provider；同一 Provider+ExternalID 唯一。
package oauth

import (
	"context"
	"time"
)

// Binding 用户 × 第三方身份。
type Binding struct {
	ID         uint64
	UserID     uint64
	Provider   string // github / google / discord / oidc
	ExternalID string // 第三方用户 id（github: login 或 id）
	Email      string
	CreatedAt  time.Time
}

// Repository 仓储。
type Repository interface {
	Create(ctx context.Context, b *Binding) error
	GetByProviderExternal(ctx context.Context, provider, externalID string) (*Binding, error)
	ListByUser(ctx context.Context, userID uint64) ([]*Binding, error)
	Delete(ctx context.Context, id uint64) error
}
