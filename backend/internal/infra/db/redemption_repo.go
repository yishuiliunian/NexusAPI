package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domainredemption "github.com/yishuiliunian/nexusapi/backend/internal/domain/redemption"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// RedemptionRepo GORM 实现 redemption.Store。
type RedemptionRepo struct{ db *gorm.DB }

// NewRedemptionRepo 构造。
func NewRedemptionRepo(db *gorm.DB) *RedemptionRepo { return &RedemptionRepo{db: db} }

func toVoucherRow(v *domainredemption.Voucher) *RedemptionRow {
	return &RedemptionRow{
		ID:        v.ID,
		Code:      v.Code,
		Amount:    v.Amount,
		UsedBy:    v.UsedBy,
		UsedAt:    v.UsedAt,
		ExpiresAt: v.ExpiresAt,
		Note:      v.Note,
		CreatedAt: v.CreatedAt,
	}
}

func fromVoucherRow(r *RedemptionRow) *domainredemption.Voucher {
	return &domainredemption.Voucher{
		ID:        r.ID,
		Code:      r.Code,
		Amount:    r.Amount,
		UsedBy:    r.UsedBy,
		UsedAt:    r.UsedAt,
		ExpiresAt: r.ExpiresAt,
		Note:      r.Note,
		CreatedAt: r.CreatedAt,
	}
}

func (r *RedemptionRepo) Create(ctx context.Context, v *domainredemption.Voucher) error {
	row := toVoucherRow(v)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create voucher", err)
	}
	v.ID = row.ID
	return nil
}

func (r *RedemptionRepo) List(ctx context.Context, offset, limit int) ([]*domainredemption.Voucher, int64, error) {
	var rows []RedemptionRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&RedemptionRow{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count vouchers", err)
	}
	if err := tx.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list vouchers", err)
	}
	out := make([]*domainredemption.Voucher, 0, len(rows))
	for i := range rows {
		out = append(out, fromVoucherRow(&rows[i]))
	}
	return out, total, nil
}

// ClaimAtomic 事务：行锁读 → 校验 → 标记已用。
func (r *RedemptionRepo) ClaimAtomic(ctx context.Context, code string, userID uint64) (*domainredemption.Voucher, error) {
	var claimed *domainredemption.Voucher
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row RedemptionRow
		if err := tx.Where("code = ?", code).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return derrors.ErrNotFound
			}
			return err
		}
		if row.UsedBy != nil {
			return derrors.New(derrors.CodeAlreadyExists, "兑换码已使用")
		}
		if row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()) {
			return derrors.New(derrors.CodeInvalidArgument, "兑换码已过期")
		}
		now := time.Now()
		if err := tx.Exec(
			"UPDATE redemptions SET used_by = ?, used_at = ? WHERE id = ? AND used_by IS NULL",
			userID, now, row.ID,
		).Error; err != nil {
			return err
		}
		row.UsedBy = &userID
		row.UsedAt = &now
		claimed = fromVoucherRow(&row)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}
