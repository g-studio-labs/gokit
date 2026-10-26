package response

import (
	"context"
	"errors"
	"net/http"
)

// Error es un error que ya sabe con qué status, código y mensaje llega al
// cliente. Los services devuelven estos; cualquier otro error se responde como
// 500 sin exponer el detalle (queda en el log con su causa).
type Error struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
	Err     error // causa interna, solo para el log
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// Is permite errors.Is(err, response.ErrNotFound): iguales si comparten código.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// New crea un error con status y código propios. message vacío usa Messages[code]
// o el texto del status.
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

// Los constructores aceptan el mensaje para humanos; vacío = el de Messages.
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

// Internal envuelve una falla inesperada. El cliente ve el mensaje genérico;
// la causa va al log.
func Internal(err error) *Error {
	e := New(http.StatusInternalServerError, CodeInternal, "")
	e.Err = err
	return e
}

// Validation es un 400 con errores por campo.
func Validation(fields ...FieldError) *Error {
	e := New(http.StatusBadRequest, CodeValidation, "")
	e.Fields = fields
	return e
}

// Field es un atajo para construir FieldError en línea.
func Field(field, message string) FieldError { return FieldError{Field: field, Message: message} }

// Errores centinela, para errors.Is y para devolver sin construir nada.
var (
	ErrBadRequest   = BadRequest("")
	ErrUnauthorized = Unauthorized("")
	ErrForbidden    = Forbidden("")
	ErrNotFound     = NotFound("")
	ErrConflict     = Conflict("")
	ErrInternal     = Internal(nil)
)

// WithCode devuelve una copia con otro código estable (propio del proyecto).
func (e *Error) WithCode(code string) *Error {
	c := *e
	c.Code = code
	if m, ok := Messages[code]; ok && c.Message == Messages[e.Code] {
		c.Message = m
	}
	return &c
}

// WithMessage devuelve una copia con otro mensaje para humanos.
func (e *Error) WithMessage(message string) *Error {
	c := *e
	c.Message = message
	return &c
}

// WithFields devuelve una copia con errores por campo.
func (e *Error) WithFields(fields ...FieldError) *Error {
	c := *e
	c.Fields = append(append([]FieldError(nil), e.Fields...), fields...)
	return &c
}

// Wrap devuelve una copia con la causa interna (para el log).
func (e *Error) Wrap(err error) *Error {
	c := *e
	c.Err = err
	return &c
}

// From convierte cualquier error en *Error para responderlo:
//   - *Error se devuelve tal cual;
//   - context.DeadlineExceeded → 504 timeout; context.Canceled → 499 (cliente se fue);
//   - *http.MaxBytesError → 413;
//   - lo demás es un 500 que no filtra el detalle.
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
