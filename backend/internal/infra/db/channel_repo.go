package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/domain/channel"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// ChannelRepo GORM 实现 channel.Repository。
//
// 凭据字段（Credentials）在落库前经 Cipher 加密。
// GroupIDs 存在独立的关联表 channel_groups，避免 JSON blob。
type ChannelRepo struct {
	db     *gorm.DB
	cipher *cryptoutil.Cipher
}

// NewChannelRepo 构造。cipher 不可为 nil（传 crypto.Noop() 禁用加密）。
func NewChannelRepo(db *gorm.DB, cipher *cryptoutil.Cipher) *ChannelRepo {
	return &ChannelRepo{db: db, cipher: cipher}
}

func (r *ChannelRepo) toRow(c *channel.Channel) (*ChannelRow, error) {
	enc, err := r.cipher.EncryptString(c.Credentials)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "encrypt credentials", err)
	}
	return &ChannelRow{
		ID:              c.ID,
		Name:            c.Name,
		Provider:        c.Provider,
		BaseURL:         c.BaseURL,
		Credentials:     enc,
		Models:          jsonArray[string](c.Models),
		Weight:          c.Weight,
		PriceMultiplier: c.PriceMultiplier,
		Status:          string(c.Status),
		TestedAt:        c.TestedAt,
		LatencyMs:       c.LatencyMs,
		Note:            c.Note,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}, nil
}

func (r *ChannelRepo) fromRow(row *ChannelRow) (*channel.Channel, error) {
	plain, err := r.cipher.DecryptString(row.Credentials)
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "decrypt credentials", err)
	}
	return &channel.Channel{
		ID:              row.ID,
		Name:            row.Name,
		Provider:        row.Provider,
		BaseURL:         row.BaseURL,
		Credentials:     plain,
		Models:          []string(row.Models),
		// GroupIDs 单独加载
		Weight:          row.Weight,
		PriceMultiplier: row.PriceMultiplier,
		Status:          channel.Status(row.Status),
		TestedAt:        row.TestedAt,
		LatencyMs:       row.LatencyMs,
		Note:            row.Note,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

// loadGroups 从 channel_groups 关联表读取指定渠道的分组集合。
func (r *ChannelRepo) loadGroups(ctx context.Context, channelID uint64) ([]uint64, error) {
	var rows []ChannelGroupRow
	if err := r.db.WithContext(ctx).Where("channel_id = ?", channelID).Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "load channel groups", err)
	}
	out := make([]uint64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.GroupID)
	}
	return out, nil
}

// syncGroups 同步 channel_groups 关联记录（全量替换语义）。
// 约定：在 Create/Update 的同一事务内调用。
func (r *ChannelRepo) syncGroups(tx *gorm.DB, channelID uint64, groupIDs []uint64) error {
	if err := tx.Where("channel_id = ?", channelID).Delete(&ChannelGroupRow{}).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "clear channel groups", err)
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]ChannelGroupRow, 0, len(groupIDs))
	now := time.Now()
	for _, gid := range groupIDs {
		rows = append(rows, ChannelGroupRow{ChannelID: channelID, GroupID: gid, CreatedAt: now})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "insert channel groups", err)
	}
	return nil
}

func (r *ChannelRepo) Create(ctx context.Context, c *channel.Channel) error {
	row, err := r.toRow(c)
	if err != nil {
		return err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "create channel", err)
		}
		return r.syncGroups(tx, row.ID, c.GroupIDs)
	})
	if err != nil {
		return err
	}
	c.ID = row.ID
	c.CreatedAt = row.CreatedAt
	c.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *ChannelRepo) GetByID(ctx context.Context, id uint64) (*channel.Channel, error) {
	var row ChannelRow
	err := r.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, derrors.ErrNotFound
	}
	if err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "get channel", err)
	}
	ch, err := r.fromRow(&row)
	if err != nil {
		return nil, err
	}
	ch.GroupIDs, err = r.loadGroups(ctx, id)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (r *ChannelRepo) List(ctx context.Context, offset, limit int) ([]*channel.Channel, int64, error) {
	var rows []ChannelRow
	var total int64
	tx := r.db.WithContext(ctx).Model(&ChannelRow{})
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "count channels", err)
	}
	if err := tx.Order("id").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "list channels", err)
	}
	return r.hydrate(ctx, rows, total)
}

func (r *ChannelRepo) ListActive(ctx context.Context) ([]*channel.Channel, error) {
	var rows []ChannelRow
	if err := r.db.WithContext(ctx).Where("status = ?", string(channel.StatusActive)).
		Order("weight DESC, id").Find(&rows).Error; err != nil {
		return nil, derrors.Wrap(derrors.CodeInternal, "list active channels", err)
	}
	out, _, err := r.hydrate(ctx, rows, 0)
	return out, err
}

// hydrate 对一批渠道批量补齐 GroupIDs。一次查询避免 N+1。
func (r *ChannelRepo) hydrate(ctx context.Context, rows []ChannelRow, total int64) ([]*channel.Channel, int64, error) {
	out := make([]*channel.Channel, 0, len(rows))
	if len(rows) == 0 {
		return out, total, nil
	}
	ids := make([]uint64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	var links []ChannelGroupRow
	if err := r.db.WithContext(ctx).Where("channel_id IN ?", ids).Find(&links).Error; err != nil {
		return nil, 0, derrors.Wrap(derrors.CodeInternal, "load channel groups", err)
	}
	groupsByChannel := map[uint64][]uint64{}
	for _, l := range links {
		groupsByChannel[l.ChannelID] = append(groupsByChannel[l.ChannelID], l.GroupID)
	}
	for i := range rows {
		ch, err := r.fromRow(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		ch.GroupIDs = groupsByChannel[ch.ID]
		out = append(out, ch)
	}
	return out, total, nil
}

func (r *ChannelRepo) Update(ctx context.Context, c *channel.Channel) error {
	row, err := r.toRow(c)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(row).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "update channel", err)
		}
		return r.syncGroups(tx, c.ID, c.GroupIDs)
	})
}

func (r *ChannelRepo) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", id).Delete(&ChannelGroupRow{}).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "delete channel groups", err)
		}
		if err := tx.Delete(&ChannelRow{}, id).Error; err != nil {
			return derrors.Wrap(derrors.CodeInternal, "delete channel", err)
		}
		return nil
	})
}

func (r *ChannelRepo) UpdateHealth(ctx context.Context, id uint64, latencyMs int, testedAt time.Time) error {
	if err := r.db.WithContext(ctx).Model(&ChannelRow{}).Where("id = ?", id).
		Updates(map[string]any{
			"latency_ms": latencyMs,
			"tested_at":  testedAt,
		}).Error; err != nil {
		return derrors.Wrap(derrors.CodeInternal, "update channel health", err)
	}
	return nil
}
