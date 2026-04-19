// Package queue 封装 asynq 客户端，用于投递异步任务。
//
// 任务类型在 queue/tasks.go 中定义。Worker 端在 cmd/worker 实现。
package queue

import (
	"github.com/hibiken/asynq"
)

// Config asynq 配置。对齐 config.RedisConfig。
type Config struct {
	Addr     string
	Password string
	DB       int
}

// NewClient 构造 asynq 客户端。
func NewClient(cfg Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

// RedisOpt 为 server 端（cmd/worker）提供相同的 Redis 选项。
func RedisOpt(cfg Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
}
