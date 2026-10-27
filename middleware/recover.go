package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/g-studio-labs/gokit/lg"
	"github.com/g-studio-labs/gokit/response"
)

// Recover catches panics, logs them with the stack and responds 500 with the
// envelope. http.ErrAbortHandler is re-panicked: that is how the reverse proxy
// aborts when the client hangs up, and it is not our error.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				lg.Ctx(r.Context()).Error("panic", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				response.Fail(w, r, response.Internal(nil))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
