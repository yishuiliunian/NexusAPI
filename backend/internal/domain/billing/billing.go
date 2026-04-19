// Package billing 定义计费域。
//
// 计费以 micro-unit 为基本单位（1 元 = 1_000_000 micro），避免浮点误差。
// 核心实体：
//   - ModelPrice：单价表
//   - Usage：调用日志 + 费用记录
//   - Ledger：配额账本（增删扣返四种变更）
package billing

import (
	"context"
	"time"
)

// Capability 能力形态。与 relay.Capability 对齐。
type Capability string

const (
	CapChat             Capability = "chat"
	CapResponses        Capability = "responses"
	CapEmbedding        Capability = "embedding"
	CapRerank           Capability = "rerank"
	CapImage            Capability = "image"
	CapImageEdit        Capability = "image_edit"
	CapImageVariation   Capability = "image_variation"
	CapTTS              Capability = "tts"
	CapSTT              Capability = "stt"
	CapAudioTranslation Capability = "audio_translation"
	CapModeration       Capability = "moderation"
	CapRealtime         Capability = "realtime"
	CapTask             Capability = "task"
)

// ModelPrice 单个模型在单一能力下的价格。
type ModelPrice struct {
	ID               uint64     `json:"id"`
	Model            string     `json:"model"`
	Capability       Capability `json:"capability"`
	InputPrice       int64      `json:"input_price"`  // 每 1M token 的 micro 数
	OutputPrice      int64      `json:"output_price"`
	CachePrice       int64      `json:"cache_price"`  // 缓存命中价
	OutputMultiplier float64    `json:"output_multiplier"` // thinking 模型输出翻倍等
	TaskPrice        int64      `json:"task_price"`   // 按次任务价格
	Enabled          bool       `json:"enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ModelPriceRepository 单价表仓储。
type ModelPriceRepository interface {
	Upsert(ctx context.Context, p *ModelPrice) error
	Get(ctx context.Context, model string, cap Capability) (*ModelPrice, error)
	List(ctx context.Context) ([]*ModelPrice, error)
	Delete(ctx context.Context, id uint64) error
}

// Status 调用状态。
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

// Usage 调用日志 + 计费流水。
type Usage struct {
	ID               uint64     `json:"id"`
	UserID           uint64     `json:"user_id"`
	ApiKeyID         uint64     `json:"api_key_id"`
	ChannelID        uint64     `json:"channel_id"`
	Model            string     `json:"model"`
	Capability       Capability `json:"capability"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	// CacheTokens 缓存命中（Anthropic cache_read_input_tokens / OpenAI cached_tokens）。
	CacheTokens int `json:"cache_tokens"`
	// CacheWriteTokens 缓存创建 5min TTL（Anthropic ephemeral_5m_input_tokens）。
	// 按 Anthropic 定价标准：input_price × 1.25。
	CacheWriteTokens int `json:"cache_write_tokens"`
	// CacheWrite1hTokens 缓存创建 1h TTL（Anthropic ephemeral_1h_input_tokens）。
	// 按 Anthropic 定价标准：input_price × 2.0。
	CacheWrite1hTokens int `json:"cache_write_1h_tokens"`
	// ReasoningTokens o1 / deepseek-r1 思考 tokens。与 completion 同 output 价。
	ReasoningTokens int       `json:"reasoning_tokens"`
	Cost            int64     `json:"cost"`        // micro
	LatencyMs       int       `json:"latency_ms"`
	Status          Status    `json:"status"`
	ErrorMessage    string    `json:"error_message"`
	RequestID       string    `json:"request_id"`
	CreatedAt       time.Time `json:"created_at"`
}

// UsageRepository 调用日志仓储（只读）。
//
// 写操作统一走 QuotaDelta.Apply 的事务内 Create。
type UsageRepository interface {
	ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*Usage, int64, error)
	ListAll(ctx context.Context, offset, limit int) ([]*Usage, int64, error)
	SumCostByUser(ctx context.Context, userID uint64, since time.Time) (int64, error)

	// ---------- 聚合统计（供 Dashboard / Admin Overview 画图）----------

	// AggByDay 按天分组，返回 (date, requests, prompt_tokens, completion_tokens, cost)。
	// userID = 0 表示全量（admin 用）。
	AggByDay(ctx context.Context, userID uint64, since time.Time) ([]DailyAgg, error)
	// AggByModel 按 model 分组。
	AggByModel(ctx context.Context, userID uint64, since time.Time) ([]ModelAgg, error)
	// AggByCapability 按能力分组。
	AggByCapability(ctx context.Context, userID uint64, since time.Time) ([]CapabilityAgg, error)
	// AggByStatus 成功/失败分布（供 Admin 错误率图）。
	AggByStatus(ctx context.Context, userID uint64, since time.Time) ([]StatusAgg, error)
	// TopUsersByCost 按 cost 排 Top N 用户（Admin 专用）。
	TopUsersByCost(ctx context.Context, since time.Time, limit int) ([]TopUserAgg, error)
	// CountRequestsByUser 单 user 的总请求数。
	CountRequestsByUser(ctx context.Context, userID uint64, since time.Time) (int64, error)
}

// DailyAgg 单日聚合。Date 是 "2026-04-18" 形式。
type DailyAgg struct {
	Date             string `json:"date"`
	Requests         int64  `json:"requests"`
	PromptTokens    int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Cost             int64  `json:"cost"`
}

// ModelAgg 按模型聚合。
type ModelAgg struct {
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
	Cost     int64  `json:"cost"`
}

// CapabilityAgg 按能力聚合。
type CapabilityAgg struct {
	Capability string `json:"capability"`
	Requests   int64  `json:"requests"`
	Cost       int64  `json:"cost"`
}

// StatusAgg 按调用状态分组。
type StatusAgg struct {
	Status   string `json:"status"`
	Requests int64  `json:"requests"`
}

// TopUserAgg 花费 Top N 用户（Admin 统计）。
type TopUserAgg struct {
	UserID   uint64 `json:"user_id"`
	Email    string `json:"email"`
	Requests int64  `json:"requests"`
	Cost     int64  `json:"cost"`
}

// LedgerType 账本变更类型。
type LedgerType string

const (
	LedgerReserve     LedgerType = "reserve"      // 中继请求预占（负数）
	LedgerSettle      LedgerType = "settle"       // 中继请求结算调整（正/负，使实际=预占+差额）
	LedgerRefund      LedgerType = "refund"       // 中继请求失败退款（正数）
	LedgerTaskCharge  LedgerType = "task_charge"  // 任务扣费（负数）
	LedgerTaskRefund  LedgerType = "task_refund"  // 任务失败退款（正数）
	LedgerTopUp       LedgerType = "topup"        // 充值（正数）
	LedgerRedeem      LedgerType = "redeem"       // 兑换码（正数）
	LedgerSubscribe   LedgerType = "subscribe"    // 订阅周期额度（正数）
	LedgerAdjust      LedgerType = "adjust"       // 管理员调整（正/负）
)

// Ledger 账本流水。所有对 User.Quota 的变动均通过账本记录。
type Ledger struct {
	ID        uint64
	UserID    uint64
	Type      LedgerType
	Amount    int64 // 有符号，micro
	Balance   int64 // 变更后 user.Quota
	RefID     string // 关联 UsageID / OrderID / ReservationID
	Note      string
	CreatedAt time.Time
}

// LedgerRepository 账本仓储（只读）。
//
// 写操作统一走 QuotaDelta.Apply 的事务内 Create。
type LedgerRepository interface {
	ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*Ledger, int64, error)
}
