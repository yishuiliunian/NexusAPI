package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// QuotaDelta GORM 实现 billing.QuotaDelta。
//
// 单事务内完成：行锁读 → 校验 → 更新 quota → 写 ledger →（可选）写 usage。
type QuotaDelta struct {
	db *gorm.DB
}

// NewQuotaDelta 构造。
func NewQuotaDelta(db *gorm.DB) *QuotaDelta {
	return &QuotaDelta{db: db}
}

// Apply 实现 billing.QuotaDelta。
func (d *QuotaDelta) Apply(ctx context.Context, op billing.QuotaOp) (int64, error) {
	var newBal int64

	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ Quota int64 }
		if err := tx.Raw("SELECT quota FROM users WHERE id = ?", op.UserID).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Scan(&row).Error; err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		if op.Amount < 0 && row.Quota+op.Amount < 0 {
			return derrors.ErrInsufficientQuota
		}
		newBal = row.Quota + op.Amount

		if op.AddUsed > 0 {
			if err := tx.Exec(
				"UPDATE users SET quota = ?, used_quota = used_quota + ? WHERE id = ?",
				newBal, op.AddUsed, op.UserID,
			).Error; err != nil {
				return fmt.Errorf("update quota: %w", err)
			}
		} else {
			if err := tx.Exec(
				"UPDATE users SET quota = ? WHERE id = ?",
				newBal, op.UserID,
			).Error; err != nil {
				return fmt.Errorf("update quota: %w", err)
			}
		}

		if op.Ledger != nil {
			op.Ledger.Balance = newBal
			if err := tx.Create(toLedgerRow(op.Ledger)).Error; err != nil {
				return fmt.Errorf("create ledger: %w", err)
			}
		}
		if op.Usage != nil {
			if err := tx.Create(toUsageRow(op.Usage)).Error; err != nil {
				return fmt.Errorf("create usage: %w", err)
			}
		}
		if op.ApiKeyID != 0 && op.AddUsed > 0 {
			if err := tx.Exec(
				"UPDATE api_keys SET used_quota = used_quota + ? WHERE id = ?",
				op.AddUsed, op.ApiKeyID,
			).Error; err != nil {
				return fmt.Errorf("incr apikey used_quota: %w", err)
			}
		}
		return nil
	})

	if err != nil {
		if derrors.Is(err, derrors.CodeInsufficientQuota) {
			return 0, err
		}
		return 0, derrors.Wrap(derrors.CodeInternal, "quota delta", err)
	}
	return newBal, nil
}
