package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ---------- ModelPrice Repo ----------

type ModelPriceRepo struct{ db *gorm.DB }

func NewModelPriceRepo(db *gorm.DB) *ModelPriceRepo { return &ModelPriceRepo{db: db} }

func toPriceRow(p *billing.ModelPrice) *ModelPriceRow {
	return &ModelPriceRow{
		ID:               p.ID,
		Model:            p.Model,
		Capability:       string(p.Capability),
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CachePrice:       p.CachePrice,
		OutputMultiplier: p.OutputMultiplier,
		TaskPrice:        p.TaskPrice,
		Enabled:          p.Enabled,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func fromPriceRow(r *ModelPriceRow) *billing.ModelPrice {
	return &billing.ModelPrice{
		ID:               r.ID,
		Model:            r.Model,
		Capability:       billing.Capability(r.Capability),
		InputPrice:       r.InputPrice,
		OutputPrice:      r.OutputPrice,
		CachePrice:       r.CachePrice,
		OutputMultiplier: r.OutputMultiplier,
		TaskPrice:        r.TaskPrice,
		Enabled:          r.Enabled,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

// Upsert 按 (model, capability) 唯一键 upsert。
func (r *ModelPriceRepo) Upsert(ctx context.Context, p *billing.ModelPrice) error {
	row := toPriceRow(p)
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "model"}, {Name: "capability"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"input_price", "output_price", "cache_price",
			"output_multiplier", "task_price", "enabled", "updated_at",
		}),
	}).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "upsert model price", err)
	}
	p.ID = row.ID
	return nil
}

func (r *ModelPriceRepo) Get(ctx context.Context, model string, cap billing.Capability) (*billing.ModelPrice, error) {
	var row ModelPriceRow
	err := r.db.WithContext(ctx).Where("model = ? AND capability = ?", model, string(cap)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get model price", err)
	}
	return fromPriceRow(&row), nil
}

func (r *ModelPriceRepo) List(ctx context.Context) ([]*billing.ModelPrice, error) {
	var rows []ModelPriceRow
	if err := r.db.WithContext(ctx).Order("model, capability").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list model prices", err)
	}
	out := make([]*billing.ModelPrice, 0, len(rows))
	for i := range rows {
		out = append(out, fromPriceRow(&rows[i]))
	}
	return out, nil
}

func (r *ModelPriceRepo) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&ModelPriceRow{}, id).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete model price", err)
	}
	return nil
}

// ReplaceNonTask 事务内删除所有 capability != 'task' 的价格，再批量插入新记录。
// 用于从 LiteLLM 等上游全量同步时保留本地按次计费（Midjourney/Suno）记录。
// 返回 (inserted, deleted, error)。
func (r *ModelPriceRepo) ReplaceNonTask(ctx context.Context, prices []*billing.ModelPrice) (inserted int, deleted int, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("capability <> ?", string(billing.CapTask)).Delete(&ModelPriceRow{})
		if res.Error != nil {
			return derrors.Wrap(derrors.CodeInternal, "delete non-task prices", res.Error)
		}
		deleted = int(res.RowsAffected)

		if len(prices) == 0 {
			return nil
		}
		rows := make([]*ModelPriceRow, 0, len(prices))
		now := time.Now()
		for _, p := range prices {
			row := toPriceRow(p)
			if row.CreatedAt.IsZero() {
				row.CreatedAt = now
			}
			row.UpdatedAt = now
			rows = append(rows, row)
		}
		if err := tx.CreateInBatches(rows, 100).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "bulk insert prices", err)
		}
		inserted = len(rows)
		return nil
	})
	return inserted, deleted, err
}

// ---------- Usage Repo ----------

type UsageRepo struct{ db *gorm.DB }

func NewUsageRepo(db *gorm.DB) *UsageRepo { return &UsageRepo{db: db} }

func toUsageRow(u *billing.Usage) *UsageRow {
	return &UsageRow{
		ID:                 u.ID,
		UserID:             u.UserID,
		ApiKeyID:           u.ApiKeyID,
		ChannelID:          u.ChannelID,
		Model:              u.Model,
		Capability:         string(u.Capability),
		PromptTokens:       u.PromptTokens,
		CompletionTokens:   u.CompletionTokens,
		CacheTokens:        u.CacheTokens,
		CacheWriteTokens:   u.CacheWriteTokens,
		CacheWrite1hTokens: u.CacheWrite1hTokens,
		ReasoningTokens:    u.ReasoningTokens,
		Cost:               u.Cost,
		LatencyMs:          u.LatencyMs,
		Status:             string(u.Status),
		ErrorMessage:       u.ErrorMessage,
		RequestID:          u.RequestID,
		CreatedAt:          u.CreatedAt,
	}
}

func fromUsageRow(r *UsageRow) *billing.Usage {
	return &billing.Usage{
		ID:                 r.ID,
		UserID:             r.UserID,
		ApiKeyID:           r.ApiKeyID,
		ChannelID:          r.ChannelID,
		Model:              r.Model,
		Capability:         billing.Capability(r.Capability),
		PromptTokens:       r.PromptTokens,
		CompletionTokens:   r.CompletionTokens,
		CacheTokens:        r.CacheTokens,
		CacheWriteTokens:   r.CacheWriteTokens,
		CacheWrite1hTokens: r.CacheWrite1hTokens,
		ReasoningTokens:    r.ReasoningTokens,
		Cost:               r.Cost,
		LatencyMs:          r.LatencyMs,
		Status:             billing.Status(r.Status),
		ErrorMessage:       r.ErrorMessage,
		RequestID:          r.RequestID,
		CreatedAt:          r.CreatedAt,
	}
}

// UsageRepo 只提供只读查询；Create 由 QuotaDelta.Apply 在事务内完成。
func (r *UsageRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*billing.Usage, int64, error) {
	var rows []UsageRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&UsageRow{}).Where("user_id = ?", userID)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count usages", err)
	}
	if err := tx.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list usages", err)
	}
	out := make([]*billing.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, fromUsageRow(&rows[i]))
	}
	return out, total, nil
}

func (r *UsageRepo) ListAll(ctx context.Context, offset, limit int) ([]*billing.Usage, int64, error) {
	var rows []UsageRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&UsageRow{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count usages", err)
	}
	if err := tx.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list usages", err)
	}
	out := make([]*billing.Usage, 0, len(rows))
	for i := range rows {
		out = append(out, fromUsageRow(&rows[i]))
	}
	return out, total, nil
}

func (r *UsageRepo) SumCostByUser(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Select("COALESCE(SUM(cost), 0)").Scan(&total).Error
	if err != nil {
		return 0, derrors.Wrap(derrors.CodeInternal, "sum usage cost", err)
	}
	return total, nil
}

// ---------- 聚合统计 ----------

// scopeUser userID=0 表示全量（admin 场景）。
func scopeUser(tx *gorm.DB, userID uint64) *gorm.DB {
	if userID > 0 {
		return tx.Where("user_id = ?", userID)
	}
	return tx
}

// AggByDay 按天分组。SQLite 用 strftime；Postgres 用 to_char。
// 统一通过 GORM 的 Rows() 返回，由调用方解析。
func (r *UsageRepo) AggByDay(ctx context.Context, userID uint64, since time.Time) ([]billing.DailyAgg, error) {
	// SQLite 与 PG 都支持 date() 函数，返回 YYYY-MM-DD
	tx := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("created_at >= ?", since).
		Select(`date(created_at) AS day,
			COUNT(*) AS requests,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(cost), 0) AS cost`).
		Group("day").
		Order("day ASC")
	tx = scopeUser(tx, userID)

	type row struct {
		Day              string
		Requests         int64
		PromptTokens     int64
		CompletionTokens int64
		Cost             int64
	}
	var rows []row
	if err := tx.Scan(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "agg by day", err)
	}
	out := make([]billing.DailyAgg, 0, len(rows))
	for _, r := range rows {
		out = append(out, billing.DailyAgg{
			Date:             r.Day,
			Requests:         r.Requests,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			Cost:             r.Cost,
		})
	}
	return out, nil
}

// AggByModel 按 model 分组，按 cost DESC 排。
func (r *UsageRepo) AggByModel(ctx context.Context, userID uint64, since time.Time) ([]billing.ModelAgg, error) {
	tx := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("created_at >= ?", since).
		Select(`model, COUNT(*) AS requests, COALESCE(SUM(cost), 0) AS cost`).
		Group("model").
		Order("cost DESC").
		Limit(20)
	tx = scopeUser(tx, userID)

	var out []billing.ModelAgg
	if err := tx.Scan(&out).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "agg by model", err)
	}
	return out, nil
}

// AggByCapability 按 capability 分组。
func (r *UsageRepo) AggByCapability(ctx context.Context, userID uint64, since time.Time) ([]billing.CapabilityAgg, error) {
	tx := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("created_at >= ?", since).
		Select(`capability, COUNT(*) AS requests, COALESCE(SUM(cost), 0) AS cost`).
		Group("capability").
		Order("cost DESC")
	tx = scopeUser(tx, userID)

	var out []billing.CapabilityAgg
	if err := tx.Scan(&out).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "agg by capability", err)
	}
	return out, nil
}

// AggByStatus 成功/失败分组。
func (r *UsageRepo) AggByStatus(ctx context.Context, userID uint64, since time.Time) ([]billing.StatusAgg, error) {
	tx := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("created_at >= ?", since).
		Select(`status, COUNT(*) AS requests`).
		Group("status")
	tx = scopeUser(tx, userID)

	var out []billing.StatusAgg
	if err := tx.Scan(&out).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "agg by status", err)
	}
	return out, nil
}

// TopUsersByCost 花费 Top N 用户（admin 用）。
func (r *UsageRepo) TopUsersByCost(ctx context.Context, since time.Time, limit int) ([]billing.TopUserAgg, error) {
	if limit <= 0 {
		limit = 10
	}
	// JOIN users 补 email
	var out []billing.TopUserAgg
	err := r.db.WithContext(ctx).
		Table("usages AS u").
		Joins("LEFT JOIN users AS usr ON usr.id = u.user_id").
		Where("u.created_at >= ?", since).
		Select(`u.user_id AS user_id,
			COALESCE(usr.email, '') AS email,
			COUNT(*) AS requests,
			COALESCE(SUM(u.cost), 0) AS cost`).
		Group("u.user_id, usr.email").
		Order("cost DESC").
		Limit(limit).
		Scan(&out).Error
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "top users by cost", err)
	}
	return out, nil
}

// CountRequestsByUser 单 user 的总请求数（since 之后）。
func (r *UsageRepo) CountRequestsByUser(ctx context.Context, userID uint64, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&UsageRow{}).
		Where("user_id = ? AND created_at >= ?", userID, since).
		Count(&count).Error
	if err != nil {
		return 0, derrors.Wrap(derrors.CodeInternal, "count requests", err)
	}
	return count, nil
}

// ---------- Ledger Repo ----------

type LedgerRepo struct{ db *gorm.DB }

func NewLedgerRepo(db *gorm.DB) *LedgerRepo { return &LedgerRepo{db: db} }

func toLedgerRow(l *billing.Ledger) *LedgerRow {
	return &LedgerRow{
		ID:        l.ID,
		UserID:    l.UserID,
		Type:      string(l.Type),
		Amount:    l.Amount,
		Balance:   l.Balance,
		RefID:     l.RefID,
		Note:      l.Note,
		CreatedAt: l.CreatedAt,
	}
}

func fromLedgerRow(r *LedgerRow) *billing.Ledger {
	return &billing.Ledger{
		ID:        r.ID,
		UserID:    r.UserID,
		Type:      billing.LedgerType(r.Type),
		Amount:    r.Amount,
		Balance:   r.Balance,
		RefID:     r.RefID,
		Note:      r.Note,
		CreatedAt: r.CreatedAt,
	}
}

// LedgerRepo 只提供只读查询；Create 由 QuotaDelta.Apply 在事务内完成。
func (r *LedgerRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*billing.Ledger, int64, error) {
	var rows []LedgerRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&LedgerRow{}).Where("user_id = ?", userID)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count ledgers", err)
	}
	if err := tx.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list ledgers", err)
	}
	out := make([]*billing.Ledger, 0, len(rows))
	for i := range rows {
		out = append(out, fromLedgerRow(&rows[i]))
	}
	return out, total, nil
}
