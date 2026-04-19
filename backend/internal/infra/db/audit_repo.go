package db

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/audit"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// AuditLogRepo GORM 实现 audit.Repository。
type AuditLogRepo struct{ db *gorm.DB }

// NewAuditLogRepo 构造。
func NewAuditLogRepo(db *gorm.DB) *AuditLogRepo { return &AuditLogRepo{db: db} }

func (r *AuditLogRepo) Create(ctx context.Context, l *audit.Log) error {
	row := AuditLogRow{
		ActorID:   l.ActorID,
		Action:    l.Action,
		Target:    l.Target,
		Meta:      l.Meta,
		IP:        l.IP,
		CreatedAt: time.Now(),
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create audit log", err)
	}
	l.ID = row.ID
	l.CreatedAt = row.CreatedAt
	return nil
}

func (r *AuditLogRepo) List(ctx context.Context, offset, limit int) ([]*audit.Log, int64, error) {
	return r.listWhere(ctx, "", nil, offset, limit)
}

func (r *AuditLogRepo) ListByActor(ctx context.Context, actorID uint64, offset, limit int) ([]*audit.Log, int64, error) {
	return r.listWhere(ctx, "actor_id = ?", []any{actorID}, offset, limit)
}

func (r *AuditLogRepo) listWhere(ctx context.Context, where string, args []any, offset, limit int) ([]*audit.Log, int64, error) {
	var rows []AuditLogRow
	var total int64
	q := r.db.WithContext(ctx).Model(&AuditLogRow{})
	if where != "" {
		q = q.Where(where, args...)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count audits", err)
	}
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list audits", err)
	}
	out := make([]*audit.Log, 0, len(rows))
	for i := range rows {
		out = append(out, &audit.Log{
			ID:        rows[i].ID,
			ActorID:   rows[i].ActorID,
			Action:    rows[i].Action,
			Target:    rows[i].Target,
			Meta:      rows[i].Meta,
			IP:        rows[i].IP,
			CreatedAt: rows[i].CreatedAt,
		})
	}
	return out, total, nil
}
