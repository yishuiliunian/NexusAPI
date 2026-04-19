// redemption.go —— Admin 激活码批量管理
package admin

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/infra/db"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httperr"
)

// batchReq 批量生成参数。
type batchReq struct {
	Name      string  `json:"name" binding:"required"`
	Prefix    string  `json:"prefix"`
	Count     int     `json:"count" binding:"required,min=1,max=10000"`
	Amount    int64   `json:"amount" binding:"required,min=1"`
	ExpiresAt *string `json:"expires_at"` // ISO date
}

type batchSummary struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Amount    int64  `json:"amount"`
	Count     int64  `json:"count"`
	Redeemed  int64  `json:"redeemed"`
	ExpiresAt string `json:"expires_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

type redemptionOut struct {
	ID        uint64  `json:"id"`
	Code      string  `json:"code"`
	Amount    int64   `json:"amount"`
	ExpiresAt *string `json:"expires_at"`
	UsedBy    *uint64 `json:"used_by"`
	UsedAt    *string `json:"used_at"`
}

// createBatch 生成一批激活码。
func (h *Handler) createBatch(c *gin.Context) {
	var req batchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.BadRequest(c, err.Error())
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", *req.ExpiresAt)
		if err != nil {
			httperr.BadRequest(c, "expires_at 日期格式应为 YYYY-MM-DD")
			return
		}
		t = t.Add(24 * time.Hour) // 到期日结束时刻
		expiresAt = &t
	}

	prefix := strings.TrimSpace(req.Prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "-") {
		prefix += "-"
	}

	rows := make([]db.RedemptionRow, 0, req.Count)
	now := time.Now()
	for i := 0; i < req.Count; i++ {
		code, err := generateCode(prefix)
		if err != nil {
			httperr.Abort(c, err)
			return
		}
		rows = append(rows, db.RedemptionRow{
			Code:      code,
			Amount:    req.Amount,
			BatchName: req.Name,
			Prefix:    prefix,
			ExpiresAt: expiresAt,
			CreatedAt: now,
		})
	}

	// 批量插入
	if err := h.db().Session(&gorm.Session{}).CreateInBatches(&rows, 500).Error; err != nil {
		httperr.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":  req.Name,
		"count": len(rows),
	})
}

// listBatches 按 batch_name 聚合，每组一条记录。
func (h *Handler) listBatches(c *gin.Context) {
	type groupRow struct {
		BatchName string
		Prefix    string
		Amount    int64
		Count     int64
		Redeemed  int64
		ExpiresAt *time.Time
		CreatedAt time.Time
	}
	var rows []groupRow
	err := h.db().
		Model(&db.RedemptionRow{}).
		Select(`batch_name, prefix, amount,
			COUNT(*) AS count,
			SUM(CASE WHEN used_by IS NOT NULL THEN 1 ELSE 0 END) AS redeemed,
			MAX(expires_at) AS expires_at,
			MAX(created_at) AS created_at`).
		Where("batch_name <> ''").
		Group("batch_name, prefix, amount").
		Order("created_at DESC").
		Limit(50).
		Scan(&rows).Error
	if err != nil {
		httperr.Abort(c, err)
		return
	}

	out := make([]batchSummary, 0, len(rows))
	for i, r := range rows {
		bs := batchSummary{
			ID:        uint64(i + 1),
			Name:      r.BatchName,
			Prefix:    r.Prefix,
			Amount:    r.Amount,
			Count:     r.Count,
			Redeemed:  r.Redeemed,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
		}
		if r.ExpiresAt != nil {
			bs.ExpiresAt = r.ExpiresAt.Format(time.RFC3339)
		}
		out = append(out, bs)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

// listRedemptions 根据 batch_name 或 q 列激活码。
func (h *Handler) listRedemptions(c *gin.Context) {
	batchName := c.Query("batch_name")
	batchID := c.Query("batch_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 || limit > 2000 {
		limit = 200
	}

	tx := h.db().Model(&db.RedemptionRow{})
	if batchName != "" {
		tx = tx.Where("batch_name = ?", batchName)
	}
	// 兼容 batch_id：按 listBatches 中的序号再映射回 batch_name
	// 为简化，前端可直接传 batch_name；此处仅保留兼容字段占位。
	_ = batchID

	var rows []db.RedemptionRow
	if err := tx.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		httperr.Abort(c, err)
		return
	}

	out := make([]redemptionOut, 0, len(rows))
	for _, r := range rows {
		item := redemptionOut{
			ID:     r.ID,
			Code:   r.Code,
			Amount: r.Amount,
			UsedBy: r.UsedBy,
		}
		if r.ExpiresAt != nil {
			s := r.ExpiresAt.Format(time.RFC3339)
			item.ExpiresAt = &s
		}
		if r.UsedAt != nil {
			s := r.UsedAt.Format(time.RFC3339)
			item.UsedAt = &s
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func generateCode(prefix string) (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// 12 char base32 无 padding
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)[:12]
	return fmt.Sprintf("%s%s", prefix, strings.ToUpper(suffix)), nil
}