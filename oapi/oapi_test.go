package oapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"

	"github.com/g-studio-labs/gokit/lg"
)

func TestMain(m *testing.M) {
	lg.Setup(lg.Options{Out: io.Discard, Format: "json"})
	os.Exit(m.Run())
}

const spec = `
openapi: 3.0.3
info: {title: t, version: "1"}
components:
  securitySchemes:
    session: {type: apiKey, in: cookie, name: sid}
paths:
  /login:
    post:
      operationId: login
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [user, password]
              additionalProperties: false
              properties:
                user: {type: string, minLength: 3}
                password: {type: string, minLength: 8}
      responses:
        "200": {description: ok}
  /admin:
    get:
      operationId: admin
      security: [{session: []}]
      parameters:
        - {name: page, in: query, schema: {type: integer, minimum: 1}}
      responses:
        "200": {description: ok}
`

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	// kin-openapi needs a router; the standard mux works with the routes
	// registered through the middleware (same as oapi-codegen does).
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	v := Validator(doc, ValidatorOptions{Auth: func(_ context.Context, in *openapi3filter.AuthenticationInput) error {
		if c, err := in.RequestValidationInput.Request.Cookie("sid"); err == nil && c.Value == "good" {
			return nil
		}
		return context.Canceled
	}})
	mux.Handle("POST /login", v(ok))
	mux.Handle("GET /admin", v(ok))
	return mux
}

func TestValidatorFieldsWithoutLeakingValues(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/login", strings.NewReader(`{"user":"ab","password":"SECRET7","extra":1}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, r)
	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	if rec.Code != 400 || m["error"] != "validation" {
		t.Fatalf("%d %v", rec.Code, m)
	}
	fields := m["fields"].([]any)
	if len(fields) < 3 {
		t.Errorf("all fields at once: %v", fields)
	}
	if strings.Contains(rec.Body.String(), "SECRET7") {
		t.Error("the received value is not returned")
	}
	names := map[string]bool{}
	for _, f := range fields {
		names[f.(map[string]any)["field"].(string)] = true
	}
	if !names["user"] || !names["password"] || !names["extra"] {
		t.Errorf("fields: %v", names)
	}
}

func TestValidatorSecurityAndParameters(t *testing.T) {
	h := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/admin", nil))
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("no cookie: %d %s", rec.Code, rec.Body.String())
	}
	r := httptest.NewRequest("GET", "/admin?page=0", nil)
	r.AddCookie(&http.Cookie{Name: "sid", Value: "good"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"field":"page"`) {
		t.Errorf("parameter: %d %s", rec.Code, rec.Body.String())
	}
	r = httptest.NewRequest("GET", "/admin?page=2", nil)
	r.AddCookie(&http.Cookie{Name: "sid", Value: "good"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Errorf("valid: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRequestErrorHandlerNamesTheParameter(t *testing.T) {
	rec := httptest.NewRecorder()
	RequestErrorHandler(rec, httptest.NewRequest("GET", "/x", nil), errors.New(`Invalid format for parameter id: error binding string parameter: strconv.ParseInt: parsing "abc": invalid syntax`))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"field":"id"`) || strings.Contains(rec.Body.String(), "strconv") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}
