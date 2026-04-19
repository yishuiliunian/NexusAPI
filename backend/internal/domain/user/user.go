// Package user 定义用户领域实体与仓储接口。
//
// 用户是 NexusAPI 的核心身份实体。用户持有余额 Quota（以 micro-unit 存储，
// 1 元 = 1_000_000 micro），可创建多个 ApiKey 用于调用中继。
package user

import (
	"context"
	"time"
)

// Role 用户角色。
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Status 用户状态。
type Status string

const (
	StatusActive Status = "active"
	StatusBanned Status = "banned"
)

// User 核心用户实体。
//
// 敏感字段加 json:"-" 防止 handler 直接 c.JSON(user) 时泄漏。
// 对外响应一律通过 handler 的 DTO 显式构造。
type User struct {
	ID           uint64    `json:"id"`
	Email        string    `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	GroupID      uint64    `json:"group_id"`
	Quota        int64     `json:"quota"`
	UsedQuota    int64     `json:"used_quota"`
	Status       Status    `json:"status"`
	TwoFASecret  string    `json:"-"`
	// QuotaAlertAt quota 低于此值时 worker 会触发邮件告警；0=不告警。
	QuotaAlertAt int64     `json:"quota_alert_at"`
	// QuotaAlertSentAt 记录最近一次告警时间，避免重复骚扰（24h 冷却）。
	QuotaAlertSentAt *time.Time `json:"-"`
	// RPMLimit 用户级每分钟请求数上限。0 = 不使用用户级限制（只受 ApiKey/系统默认约束）。
	// 与 ApiKey.RPMLimit 独立生效：两者都会被检查，任一超限即 429。
	RPMLimit     int       `json:"rpm_limit"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// IsAdmin 返回是否管理员。
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// Active 返回是否处于可用状态。
func (u *User) Active() bool { return u.Status == StatusActive }

// Repository 用户仓储接口。
//
// used_quota 累加由 billing.QuotaDelta 在事务内完成，仓储不暴露累加方法。
// SetQuota 保留给管理员直接覆盖；推荐改走 billing.Engine.Adjust 以走账本。
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, u *User) error
	List(ctx context.Context, offset, limit int) ([]*User, int64, error)
	SetQuota(ctx context.Context, id uint64, quota int64) error
	// ListLowQuotaForAlert 扫 quota_alert_at > 0 AND quota <= quota_alert_at
	// AND (quota_alert_sent_at IS NULL OR quota_alert_sent_at < cutoff) 的用户，
	// 用于 worker 的余额预警 cron。
	ListLowQuotaForAlert(ctx context.Context, cutoff time.Time, limit int) ([]*User, error)
}

// Group 用户分组。不同分组可配不同模型白名单与价格倍率。
type Group struct {
	ID              uint64    `json:"id"`
	Name            string    `json:"name"`
	PriceMultiplier float64   `json:"price_multiplier"` // 对该组用户的全局计费倍率
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GroupRepository 分组仓储接口。
type GroupRepository interface {
	Create(ctx context.Context, g *Group) error
	GetByID(ctx context.Context, id uint64) (*Group, error)
	GetByName(ctx context.Context, name string) (*Group, error)
	List(ctx context.Context) ([]*Group, error)
	Update(ctx context.Context, g *Group) error
	Delete(ctx context.Context, id uint64) error
}

// Session 用户会话（cookie-based）。
type Session struct {
	ID        string    // cookie token，URL safe base64
	UserID    uint64
	ExpiresAt time.Time
	IP        string
	UserAgent string
	CreatedAt time.Time
}

// SessionRepository 会话仓储接口。
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	Get(ctx context.Context, id string) (*Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID uint64) error
}
