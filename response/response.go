// Package response es el sobre JSON de la casa y todo lo que hace falta para
// responder con él: errores tipados que ya saben su status y su código,
// escritores, decodificación estricta del cuerpo y validación por campo.
//
// Toda respuesta tiene esta forma:
//
//	{
//	  "success": true,
//	  "message": "Creado",                  // opcional, para humanos
//	  "data": {...},                        // lo que pidió el cliente
//	  "error": "not-found",                 // solo en fallos: código estable
//	  "fields": [{"field":"email","message":"es obligatorio","rule":"required"}],
//	  "meta": {"page":1,"pageSize":20,"total":57,"totalPages":3},
//	  "requestId": "4f2c..."                // el X-Request-ID de la petición
//	}
//
// El cliente decide por `error` (código), nunca por el texto de `message`, que
// puede cambiar o traducirse. El status HTTP siempre es coherente con
// `success`: un fallo nunca viaja con 200.
package response

import "net/http"

// Response es el sobre. T es el tipo de data; usa any cuando no hay.
type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	// Data se omite solo cuando es nil: una lista vacía viaja como [].
	Data      T            `json:"data,omitzero"`
	Error     string       `json:"error,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
	Meta      *Meta        `json:"meta,omitempty"`
	RequestID string       `json:"requestId,omitempty"`
}

// FieldError es un error de validación de un campo concreto. Field usa el
// nombre JSON que conoce el cliente ("email", "address.zip").
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Rule    string `json:"rule,omitempty"` // required | email | min | max | oneof | ...
}

// Meta acompaña a los listados paginados.
type Meta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"totalPages"`
}

// NewMeta calcula TotalPages.
func NewMeta(page, pageSize int, total int64) *Meta {
	m := &Meta{Page: page, PageSize: pageSize, Total: total}
	if pageSize > 0 {
		m.TotalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return m
}

// Códigos estables. Son parte del contrato con los clientes: no se renombran.
// Un proyecto añade los suyos ("bot-paused", "outside-24h-window") con
// New(status, code, message) o e.WithCode(code).
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

// CodeFor devuelve el código estable por defecto de un status HTTP.
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

// Messages son los textos por defecto de cada código. Un proyecto puede
// cambiarlos al arrancar (p. ej. a inglés): response.Messages[CodeNotFound] = "Not found".
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
