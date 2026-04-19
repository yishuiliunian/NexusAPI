// worker 是 NexusAPI 的异步任务执行器。
//
// 职责：
//   - asynq 消费队列任务（预留）
//   - 定时轮询 tasks 表中 pending/running 的任务，向上游 provider 查询并更新状态
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	billingapp "github.com/yishuiliunian/nexusapi/backend/internal/app/billing"
	alertapp "github.com/yishuiliunian/nexusapi/backend/internal/app/alert"
	subapp "github.com/yishuiliunian/nexusapi/backend/internal/app/subscription"
	taskapp "github.com/yishuiliunian/nexusapi/backend/internal/app/task"
	"github.com/yishuiliunian/nexusapi/backend/pkg/mailer"
	"github.com/yishuiliunian/nexusapi/backend/internal/config"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/db"
	"github.com/yishuiliunian/nexusapi/backend/internal/infra/provider"
	_ "github.com/yishuiliunian/nexusapi/backend/internal/infra/provider/providers"
	cryptoutil "github.com/yishuiliunian/nexusapi/backend/pkg/crypto"
	"github.com/yishuiliunian/nexusapi/backend/pkg/httpclient"
	"github.com/yishuiliunian/nexusapi/backend/pkg/logger"
)

const taskTypePing = "system:ping"

func main() {
	cfg, err := config.Load(os.Getenv("NEXUSAPI_CONFIG"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	log := logger.Must(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})
	defer func() { _ = log.Sync() }()

	// 数据库
	gormDB, err := db.Open(db.Config{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		log.Fatal("open db failed", zap.Error(err))
	}
	if err := db.AutoMigrate(gormDB); err != nil {
		log.Fatal("auto migrate failed", zap.Error(err))
	}

	// 加密 Cipher（worker 需正确解密渠道凭据）
	var cipher *cryptoutil.Cipher
	switch {
	case cfg.Security.CryptoSecret != "":
		cipher, err = cryptoutil.New(cryptoutil.DeriveKey(cfg.Security.CryptoSecret))
		if err != nil {
			log.Fatal("init cipher failed", zap.Error(err))
		}
	case cfg.App.Env == "prod":
		log.Fatal("security.crypto_secret 未配置，prod 环境拒绝启动")
	default:
		cipher = cryptoutil.Noop()
	}

	// 替换 task provider 的共享 HTTP client
	provider.SetHTTPClient(httpclient.New())

	taskRepo := db.NewTaskRepo(gormDB)
	channelRepo := db.NewChannelRepo(gormDB, cipher)
	priceRepo := db.NewModelPriceRepo(gormDB)
	userRepo := db.NewUserRepo(gormDB, cipher)
	groupRepo := db.NewGroupRepo(gormDB)
	planRepo := db.NewPlanRepo(gormDB)
	subRepo := db.NewSubscriptionRepo(gormDB)
	quotaDelta := db.NewQuotaDelta(gormDB)
	// worker 的 reservations 只读 Redis（不直接产生 Reserve），传 nil 退化 mem 即可
	billingEngine := billingapp.NewEngine(quotaDelta, userRepo, priceRepo, groupRepo, nil)
	taskSvc := taskapp.NewService(taskRepo, channelRepo, priceRepo, billingEngine, provider.Task)
	subSvc := subapp.NewService(planRepo, subRepo, billingEngine)

	// 余额预警
	mailClient := mailer.New(mailer.Config{
		Host: cfg.Mail.Host, Port: cfg.Mail.Port, Username: cfg.Mail.Username,
		Password: cfg.Mail.Password, From: cfg.Mail.From, UseTLS: cfg.Mail.UseTLS,
	})
	alertSvc := alertapp.NewService(userRepo, mailClient)

	// asynq server
	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
		Queues:      map[string]int{"critical": 6, "default": 3, "low": 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(taskTypePing, func(ctx context.Context, t *asynq.Task) error {
		log.Info("ping task", zap.ByteString("payload", t.Payload()))
		return nil
	})

	// TaskPoller：每 5 秒扫一次 pending/running 任务并轮询上游
	pollCtx, pollCancel := context.WithCancel(context.Background())
	go runTaskPoller(pollCtx, log, taskSvc)
	// SubscriptionRefresher：每 10 分钟扫到期订阅做兜底发放（防 Stripe webhook 丢失）
	go runSubRefresher(pollCtx, log, subSvc)
	// QuotaAlertChecker：每 30 分钟扫一次余额预警
	go runAlertChecker(pollCtx, log, alertSvc)

	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatal("asynq server stopped", zap.Error(err))
		}
	}()
	log.Info("nexusapi worker running", zap.String("redis", cfg.Redis.Addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutdown signal received, stopping worker")
	pollCancel()
	srv.Shutdown()
	log.Info("worker exited")
}

// runTaskPoller 定时轮询 pending/running 任务，查询上游并更新数据库。
func runTaskPoller(ctx context.Context, log *zap.Logger, svc *taskapp.Service) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := svc.Tasks.ListPending(ctx, 50)
			if err != nil {
				log.Warn("task poller list failed", zap.Error(err))
				continue
			}
			if len(items) == 0 {
				continue
			}
			for _, t := range items {
				if err := svc.Poll(ctx, t); err != nil {
					log.Warn("task poll failed", zap.String("task_id", t.ID), zap.Error(err))
				}
			}
		}
	}
}

// runSubRefresher 每 10 分钟扫一次到期订阅，兜底发放配额 + 前滚 NextResetAt。
// Stripe webhook 正常时这里不会匹配到 due 记录；主要防 webhook 丢失、本地
// 发放的订阅（无 GatewayRef）续期。
func runSubRefresher(ctx context.Context, log *zap.Logger, svc *subapp.Service) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			n, err := svc.ApplyDueSubscriptions(ctx, now, 100)
			if err != nil {
				log.Warn("sub refresher failed", zap.Error(err))
				continue
			}
			if n > 0 {
				log.Info("subscription refresh applied", zap.Int("count", n))
			}
		}
	}
}

// runAlertChecker 每 30 分钟扫低余额用户发送邮件预警。
func runAlertChecker(ctx context.Context, log *zap.Logger, svc *alertapp.Service) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := svc.CheckAndNotify(ctx, 500)
			if err != nil {
				log.Warn("alert checker failed", zap.Error(err))
				continue
			}
			if n > 0 {
				log.Info("quota alert sent", zap.Int("count", n))
			}
		}
	}
}
