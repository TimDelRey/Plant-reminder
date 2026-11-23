// Package apperr — единый тип ошибок приложения, который HTTP-слой превращает в {"error": {...}}.
package apperr

import "net/http"

type Error struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Проверьте введённые данные", Fields: fields}
}

func NotFound() *Error {
	return New(http.StatusNotFound, "not_found", "Не найдено")
}

func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "unauthorized", "Требуется авторизация")
}

func Forbidden(msg string) *Error { return New(http.StatusForbidden, "forbidden", msg) }

func Conflict(msg string) *Error { return New(http.StatusConflict, "conflict", msg) }

func BadRequest(msg string) *Error { return New(http.StatusBadRequest, "bad_request", msg) }
