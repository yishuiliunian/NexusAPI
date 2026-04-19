package db

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/task"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// TaskRepo GORM 实现 task.Repository。
type TaskRepo struct{ db *gorm.DB }

// NewTaskRepo 构造。
func NewTaskRepo(db *gorm.DB) *TaskRepo { return &TaskRepo{db: db} }

func toTaskRow(t *task.Task) *TaskRow {
	return &TaskRow{
		ID:         t.ID,
		UserID:     t.UserID,
		ApiKeyID:   t.ApiKeyID,
		ChannelID:  t.ChannelID,
		Provider:   t.Provider,
		Action:     t.Action,
		Model:      t.Model,
		Input:      []byte(t.Input),
		Status:     string(t.Status),
		Progress:   t.Progress,
		Result:     []byte(t.Result),
		ExternalID: t.ExternalID,
		Cost:       t.Cost,
		Refunded:   t.Refunded,
		Error:      t.Error,
		CreatedAt:  t.CreatedAt,
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}

func fromTaskRow(r *TaskRow) *task.Task {
	return &task.Task{
		ID:         r.ID,
		UserID:     r.UserID,
		ApiKeyID:   r.ApiKeyID,
		ChannelID:  r.ChannelID,
		Provider:   r.Provider,
		Action:     r.Action,
		Model:      r.Model,
		Input:      r.Input,
		Status:     task.Status(r.Status),
		Progress:   r.Progress,
		Result:     r.Result,
		ExternalID: r.ExternalID,
		Cost:       r.Cost,
		Refunded:   r.Refunded,
		Error:      r.Error,
		CreatedAt:  r.CreatedAt,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
	}
}

func (r *TaskRepo) Create(ctx context.Context, t *task.Task) error {
	if err := r.db.WithContext(ctx).Create(toTaskRow(t)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create task", err)
	}
	return nil
}

func (r *TaskRepo) GetByID(ctx context.Context, id string) (*task.Task, error) {
	var row TaskRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get task", err)
	}
	return fromTaskRow(&row), nil
}

func (r *TaskRepo) ListByUser(ctx context.Context, userID uint64, offset, limit int) ([]*task.Task, int64, error) {
	var rows []TaskRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&TaskRow{}).Where("user_id = ?", userID)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count tasks", err)
	}
	if err := tx.Order("created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list tasks", err)
	}
	out := make([]*task.Task, 0, len(rows))
	for i := range rows {
		out = append(out, fromTaskRow(&rows[i]))
	}
	return out, total, nil
}

func (r *TaskRepo) Update(ctx context.Context, t *task.Task) error {
	if err := r.db.WithContext(ctx).Save(toTaskRow(t)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update task", err)
	}
	return nil
}

// ListAll 管理台用：不限 user。
func (r *TaskRepo) ListAll(ctx context.Context, offset, limit int) ([]*task.Task, int64, error) {
	var rows []TaskRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&TaskRow{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count tasks all", err)
	}
	if err := tx.Order("created_at DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list tasks all", err)
	}
	out := make([]*task.Task, 0, len(rows))
	for i := range rows {
		out = append(out, fromTaskRow(&rows[i]))
	}
	return out, total, nil
}

// ListPending 返回所有 pending/running 任务，供 worker 轮询。
func (r *TaskRepo) ListPending(ctx context.Context, limit int) ([]*task.Task, error) {
	var rows []TaskRow
	err := r.db.WithContext(ctx).
		Where("status IN ?", []string{string(task.StatusPending), string(task.StatusRunning)}).
		Order("created_at").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list pending tasks", err)
	}
	out := make([]*task.Task, 0, len(rows))
	for i := range rows {
		out = append(out, fromTaskRow(&rows[i]))
	}
	return out, nil
}
