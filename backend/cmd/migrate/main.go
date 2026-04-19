// migrate 是数据库迁移 CLI。
//
// 用法：
//
//	migrate up                    # 向上迁移到最新
//	migrate down                  # 回退一步
//	migrate goto <version>        # 迁移到指定版本
//	migrate version               # 查看当前版本
//
// 迁移 SQL 文件位于 deploy/migrations/。
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/spf13/cobra"

	"github.com/yishuiliunian/nexusapi/backend/internal/config"
)

const defaultMigrationsPath = "file://deploy/migrations"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	var migrationsPath string

	newMigrator := func() (*migrate.Migrate, error) {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("config load: %w", err)
		}
		dbURL, err := buildDatabaseURL(cfg.Database.Driver, cfg.Database.DSN)
		if err != nil {
			return nil, err
		}
		return migrate.New(migrationsPath, dbURL)
	}

	root := &cobra.Command{
		Use:   "migrate",
		Short: "NexusAPI 数据库迁移工具",
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", os.Getenv("NEXUSAPI_CONFIG"), "配置文件路径")
	root.PersistentFlags().StringVar(&migrationsPath, "source", defaultMigrationsPath, "迁移文件目录（file://... 格式）")

	root.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "向上迁移到最新版本",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newMigrator()
			if err != nil {
				return err
			}
			if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
				return err
			}
			return nil
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "down",
		Short: "回退一步",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newMigrator()
			if err != nil {
				return err
			}
			return m.Steps(-1)
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "打印当前迁移版本",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := newMigrator()
			if err != nil {
				return err
			}
			v, dirty, err := m.Version()
			if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
				return err
			}
			fmt.Printf("version=%d dirty=%v\n", v, dirty)
			return nil
		},
	})

	return root
}

// buildDatabaseURL 把 Driver+DSN 转为 golang-migrate 可识别的 URL。
func buildDatabaseURL(driver, dsn string) (string, error) {
	switch driver {
	case "postgres":
		// DSN 已是 postgres:// 或 host=... 形式；前者直用，后者需要转
		return dsn, nil
	case "sqlite":
		return "sqlite3://" + dsn, nil
	default:
		return "", fmt.Errorf("unsupported database driver %q", driver)
	}
}
