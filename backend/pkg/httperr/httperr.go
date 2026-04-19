// Package httperr 提供 HTTP 层统一的错误响应工具。
//
// 消除各 handler 包内分散的 respond/respondErr/errorJSON/abortErr。
//
// 使用：
//
//	httperr.Abort(c, err)              // 自动从 *derrors.Error 取状态码与消息
//	httperr.AbortStatus(c, 400, err)   // 指定状态码
package httperr

import (
	"github.com/gin-gonic/gin"

	derrors "github.com/yishuiliunian/nexusapi/backend/pkg/errors"
)

// RequestIDKey 与 middleware 包保持一致的 context key。
// 刻意在此包内复制字符串而非 import middleware，避免 pkg/errors 反向依赖 interface 层。
const RequestIDKey = "nexus.reqid"

// Abort 根据 err 自动选择状态码与响应体。
//   - 若 err 是 *derrors.Error，使用其 Code+Message+HTTPStatus 映射
//   - 否则按 500 包装
func Abort(c *gin.Context, err error) {
	de := toDErr(err)
	c.AbortWithStatusJSON(derrors.HTTPStatus(de.C), body(c, de))
}

// AbortStatus 显式指定状态码（主要用于校验失败的 400，status 优先级高于 de.C 映射）。
func AbortStatus(c *gin.Context, status int, err error) {
	de := toDErr(err)
	c.AbortWithStatusJSON(status, body(c, de))
}

// AbortCode 以 code+message 直接中断。
func AbortCode(c *gin.Context, status int, code derrors.Code, message string) {
	c.AbortWithStatusJSON(status, body(c, &derrors.Error{C: code, M: message}))
}

// BadRequest 简写：400 + CodeInvalidArgument。
func BadRequest(c *gin.Context, message string) {
	AbortCode(c, 400, derrors.CodeInvalidArgument, message)
}

// Unauthorized 简写：401。
func Unauthorized(c *gin.Context, message string) {
	if message == "" {
		message = "未认证"
	}
	AbortCode(c, 401, derrors.CodeUnauthenticated, message)
}

// Forbidden 简写：403。
func Forbidden(c *gin.Context, message string) {
	if message == "" {
		message = "权限不足"
	}
	AbortCode(c, 403, derrors.CodePermissionDenied, message)
}

// ------ private ------

func body(c *gin.Context, de *derrors.Error) gin.H {
	return gin.H{
		"code":       de.C,
		"message":    de.M,
		"request_id": c.GetString(RequestIDKey),
	}
}

func toDErr(err error) *derrors.Error {
	if de, ok := err.(*derrors.Error); ok {
		return de
	}
	if err == nil {
		return &derrors.Error{C: derrors.CodeInternal, M: "unknown"}
	}
	return &derrors.Error{C: derrors.CodeInternal, M: err.Error(), Cause: err}
}
