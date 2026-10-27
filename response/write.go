package response

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"github.com/g-studio-labs/gokit/lg"
)

// Write serializes resp with the given status and sets the context's requestId.
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
		lg.Ctx(r.Context()).Error("could not serialize the response", lg.Err(err))
	}
}

// OK responds 200 with data. The message is optional.
func OK[T any](w http.ResponseWriter, r *http.Request, data T, message ...string) {
	Write(w, r, http.StatusOK, Response[T]{Success: true, Data: data, Message: first(message)})
}

// Created responds 201 with data.
func Created[T any](w http.ResponseWriter, r *http.Request, data T, message ...string) {
	Write(w, r, http.StatusCreated, Response[T]{Success: true, Data: data, Message: first(message)})
}

// Message responds 200 without data, only with a message ("Correo enviado").
func Message(w http.ResponseWriter, r *http.Request, message string) {
	Write(w, r, http.StatusOK, Response[any]{Success: true, Message: message})
}

// NoContent responds 204 with no body.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Paged responds 200 with a listing and its meta. A nil slice travels as [].
func Paged[T any](w http.ResponseWriter, r *http.Request, items []T, meta *Meta) {
	if items == nil {
		items = []T{}
	}
	Write(w, r, http.StatusOK, Response[[]T]{Success: true, Data: items, Meta: meta})
}

// Fail responds the error with its status and code. 5xx are logged with the
// cause; the client only gets the generic message. 4xx are not logged here
// (the Logger middleware already writes the request line).
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
		lg.Ctx(ctxOf(r)).Error("internal error", attrs...)
	}
	Write(w, r, e.Status, Response[any]{Success: false, Message: e.Message, Error: e.Code, Fields: e.Fields})
}

// Respond wraps up the common pattern: err → Fail, otherwise → OK with data.
func Respond[T any](w http.ResponseWriter, r *http.Request, data T, err error, message ...string) {
	if err != nil {
		Fail(w, r, err)
		return
	}
	OK(w, r, data, message...)
}

// ToolReply is for endpoints consumed by agent tools (n8n, MCP): always 200,
// and the failure goes inside the envelope with success:false so the agent
// reads what happened instead of getting a network error.
func ToolReply[T any](w http.ResponseWriter, r *http.Request, data T, err error) {
	if err != nil {
		e := From(err)
		Write(w, r, http.StatusOK, Response[any]{Success: false, Message: e.Message, Error: e.Code, Fields: e.Fields})
		return
	}
	Write(w, r, http.StatusOK, Response[T]{Success: true, Data: data})
}

// HandlerFunc is a handler that returns an error; H turns it into an http.Handler
// responding the error with Fail. It keeps handlers free of response ifs:
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

// H adapts a HandlerFunc. A panic inside is answered as 500 (the Recover
// middleware does the same for whatever does not go through here).
func H(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				lg.Ctx(r.Context()).Error("panic in handler", "method", r.Method, "path", r.URL.Path, "panic", rec, "stack", string(debug.Stack()))
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
