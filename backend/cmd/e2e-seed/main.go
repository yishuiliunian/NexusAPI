// e2e-seed 是 E2E 测试专用的数据库种子工具。
//
// 用途：
//   - 可选地清空指定表（--reset）
//   - 建立 admin + 普通 user（密码 bcrypt 哈希）
//   - 插入 mock channel + model prices
//   - 插入 1 条兑换码（code=E2E-REDEEM-CODE，面额 1_000_000 micro）
//   - 插入 1 条订阅套餐（code=e2e_monthly）
//
// 用法：
//
//	e2e-seed --sqlite /tmp/nexus-e2e.db \
//	         --admin-email admin@e2e.test --admin-password admin12345 \
//	         --user-email alice@e2e.test --user-password user12345 \
//	         --upstream-url http://127.0.0.1:18090 \
//	         --reset
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/yishuiliunian/nexusapi/backend/internal/infra/db"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
)

// E2E 固定常量，测试代码用。
const (
	SeedRedemptionCode   = "E2E-REDEEM-CODE"
	SeedRedemptionAmount = int64(1_000_000) // 1 元
	SeedPlanCode         = "e2e_monthly"
	SeedPlanQuota        = int64(5_000_000) // 5 元/周期
)

func main() {
	var (
		sqlitePath    = flag.String("sqlite", "/tmp/nexus-e2e.db", "SQLite 文件路径")
		adminEmail    = flag.String("admin-email", "admin@e2e.test", "admin 邮箱")
		adminPassword = flag.String("admin-password", "admin12345", "admin 密码")
		userEmail     = flag.String("user-email", "alice@e2e.test", "普通 user 邮箱")
		userPassword  = flag.String("user-password", "user12345", "普通 user 密码")
		upstreamURL   = flag.String("upstream-url", "http://127.0.0.1:18090", "上游 mock URL（channels.base_url）")
		reset         = flag.Bool("reset", false, "清空 user/session/apikey/channel/price 等表再 seed")
	)
	flag.Parse()

	gormDB, err := db.Open(db.Config{Driver: "sqlite", DSN: *sqlitePath})
	if err != nil {
		fail("open db: %v", err)
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		fail("migrate: %v", err)
	}

	ctx := context.Background()
	if *reset {
		if err := resetTables(ctx, gormDB); err != nil {
			fail("reset: %v", err)
		}
	}

	if err := seedAdmin(ctx, gormDB, *adminEmail, *adminPassword); err != nil {
		fail("seed admin: %v", err)
	}
	if err := seedUser(ctx, gormDB, *userEmail, *userPassword); err != nil {
		fail("seed user: %v", err)
	}
	if err := seedChannel(ctx, gormDB, *upstreamURL); err != nil {
		fail("seed channel: %v", err)
	}
	if err := seedPrices(ctx, gormDB); err != nil {
		fail("seed prices: %v", err)
	}
	if err := seedRedemption(ctx, gormDB); err != nil {
		fail("seed redemption: %v", err)
	}
	if err := seedPlan(ctx, gormDB); err != nil {
		fail("seed plan: %v", err)
	}
	fmt.Println("seed ok")
}

// resetTables 清掉业务表（保留 schema）。
// 顺序：先清依赖子表，再清父表。
func resetTables(ctx context.Context, g *gorm.DB) error {
	tables := []string{
		"audit_logs", "verify_tokens", "oauth_bindings",
		"ledgers", "usages",
		"tasks", "subscriptions", "subscription_plans",
		"payment_orders", "redemptions",
		"channel_groups", "channels", "model_prices",
		"api_keys", "sessions", "users", "groups",
	}
	for _, t := range tables {
		if err := g.WithContext(ctx).Exec("DELETE FROM " + t).Error; err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
	}
	return nil
}

func seedAdmin(ctx context.Context, g *gorm.DB, email, password string) error {
	return upsertUser(ctx, g, email, password, "admin")
}

func seedUser(ctx context.Context, g *gorm.DB, email, password string) error {
	return upsertUser(ctx, g, email, password, "user")
}

func upsertUser(ctx context.Context, g *gorm.DB, email, password, role string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	row := db.UserRow{
		Email:         email,
		PasswordHash:  string(hash),
		Role:          role,
		Status:        "active",
		EmailVerified: true,
		Quota:         100_000_000, // 100M micro，够跑测试
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	// upsert on email
	var existing db.UserRow
	err = g.WithContext(ctx).Where("email = ?", email).First(&existing).Error
	if err == nil {
		row.ID = existing.ID
		return g.WithContext(ctx).Save(&row).Error
	}
	return g.WithContext(ctx).Create(&row).Error
}

func seedChannel(ctx context.Context, g *gorm.DB, upstream string) error {
	// 使用 Noop cipher 写明文；server 端也必须配 NEXUSAPI_SECURITY_ENCRYPTION_KEY=""
	// 使配对解密为 noop。但默认配置下 cipher 可能非空；为避免耦合，直接使用 raw bytes
	// 存储明文 credentials。E2E 不考虑密文场景。
	ch := db.ChannelRow{
		Name:            "mock-upstream",
		Provider:        "claude",
		BaseURL:         upstream,
		Credentials:     "sk-mock-upstream-key",
		Models:          []string{"claude-3-5-sonnet", "gpt-4o-mini"},
		Weight:          100,
		PriceMultiplier: 1.0,
		Status:          "active",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	// 也要再插一个 openai provider 的（/v1/chat/completions 路径用）
	ch2 := db.ChannelRow{
		Name:            "mock-openai",
		Provider:        "openai",
		BaseURL:         upstream,
		Credentials:     "sk-mock-openai-key",
		Models:          []string{"gpt-4o-mini"},
		Weight:          100,
		PriceMultiplier: 1.0,
		Status:          "active",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := upsertChannel(ctx, g, &ch); err != nil {
		return err
	}
	return upsertChannel(ctx, g, &ch2)
}

func upsertChannel(ctx context.Context, g *gorm.DB, ch *db.ChannelRow) error {
	var exists db.ChannelRow
	err := g.WithContext(ctx).Where("name = ?", ch.Name).First(&exists).Error
	if err == nil {
		ch.ID = exists.ID
		return g.WithContext(ctx).Save(ch).Error
	}
	return g.WithContext(ctx).Create(ch).Error
}

func seedPrices(ctx context.Context, g *gorm.DB) error {
	prices := []db.ModelPriceRow{
		{Model: "gpt-4o-mini", Capability: "chat", InputPrice: 150, OutputPrice: 600, OutputMultiplier: 1, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{Model: "claude-3-5-sonnet", Capability: "chat", InputPrice: 3000, OutputPrice: 15000, OutputMultiplier: 1, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	for i := range prices {
		var ex db.ModelPriceRow
		err := g.WithContext(ctx).Where("model = ? AND capability = ?", prices[i].Model, prices[i].Capability).First(&ex).Error
		if err == nil {
			prices[i].ID = ex.ID
			if err := g.WithContext(ctx).Save(&prices[i]).Error; err != nil {
				return err
			}
			continue
		}
		if err := g.WithContext(ctx).Create(&prices[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedRedemption 预置一条兑换码 E2E-REDEEM-CODE，面额 1_000_000 micro，永不过期。
func seedRedemption(ctx context.Context, g *gorm.DB) error {
	row := db.RedemptionRow{
		Code:      SeedRedemptionCode,
		Amount:    SeedRedemptionAmount,
		Note:      "e2e seed",
		CreatedAt: time.Now(),
	}
	var ex db.RedemptionRow
	err := g.WithContext(ctx).Where("code = ?", row.Code).First(&ex).Error
	if err == nil {
		// 重置：清 UsedBy/UsedAt 以便重复兑换测试（多次 reset）
		ex.UsedBy = nil
		ex.UsedAt = nil
		ex.Amount = row.Amount
		return g.WithContext(ctx).Save(&ex).Error
	}
	return g.WithContext(ctx).Create(&row).Error
}

// seedPlan 预置一个订阅套餐 e2e_monthly。
func seedPlan(ctx context.Context, g *gorm.DB) error {
	row := db.PlanRow{
		Code:          SeedPlanCode,
		Name:          "E2E Monthly",
		PriceCents:    1000, // $10
		Currency:      "USD",
		PeriodDays:    30,
		IncludedQuota: SeedPlanQuota,
		GatewayRef:    "price_e2e_test",
		Enabled:       true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	var ex db.PlanRow
	err := g.WithContext(ctx).Where("code = ?", row.Code).First(&ex).Error
	if err == nil {
		row.ID = ex.ID
		return g.WithContext(ctx).Save(&row).Error
	}
	return g.WithContext(ctx).Create(&row).Error
}

// 使用 cipher 变量避免编译器裁剪 import。
var _ = cryptoutil.Noop

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
