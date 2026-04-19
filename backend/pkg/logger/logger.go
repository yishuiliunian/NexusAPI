// Package logger 封装 zap 日志器，提供统一的全局日志入口。
//
// 日志级别和输出格式由 config.LogConfig 控制。
package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config 与 config.LogConfig 对齐。
type Config struct {
	Level  string // debug|info|warn|error
	Format string // json|console
}

// New 构造一个 zap.Logger。Format 为 json 时输出结构化日志（生产使用）。
func New(cfg Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("parse log level %q: %w", cfg.Level, err)
	}

	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(level)

	if cfg.Format == "console" {
		zcfg = zap.NewDevelopmentConfig()
		zcfg.Level = zap.NewAtomicLevelAt(level)
		zcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	return zcfg.Build(zap.AddCallerSkip(1))
}

// Must 构造 logger，失败则 panic（仅用于 main 启动阶段）。
func Must(cfg Config) *zap.Logger {
	l, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return l
}
