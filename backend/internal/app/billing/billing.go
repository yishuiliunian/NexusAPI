// Package billing 提供计费引擎：预占 / 结算 / 退款 / 补充额度。
//
// 架构：
//   - Engine 只负责业务语义（reservation 内存簿记、计费公式、编排）
//   - DB 事务由 domain/billing.QuotaDelta 抽象出来，实现在 infra/db
//   - 测试时可用 in-memory QuotaDelta 替代，无需真实 DB
//
// 性能：Reserve 阶段把 user + group 倍率解析为 PricingContext，
// Settle/Compute 阶段复用，避免每请求重复查 users/groups。
package billing

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// PricingContext Reserve 阶段一次性解析好的计费上下文。
//
// 由 Engine.BuildContext 构造，后续 Compute / Settle 直接复用，
// 避免 3 次 DB 查询（users + groups）。
type PricingContext struct {
	UserID            uint64
	GroupPriceMultiplier float64 // 用户分组倍率；默认 1.0
}

// Engine 计费引擎。
type Engine struct {
	delta        billing.QuotaDelta
	users        user.Repository
	prices       billing.ModelPriceRepository
	groups       user.GroupRepository
	reservations billing.ReservationStore
	// StrictPricing 为 true 时，Compute 在模型价格缺失或未启用时返回 ErrNoPrice
	// （由 passthrough 层拒绝请求）。为 false 时退化为 0 成本（不推荐，会导致计费漏洞）。
	// 默认 true。
	strictPricing bool
}

// ErrNoPrice Compute 在 StrictPricing=true 且无法找到启用的价格时返回此错误。
var ErrNoPrice = derrors.New(derrors.CodeInvalidArgument, "该模型未配置价格或已禁用，请联系管理员同步价格")

// EnsurePriced 在请求转发前预检模型+能力是否已配置启用的价格。
// StrictPricing=true 时缺价 → 返回 ErrNoPrice（passthrough 拒 402，不转发上游，不消耗预占）。
// StrictPricing=false 时永远返回 nil（开发期兜底）。
func (e *Engine) EnsurePriced(ctx context.Context, model string, cap billing.Capability) error {
	if !e.strictPricing {
		return nil
	}
	price, err := e.prices.Get(ctx, model, cap)
	if err != nil {
		if derrors.Is(err, derrors.CodeNotFound) {
			return ErrNoPrice
		}
		return err
	}
	if !price.Enabled {
		return ErrNoPrice
	}
	return nil
}

// ReservationTTL 预占 TTL。超出则被视为过期，无法 Settle。
const ReservationTTL = 10 * time.Minute

// NewEngine 构造。
//
// reservations 传 nil 时退化为内存实现（单副本部署）；水平扩展应传 cache.NewReservationStore。
// strictPricing 默认建议 true：缺价直接拒绝请求而不是免费通过。
func NewEngine(
	delta billing.QuotaDelta,
	users user.Repository,
	prices billing.ModelPriceRepository,
	groups user.GroupRepository,
	reservations billing.ReservationStore,
) *Engine {
	if reservations == nil {
		reservations = billing.NewMemoryReservations()
	}
	return &Engine{
		delta:        delta,
		users:        users,
		prices:       prices,
		groups:       groups,
		reservations: reservations,
		strictPricing: true, // 默认严格，显式 WithStrictPricing(false) 禁用
	}
}

// WithStrictPricing 配置 Compute 缺价时的行为。链式调用。
func (e *Engine) WithStrictPricing(strict bool) *Engine {
	e.strictPricing = strict
	return e
}

// BuildContext 从已加载的 user 构造 PricingContext（避免重复查 users）。
// 调用方若已通过 middleware.AuthApiKey 取得 *user.User，直接传入。
// 仅额外读 groups 一次（且仅当 user.GroupID != 0）。
func (e *Engine) BuildContext(ctx context.Context, u *user.User) PricingContext {
	if u == nil {
		return PricingContext{GroupPriceMultiplier: 1.0}
	}
	pc := PricingContext{UserID: u.ID, GroupPriceMultiplier: 1.0}
	if u.GroupID == 0 {
		return pc
	}
	g, err := e.groups.GetByID(ctx, u.GroupID)
	if err != nil || g.PriceMultiplier <= 0 {
		return pc
	}
	pc.GroupPriceMultiplier = g.PriceMultiplier
	return pc
}

// BuildContextByID 兼容旧调用，若调用方只有 userID。实现上多一次 users 查询。
// 建议优先使用 BuildContext(user)。
func (e *Engine) BuildContextByID(ctx context.Context, userID uint64) PricingContext {
	if userID == 0 {
		return PricingContext{GroupPriceMultiplier: 1.0}
	}
	u, err := e.users.GetByID(ctx, userID)
	if err != nil {
		return PricingContext{UserID: userID, GroupPriceMultiplier: 1.0}
	}
	return e.BuildContext(ctx, u)
}

// Reserve 预占 estimate micro（正数）。返回 reservation ID。
func (e *Engine) Reserve(ctx context.Context, userID uint64, estimate int64) (string, error) {
	if estimate <= 0 {
		estimate = 1
	}
	op := billing.QuotaOp{
		UserID: userID,
		Amount: -estimate,
		Ledger: &billing.Ledger{
			UserID:    userID,
			Type:      billing.LedgerReserve,
			Amount:    -estimate,
			Note:      "预占",
			CreatedAt: time.Now(),
		},
	}
	if _, err := e.delta.Apply(ctx, op); err != nil {
		return "", err
	}
	rid := fmt.Sprintf("r-%d-%d", userID, time.Now().UnixNano())
	if err := e.reservations.Save(ctx, billing.Reservation{ID: rid, UserID: userID, Amount: estimate}, ReservationTTL); err != nil {
		// 回滚：预占金额已扣但 reservation 未登记，此时应立即退回余额
		_ = e.refundRaw(ctx, userID, estimate, rid, "预占登记失败退款")
		return "", derrors.Wrap(derrors.CodeInternal, "save reservation", err)
	}
	return rid, nil
}

// Settle 结算。actual 为实际费用（正数）。
func (e *Engine) Settle(ctx context.Context, reservationID string, actual int64, usage *billing.Usage) error {
	r, err := e.reservations.Take(ctx, reservationID)
	if err != nil {
		if err == billing.ErrReservationNotFound {
			return derrors.New(derrors.CodeInvalidArgument, "reservation 不存在或已过期")
		}
		return derrors.Wrap(derrors.CodeInternal, "take reservation", err)
	}
	if actual < 0 {
		actual = 0
	}

	diff := r.Amount - actual
	op := billing.QuotaOp{
		UserID:  r.UserID,
		Amount:  diff,
		AddUsed: actual,
		Ledger: &billing.Ledger{
			UserID:    r.UserID,
			Type:      billing.LedgerSettle,
			Amount:    diff,
			RefID:     reservationID,
			Note:      fmt.Sprintf("结算 actual=%d", actual),
			CreatedAt: time.Now(),
		},
	}
	if usage != nil {
		usage.Cost = actual
		usage.CreatedAt = time.Now()
		op.Usage = usage
		op.ApiKeyID = usage.ApiKeyID // 同事务累加 api_keys.used_quota
	}
	_, err = e.delta.Apply(ctx, op)
	return err
}

// Refund 全额退回（请求失败场景）。
func (e *Engine) Refund(ctx context.Context, reservationID string) error {
	r, err := e.reservations.Take(ctx, reservationID)
	if err != nil {
		if err == billing.ErrReservationNotFound {
			return nil // 幂等：已被其他路径处理
		}
		return derrors.Wrap(derrors.CodeInternal, "take reservation", err)
	}
	return e.refundRaw(ctx, r.UserID, r.Amount, reservationID, "失败退款")
}

// refundRaw 内部助手：直接给 userID 退回 amount。
func (e *Engine) refundRaw(ctx context.Context, userID uint64, amount int64, refID, note string) error {
	op := billing.QuotaOp{
		UserID: userID,
		Amount: amount,
		Ledger: &billing.Ledger{
			UserID:    userID,
			Type:      billing.LedgerRefund,
			Amount:    amount,
			RefID:     refID,
			Note:      note,
			CreatedAt: time.Now(),
		},
	}
	_, err := e.delta.Apply(ctx, op)
	return err
}

// TopUp 充值（正数）。调用方用于 redeem、订阅赠额等。类型应使用正向 LedgerType。
func (e *Engine) TopUp(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error {
	if amount <= 0 {
		return derrors.New(derrors.CodeInvalidArgument, "充值金额必须为正")
	}
	return e.writeDelta(ctx, userID, amount, typ, refID, note)
}

// Charge 扣款（传正数，内部转负）。用于任务预扣等。
// 余额不足时返回 ErrInsufficientQuota。
func (e *Engine) Charge(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error {
	if amount <= 0 {
		return derrors.New(derrors.CodeInvalidArgument, "扣款金额必须为正")
	}
	return e.writeDelta(ctx, userID, -amount, typ, refID, note)
}

// Adjust 管理员调整（允许正负）。类型固定为 LedgerAdjust。
func (e *Engine) Adjust(ctx context.Context, userID uint64, delta int64, note string) error {
	if delta == 0 {
		return derrors.New(derrors.CodeInvalidArgument, "调整金额不能为 0")
	}
	return e.writeDelta(ctx, userID, delta, billing.LedgerAdjust, "", note)
}

// writeDelta 内部辅助：写一次 QuotaOp。
func (e *Engine) writeDelta(ctx context.Context, userID uint64, amount int64, typ billing.LedgerType, refID, note string) error {
	op := billing.QuotaOp{
		UserID: userID,
		Amount: amount,
		Ledger: &billing.Ledger{
			UserID:    userID,
			Type:      typ,
			Amount:    amount,
			RefID:     refID,
			Note:      note,
			CreatedAt: time.Now(),
		},
	}
	_, err := e.delta.Apply(ctx, op)
	return err
}

// Compute 纯计算函数：根据 usage + channel 倍率 + 预解析的 PricingContext 计算 micro 费用。
// 不触 DB，不查 users/groups。
//
// 计费公式（micro）：
//   prompt_tokens           × InputPrice
// + (completion + reasoning)× OutputPrice × OutputMultiplier   // 思考 tokens 同 output 价
// + cache_read              × CachePrice                       // 缓存命中（便宜）
// + cache_write_5m          × InputPrice × 1.25                // 5m TTL 缓存创建
// + cache_write_1h          × InputPrice × 2.0                 // 1h TTL 缓存创建
//   → 除以 1_000_000（价格按每 M token 计）
//   → 乘 channel + group 倍率
//   → task / image 再加 TaskPrice 按次
func (e *Engine) Compute(ctx context.Context, pc PricingContext, ch ChannelPricing, u *billing.Usage) (int64, error) {
	price, err := e.prices.Get(ctx, u.Model, u.Capability)
	if err != nil {
		if derrors.Is(err, derrors.CodeNotFound) {
			if e.strictPricing {
				return 0, ErrNoPrice
			}
			return 0, nil
		}
		return 0, err
	}
	if !price.Enabled {
		if e.strictPricing {
			return 0, ErrNoPrice
		}
		return 0, nil
	}

	outMul := price.OutputMultiplier
	if outMul <= 0 {
		outMul = 1
	}
	effectiveCompletion := u.CompletionTokens + u.ReasoningTokens
	cost := float64(u.PromptTokens)*float64(price.InputPrice) +
		float64(effectiveCompletion)*float64(price.OutputPrice)*outMul +
		float64(u.CacheTokens)*float64(price.CachePrice) +
		float64(u.CacheWriteTokens)*float64(price.InputPrice)*1.25 +
		float64(u.CacheWrite1hTokens)*float64(price.InputPrice)*2.0
	cost = cost / 1_000_000.0
	cost *= ch.PriceMultiplier * pc.GroupPriceMultiplier

	if u.Capability == billing.CapTask || u.Capability == billing.CapImage {
		cost += float64(price.TaskPrice) * ch.PriceMultiplier * pc.GroupPriceMultiplier
	}

	return int64(math.Ceil(cost)), nil
}

// ChannelPricing 计费中用到的渠道信息。
type ChannelPricing struct {
	PriceMultiplier float64
}
