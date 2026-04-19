package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ApiKeyRepo GORM 实现 apikey.Repository。
type ApiKeyRepo struct{ db *gorm.DB }

// NewApiKeyRepo 构造。
func NewApiKeyRepo(db *gorm.DB) *ApiKeyRepo { return &ApiKeyRepo{db: db} }

func toApiKeyRow(k *apikey.ApiKey) *ApiKeyRow {
	return &ApiKeyRow{
		ID:             k.ID,
		UserID:         k.UserID,
		KeyPrefix:      k.KeyPrefix,
		KeySuffix:      k.KeySuffix,
		KeyHash:        k.KeyHash,
		Name:           k.Name,
		ModelWhitelist: jsonArray[string](k.ModelWhitelist),
		IPWhitelist:    jsonArray[string](k.IPWhitelist),
		QuotaLimit:     k.QuotaLimit,
		UsedQuota:      k.UsedQuota,
		RPMLimit:       k.RPMLimit,
		TPMLimit:       k.TPMLimit,
		ExpiresAt:      k.ExpiresAt,
		LastUsedAt:     k.LastUsedAt,
		Status:         string(k.Status),
		CreatedAt:      k.CreatedAt,
		UpdatedAt:      k.UpdatedAt,
	}
}

func fromApiKeyRow(r *ApiKeyRow) *apikey.ApiKey {
	return &apikey.ApiKey{
		ID:             r.ID,
		UserID:         r.UserID,
		KeyPrefix:      r.KeyPrefix,
		KeySuffix:      r.KeySuffix,
		KeyHash:        r.KeyHash,
		Name:           r.Name,
		ModelWhitelist: []string(r.ModelWhitelist),
		IPWhitelist:    []string(r.IPWhitelist),
		QuotaLimit:     r.QuotaLimit,
		UsedQuota:      r.UsedQuota,
		RPMLimit:       r.RPMLimit,
		TPMLimit:       r.TPMLimit,
		ExpiresAt:      r.ExpiresAt,
		LastUsedAt:     r.LastUsedAt,
		Status:         apikey.Status(r.Status),
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func (r *ApiKeyRepo) Create(ctx context.Context, k *apikey.ApiKey) error {
	row := toApiKeyRow(k)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create apikey", err)
	}
	k.ID = row.ID
	k.CreatedAt = row.CreatedAt
	k.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *ApiKeyRepo) GetByID(ctx context.Context, id uint64) (*apikey.ApiKey, error) {
	var row ApiKeyRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get apikey", err)
	}
	return fromApiKeyRow(&row), nil
}

func (r *ApiKeyRepo) GetByHash(ctx context.Context, hash string) (*apikey.ApiKey, error) {
	var row ApiKeyRow
	err := r.db.WithContext(ctx).Where("key_hash = ?", hash).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get apikey by hash", err)
	}
	return fromApiKeyRow(&row), nil
}

func (r *ApiKeyRepo) ListByUser(ctx context.Context, userID uint64) ([]*apikey.ApiKey, error) {
	var rows []ApiKeyRow
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list apikeys", err)
	}
	out := make([]*apikey.ApiKey, 0, len(rows))
	for i := range rows {
		out = append(out, fromApiKeyRow(&rows[i]))
	}
	return out, nil
}

func (r *ApiKeyRepo) Update(ctx context.Context, k *apikey.ApiKey) error {
	if err := r.db.WithContext(ctx).Save(toApiKeyRow(k)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update apikey", err)
	}
	return nil
}

func (r *ApiKeyRepo) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&ApiKeyRow{}, id).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete apikey", err)
	}
	return nil
}

func (r *ApiKeyRepo) TouchLastUsed(ctx context.Context, id uint64, t time.Time) error {
	if err := r.db.WithContext(ctx).Model(&ApiKeyRow{}).Where("id = ?", id).
		Update("last_used_at", t).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "touch apikey last_used_at", err)
	}
	return nil
}
