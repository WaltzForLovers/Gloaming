package apperr

import (
	"errors"

	"github.com/WaltzForLovers/Gloaming/internal/plan"
)

const (
	CodeValidation         = "VALIDATION"
	CodeUserExists         = "USER_EXISTS"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeTitleNotFound      = "TITLE_NOT_FOUND"
	CodeCatalogUnavailable = "CATALOG_UNAVAILABLE"
	CodeAlreadyInQueue     = "ALREADY_IN_QUEUE"
	CodeInvalidFreeTime    = "INVALID_FREE_TIME"
	CodeNotFound           = "NOT_FOUND"
	CodePortBusy           = "PORT_BUSY"
	CodeTooMany            = "TOO_MANY"
	CodeInternal           = "INTERNAL"
)

type Error struct {
	Code    string
	Message string
	cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.cause }

func (e *Error) WithCause(cause error) *Error {
	if e == nil {
		return nil
	}
	e.cause = cause
	return e
}

func Validation(message string) *Error {
	return &Error{Code: CodeValidation, Message: message}
}

func UserExists() *Error {
	return &Error{Code: CodeUserExists, Message: "That name is taken. Choose another."}
}

func InvalidCredentials() *Error {
	return &Error{Code: CodeInvalidCredentials, Message: "Wrong name or password."}
}

func Unauthorized() *Error {
	return &Error{Code: CodeUnauthorized, Message: "Sign in first."}
}

func TitleNotFound() *Error {
	return &Error{Code: CodeTitleNotFound, Message: "No title matched that. Check the spelling."}
}

func CatalogUnavailable() *Error {
	return &Error{Code: CodeCatalogUnavailable, Message: "The catalog is unavailable right now. Cards you already opened are still here."}
}

func AlreadyInQueue() *Error {
	return &Error{Code: CodeAlreadyInQueue, Message: "That title is already in the queue."}
}

func InvalidFreeTime() *Error {
	return &Error{Code: CodeInvalidFreeTime, Message: "Enter free time within seven days.", cause: plan.ErrInvalidFreeTime}
}

func TooMany() *Error {
	return &Error{Code: CodeTooMany, Message: "Too many attempts. Wait a minute."}
}

func NotFound() *Error {
	return &Error{Code: CodeNotFound, Message: "Queue item not found."}
}

func PortBusy() *Error {
	return &Error{Code: CodePortBusy, Message: "Port 8080 is in use. Close the program that is holding it."}
}

func Internal(cause error) *Error {
	return &Error{Code: CodeInternal, Message: "Something broke. Try again.", cause: cause}
}

func From(err error) *Error {
	if err == nil {
		return nil
	}
	var app *Error
	if errors.As(err, &app) {
		return app
	}
	if errors.Is(err, plan.ErrInvalidFreeTime) {
		return InvalidFreeTime()
	}
	return Internal(err)
}
