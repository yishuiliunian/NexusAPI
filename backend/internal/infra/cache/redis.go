// Package cache 提供 Redis 客户端和常用缓存工具。
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config Redis 配置。
type Config struct {
	Addr     string
	Password string
	DB       int
}

// New 构造 Redis 客户端并 ping 验证。
func New(cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis at %s: %w", cfg.Addr, err)
	}
	return client, nil
}
