package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/oauth"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// OAuthBindingRepo GORM 实现 oauth.Repository。
type OAuthBindingRepo struct{ db *gorm.DB }

// NewOAuthBindingRepo 构造。
func NewOAuthBindingRepo(db *gorm.DB) *OAuthBindingRepo { return &OAuthBindingRepo{db: db} }

func (r *OAuthBindingRepo) Create(ctx context.Context, b *oauth.Binding) error {
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	row := OAuthBindingRow{
		UserID:     b.UserID,
		Provider:   b.Provider,
		ExternalID: b.ExternalID,
		Email:      b.Email,
		CreatedAt:  b.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create oauth binding", err)
	}
	b.ID = row.ID
	return nil
}

func (r *OAuthBindingRepo) GetByProviderExternal(ctx context.Context, provider, externalID string) (*oauth.Binding, error) {
	var row OAuthBindingRow
	err := r.db.WithContext(ctx).
		Where("provider = ? AND external_id = ?", provider, externalID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get oauth binding", err)
	}
	return &oauth.Binding{
		ID:         row.ID,
		UserID:     row.UserID,
		Provider:   row.Provider,
		ExternalID: row.ExternalID,
		Email:      row.Email,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func (r *OAuthBindingRepo) ListByUser(ctx context.Context, userID uint64) ([]*oauth.Binding, error) {
	var rows []OAuthBindingRow
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list bindings", err)
	}
	out := make([]*oauth.Binding, 0, len(rows))
	for i := range rows {
		out = append(out, &oauth.Binding{
			ID:         rows[i].ID,
			UserID:     rows[i].UserID,
			Provider:   rows[i].Provider,
			ExternalID: rows[i].ExternalID,
			Email:      rows[i].Email,
			CreatedAt:  rows[i].CreatedAt,
		})
	}
	return out, nil
}

func (r *OAuthBindingRepo) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&OAuthBindingRow{}, id).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete oauth binding", err)
	}
	return nil
}
