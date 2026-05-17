package errors

import (
	"errors"
	"fmt"
)

// Error 定义了项目体系中使用的错误类型，用于描述错误的详细信息
type Error struct {
	// 基本信息
	Code    int    `json:"code"`              // HTTP 状态码
	Status  string `json:"status"`            // 错误状态，业务错误码
	Message string `json:"message,omitempty"` // 错误消息
	Details any    `json:"details,omitempty"` // 详细信息，可包含 ErrorInfo、调试信息等

	// 额外信息
	cause error // 原始错误信息，通常用于记录日志或调试
}

func New(code int, status string) *Error {
	return &Error{
		Code:   code,
		Status: status,
	}
}

func Newf(code int, status string, format string, args ...any) *Error {
	return New(code, status).WithMessage(fmt.Sprintf(format, args...))
}
func Errorf(code int, status string, format string, args ...any) error {
	return Newf(code, status, format, args...)
}

// Error 实现 error 接口中的 `Error` 方法.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("error: code = %d, status = %s, message = %s", e.Code, e.Status, e.Message)
}

func (e *Error) WithDetails(details any) *Error {
	e.Details = details
	return e
}

func (e *Error) WithCause(cause error) *Error {
	e.cause = cause
	return e
}

func (e *Error) WithMessage(message string) *Error {
	e.Message = message
	return e
}

func (e *Error) WithStatus(status string) *Error {
	e.Status = status
	return e
}

func (e *Error) WithCode(code int) *Error {
	e.Code = code
	return e
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is 用于比较两个错误是否相同，通常用于错误类型的判断
func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	se, ok := target.(*Error)
	if !ok || se == nil {
		return false
	}
	return e.Code == se.Code && e.Status == se.Status
}

func (e *Error) Clone() *Error {
	if e == nil {
		return nil
	}
	copied := *e
	return &copied
}

func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
