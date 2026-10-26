package response

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

	"github.com/g-studio-labs/gokit/lg"
)

func TestMain(m *testing.M) {
	lg.Setup(lg.Options{Out: io.Discard, Format: "json"})
	os.Exit(m.Run())
}

type item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func body(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("no es JSON: %v\n%s", err, rec.Body.String())
	}
	return m
}

func TestOKConRequestID(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	r = r.WithContext(lg.WithRequestID(r.Context(), "req-1"))
	OK(rec, r, item{1, "a"}, "listo")
	m := body(t, rec)
	if rec.Code != 200 || m["success"] != true || m["requestId"] != "req-1" || m["message"] != "listo" {
		t.Errorf("%d %v", rec.Code, m)
	}
	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Error("content-type")
	}
	if _, has := m["error"]; has {
		t.Error("error no debe aparecer en éxito")
	}
}

func TestPagedListaVaciaEsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	Paged[item](rec, httptest.NewRequest("GET", "/x", nil), nil, NewMeta(1, 20, 0))
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Errorf("lista vacía debe ser []: %s", rec.Body.String())
	}
	m := body(t, rec)
	meta := m["meta"].(map[string]any)
	if meta["totalPages"] != float64(0) || meta["pageSize"] != float64(20) {
		t.Errorf("meta: %v", meta)
	}
	if NewMeta(1, 20, 57).TotalPages != 3 {
		t.Error("TotalPages")
	}
}

func TestFailMapeaErrores(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, 404, "not-found"},
		{NotFound("no está").WithCode("promo-not-found"), 404, "promo-not-found"},
		{errors.New("sql: boom"), 500, "internal"},
		{Internal(errors.New("x")), 500, "internal"},
		{context.DeadlineExceeded, 504, "timeout"},
		{Validation(Field("email", "es obligatorio")), 400, "validation"},
		{&http.MaxBytesError{Limit: 10}, 413, "payload-too-large"},
		{TooManyRequests(""), 429, "rate-limited"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		Fail(rec, httptest.NewRequest("POST", "/x", nil), c.err)
		m := body(t, rec)
		if rec.Code != c.status || m["error"] != c.code || m["success"] != false {
			t.Errorf("%v → %d %v, quería %d %s", c.err, rec.Code, m, c.status, c.code)
		}
		if c.status >= 500 && strings.Contains(rec.Body.String(), "boom") {
			t.Error("un 500 no filtra la causa")
		}
	}
}

func TestErrorsIsPorCodigo(t *testing.T) {
	err := NotFound("promo").Wrap(errors.New("gorm: not found"))
	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is por código")
	}
	var e *Error
	if !errors.As(err, &e) || e.Err == nil {
		t.Error("errors.As conserva la causa")
	}
	if ErrNotFound.Message != Messages[CodeNotFound] {
		t.Error("los centinela no se deben mutar")
	}
}

func TestDecodeEstricto(t *testing.T) {
	type req struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"gte=18"`
	}
	run := func(raw string) (*req, error) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/x", strings.NewReader(raw))
		return Decode[req](rec, r)
	}
	if _, err := run(`{"email":"no","age":10}`); err == nil {
		t.Fatal("debe fallar")
	} else {
		e := From(err)
		if e.Status != 400 || e.Code != CodeValidation || len(e.Fields) != 2 {
			t.Errorf("%+v", e)
		}
		if e.Fields[0].Field != "email" || e.Fields[0].Rule != "email" || e.Fields[1].Message != "debe ser al menos 18" {
			t.Errorf("%+v", e.Fields)
		}
	}
	if _, err := run(`{"email":"a@b.co","age":20,"extra":1}`); err == nil || !strings.Contains(From(err).Fields[0].Message, "desconocido") {
		t.Errorf("campos desconocidos se rechazan: %v", err)
	}
	if _, err := run(`{bad`); err == nil || From(err).Fields[0].Message != "JSON mal formado" {
		t.Errorf("json malo: %v", err)
	}
	if _, err := run(``); err == nil {
		t.Error("vacío falla")
	}
	if v, err := run(`{"email":"a@b.co","age":20}`); err != nil || v.Email != "a@b.co" {
		t.Errorf("válido: %v %v", v, err)
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/x", strings.NewReader(`{"email":"`+strings.Repeat("x", 100)+`"}`))
	if _, err := DecodeLimit[req](rec, r, 10); err == nil || From(err).Status != 413 {
		t.Errorf("413 por tamaño: %v", err)
	}
}

func TestDecodeStructEmbebido(t *testing.T) {
	type common struct {
		Name string `json:"name" validate:"required"`
	}
	type req struct {
		common
		Nested struct {
			Zip string `json:"zip" validate:"len=5"`
		} `json:"address"`
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/x", strings.NewReader(`{"address":{"zip":"1"}}`))
	_, err := Decode[req](rec, r)
	e := From(err)
	if len(e.Fields) != 2 || e.Fields[0].Field != "name" || e.Fields[1].Field != "address.zip" {
		t.Errorf("%+v", e.Fields)
	}
}

func TestHandlerH(t *testing.T) {
	h := H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := PathInt(r, "id")
		if err != nil {
			return err
		}
		if id == 7 {
			panic("boom")
		}
		OK(w, r, id)
		return nil
	})
	mux := http.NewServeMux()
	mux.Handle("GET /items/{id}", h)
	for _, c := range []struct {
		path string
		code int
	}{{"/items/3", 200}, {"/items/abc", 400}, {"/items/7", 500}} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.code {
			t.Errorf("%s → %d, quería %d", c.path, rec.Code, c.code)
		}
	}
}

func TestPaginate(t *testing.T) {
	p, err := Paginate(httptest.NewRequest("GET", "/x?page=3&pageSize=10", nil))
	if err != nil || p.Offset() != 20 || p.Meta(45).TotalPages != 5 {
		t.Errorf("%v %v", p, err)
	}
	if _, err := Paginate(httptest.NewRequest("GET", "/x?pageSize=1000", nil)); err == nil {
		t.Error("tope de pageSize")
	}
	if p, _ := Paginate(httptest.NewRequest("GET", "/x", nil)); p.Page != 1 || p.PageSize != DefaultPageSize {
		t.Error("defaults")
	}
}

func TestToolReplySiempre200(t *testing.T) {
	rec := httptest.NewRecorder()
	ToolReply[any](rec, httptest.NewRequest("POST", "/tool", nil), nil, NotFound("no hay lead"))
	m := body(t, rec)
	if rec.Code != 200 || m["success"] != false || m["error"] != "not-found" {
		t.Errorf("%d %v", rec.Code, m)
	}
}
