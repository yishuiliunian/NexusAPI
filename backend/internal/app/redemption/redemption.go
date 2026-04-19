// Package redemption 实现兑换券服务。
//
// DIP：依赖 domain/redemption.Store，不触 GORM。
package redemption

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"strings"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	domainredemption "github.com/yishuiliunian/nexusapi/backend/internal/domain/redemption"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// BillingTopper 结算充值接口（避免循环依赖）。
type BillingTopper interface {
	TopUp(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error
}

// Service 兑换券服务。
type Service struct {
	Store   domainredemption.Store
	Billing BillingTopper
}

// Voucher 重新导出，方便 handler 层使用。
type Voucher = domainredemption.Voucher

// NewService 构造。
func NewService(store domainredemption.Store, b BillingTopper) *Service {
	return &Service{Store: store, Billing: b}
}

// Batch 生成一批兑换券。
func (s *Service) Batch(ctx context.Context, count int, amount int64, expiresAt *time.Time, note string) ([]*domainredemption.Voucher, error) {
	if count <= 0 || count > 1000 {
		return nil, derrors.New(derrors.CodeInvalidArgument, "count 1-1000")
	}
	if amount <= 0 {
		return nil, derrors.New(derrors.CodeInvalidArgument, "amount > 0")
	}
	now := time.Now()
	out := make([]*domainredemption.Voucher, 0, count)
	for i := 0; i < count; i++ {
		v := &domainredemption.Voucher{
			Code:      generate(),
			Amount:    amount,
			ExpiresAt: expiresAt,
			Note:      note,
			CreatedAt: now,
		}
		if err := s.Store.Create(ctx, v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Redeem 兑换一次：标记已用 + 给用户充值。
func (s *Service) Redeem(ctx context.Context, userID uint64, code string) (int64, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	v, err := s.Store.ClaimAtomic(ctx, code, userID)
	if err != nil {
		return 0, err
	}
	if err := s.Billing.TopUp(ctx, userID, v.Amount, billing.LedgerRedeem, code, "兑换券 "+code); err != nil {
		return 0, err
	}
	return v.Amount, nil
}

// List 列出兑换券（管理员）。
func (s *Service) List(ctx context.Context, offset, limit int) ([]*domainredemption.Voucher, int64, error) {
	return s.Store.List(ctx, offset, limit)
}

// generate 生成 16 位大写字母+数字兑换码。
func generate() string {
	buf := make([]byte, 10)
	_, _ = rand.Read(buf)
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	if len(s) > 16 {
		s = s[:16]
	}
	return s
}
