// Package response is the house JSON envelope and everything needed to
// respond with it: typed errors that already know their status and code,
// writers, strict body decoding and per-field validation.
//
// Every response has this shape:
//
//	{
//	  "success": true,
//	  "message": "Creado",                  // optional, for humans
//	  "data": {...},                        // what the client asked for
//	  "error": "not-found",                 // failures only: stable code
//	  "fields": [{"field":"email","message":"es obligatorio","rule":"required"}],
//	  "meta": {"page":1,"pageSize":20,"total":57,"totalPages":3},
//	  "requestId": "4f2c..."                // the request's X-Request-ID
//	}
//
// The client decides by `error` (code), never by the `message` text, which
// may change or be translated. The HTTP status is always consistent with
// `success`: a failure never travels with 200.
package response

import "net/http"

// Response is the envelope. T is the data type; use any when there is none.
type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	// Data is omitted only when nil: an empty list travels as [].
	Data      T            `json:"data,omitzero"`
	Error     string       `json:"error,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
	Meta      *Meta        `json:"meta,omitempty"`
	RequestID string       `json:"requestId,omitempty"`
}

// FieldError is a validation error for a specific field. Field uses the
// JSON name the client knows ("email", "address.zip").
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"` // required | email | min | max | oneof | ...
}

// Meta accompanies paginated listings.
type Meta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

// NewMeta computes TotalPages.
func NewMeta(page, pageSize int, total int64) *Meta {
	m := &Meta{Page: page, PageSize: pageSize, Total: total}
	if pageSize > 0 {
		m.TotalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return m
}

// Stable codes. They are part of the contract with clients: never renamed.
// A project adds its own ("bot-paused", "outside-24h-window") with
// New(status, code, message) or e.WithCode(code).
const (
	CodeBadRequest    = "bad-request"
	CodeValidation    = "validation"
	CodeUnauthorized  = "unauthorized"
	CodeForbidden     = "forbidden"
	CodeNotFound      = "not-found"
	CodeConflict      = "conflict"
	CodeGone          = "gone"
	CodeTooLarge      = "payload-too-large"
	CodeUnprocessable = "unprocessable"
	CodeRateLimited   = "rate-limited"
	CodeInternal      = "internal"
	CodeUnavailable   = "unavailable"
	CodeTimeout       = "timeout"
)

// CodeFor returns the default stable code for an HTTP status.
func CodeFor(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusGone:
		return CodeGone
	case http.StatusRequestEntityTooLarge:
		return CodeTooLarge
	case http.StatusUnprocessableEntity:
		return CodeUnprocessable
	case http.StatusTooManyRequests:
		return CodeRateLimited
	case http.StatusServiceUnavailable:
		return CodeUnavailable
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return CodeTimeout
	default:
		if status >= 500 {
			return CodeInternal
		}
		return CodeBadRequest
	}
}

// Messages are the default texts for each code. A project can change them at
// startup (e.g. to English): response.Messages[CodeNotFound] = "Not found".
var Messages = map[string]string{
	CodeBadRequest:    "Petición no válida",
	CodeValidation:    "Datos no válidos",
	CodeUnauthorized:  "Sesión no válida",
	CodeForbidden:     "No tienes acceso a este recurso",
	CodeNotFound:      "Recurso no encontrado",
	CodeConflict:      "El recurso ya existe o está en uso",
	CodeGone:          "El recurso ya no está disponible",
	CodeTooLarge:      "El cuerpo de la petición es demasiado grande",
	CodeUnprocessable: "No se pudo procesar la petición",
	CodeRateLimited:   "Demasiadas peticiones, intenta más tarde",
	CodeInternal:      "Ocurrió un error, intenta de nuevo",
	CodeUnavailable:   "Servicio no disponible",
	CodeTimeout:       "La petición tardó demasiado",
}
