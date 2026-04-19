// Package redemption 兑换券领域模型。
//
// Voucher 代表可一次性兑换成余额的凭证。字面值 Code 保存在 Voucher.Code 字段中。
package redemption

import (
	"context"
	"time"
)

// Voucher 兑换券实体。Code 字段是用户兑换时输入的字符串。
type Voucher struct {
	ID        uint64     `json:"id"`
	Code      string     `json:"code"`
	Amount    int64      `json:"amount"`
	UsedBy    *uint64    `json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Note      string     `json:"note"`
	CreatedAt time.Time  `json:"created_at"`
}

// Used 判断兑换券是否已被使用。
func (v *Voucher) Used() bool { return v.UsedBy != nil }

// Expired 判断兑换券是否已过期（ExpiresAt 为 nil 表示永久有效）。
func (v *Voucher) Expired(now time.Time) bool {
	return v.ExpiresAt != nil && v.ExpiresAt.Before(now)
}

// Store 兑换券仓储。
// ClaimAtomic 以事务方式把 code 标记为被 userID 使用：若已用或过期返回错误。
type Store interface {
	Create(ctx context.Context, v *Voucher) error
	List(ctx context.Context, offset, limit int) ([]*Voucher, int64, error)
	ClaimAtomic(ctx context.Context, code string, userID uint64) (*Voucher, error)
}
