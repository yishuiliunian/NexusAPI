package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/user"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// UserRepo GORM 实现 user.Repository。
//
// TwoFASecret 落库前经 Cipher 加密。
type UserRepo struct {
	db     *gorm.DB
	cipher *cryptoutil.Cipher
}

// NewUserRepo 构造。cipher 不可为 nil（传 crypto.Noop() 禁用加密）。
func NewUserRepo(db *gorm.DB, cipher *cryptoutil.Cipher) *UserRepo {
	return &UserRepo{db: db, cipher: cipher}
}

func (r *UserRepo) toRow(u *user.User) (*UserRow, error) {
	enc, err := r.cipher.EncryptString(u.TwoFASecret)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "encrypt 2fa secret", err)
	}
	return &UserRow{
		ID:           u.ID,
		Email:        u.Email,
		EmailVerified: u.EmailVerified,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		GroupID:      u.GroupID,
		Quota:        u.Quota,
		UsedQuota:    u.UsedQuota,
		Status:       string(u.Status),
		TwoFASecret:  enc,
		QuotaAlertAt: u.QuotaAlertAt,
		QuotaAlertSentAt: u.QuotaAlertSentAt,
		RPMLimit:     u.RPMLimit,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}, nil
}

func (r *UserRepo) fromRow(row *UserRow) (*user.User, error) {
	plain, err := r.cipher.DecryptString(row.TwoFASecret)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "decrypt 2fa secret", err)
	}
	return &user.User{
		ID:           row.ID,
		Email:        row.Email,
		EmailVerified: row.EmailVerified,
		PasswordHash: row.PasswordHash,
		Role:         user.Role(row.Role),
		GroupID:      row.GroupID,
		Quota:        row.Quota,
		UsedQuota:    row.UsedQuota,
		Status:       user.Status(row.Status),
		TwoFASecret:  plain,
		QuotaAlertAt: row.QuotaAlertAt,
		QuotaAlertSentAt: row.QuotaAlertSentAt,
		RPMLimit:     row.RPMLimit,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

// Create 创建用户。
func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	row, err := r.toRow(u)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create user", err)
	}
	u.ID = row.ID
	u.CreatedAt = row.CreatedAt
	u.UpdatedAt = row.UpdatedAt
	return nil
}

// GetByID 按主键查询。
func (r *UserRepo) GetByID(ctx context.Context, id uint64) (*user.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get user", err)
	}
	return r.fromRow(&row)
}

// GetByEmail 按邮箱查询。
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	var row UserRow
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get user by email", err)
	}
	return r.fromRow(&row)
}

// Update 更新用户字段。
func (r *UserRepo) Update(ctx context.Context, u *user.User) error {
	row, err := r.toRow(u)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Save(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update user", err)
	}
	return nil
}

// List 分页查询用户。
func (r *UserRepo) List(ctx context.Context, offset, limit int) ([]*user.User, int64, error) {
	var rows []UserRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&UserRow{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count users", err)
	}
	if err := tx.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list users", err)
	}
	out := make([]*user.User, 0, len(rows))
	for i := range rows {
		u, err := r.fromRow(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, nil
}

// SetQuota 直接设置配额（管理员用）。
func (r *UserRepo) SetQuota(ctx context.Context, id uint64, quota int64) error {
	if err := r.db.WithContext(ctx).Model(&UserRow{}).Where("id = ?", id).
		Update("quota", quota).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "set quota", err)
	}
	return nil
}

// ListLowQuotaForAlert 扫余额已触发阈值且未近期告警过的用户。
// cutoff 控制冷却期（例如 24h 前），避免重复骚扰。
func (r *UserRepo) ListLowQuotaForAlert(ctx context.Context, cutoff time.Time, limit int) ([]*user.User, error) {
	var rows []UserRow
	err := r.db.WithContext(ctx).
		Where("quota_alert_at > 0 AND quota <= quota_alert_at").
		Where("quota_alert_sent_at IS NULL OR quota_alert_sent_at < ?", cutoff).
		Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list low quota users", err)
	}
	out := make([]*user.User, 0, len(rows))
	for i := range rows {
		u, err := r.fromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// ---------- Group Repository ----------

// GroupRepo GORM 实现 user.GroupRepository。
type GroupRepo struct{ db *gorm.DB }

func NewGroupRepo(db *gorm.DB) *GroupRepo { return &GroupRepo{db: db} }

func toGroupRow(g *user.Group) *GroupRow {
	return &GroupRow{
		ID:              g.ID,
		Name:            g.Name,
		PriceMultiplier: g.PriceMultiplier,
		CreatedAt:       g.CreatedAt,
		UpdatedAt:       g.UpdatedAt,
	}
}

func fromGroupRow(r *GroupRow) *user.Group {
	return &user.Group{
		ID:              r.ID,
		Name:            r.Name,
		PriceMultiplier: r.PriceMultiplier,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

func (r *GroupRepo) Create(ctx context.Context, g *user.Group) error {
	row := toGroupRow(g)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create group", err)
	}
	g.ID = row.ID
	g.CreatedAt = row.CreatedAt
	g.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *GroupRepo) GetByID(ctx context.Context, id uint64) (*user.Group, error) {
	var row GroupRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get group", err)
	}
	return fromGroupRow(&row), nil
}

func (r *GroupRepo) GetByName(ctx context.Context, name string) (*user.Group, error) {
	var row GroupRow
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get group by name", err)
	}
	return fromGroupRow(&row), nil
}

func (r *GroupRepo) List(ctx context.Context) ([]*user.Group, error) {
	var rows []GroupRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list groups", err)
	}
	out := make([]*user.Group, 0, len(rows))
	for i := range rows {
		out = append(out, fromGroupRow(&rows[i]))
	}
	return out, nil
}

func (r *GroupRepo) Update(ctx context.Context, g *user.Group) error {
	if err := r.db.WithContext(ctx).Save(toGroupRow(g)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update group", err)
	}
	return nil
}

func (r *GroupRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先级联清理 channel_groups 关联
		if err := tx.Where("group_id = ?", id).Delete(&ChannelGroupRow{}).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "delete channel_groups", err)
		}
		// 把该组用户的 group_id 置 0（避免悬挂引用）
		if err := tx.Model(&UserRow{}).Where("group_id = ?", id).
			Update("group_id", 0).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "reset users.group_id", err)
		}
		if err := tx.Delete(&GroupRow{}, id).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "delete group", err)
		}
		return nil
	})
}

// ---------- Session Repository ----------

// SessionRepo GORM 实现 user.SessionRepository。
type SessionRepo struct{ db *gorm.DB }

func NewSessionRepo(db *gorm.DB) *SessionRepo { return &SessionRepo{db: db} }

func toSessionRow(s *user.Session) *SessionRow {
	return &SessionRow{
		ID:        s.ID,
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
		IP:        s.IP,
		UserAgent: s.UserAgent,
		CreatedAt: s.CreatedAt,
	}
}

func fromSessionRow(r *SessionRow) *user.Session {
	return &user.Session{
		ID:        r.ID,
		UserID:    r.UserID,
		ExpiresAt: r.ExpiresAt,
		IP:        r.IP,
		UserAgent: r.UserAgent,
		CreatedAt: r.CreatedAt,
	}
}

func (r *SessionRepo) Create(ctx context.Context, s *user.Session) error {
	if err := r.db.WithContext(ctx).Create(toSessionRow(s)).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "create session", err)
	}
	return nil
}

func (r *SessionRepo) Get(ctx context.Context, id string) (*user.Session, error) {
	var row SessionRow
	err := r.db.WithContext(ctx).Where("id = ? AND expires_at > ?", id, time.Now()).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get session", err)
	}
	return fromSessionRow(&row), nil
}

func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	if err := r.db.WithContext(ctx).Where("id = ?", id).Delete(&SessionRow{}).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete session", err)
	}
	return nil
}

func (r *SessionRepo) DeleteByUser(ctx context.Context, userID uint64) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&SessionRow{}).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "delete sessions by user", err)
	}
	return nil
}
