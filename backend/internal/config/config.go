// Package config 加载 NexusAPI 的全局配置。
//
// 通过 viper 支持：配置文件（yaml）+ 环境变量（NEXUSAPI_* 前缀）。
// 环境变量优先级高于文件。
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 聚合所有子模块配置。
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Log       LogConfig       `mapstructure:"log"`
	Security  SecurityConfig  `mapstructure:"security"`
	Auth      AuthConfig      `mapstructure:"auth"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Relay     RelayConfig     `mapstructure:"relay"`
	Payment   PaymentConfig   `mapstructure:"payment"`
	Mail      MailConfig      `mapstructure:"mail"`
	Site      SiteConfig      `mapstructure:"site"`
	OAuth     OAuthConfig     `mapstructure:"oauth"`
	Billing   BillingConfig   `mapstructure:"billing"`
}

// BillingConfig 计费相关配置。
type BillingConfig struct {
	// USDToCNY LiteLLM 价格同步时 USD → CNY 汇率。默认 7.2。
	USDToCNY float64 `mapstructure:"usd_to_cny"`
	// LiteLLMURL LiteLLM 价格清单 URL。留空走 DefaultLiteLLMURL。
	LiteLLMURL string `mapstructure:"litellm_url"`
	// StrictPricing 为 true 时，未在 model_prices 中配置的模型会被 402 拒绝，
	// 避免新模型上线或上游价格缺失导致的"免费通过"漏洞。默认 true。
	// 开发环境可显式设为 false 绕过检查。
	StrictPricing bool `mapstructure:"strict_pricing"`
	// AutoSyncOnStartup 启动时异步跑一次 LiteLLM 同步。默认 true。
	AutoSyncOnStartup bool `mapstructure:"auto_sync_on_startup"`
}

// OAuthConfig 第三方登录配置。Enabled 为 false 时不注册。
type OAuthConfig struct {
	// PostLoginURL 登录成功后跳转目标（通常是前端 /dashboard）。
	PostLoginURL string            `mapstructure:"post_login_url"`
	GitHub       OAuthProviderConfig `mapstructure:"github"`
	Google       OAuthProviderConfig `mapstructure:"google"`
}

// OAuthProviderConfig 单家 provider。
type OAuthProviderConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	// E2E / self-hosted 可覆盖默认上游 URL；生产留空走真实服务。
	AuthorizeURL string `mapstructure:"authorize_url"`
	TokenURL     string `mapstructure:"token_url"`
	APIBase      string `mapstructure:"api_base"`
}

// SiteConfig 站点元信息。
type SiteConfig struct {
	// BaseURL 用户端前端基地址；邮件链接、oauth 回调拼接用。
	BaseURL string `mapstructure:"base_url"`
}

// MailConfig SMTP 配置（可选）。Host/From 为空时视为未启用。
type MailConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
	UseTLS   bool   `mapstructure:"use_tls"`
}

// PaymentConfig 支付网关配置。
type PaymentConfig struct {
	// MicroPerCent 每 cent 兑多少 micro 配额（billing 单位换算）。
	// 默认 10_000，即 1 USD = 1_000_000 micro（一元等价）。
	MicroPerCent int64          `mapstructure:"micro_per_cent"`
	Stripe       StripeConfig   `mapstructure:"stripe"`
}

// StripeConfig Stripe 网关配置。
type StripeConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	SecretKey     string `mapstructure:"secret_key"`
	WebhookSecret string `mapstructure:"webhook_secret"`
	SuccessURL    string `mapstructure:"success_url"`
	CancelURL     string `mapstructure:"cancel_url"`
	ProductName   string `mapstructure:"product_name"`
	// APIBase E2E / self-hosted 可覆盖；生产留空走 https://api.stripe.com
	APIBase string `mapstructure:"api_base"`
}

// RateLimitConfig 限流默认值（ApiKey 单独配置 > 0 则覆盖）。
type RateLimitConfig struct {
	DefaultRPM int `mapstructure:"default_rpm"` // 0 = 不限
	DefaultTPM int `mapstructure:"default_tpm"` // 0 = 不限
}

// RelayConfig 中继行为配置。
type RelayConfig struct {
	// FailoverAttempts Runner 执行失败后尝试下一渠道的次数（含第一次）。
	// 1 = 不重试；建议 3。
	FailoverAttempts int `mapstructure:"failover_attempts"`
	// BreakerFailures 连续失败次数阈值，达到后该渠道进入短暂冷却。
	BreakerFailures int `mapstructure:"breaker_failures"`
	// BreakerCooldownSec 冷却秒数。
	BreakerCooldownSec int `mapstructure:"breaker_cooldown_sec"`
	// AffinityTTLSec 选择器亲和度缓存（同 user+model 在 TTL 内走同一渠道）。0 = 禁用。
	AffinityTTLSec int `mapstructure:"affinity_ttl_sec"`
}

// AuthConfig 认证相关。
type AuthConfig struct {
	// SessionTTLHours session cookie 有效期（小时）。0 = 使用默认 720（30 天）。
	SessionTTLHours int `mapstructure:"session_ttl_hours"`
}

// SecurityConfig 敏感字段加密相关配置。
type SecurityConfig struct {
	// CryptoSecret 对称加密密钥（32 字节）。
	// 留空时启用 no-op 加密（仅限开发环境），生产必须配置。
	CryptoSecret string `mapstructure:"crypto_secret"`
}

// AppConfig 应用元信息。
type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"` // dev | staging | prod
}

// ServerConfig HTTP 服务器配置。
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig 数据库配置。Driver 支持 postgres / sqlite。
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

// RedisConfig Redis 连接配置。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug|info|warn|error
	Format string `mapstructure:"format"` // json|console
}

// Load 从指定路径（可为空）加载配置。环境变量覆盖文件。
func Load(path string) (*Config, error) {
	v := viper.New()

	// 默认值
	v.SetDefault("app.name", "nexusapi")
	v.SetDefault("app.env", "dev")
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "nexusapi.db")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.db", 0)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("security.crypto_secret", "")
	v.SetDefault("auth.session_ttl_hours", 720)
	v.SetDefault("rate_limit.default_rpm", 0)
	v.SetDefault("rate_limit.default_tpm", 0)
	v.SetDefault("relay.failover_attempts", 3)
	v.SetDefault("relay.breaker_failures", 5)
	v.SetDefault("relay.breaker_cooldown_sec", 60)
	v.SetDefault("relay.affinity_ttl_sec", 300)
	v.SetDefault("payment.micro_per_cent", 10000)
	v.SetDefault("payment.stripe.enabled", false)
	v.SetDefault("payment.stripe.secret_key", "")
	v.SetDefault("payment.stripe.webhook_secret", "")
	v.SetDefault("payment.stripe.success_url", "")
	v.SetDefault("payment.stripe.cancel_url", "")
	v.SetDefault("payment.stripe.product_name", "")
	v.SetDefault("payment.stripe.api_base", "")
	v.SetDefault("mail.host", "")
	v.SetDefault("mail.port", 587)
	v.SetDefault("mail.username", "")
	v.SetDefault("mail.password", "")
	v.SetDefault("mail.from", "")
	v.SetDefault("mail.use_tls", false)
	v.SetDefault("site.base_url", "http://localhost:3000")
	v.SetDefault("oauth.post_login_url", "")
	v.SetDefault("oauth.github.enabled", false)
	v.SetDefault("oauth.github.client_id", "")
	v.SetDefault("oauth.github.client_secret", "")
	v.SetDefault("oauth.github.authorize_url", "")
	v.SetDefault("oauth.github.token_url", "")
	v.SetDefault("oauth.github.api_base", "")
	v.SetDefault("oauth.google.enabled", false)
	v.SetDefault("oauth.google.client_id", "")
	v.SetDefault("oauth.google.client_secret", "")
	v.SetDefault("oauth.google.authorize_url", "")
	v.SetDefault("oauth.google.token_url", "")
	v.SetDefault("oauth.google.api_base", "")
	v.SetDefault("billing.usd_to_cny", 7.2)
	v.SetDefault("billing.litellm_url", "")
	v.SetDefault("billing.strict_pricing", true)
	v.SetDefault("billing.auto_sync_on_startup", true)

	// 环境变量：NEXUSAPI_SERVER_PORT → server.port
	v.SetEnvPrefix("NEXUSAPI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 配置文件（可选）
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %q: %w", path, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
