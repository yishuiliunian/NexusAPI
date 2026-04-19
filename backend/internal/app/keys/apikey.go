// Package keys 提供 ApiKey 创建、删除、查询的服务。
//
// 包名改为 keys 避免与 domain/apikey 重名。
// 导入方建议用 keysapp 别名或直接 keys。
//
// 密钥格式：sk-nexus-<32 字节 base64url>
// 存储：完整 key 的 SHA-256 hash；明文仅在创建时一次性返回给用户。
package keys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/apikey"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// prefix key 前缀。
const prefix = "sk-nexus-"

// Service ApiKey 服务。
type Service struct {
	repo apikey.Repository
}

// NewService 构造。
func NewService(repo apikey.Repository) *Service { return &Service{repo: repo} }

// CreateResult 创建 key 的返回值（明文只此一次）。
type CreateResult struct {
	Key    *apikey.ApiKey
	Secret string // 明文完整 key（仅返回一次）
}

// Create 为用户创建一把 key。
func (s *Service) Create(ctx context.Context, userID uint64, name string, modelWhitelist []string, quotaLimit int64, expiresAt *time.Time) (*CreateResult, error) {
	if name == "" {
		name = fmt.Sprintf("key-%d", time.Now().Unix())
	}
	secret, hash, suffix, err := generateKey()
	if err != nil {
		return nil, err
	}
	k := &apikey.ApiKey{
		UserID:         userID,
		KeyPrefix:      prefix + secret[:8], // 前缀展示用
		KeySuffix:      suffix,
		KeyHash:        hash,
		Name:           name,
		ModelWhitelist: modelWhitelist,
		QuotaLimit:     quotaLimit,
		ExpiresAt:      expiresAt,
		Status:         apikey.StatusActive,
	}
	if err := s.repo.Create(ctx, k); err != nil {
		return nil, err
	}
	return &CreateResult{Key: k, Secret: prefix + secret}, nil
}

// ListByUser 列出用户的所有 key。
func (s *Service) ListByUser(ctx context.Context, userID uint64) ([]*apikey.ApiKey, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Delete 删除 key（需属于同一用户）。
func (s *Service) Delete(ctx context.Context, userID, keyID uint64) error {
	k, err := s.repo.GetByID(ctx, keyID)
	if err != nil {
		return err
	}
	if k.UserID != userID {
		return derrors.ErrPermissionDenied
	}
	return s.repo.Delete(ctx, keyID)
}

// ResolveBearer 根据 Bearer token 反查 ApiKey 实体，并校验可用性。
func (s *Service) ResolveBearer(ctx context.Context, bearer string) (*apikey.ApiKey, error) {
	if len(bearer) < len(prefix)+16 {
		return nil, derrors.ErrUnauthenticated
	}
	hash := hashKey(bearer)
	k, err := s.repo.GetByHash(ctx, hash)
	if err != nil {
		return nil, derrors.ErrUnauthenticated
	}
	if !k.Active() {
		return nil, derrors.New(derrors.CodePermissionDenied, "密钥已停用或过期")
	}
	return k, nil
}

// TouchUsed 更新最后使用时间。
func (s *Service) TouchUsed(ctx context.Context, id uint64) error {
	return s.repo.TouchLastUsed(ctx, id, time.Now())
}

// generateKey 生成 (secret, hash, suffix4)。
// secret：不含 prefix 的随机部分；完整 bearer = prefix + secret
func generateKey() (string, string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("rand: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	bearer := prefix + secret
	hash := hashKey(bearer)
	suffix := secret[len(secret)-4:]
	return secret, hash, suffix, nil
}

// hashKey 计算 bearer 的 SHA-256 hex。
func hashKey(bearer string) string {
	sum := sha256.Sum256([]byte(bearer))
	return hex.EncodeToString(sum[:])
}
