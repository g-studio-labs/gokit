package response

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"github.com/RomanshkVolkov/gokit/lg"
)

// Write serializa resp con el status dado y pone el requestId del contexto.
func Write[T any](w http.ResponseWriter, r *http.Request, status int, resp Response[T]) {
	if resp.RequestID == "" && r != nil {
		resp.RequestID = lg.RequestID(r.Context())
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		lg.Ctx(r.Context()).Error("no se pudo serializar la respuesta", lg.Err(err))
	}
}

// OK responde 200 con data. El mensaje es opcional.
func OK[T any](w http.ResponseWriter, r *http.Request, data T, message ...string) {
	Write(w, r, http.StatusOK, Response[T]{Success: true, Data: data, Message: first(message)})
}

// Created responde 201 con data.
func Created[T any](w http.ResponseWriter, r *http.Request, data T, message ...string) {
	Write(w, r, http.StatusCreated, Response[T]{Success: true, Data: data, Message: first(message)})
}

// Message responde 200 sin data, solo con un mensaje ("Correo enviado").
func Message(w http.ResponseWriter, r *http.Request, message string) {
	Write(w, r, http.StatusOK, Response[any]{Success: true, Message: message})
}

// NoContent responde 204 sin cuerpo.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Paged responde 200 con un listado y su meta. Un slice nil viaja como [].
func Paged[T any](w http.ResponseWriter, r *http.Request, items []T, meta *Meta) {
	if items == nil {
		items = []T{}
	}
	Write(w, r, http.StatusOK, Response[[]T]{Success: true, Data: items, Meta: meta})
}

// Fail responde el error con su status y código. Los 5xx se registran con la
// causa; el cliente solo recibe el mensaje genérico. Los 4xx no se registran
// aquí (el middleware Logger ya deja la línea del request).
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	e := From(err)
	if e == nil {
		e = Internal(nil)
	}
	if e.Status >= 500 {
		attrs := []any{"status", e.Status, "code", e.Code}
		if r != nil {
			attrs = append(attrs, "method", r.Method, "path", r.URL.Path)
		}
		if e.Err != nil {
			attrs = append(attrs, lg.Err(e.Err))
		}
		lg.Ctx(ctxOf(r)).Error("error interno", attrs...)
	}
	Write(w, r, e.Status, Response[any]{Success: false, Message: e.Message, Error: e.Code, Fields: e.Fields})
}

// Respond cierra el patrón común: err → Fail, si no → OK con data.
func Respond[T any](w http.ResponseWriter, r *http.Request, data T, err error, message ...string) {
	if err != nil {
		Fail(w, r, err)
		return
	}
	OK(w, r, data, message...)
}

// ToolReply es para endpoints que consumen herramientas de agentes (n8n, MCP):
// siempre 200, y el fallo va dentro del sobre con success:false para que el
// agente lea qué pasó en vez de recibir un error de red.
func ToolReply[T any](w http.ResponseWriter, r *http.Request, data T, err error) {
	if err != nil {
		e := From(err)
		Write(w, r, http.StatusOK, Response[any]{Success: false, Message: e.Message, Error: e.Code, Fields: e.Fields})
		return
	}
	Write(w, r, http.StatusOK, Response[T]{Success: true, Data: data})
}

// HandlerFunc es un handler que devuelve error; H lo convierte en http.Handler
// respondiendo el error con Fail. Deja los handlers sin ifs de respuesta:
//
//	mux.Handle("GET /items/{id}", response.H(func(w http.ResponseWriter, r *http.Request) error {
//	    id, err := response.PathInt(r, "id")
//	    if err != nil { return err }
//	    item, err := svc.Get(r.Context(), id)
//	    if err != nil { return err }
//	    response.OK(w, r, item)
//	    return nil
//	}))
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// H adapta un HandlerFunc. Un panic dentro se responde como 500 (el
// middleware Recover hace lo mismo para lo que no pase por aquí).
func H(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				lg.Ctx(r.Context()).Error("panic en handler", "method", r.Method, "path", r.URL.Path, "panic", rec, "stack", string(debug.Stack()))
				Fail(w, r, Internal(nil))
			}
		}()
		if err := fn(w, r); err != nil {
			Fail(w, r, err)
		}
	}
}

func first(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}
