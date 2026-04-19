package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// jsonArray 实现 sql.Scanner/Valuer，支持 []string/[]uint64 的 JSON 存储。
type jsonArray[T any] []T

func (a jsonArray[T]) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

func (a *jsonArray[T]) Scan(src any) error {
	if src == nil {
		*a = nil
		return nil
	}
	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("jsonArray.Scan: unsupported type %T", src)
	}
	if len(data) == 0 {
		*a = nil
		return nil
	}
	return json.Unmarshal(data, a)
}

// UserRow 用户表映射。
type UserRow struct {
	ID           uint64     `gorm:"primaryKey;autoIncrement"`
	Email        string     `gorm:"uniqueIndex;size:191;not null"`
	EmailVerified bool      `gorm:"not null;default:false"`
	PasswordHash string     `gorm:"size:255;not null"`
	Role         string     `gorm:"size:16;not null;index"`
	GroupID      uint64     `gorm:"index"`
	Quota        int64      `gorm:"not null;default:0"`
	UsedQuota    int64      `gorm:"not null;default:0"`
	Status       string     `gorm:"size:16;not null;index"`
	TwoFASecret  string     `gorm:"size:64"`
	QuotaAlertAt int64      `gorm:"not null;default:0"`
	QuotaAlertSentAt *time.Time
	RPMLimit     int        `gorm:"not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (UserRow) TableName() string { return "users" }

// GroupRow 分组表映射。
type GroupRow struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement"`
	Name            string `gorm:"uniqueIndex;size:64;not null"`
	PriceMultiplier float64 `gorm:"not null;default:1"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (GroupRow) TableName() string { return "groups" }

// SessionRow 会话表映射。
type SessionRow struct {
	ID        string    `gorm:"primaryKey;size:64"`
	UserID    uint64    `gorm:"not null;index"`
	ExpiresAt time.Time `gorm:"not null;index"`
	IP        string    `gorm:"size:64"`
	UserAgent string    `gorm:"size:255"`
	CreatedAt time.Time
}

func (SessionRow) TableName() string { return "sessions" }

// ApiKeyRow API 密钥表映射。
type ApiKeyRow struct {
	ID             uint64              `gorm:"primaryKey;autoIncrement"`
	UserID         uint64              `gorm:"not null;index"`
	KeyPrefix      string              `gorm:"size:32;not null"`
	KeySuffix      string              `gorm:"size:8;not null"`
	KeyHash        string              `gorm:"uniqueIndex;size:64;not null"`
	Name           string              `gorm:"size:64;not null"`
	ModelWhitelist jsonArray[string]   `gorm:"type:text"`
	IPWhitelist    jsonArray[string]   `gorm:"type:text"`
	QuotaLimit     int64               `gorm:"not null;default:0"`
	UsedQuota      int64               `gorm:"not null;default:0"`
	RPMLimit       int                 `gorm:"not null;default:0"`
	TPMLimit       int                 `gorm:"not null;default:0"`
	ExpiresAt      *time.Time
	LastUsedAt     *time.Time
	Status         string              `gorm:"size:16;not null;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ApiKeyRow) TableName() string { return "api_keys" }

// ChannelRow 渠道表映射。
// GroupIDs 不再作为 JSON blob 存储，改为 channel_groups 关联表（见 ChannelGroupRow）。
type ChannelRow struct {
	ID              uint64            `gorm:"primaryKey;autoIncrement"`
	Name            string            `gorm:"size:64;not null"`
	Provider        string            `gorm:"size:32;not null;index"`
	BaseURL         string            `gorm:"size:255"`
	Credentials     string            `gorm:"type:text;not null"` // 加密后密文
	Models          jsonArray[string] `gorm:"type:text"`
	Weight          int               `gorm:"not null;default:100"`
	PriceMultiplier float64           `gorm:"not null;default:1"`
	Status          string            `gorm:"size:16;not null;index"`
	TestedAt        *time.Time
	LatencyMs       int
	Note            string `gorm:"size:512"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (ChannelRow) TableName() string { return "channels" }

// ChannelGroupRow 渠道 × 分组 多对多关联表。
//
// 替代原先 channels.group_ids JSON blob，建立真实外键关系：
//   - 删除 group 时可级联清理
//   - 可用索引查询
type ChannelGroupRow struct {
	ChannelID uint64 `gorm:"primaryKey;autoIncrement:false"`
	GroupID   uint64 `gorm:"primaryKey;autoIncrement:false;index"`
	CreatedAt time.Time
}

func (ChannelGroupRow) TableName() string { return "channel_groups" }

// ModelPriceRow 模型价格表映射。
type ModelPriceRow struct {
	ID               uint64  `gorm:"primaryKey;autoIncrement"`
	Model            string  `gorm:"size:128;not null;uniqueIndex:uq_model_cap,priority:1"`
	Capability       string  `gorm:"size:16;not null;uniqueIndex:uq_model_cap,priority:2"`
	InputPrice       int64   `gorm:"not null;default:0"`
	OutputPrice      int64   `gorm:"not null;default:0"`
	CachePrice       int64   `gorm:"not null;default:0"`
	OutputMultiplier float64 `gorm:"not null;default:1"`
	TaskPrice        int64   `gorm:"not null;default:0"`
	Enabled          bool    `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (ModelPriceRow) TableName() string { return "model_prices" }

// UsageRow 调用日志表映射。
type UsageRow struct {
	ID                 uint64 `gorm:"primaryKey;autoIncrement"`
	UserID             uint64 `gorm:"not null;index"`
	ApiKeyID           uint64 `gorm:"index"`
	ChannelID          uint64 `gorm:"index"`
	Model              string `gorm:"size:128;not null;index"`
	Capability         string `gorm:"size:16;not null"`
	PromptTokens       int
	CompletionTokens   int
	CacheTokens        int
	CacheWriteTokens   int // 5m TTL 缓存创建
	CacheWrite1hTokens int // 1h TTL 缓存创建
	ReasoningTokens    int
	Cost               int64 `gorm:"not null;default:0"`
	LatencyMs          int
	Status             string `gorm:"size:16;not null"`
	ErrorMessage       string `gorm:"size:512"`
	RequestID          string `gorm:"size:64;index"`
	CreatedAt          time.Time `gorm:"index"`
}

func (UsageRow) TableName() string { return "usages" }

// LedgerRow 账本流水表映射。
type LedgerRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	UserID    uint64 `gorm:"not null;index"`
	Type      string `gorm:"size:16;not null;index"`
	Amount    int64  `gorm:"not null"`
	Balance   int64  `gorm:"not null"`
	RefID     string `gorm:"size:64;index"`
	Note      string `gorm:"size:255"`
	CreatedAt time.Time `gorm:"index"`
}

func (LedgerRow) TableName() string { return "ledgers" }

// TaskRow 异步任务表映射。
type TaskRow struct {
	ID         string `gorm:"primaryKey;size:64"`
	UserID     uint64 `gorm:"not null;index"`
	ApiKeyID   uint64 `gorm:"index"`
	ChannelID  uint64 `gorm:"index"`
	Provider   string `gorm:"size:32;not null;index"`
	Action     string `gorm:"size:32;not null"`
	Model      string `gorm:"size:128"`
	Input      []byte `gorm:"type:blob"`
	Status     string `gorm:"size:16;not null;index"`
	Progress   int
	Result     []byte `gorm:"type:blob"`
	ExternalID string `gorm:"size:128;index"`
	Cost       int64
	Refunded   bool      `gorm:"not null;default:false"`
	Error      string    `gorm:"size:512"`
	CreatedAt  time.Time `gorm:"index"`
	StartedAt  *time.Time
	FinishedAt *time.Time
}

func (TaskRow) TableName() string { return "tasks" }

// RedemptionRow 兑换码表映射。
type RedemptionRow struct {
	ID        uint64  `gorm:"primaryKey;autoIncrement"`
	Code      string  `gorm:"uniqueIndex;size:32;not null"`
	Amount    int64   `gorm:"not null"`
	UsedBy    *uint64 `gorm:"index"`
	UsedAt    *time.Time
	ExpiresAt *time.Time
	// BatchName 用于管理后台按批次分组显示；同一批次内 name/prefix/amount 一致。
	BatchName string `gorm:"size:128;index"`
	Prefix    string `gorm:"size:32"`
	Note      string `gorm:"size:128"`
	CreatedAt time.Time
}

func (RedemptionRow) TableName() string { return "redemptions" }

// 已删除（YAGNI）：SubscriptionRow / PaymentOrderRow / OAuthBindingRow。
// 这三张表在 M5 阶段建过，但没有对应的 domain 实体与 service，
// 引入时只是占位。等真有订阅/支付/OAuth 登录业务再补回来。

// PaymentOrderRow 支付订单表。M5 阶段接入 Stripe 后正式启用。
type PaymentOrderRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	UserID      uint64 `gorm:"not null;index"`
	Amount      int64  `gorm:"not null"` // micro
	AmountCents int64  `gorm:"not null"` // cents
	Currency    string `gorm:"size:8;not null"`
	Gateway     string `gorm:"size:16;not null;index"`
	GatewayRef  string `gorm:"size:128;index"` // 网关 session/order id，防重幂等
	Mode        string `gorm:"size:16;not null;default:'payment'"`
	PlanCode    string `gorm:"size:64;index"`
	Status      string `gorm:"size:16;not null;index"`
	CheckoutURL string `gorm:"size:512"`
	PaidAt      *time.Time
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time
}

func (PaymentOrderRow) TableName() string { return "payment_orders" }

// SubscriptionRow 订阅表。
type SubscriptionRow struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"not null;index"`
	PlanCode       string    `gorm:"size:64;not null;index"`
	Status         string    `gorm:"size:16;not null;index"`
	PeriodQuota    int64     `gorm:"not null;default:0"` // 本周期配额 (micro)
	GatewayRef     string    `gorm:"size:128;index"`
	NextResetAt    time.Time `gorm:"not null;index"`
	CurrentPeriodEnd *time.Time
	CanceledAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (SubscriptionRow) TableName() string { return "subscriptions" }

// PlanRow 订阅套餐定义。
type PlanRow struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement"`
	Code            string `gorm:"uniqueIndex;size:64;not null"` // "pro_monthly"
	Name            string `gorm:"size:128;not null"`
	PriceCents      int64  `gorm:"not null"`
	Currency        string `gorm:"size:8;not null"`
	PeriodDays      int    `gorm:"not null;default:30"`
	IncludedQuota   int64  `gorm:"not null"` // 每周期发放配额 (micro)
	GatewayRef      string `gorm:"size:128"`  // Stripe price id 等
	Enabled         bool   `gorm:"not null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (PlanRow) TableName() string { return "subscription_plans" }

// OAuthBindingRow OAuth 第三方绑定（github / google / discord / 通用 oidc）。
type OAuthBindingRow struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	UserID     uint64 `gorm:"not null;index"`
	Provider   string `gorm:"size:32;not null;uniqueIndex:uq_oauth_provider_eid,priority:1"`
	ExternalID string `gorm:"size:128;not null;uniqueIndex:uq_oauth_provider_eid,priority:2"`
	Email      string `gorm:"size:255"`
	CreatedAt  time.Time
}

func (OAuthBindingRow) TableName() string { return "oauth_bindings" }

// AuditLogRow 审计日志。
type AuditLogRow struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	ActorID   uint64 `gorm:"not null;index"` // 操作发起者（管理员 user.id）
	Action    string `gorm:"size:64;not null;index"` // channel.create / user.ban / ...
	Target    string `gorm:"size:128"`
	Meta      []byte `gorm:"type:blob"` // JSON
	IP        string `gorm:"size:64"`
	CreatedAt time.Time `gorm:"index"`
}

func (AuditLogRow) TableName() string { return "audit_logs" }

// VerifyTokenRow 一次性验证 token（邮箱验证 / 密码重置）。
type VerifyTokenRow struct {
	ID        string `gorm:"primaryKey;size:64"`
	UserID    uint64 `gorm:"not null;index"`
	Purpose   string `gorm:"size:32;not null;index"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (VerifyTokenRow) TableName() string { return "verify_tokens" }

// errNotFound 包内统一未找到错误，供 repo 复用。
var errNotFound = errors.New("not found")
