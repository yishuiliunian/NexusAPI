// Package errors 定义 NexusAPI 全局错误码与封装。
//
// 错误体系分三层：
//  1. sentinel 错误（包级变量，如 ErrNotFound）用于 errors.Is 比较
//  2. Error 结构体携带 code + message + 可选 wrap 的原因
//  3. HTTP handler 通过 Code() 映射到响应状态码
package errors

import (
	"errors"
	"fmt"
)

// Code 错误码字符串。使用 snake_case。
type Code string

const (
	CodeInternal         Code = "internal"
	CodeInvalidArgument  Code = "invalid_argument"
	CodeUnauthenticated  Code = "unauthenticated"
	CodePermissionDenied Code = "permission_denied"
	CodeNotFound         Code = "not_found"
	CodeAlreadyExists    Code = "already_exists"
	CodeRateLimited      Code = "rate_limited"
	CodeInsufficientQuota Code = "insufficient_quota"
	CodeUpstream         Code = "upstream_error"
	CodeTimeout          Code = "timeout"
)

// 常用 sentinel 错误。
var (
	ErrNotFound          = &Error{C: CodeNotFound, M: "资源不存在"}
	ErrAlreadyExists     = &Error{C: CodeAlreadyExists, M: "资源已存在"}
	ErrUnauthenticated   = &Error{C: CodeUnauthenticated, M: "未认证"}
	ErrPermissionDenied  = &Error{C: CodePermissionDenied, M: "权限不足"}
	ErrInsufficientQuota = &Error{C: CodeInsufficientQuota, M: "余额不足"}
)

// Error 结构化错误。
type Error struct {
	C     Code   // 错误码
	M     string // 用户可见消息
	Cause error  // 底层错误（可选）
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.C, e.M, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.C, e.M)
}

// Unwrap 支持 errors.Is / errors.As。
func (e *Error) Unwrap() error { return e.Cause }

// Code 返回错误码。
func (e *Error) Code() Code { return e.C }

// Message 返回用户可见消息。
func (e *Error) Message() string { return e.M }

// New 构造一个新 Error。
func New(code Code, message string) *Error {
	return &Error{C: code, M: message}
}

// Wrap 包装底层错误。
func Wrap(code Code, message string, cause error) *Error {
	return &Error{C: code, M: message, Cause: cause}
}

// Is 判断是否为指定 Code 的错误。
func Is(err error, code Code) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.C == code
	}
	return false
}

// HTTPStatus 把 Code 映射为 HTTP 状态码。
func HTTPStatus(code Code) int {
	switch code {
	case CodeInvalidArgument:
		return 400
	case CodeUnauthenticated:
		return 401
	case CodePermissionDenied:
		return 403
	case CodeNotFound:
		return 404
	case CodeAlreadyExists:
		return 409
	case CodeRateLimited:
		return 429
	case CodeInsufficientQuota:
		return 402
	case CodeTimeout:
		return 504
	case CodeUpstream:
		return 502
	default:
		return 500
	}
}
