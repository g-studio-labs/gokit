package response

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

// MaxBodyBytes is the default JSON body limit (1 MiB). Uploads go through
// multipart with their own limit.
var MaxBodyBytes int64 = 1 << 20

// Validator is the shared validator. Register custom rules on it at startup:
//
//	response.Validator.RegisterValidation("date", func(fl validator.FieldLevel) bool { ... })
var Validator = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	// Errors are reported with the field's JSON name, which is the one the
	// client knows.
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		if name == "" {
			return f.Name
		}
		return name
	})
	return v
}

// Decode reads the JSON body into T and validates it with the `validate:"..."` rules.
// It is strict: it rejects unknown fields, bodies over MaxBodyBytes and
// malformed JSON, always as *Error (400 validation or 413) ready for Fail.
func Decode[T any](w http.ResponseWriter, r *http.Request) (*T, error) {
	return DecodeLimit[T](w, r, MaxBodyBytes)
}

// DecodeLimit is Decode with a different limit for that endpoint.
func DecodeLimit[T any](w http.ResponseWriter, r *http.Request, limit int64) (*T, error) {
	var req T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, Validation(FieldError{Field: "body", Message: "el cuerpo está vacío", Rule: "required"})
		}
		if e, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return nil, From(e)
		}
		return nil, Validation(FieldError{Field: "body", Message: decodeMessage(err), Rule: "json"})
	}
	if err := Validate(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

// Validate runs the `validate` rules and returns a 400 *Error with the
// fields, or nil.
func Validate(v any) error {
	return ValidateCtx(context.Background(), v)
}

// ValidateCtx is Validate with a context (for rules that look something up).
func ValidateCtx(ctx context.Context, v any) error {
	err := Validator.StructCtx(ctx, v)
	if err == nil {
		return nil
	}
	verrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return Validation(Field("body", err.Error()))
	}
	embedded := embeddedNames(reflect.TypeOf(v))
	fields := make([]FieldError, 0, len(verrs))
	for _, fe := range verrs {
		parts := strings.Split(fe.Namespace(), ".")[1:] // without the root struct
		// An embedded struct (CreateReq ⊃ Common) adds its name to the
		// namespace, but in JSON its fields are at the same level.
		kept := parts[:0]
		for _, p := range parts {
			if !embedded[p] {
				kept = append(kept, p)
			}
		}
		fields = append(fields, FieldError{Field: strings.Join(kept, "."), Message: RuleMessage(fe), Rule: fe.Tag()})
	}
	return Validation(fields...)
}

// RuleMessages maps rules to messages; %s is the rule's parameter.
// A project can add or change entries at startup.
var RuleMessages = map[string]string{
	"required": "es obligatorio",
	"email":    "debe ser un correo válido",
	"url":      "debe ser una URL válida",
	"uuid":     "debe ser un UUID válido",
	"min":      "debe tener al menos %s",
	"max":      "debe tener como máximo %s",
	"gte":      "debe ser al menos %s",
	"lte":      "debe ser como máximo %s",
	"gt":       "debe ser mayor que %s",
	"lt":       "debe ser menor que %s",
	"len":      "debe tener exactamente %s",
	"oneof":    "debe ser uno de: %s",
	"eqfield":  "debe coincidir con %s",
	"e164":     "debe ser un teléfono en formato internacional",
	"datetime": "debe ser una fecha con formato %s",
	"numeric":  "debe ser numérico",
	"alphanum": "solo letras y números",
}

// RuleMessage returns the message for a validation error.
func RuleMessage(fe validator.FieldError) string {
	if m, ok := RuleMessages[fe.Tag()]; ok {
		if strings.Contains(m, "%s") {
			return strings.Replace(m, "%s", fe.Param(), 1)
		}
		return m
	}
	return "no cumple la regla " + fe.Tag()
}

// embeddedNames collects the names of embedded structs, at any
// depth.
func embeddedNames(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	seen := map[reflect.Type]bool{} // stops recursive types
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map) {
			t = t.Elem()
		}
		if t == nil || t.Kind() != reflect.Struct || seen[t] {
			return
		}
		seen[t] = true
		for i := range t.NumField() {
			f := t.Field(i)
			if f.Anonymous {
				out[f.Name] = true
			}
			walk(f.Type)
		}
	}
	walk(t)
	return out
}

func decodeMessage(err error) string {
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return "JSON mal formado"
	}
	if typ, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return "el campo " + typ.Field + " debe ser " + typ.Type.String()
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return "campo desconocido " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	}
	return "cuerpo no válido"
}

// PathInt reads a path {name} as a positive integer.
func PathInt(r *http.Request, name string) (int, error) {
	n, err := strconv.Atoi(r.PathValue(name))
	if err != nil || n <= 0 {
		return 0, Validation(FieldError{Field: name, Message: "debe ser un entero positivo", Rule: "int"})
	}
	return n, nil
}

// PathInt64 is PathInt for int64 (large ids).
func PathInt64(r *http.Request, name string) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || n <= 0 {
		return 0, Validation(FieldError{Field: name, Message: "debe ser un entero positivo", Rule: "int"})
	}
	return n, nil
}

// PathString reads a non-empty path {name}.
func PathString(r *http.Request, name string) (string, error) {
	v := strings.TrimSpace(r.PathValue(name))
	if v == "" {
		return "", Validation(FieldError{Field: name, Message: "es obligatorio", Rule: "required"})
	}
	return v, nil
}

// QueryInt reads ?name= as an integer, with a default if missing. Invalid is an error.
func QueryInt(r *http.Request, name string, def int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, Validation(FieldError{Field: name, Message: "debe ser un entero", Rule: "int"})
	}
	return n, nil
}

// Pagination reads ?page= and ?pageSize= with defaults and a cap.
type Pagination struct {
	Page, PageSize int
}

// Offset is how many rows the query skips.
func (p Pagination) Offset() int { return (p.Page - 1) * p.PageSize }

// Meta builds the response Meta with the total.
func (p Pagination) Meta(total int64) *Meta { return NewMeta(p.Page, p.PageSize, total) }

// DefaultPageSize and MaxPageSize bound ?pageSize=.
var (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Paginate reads page and pageSize from the query (page ≥ 1, 1 ≤ pageSize ≤ MaxPageSize).
func Paginate(r *http.Request) (Pagination, error) {
	page, err := QueryInt(r, "page", 1)
	if err != nil {
		return Pagination{}, err
	}
	size, err := QueryInt(r, "pageSize", DefaultPageSize)
	if err != nil {
		return Pagination{}, err
	}
	if page < 1 {
		return Pagination{}, Validation(FieldError{Field: "page", Message: "debe ser al menos 1", Rule: "gte"})
	}
	if size < 1 || size > MaxPageSize {
		return Pagination{}, Validation(FieldError{Field: "pageSize", Message: "debe estar entre 1 y " + strconv.Itoa(MaxPageSize), Rule: "range"})
	}
	return Pagination{Page: page, PageSize: size}, nil
}

func ctxOf(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
