package billing

import "context"

// QuotaOp 描述一次配额原子变更的全部输入。
type QuotaOp struct {
	UserID uint64

	// Amount 正数=充值/退款；负数=预占/扣减；零=仅写 Ledger 不改余额。
	Amount int64

	// AddUsed 计入 users.used_quota 累计（通常只在 Settle 阶段 > 0）。
	AddUsed int64

	// ApiKeyID 非零时，同事务按 AddUsed 累加 api_keys.used_quota。
	ApiKeyID uint64

	// Ledger 必填。Balance 字段由 QuotaDelta.Apply 覆写。
	Ledger *Ledger

	// Usage 可选。非 nil 时持久化为一条消耗日志。
	Usage *Usage
}

// QuotaDelta 配额变更原子操作契约。
//
// Engine（app 层）依赖此接口，不触碰 GORM。infra/db 实现事务内完成：
//  1. 行锁读 user.quota
//  2. 校验余额（Amount 为负时若余额不足返回 ErrInsufficientQuota）
//  3. UPDATE user.quota (+ used_quota)
//  4. 写 ledger 流水（Balance 由 infra 填入）
//  5. 可选：写 usage 记录
//  6. 可选：若 ApiKeyID 非零，累加 api_keys.used_quota
//
// 整个事务要么全成功，要么全回滚。
type QuotaDelta interface {
	Apply(ctx context.Context, op QuotaOp) (newBalance int64, err error)
}
