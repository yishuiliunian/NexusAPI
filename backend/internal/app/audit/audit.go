// Package audit 提供审计日志的 HTTP 中间件与服务封装。
//
// 使用方式：对需要审计的 Handler（或路由 group）挂 Middleware，
// 它会记录 response 状态码 + 操作者 id + 方法路径 + body meta。
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	daudit "github.com/yishuiliunian/nexusapi/backend/internal/domain/audit"
	"github.com/yishuiliunian/nexusapi/backend/internal/interface/http/middleware"
)

// Recorder 审计记录器。
type Recorder struct {
	repo daudit.Repository
	log  *zap.Logger
}

// NewRecorder 构造。log 可为 nil（静默）。
func NewRecorder(repo daudit.Repository, log *zap.Logger) *Recorder {
	return &Recorder{repo: repo, log: log}
}

// Record 手动写一条审计（用于需定制 Action/Target 的 handler）。
func (r *Recorder) Record(ctx context.Context, l *daudit.Log) {
	if err := r.repo.Create(ctx, l); err != nil && r.log != nil {
		r.log.Warn("audit write failed", zap.Error(err))
	}
}

// Middleware 返回一个中间件：在所有修改性请求（非 GET）完成后自动记录。
// Action 由 method+path 拼成（例如 "POST /api/admin/channels"）；
// Body 作为 Meta 存入（<= 1KB 截断，过滤 password/secret 字段）。
func (r *Recorder) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		// 缓存 body 供事后用（gin.Handler 自身已消费）
		var rawBody []byte
		if c.Request.Body != nil {
			rawBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))
		}
		c.Next()

		if c.Writer.Status() >= 400 {
			return // 失败请求不入审计（减少噪音；调用方可手动 Record）
		}
		actor := middleware.CurrentUser(c)
		if actor == nil {
			return
		}
		meta := sanitizeMeta(rawBody, 1024)
		r.Record(c.Request.Context(), &daudit.Log{
			ActorID: actor.ID,
			Action:  c.Request.Method + " " + c.FullPath(),
			Target:  c.Request.URL.Path,
			Meta:    meta,
			IP:      c.ClientIP(),
		})
	}
}

// sanitizeMeta 做 JSON 字段过滤 + 截断。非 JSON 原样写（截断）。
func sanitizeMeta(raw []byte, max int) []byte {
	if len(raw) == 0 {
		return nil
	}
	trimmed := raw
	if len(trimmed) > max {
		trimmed = trimmed[:max]
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return trimmed
	}
	for _, key := range []string{"password", "new_password", "old_password", "secret", "client_secret", "credentials"} {
		if _, ok := obj[key]; ok {
			obj[key] = "***"
		}
	}
	b, _ := json.Marshal(obj)
	return b
}
