// server 是 NexusAPI 的 HTTP 入口。
//
// 组装顺序：config → logger → db（+AutoMigrate）→ 仓储 → 应用服务 → HTTP 路由 → listen。
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	keysapp "github.com/yishuiliunian/nexusapi/backend/internal/app/keys"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/auth"
	auditapp "github.com/yishuiliunian/nexusapi/backend/internal/app/audit"
	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	paymentapp "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/pricing"
	"github.com/yishuiliunian/nexusapi/backend/internal/app/redemption"
	relayapp "github.com/yishuiliunian/nexusapi/backend/internal/app/relay"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	taskapp "github.com/yishuiliunian/nexusapi/backend/internal/app/task"
	verifyapp "github.com/yishuiliunian/nexusapi/backend/internal/app/verify"
	oauthapp "github.com/yishuiliunian/nexusapi/backend/internal/app/oauth"
	"github.com/yishuiliunian/nexusapi/backend/internal/config"
	"github.com/yishuiliunian/nexusapi/backend/internal/domain/billing"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/cache"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/db"
	stripegw "github.com/yishuiliunian/nexusapi/backend/internal/infra/payment/stripe"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"

	// 空导入触发所有 provider 适配器注册
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/providers"

	ghoauth "github.com/yishuiliunian/nexusapi/backend/internal/infra/oauth/github"
	googleoauth "github.com/yishuiliunian/nexusapi/backend/internal/infra/oauth/google"

	httpiface "github.com/yishuiliunian/nexusapi/backend/internal/interface/http"
	mw "github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httpclient"
	"github.com/yishuiliunian/nexusapi/backend/pkg/logger"
	"github.com/yishuiliunian/nexusapi/backend/pkg/mailer"
	"github.com/yishuiliunian/nexusapi/backend/pkg/proxy"
	"github.com/yishuiliunian/nexusapi/backend/pkg/ratelimit"
)

func main() {
	cfg, err := config.Load(os.Getenv("NEXUSAPI_CONFIG"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	log := logger.Must(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})
	defer func() { _ = log.Sync() }()

	log.Info("nexusapi starting",
		zap.String("env", cfg.App.Env),
		zap.Int("port", cfg.Server.Port),
	)

	// ---------- 数据库 ----------
	gormDB, err := db.Open(db.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		log.Fatal("open db failed", zap.Error(err))
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		log.Fatal("auto migrate failed", zap.Error(err))
	}

	// 替换 task provider 用的共享 HTTP client，带连接池与超时
	provider.SetHTTPClient(httpclient.New())

	// ---------- 加密 Cipher ----------
	var cipher *cryptoutil.Cipher
	switch {
	case cfg.Security.CryptoSecret != "":
		cipher, err = cryptoutil.New(cryptoutil.DeriveKey(cfg.Security.CryptoSecret))
		if err != nil {
			log.Fatal("init cipher failed", zap.Error(err))
		}
	case cfg.App.Env == "prod":
		log.Fatal("security.crypto_secret 未配置，prod 环境拒绝启动（敏感字段会明文落库）")
	default:
		log.Warn("security.crypto_secret 未配置，使用 no-op cipher（仅限开发环境）")
		cipher = cryptoutil.Noop()
	}

	// ---------- 仓储 ----------
	userRepo := db.NewUserRepo(gormDB, cipher)
	groupRepo := db.NewGroupRepo(gormDB)
	sessionRepo := db.NewSessionRepo(gormDB)
	apiKeyRepo := db.NewApiKeyRepo(gormDB)
	channelRepo := db.NewChannelRepo(gormDB, cipher)
	priceRepo := db.NewModelPriceRepo(gormDB)
	usageRepo := db.NewUsageRepo(gormDB)
	ledgerRepo := db.NewLedgerRepo(gormDB)
	taskRepo := db.NewTaskRepo(gormDB)
	redemptionRepo := db.NewRedemptionRepo(gormDB)
	orderRepo := db.NewPaymentOrderRepo(gormDB)
	planRepo := db.NewPlanRepo(gormDB)
	subRepo := db.NewSubscriptionRepo(gormDB)
	verifyTokenRepo := db.NewVerifyTokenRepo(gormDB)
	oauthRepo := db.NewOAuthBindingRepo(gormDB)
	auditRepo := db.NewAuditLogRepo(gormDB)
	quotaDelta := db.NewQuotaDelta(gormDB)

	// ---------- Redis + reservations（可选）----------
	var reservations billing.ReservationStore
	var limiter *ratelimit.Limiter
	redisClient, err := cache.New(cache.Config{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	if err != nil {
		log.Warn("redis 未就绪，reservations 退化为单副本内存存储，限流退化为不限", zap.Error(err))
	} else {
		reservations = cache.NewReservationStore(redisClient, "nexus:reserve:")
		limiter = ratelimit.New(redisClient, "nexus:rl")
		log.Info("reservations/ratelimit 使用 Redis 存储（支持水平扩展）")
	}

	// ---------- 应用服务 ----------
	authSvc := auth.NewService(userRepo, sessionRepo, time.Duration(cfg.Auth.SessionTTLHours)*time.Hour)
	apiKeySvc := keysapp.NewService(apiKeyRepo)
	billingEngine := billingapp.NewEngine(quotaDelta, userRepo, priceRepo, groupRepo, reservations).
		WithStrictPricing(cfg.Billing.StrictPricing)

	// 熔断 + 亲和度：简化用内存实现（横向扩展可改 Redis 版）。
	breaker := relayapp.NewMemoryBreaker(relayapp.BreakerConfig{
		Threshold: cfg.Relay.BreakerFailures,
		Cooldown:  time.Duration(cfg.Relay.BreakerCooldownSec) * time.Second,
	})
	affinity := relayapp.NewMemoryAffinity(time.Duration(cfg.Relay.AffinityTTLSec) * time.Second)
	relaySelector := relayapp.NewSelector(channelRepo, provider.Sync).
		WithBreaker(breaker).
		WithAffinity(affinity)

	taskSvc := taskapp.NewService(taskRepo, channelRepo, priceRepo, billingEngine, provider.Task)
	redemptionSvc := redemption.NewService(redemptionRepo, billingEngine)
	subSvc := subapp.NewService(planRepo, subRepo, billingEngine)

	// 价格同步器：从 LiteLLM JSON 拉，统一 USD 单位。
	pricingSyncer := pricing.New(httpclient.New(), priceRepo, cfg.Billing.LiteLLMURL)

	// 启动时异步同步一次 LiteLLM，避免空价表导致 strictPricing 全拦。
	// 失败仅告警，不阻塞服务启动（网络问题不应影响服务可用性）。
	if cfg.Billing.AutoSyncOnStartup {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			result, err := pricingSyncer.Sync(ctx)
			if err != nil {
				log.Warn("启动时同步 LiteLLM 价格失败（服务继续运行，管理员可手动同步）", zap.Error(err))
				return
			}
			log.Info("启动时同步 LiteLLM 价格完成",
				zap.Int("inserted", result.Inserted),
				zap.Int("deleted", result.Deleted),
				zap.Int("skipped", result.Skipped),
			)
		}()
	}

	// ---------- 邮件 + 邮箱验证 / 密码重置 ----------
	mailClient := mailer.New(mailer.Config{
		Host:     cfg.Mail.Host,
		Port:     cfg.Mail.Port,
		Username: cfg.Mail.Username,
		Password: cfg.Mail.Password,
		From:     cfg.Mail.From,
		UseTLS:   cfg.Mail.UseTLS,
	})
	if mailClient.Enabled() {
		log.Info("SMTP 已启用", zap.String("host", cfg.Mail.Host))
	}
	verifySvc := verifyapp.NewService(verifyTokenRepo, userRepo, mailClient, cfg.Site.BaseURL)

	// ---------- OAuth（可选）----------
	var oauthSvc *oauthapp.Service
	{
		providers := []oauthapp.Provider{}
		if cfg.OAuth.GitHub.Enabled {
			providers = append(providers, ghoauth.New(ghoauth.Config{
				ClientID:     cfg.OAuth.GitHub.ClientID,
				ClientSecret: cfg.OAuth.GitHub.ClientSecret,
				HTTPClient:   httpclient.New(),
				AuthorizeURL: cfg.OAuth.GitHub.AuthorizeURL,
				TokenURL:     cfg.OAuth.GitHub.TokenURL,
				APIBase:      cfg.OAuth.GitHub.APIBase,
			}))
			log.Info("OAuth github 已启用")
		}
		if cfg.OAuth.Google.Enabled {
			providers = append(providers, googleoauth.New(googleoauth.Config{
				ClientID:     cfg.OAuth.Google.ClientID,
				ClientSecret: cfg.OAuth.Google.ClientSecret,
				HTTPClient:   httpclient.New(),
				AuthorizeURL: cfg.OAuth.Google.AuthorizeURL,
				TokenURL:     cfg.OAuth.Google.TokenURL,
				UserInfoURL:  cfg.OAuth.Google.APIBase, // google 的 APIBase 语义是 userinfo 端点
			}))
			log.Info("OAuth google 已启用")
		}
		if len(providers) > 0 {
			oauthSvc = oauthapp.NewService(oauthRepo, userRepo, providers...)
		}
	}
	oauthRedirect := func(provider string) string {
		return cfg.Site.BaseURL + "/api/auth/oauth/" + provider + "/callback"
	}

	// ---------- 支付（可选）----------
	var paymentSvc *paymentapp.Service
	{
		gws := []paymentapp.Gateway{}
		if cfg.Payment.Stripe.Enabled {
			gws = append(gws, stripegw.New(stripegw.Config{
				SecretKey:     cfg.Payment.Stripe.SecretKey,
				WebhookSecret: cfg.Payment.Stripe.WebhookSecret,
				SuccessURL:    cfg.Payment.Stripe.SuccessURL,
				CancelURL:     cfg.Payment.Stripe.CancelURL,
				ProductName:   cfg.Payment.Stripe.ProductName,
				APIBase:       cfg.Payment.Stripe.APIBase,
				HTTPClient:    httpclient.New(),
			}))
			log.Info("支付网关 stripe 已启用")
		}
		if len(gws) > 0 {
			paymentSvc = paymentapp.NewService(orderRepo, billingEngine, cfg.Payment.MicroPerCent, gws...)
			// 注入订阅处理器：invoice.paid webhook → SubService.HandleInvoicePaid
			paymentSvc.WithSubscriptions(subSvc, subSvc)
		}
	}

	// ---------- 构建路由 ----------
	if cfg.App.Env == "prod" {
		// gin release mode
		os.Setenv("GIN_MODE", "release")
	}
	engine := httpiface.NewRouter(httpiface.Deps{
		Infra: httpiface.InfraDeps{Logger: log},
		Auth: httpiface.AuthDeps{
			Auth:              authSvc,
			ApiKey:            apiKeySvc,
			Users:             userRepo,
			Verify:            verifySvc,
			OAuth:             oauthSvc,
			OAuthRedirectURI:  oauthRedirect,
			OAuthPostLoginURL: cfg.OAuth.PostLoginURL,
		},
		Relay: httpiface.RelayDeps{
			Selector: relaySelector,
			Billing:  billingEngine,
			Channels: channelRepo,
			RateLimit: mw.RateLimitConfig{
				Limiter:    limiter,
				DefaultRPM: cfg.RateLimit.DefaultRPM,
			},
		},
		Task: httpiface.TaskDeps{Service: taskSvc},
		Billing: httpiface.BillingDeps{
			Redemption: redemptionSvc,
			Payments:   paymentSvc,
			Orders:     orderRepo,
			Subs:       subSvc,
		},
		Admin: httpiface.AdminDeps{
			Groups:    groupRepo,
			Channels:  channelRepo,
			Prices:    priceRepo,
			Users:     userRepo,
			Audits:    auditRepo,
			Orders:    orderRepo,
			Tasks:     taskRepo,
			Subs:      subSvc,
			Providers: providerChecker{},
			QuotaAdj:  billingEngine,
			Pricing:   pricingSyncer,
			Recorder:  auditapp.NewRecorder(auditRepo, log),
			DB:        gormDB,
		},
		Query: httpiface.QueryDeps{
			Usages:  usageRepo,
			Ledgers: ledgerRepo,
		},
		Proxy: proxy.New(proxy.Config{
			Client: httpclient.New(),
		}),
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:           engine,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server listen failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server shutdown failed", zap.Error(err))
	}
	log.Info("server exited")
}
