package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/verify"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// VerifyTokenRepo GORM 实现 verify.Repository。
type VerifyTokenRepo struct{ db *gorm.DB }

// NewVerifyTokenRepo 构造。
func NewVerifyTokenRepo(db *gorm.DB) *VerifyTokenRepo { return &VerifyTokenRepo{db: db} }

func (r *VerifyTokenRepo) Create(ctx context.Context, t *verify.Token) error {
	row := VerifyTokenRow{
		ID:        t.ID,
		UserID:    t.UserID,
		Purpose:   string(t.Purpose),
		ExpiresAt: t.ExpiresAt,
		UsedAt:    t.UsedAt,
		CreatedAt: t.CreatedAt,
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create verify token", err)
	}
	return nil
}

func (r *VerifyTokenRepo) Get(ctx context.Context, id string) (*verify.Token, error) {
	var row VerifyTokenRow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get verify token", err)
	}
	return &verify.Token{
		ID:        row.ID,
		UserID:    row.UserID,
		Purpose:   verify.Purpose(row.Purpose),
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

// Consume 标记为已使用。返回 true 表示刚消费成功；false 表示无效（已使用/过期/不存在）。
func (r *VerifyTokenRepo) Consume(ctx context.Context, id string, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&VerifyTokenRow{}).
		Where("id = ? AND used_at IS NULL AND expires_at > ?", id, now).
		Update("used_at", now)
	if res.Error != nil {
		return false, derrors.Wrap(derrors.CodeInternal, "consume token", res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *VerifyTokenRepo) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("expires_at < ?", now).Delete(&VerifyTokenRow{})
	if res.Error != nil {
		return 0, derrors.Wrap(derrors.CodeInternal, "delete expired tokens", res.Error)
	}
	return res.RowsAffected, nil
}
