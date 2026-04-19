// Package db 提供数据库连接初始化与 GORM 仓储实现。
//
// 设计：
//   - domain 实体保持纯，不含 GORM 标签
//   - 本包内部定义 *Row 持久化镜像，添加 gorm tag
//   - Repository 方法在 domain ↔ Row 之间转换
//
// 支持 postgres 和 sqlite 两种驱动，由 config.Database.Driver 决定。
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config 数据库配置（对齐 config.DatabaseConfig）。
type Config struct {
	Driver string // postgres | sqlite
	DSN    string
}

// Open 打开数据库连接并自动迁移 schema（开发便利；生产应用 migrate CLI）。
func Open(cfg Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "sqlite", "":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return db, nil
}

// AutoMigrate 对所有已知表执行 GORM AutoMigrate。
// 仅推荐开发期使用；生产通过 migrate CLI。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(allModels()...)
}

// allModels 列出全部持久化模型。新增实体时在此注册。
func allModels() []any {
	return []any{
		&UserRow{},
		&GroupRow{},
		&SessionRow{},
		&ApiKeyRow{},
		&ChannelRow{},
		&ChannelGroupRow{},
		&ModelPriceRow{},
		&UsageRow{},
		&LedgerRow{},
		&TaskRow{},
		&RedemptionRow{},
		&PaymentOrderRow{},
		&SubscriptionRow{},
		&PlanRow{},
		&OAuthBindingRow{},
		&AuditLogRow{},
		&VerifyTokenRow{},
	}
}
