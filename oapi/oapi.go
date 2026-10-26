// Package oapi conecta el servidor generado por oapi-codegen (strict server
// sobre net/http) y el validador de kin-openapi con el sobre de la casa.
//
// Flujo contract-first: contracts/openapi.yaml es la fuente de verdad →
// `go generate ./...` produce el ServerInterface → se implementa
// StrictServerInterface → este paquete traduce lo que rechaza el validador y
// lo que devuelven los handlers al sobre {success:false, error, fields}.
//
//	spec, _ := gen.GetSpec()
//	validador := oapi.Validator(spec, oapi.ValidatorOptions{Auth: autenticar})
//	strict := gen.NewStrictHandlerWithOptions(srv, nil, oapi.StrictOptions())
//	gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
//	    BaseRouter: mux, Middlewares: []gen.MiddlewareFunc{validador},
//	    ErrorHandlerFunc: oapi.RequestErrorHandler,
//	})
//
// Un handler strict puede devolver response.NotFound("...") y
// ResponseErrorHandler lo responde con su status y código; no hace falta
// construir los tipos de respuesta de error generados.
package oapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/g-studio-labs/gokit/response"
)

// ValidatorOptions ajusta Validator.
type ValidatorOptions struct {
	// Auth valida los security schemes del contrato (cookie, bearer, apiKey).
	// Devolver error responde 401.
	Auth openapi3filter.AuthenticationFunc
	// ExcludeBody salta la validación del cuerpo (uploads grandes).
	ExcludeBody bool
}

// Validator es el middleware de kin-openapi para las rutas del contrato: valida
// parámetros, cuerpo y seguridad, y reporta todos los campos inválidos a la
// vez con el sobre de la casa.
func Validator(spec *openapi3.T, o ValidatorOptions) func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			MultiError:         true,
			AuthenticationFunc: o.Auth,
			ExcludeRequestBody: o.ExcludeBody,
		},
		ErrorHandlerWithOpts: ValidationErrorHandler,
	})
}

// StrictOptions son las opciones del strict handler con los manejadores de
// este paquete. Si el proyecto no usa Validator, el strict server igual
// responde con el sobre.
func StrictOptions() struct {
	RequestErrorHandlerFunc  func(w http.ResponseWriter, r *http.Request, err error)
	ResponseErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
} {
	return struct {
		RequestErrorHandlerFunc  func(w http.ResponseWriter, r *http.Request, err error)
		ResponseErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, err error)
	}{RequestErrorHandler, ResponseErrorHandler}
}

// ValidationErrorHandler traduce lo que rechazó kin-openapi (parámetros,
// cuerpo o seguridad) al sobre. Va en Validator como ErrorHandlerWithOpts.
func ValidationErrorHandler(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
	if _, ok := errors.AsType[*openapi3filter.SecurityRequirementsError](err); ok {
		response.Fail(w, r, response.Unauthorized("").Wrap(err))
		return
	}
	response.Fail(w, r, response.Validation(Fields(err)...))
}

// RequestErrorHandler cubre lo que el router generado no pudo parsear (raro:
// el validador corre antes). Sirve como ErrorHandlerFunc y
// RequestErrorHandlerFunc.
func RequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	field := "request"
	if m := reParam.FindStringSubmatch(err.Error()); m != nil {
		field = m[1]
	}
	response.Fail(w, r, response.Validation(response.FieldError{Field: field, Message: "formato no válido", Rule: "format"}).Wrap(err))
}

var reParam = regexp.MustCompile(`parameter "?([A-Za-z0-9_.-]+)"?`)

// ResponseErrorHandler es el ResponseErrorHandlerFunc del strict server: un
// handler devolvió error. Un *response.Error llega al cliente con su status y
// código; cualquier otro es 500 (registrado con causa, mensaje genérico).
func ResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	response.Fail(w, r, err)
}

var reMissing = regexp.MustCompile(`property "([^"]+)" is (?:missing|unsupported)`)

// Fields aplana los errores de kin-openapi a {field, message}. Vienen
// anidados (MultiError → RequestError → MultiError → SchemaError), así que se
// recorren por tipo exacto, nivel por nivel: errors.As saltaría al MultiError
// de adentro y perdería qué parámetro era.
//
// El mensaje es siempre SchemaError.Reason, que no incluye el valor recibido:
// err.Error() sí lo incluye, y en un login sería la contraseña.
func Fields(err error) []response.FieldError {
	switch e := err.(type) {
	case openapi3.MultiError:
		var out []response.FieldError
		for _, x := range e {
			out = append(out, Fields(x)...)
		}
		return out
	case *openapi3filter.RequestError:
		schemas := schemaErrors(e.Err)
		if e.Parameter != nil {
			msg := e.Reason
			if len(schemas) > 0 {
				msg = schemas[0].Reason
			}
			return []response.FieldError{{Field: e.Parameter.Name, Message: orGeneric(msg), Rule: "schema"}}
		}
		if len(schemas) == 0 {
			return []response.FieldError{{Field: "body", Message: orGeneric(e.Reason), Rule: "schema"}}
		}
		out := make([]response.FieldError, 0, len(schemas))
		for _, se := range schemas {
			out = append(out, fieldOf(se))
		}
		return out
	case *openapi3.SchemaError:
		return []response.FieldError{fieldOf(e)}
	default:
		return []response.FieldError{{Field: "request", Message: "petición no válida", Rule: "schema"}}
	}
}

func schemaErrors(err error) []*openapi3.SchemaError {
	switch e := err.(type) {
	case *openapi3.SchemaError:
		return []*openapi3.SchemaError{e}
	case openapi3.MultiError:
		var out []*openapi3.SchemaError
		for _, x := range e {
			out = append(out, schemaErrors(x)...)
		}
		return out
	}
	return nil
}

// fieldOf: la ruta JSON del valor ("password", "details.origin"). Si el error
// es de un objeto al que le falta o le sobra una propiedad, se nombra la
// propiedad.
func fieldOf(se *openapi3.SchemaError) response.FieldError {
	field := strings.Join(se.JSONPointer(), ".")
	if field == "" {
		if m := reMissing.FindStringSubmatch(se.Reason); m != nil {
			field = m[1]
		}
	}
	if field == "" {
		field = "body"
	}
	rule := se.SchemaField
	if rule == "" {
		rule = "schema"
	}
	return response.FieldError{Field: field, Message: orGeneric(se.Reason), Rule: rule}
}

func orGeneric(m string) string {
	if m == "" {
		return "valor no válido"
	}
	return m
}
