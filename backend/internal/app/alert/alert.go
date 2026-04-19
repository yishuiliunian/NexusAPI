// Package alert 余额预警服务。
//
// worker 每小时调用 CheckAndNotify 扫一次低于阈值且未在冷却期内的用户，
// 发送邮件并更新 QuotaAlertSentAt。
package alert

import (
	"context"
	"fmt"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	"github.com/yishuiliunian/nexusapi/backend/pkg/mailer"
)

// Mailer 接口（只用到 Send/Enabled）。
type Mailer interface {
	Send(to []string, subject, body string) error
	Enabled() bool
}

// Service 预警服务。
type Service struct {
	users    user.Repository
	mail     Mailer
	cooldown time.Duration
}

// NewService 构造。cooldown 默认 24h。
func NewService(users user.Repository, mail Mailer) *Service {
	return &Service{users: users, mail: mail, cooldown: 24 * time.Hour}
}

// CheckAndNotify 扫当前所有触发预警的用户并发邮件。返回通知条数。
// 没有 mailer 时直接跳过（不写 sent_at，等 SMTP 上线后再通知）。
func (s *Service) CheckAndNotify(ctx context.Context, limit int) (int, error) {
	if !s.mail.Enabled() {
		return 0, nil
	}
	cutoff := time.Now().Add(-s.cooldown)
	candidates, err := s.users.ListLowQuotaForAlert(ctx, cutoff, limit)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, u := range candidates {
		subject := "NexusAPI 余额不足提醒"
		body := fmt.Sprintf(
			`<p>您好 %s，</p><p>您的 NexusAPI 当前余额 %.4f 元（= %d micro-unit），低于您设定的阈值 %.4f 元。请及时充值或兑换以免影响正常使用。</p>`,
			u.Email, float64(u.Quota)/1_000_000, u.Quota, float64(u.QuotaAlertAt)/1_000_000,
		)
		if err := s.mail.Send([]string{u.Email}, subject, body); err != nil {
			continue
		}
		now := time.Now()
		u.QuotaAlertSentAt = &now
		_ = s.users.Update(ctx, u)
		sent++
	}
	return sent, nil
}

// 避免未引用的 mailer 包。
var _ = mailer.ErrDisabled
