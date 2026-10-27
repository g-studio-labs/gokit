package response

import (
	"context"
	"errors"
	"net/http"
)

// Error is an error that already knows the status, code and message it reaches
// the client with. Services return these; any other error is answered as a
// 500 without exposing the detail (it stays in the log with its cause).
type Error struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
	Err     error // internal cause, for the log only
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// Is enables errors.Is(err, response.ErrNotFound): equal if they share a code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// New creates an error with its own status and code. An empty message uses
// Messages[code] or the status text.
func New(status int, code, message string) *Error {
	if code == "" {
		code = CodeFor(status)
	}
	if message == "" {
		if m, ok := Messages[code]; ok {
			message = m
		} else {
			message = http.StatusText(status)
		}
	}
	return &Error{Status: status, Code: code, Message: message}
}

// The constructors take the human-readable message; empty = the one from Messages.
func BadRequest(message string) *Error { return New(http.StatusBadRequest, CodeBadRequest, message) }
func Unauthorized(message string) *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, message)
}
func Forbidden(message string) *Error { return New(http.StatusForbidden, CodeForbidden, message) }
func NotFound(message string) *Error  { return New(http.StatusNotFound, CodeNotFound, message) }
func Conflict(message string) *Error  { return New(http.StatusConflict, CodeConflict, message) }
func Gone(message string) *Error      { return New(http.StatusGone, CodeGone, message) }
func Unprocessable(message string) *Error {
	return New(http.StatusUnprocessableEntity, CodeUnprocessable, message)
}
func TooManyRequests(message string) *Error {
	return New(http.StatusTooManyRequests, CodeRateLimited, message)
}
func Unavailable(message string) *Error {
	return New(http.StatusServiceUnavailable, CodeUnavailable, message)
}

// Internal wraps an unexpected failure. The client sees the generic message;
// the cause goes to the log.
func Internal(err error) *Error {
	e := New(http.StatusInternalServerError, CodeInternal, "")
	e.Err = err
	return e
}

// Validation is a 400 with per-field errors.
func Validation(fields ...FieldError) *Error {
	e := New(http.StatusBadRequest, CodeValidation, "")
	e.Fields = fields
	return e
}

// Field is a shortcut to build a FieldError inline.
func Field(field, message string) FieldError { return FieldError{Field: field, Message: message} }

// Sentinel errors, for errors.Is and to return without building anything.
var (
	ErrBadRequest   = BadRequest("")
	ErrUnauthorized = Unauthorized("")
	ErrForbidden    = Forbidden("")
	ErrNotFound     = NotFound("")
	ErrConflict     = Conflict("")
	ErrInternal     = Internal(nil)
)

// WithCode returns a copy with another stable code (the project's own).
func (e *Error) WithCode(code string) *Error {
	c := *e
	c.Code = code
	if m, ok := Messages[code]; ok && c.Message == Messages[e.Code] {
		c.Message = m
	}
	return &c
}

// WithMessage returns a copy with another human-readable message.
func (e *Error) WithMessage(message string) *Error {
	c := *e
	c.Message = message
	return &c
}

// WithFields returns a copy with per-field errors.
func (e *Error) WithFields(fields ...FieldError) *Error {
	c := *e
	c.Fields = append(append([]FieldError(nil), e.Fields...), fields...)
	return &c
}

// Wrap returns a copy with the internal cause (for the log).
func (e *Error) Wrap(err error) *Error {
	c := *e
	c.Err = err
	return &c
}

// From converts any error into an *Error to respond with:
//   - *Error is returned as is;
//   - context.DeadlineExceeded → 504 timeout; context.Canceled → 499 (client left);
//   - *http.MaxBytesError → 413;
//   - anything else is a 500 that does not leak the detail.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return New(http.StatusRequestEntityTooLarge, CodeTooLarge, "").Wrap(err)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return New(http.StatusGatewayTimeout, CodeTimeout, "").Wrap(err)
	case errors.Is(err, context.Canceled):
		return New(499, CodeTimeout, "Petición cancelada").Wrap(err)
	}
	return Internal(err)
}
