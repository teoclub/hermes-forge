package errors

const (
	StatusUnauthorized        = "UNAUTHORIZED"
	StatusForbidden           = "FORBIDDEN"
	StatusNotFound            = "NOT_FOUND"
	StatusInternalServerError = "INTERNAL_SERVER_ERROR"
	StatusBadRequest          = "BAD_REQUEST"
)

func BadRequest(message string) *Error {
	return New(400, StatusBadRequest).WithMessage(message)
}

func NotFound(message string) *Error {
	return New(404, StatusNotFound).WithMessage(message)
}

func InternalServerError(message string) *Error {
	return New(500, StatusInternalServerError).WithMessage(message)
}

func Unauthorized(message string) *Error {
	return New(401, StatusUnauthorized).WithMessage(message)
}

func Forbidden(message string) *Error {
	return New(403, StatusForbidden).WithMessage(message)
}
