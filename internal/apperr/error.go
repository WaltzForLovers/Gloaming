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
	return &Error{Code: CodeUserExists, Message: "Такое имя уже есть. Выберите другое."}
}

func InvalidCredentials() *Error {
	return &Error{Code: CodeInvalidCredentials, Message: "Неверное имя или пароль."}
}

func Unauthorized() *Error {
	return &Error{Code: CodeUnauthorized, Message: "Сначала войдите."}
}

func TitleNotFound() *Error {
	return &Error{Code: CodeTitleNotFound, Message: "Такого названия не нашли. Проверьте написание."}
}

func CatalogUnavailable() *Error {
	return &Error{Code: CodeCatalogUnavailable, Message: "Каталог сейчас недоступен. Уже открытые карточки на месте."}
}

func AlreadyInQueue() *Error {
	return &Error{Code: CodeAlreadyInQueue, Message: "Этот тайтл уже в очереди."}
}

func InvalidFreeTime() *Error {
	return &Error{Code: CodeInvalidFreeTime, Message: "Укажите свободное время в пределах семи суток.", cause: plan.ErrInvalidFreeTime}
}

func NotFound() *Error {
	return &Error{Code: CodeNotFound, Message: "Пункт очереди не найден."}
}

func PortBusy() *Error {
	return &Error{Code: CodePortBusy, Message: "Порт 8080 занят. Закройте программу, которая его держит."}
}

func Internal(cause error) *Error {
	return &Error{Code: CodeInternal, Message: "Что-то сломалось. Попробуйте ещё раз.", cause: cause}
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
