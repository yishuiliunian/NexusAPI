// testutil.go —— db 包测试的共享工具。
//
// 选 SQLite :memory: 而非 testcontainers-go 是因为：
//   - 项目本来就支持 sqlite 生产模式（config.driver=sqlite），schema 兼容
//   - 无外部依赖，能在任何环境（含 bazel sandbox）一键跑
//   - 速度快（每条 test < 20ms）
//
// 缺点：无法验证 Postgres-only 的行为（如 FOR UPDATE 锁语义、JSONB 索引）；
// 那部分由线上冒烟 + 生产 staging 负责。
package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestDB 返回一个已完成 AutoMigrate 的独立 in-memory sqlite DB。
// 每次调用返回新 DB，测试间零污染。
//
// 自动注册 t.Cleanup 关闭连接。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// shared cache + :memory: 让事务也能看到同一份 DB
	// 用不同 DSN 让每个测试独立实例。
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
