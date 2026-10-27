// Package lg is the house logger. It keeps the usual API
// (lg.Info("text"), lg.Errorf("%v", err)) and underneath it is log/slog: colored
// text when the output is a terminal, JSON when it is not (Docker, Swarm, k8s),
// level from LOG_LEVEL and request_id on every line when the context carries it.
//
// Without calling Setup it works with the defaults; Setup is called once in main
// to set service, level or format.
package lg

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

// Options configures the root logger. Everything is optional.
type Options struct {
	// Level: "debug" | "info" | "warn" | "error". Empty = LOG_LEVEL or info.
	Level string
	// Format: "auto" | "text" | "json". Empty = LOG_FORMAT or auto (text if Out
	// is a terminal, JSON if not).
	Format string
	// Out: destination; nil = os.Stdout.
	Out io.Writer
	// Service is added as a "service" attribute on every line (useful when several
	// services share the log aggregator). Empty = not added.
	Service string
	// AddSource adds file:line. Expensive; for debugging only.
	AddSource bool
}

var (
	root  atomic.Pointer[slog.Logger]
	level = new(slog.LevelVar)
	once  sync.Once
)

// Setup builds the root logger. Calling it more than once replaces the previous one.
func Setup(o Options) *slog.Logger {
	out := o.Out
	if out == nil {
		out = os.Stdout
	}
	level.Set(parseLevel(firstNonEmpty(o.Level, os.Getenv("LOG_LEVEL"), "info")))
	format := firstNonEmpty(o.Format, os.Getenv("LOG_FORMAT"), "auto")
	if format == "auto" {
		if isTerminal(out) {
			format = "text"
		} else {
			format = "json"
		}
	}
	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level, AddSource: o.AddSource})
	default:
		h = newTextHandler(out, level, os.Getenv("NO_COLOR") == "")
	}
	l := slog.New(h)
	if o.Service != "" {
		l = l.With("service", o.Service)
	}
	root.Store(l)
	slog.SetDefault(l) // whatever the stdlib logs (http.Server.ErrorLog) comes out the same
	return l
}

// Logger returns the root *slog.Logger (to hand to libraries).
func Logger() *slog.Logger {
	if l := root.Load(); l != nil {
		return l
	}
	once.Do(func() {
		if root.Load() == nil {
			Setup(Options{})
		}
	})
	return root.Load()
}

// SetLevel changes the level at runtime ("debug", "info", "warn", "error").
func SetLevel(s string) { level.Set(parseLevel(s)) }

type ctxKey struct{}

// WithRequestID stores the request id in the context; Ctx adds it to every
// line as request_id. Used by middleware.RequestID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID returns the id stored by WithRequestID, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Ctx returns the logger with the context's request_id (if any). Use it inside
// handlers and services: lg.Ctx(ctx).Warn("out of stock", "sku", sku).
func Ctx(ctx context.Context) *slog.Logger {
	l := Logger()
	if ctx == nil {
		return l
	}
	if id := RequestID(ctx); id != "" {
		return l.With("request_id", id)
	}
	return l
}

// Debug, Info, Warn and Error accept attributes like slog: lg.Info("db ready", "pool", 10).
func Debug(msg string, attrs ...any) { Logger().Debug(msg, attrs...) }
func Info(msg string, attrs ...any)  { Logger().Info(msg, attrs...) }
func Warn(msg string, attrs ...any)  { Logger().Warn(msg, attrs...) }
func Error(msg string, attrs ...any) { Logger().Error(msg, attrs...) }

// Debugf, Infof, Warnf and Errorf format like fmt.Sprintf.
func Debugf(format string, a ...any) { Logger().Debug(fmt.Sprintf(format, a...)) }
func Infof(format string, a ...any)  { Logger().Info(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)  { Logger().Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any) { Logger().Error(fmt.Sprintf(format, a...)) }

// Err is the standard attribute for errors: lg.Error("failed", lg.Err(err)).
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String("error", err.Error())
}

// Secret is an attribute that never shows the value: lg.Info("config", lg.Secret("JWT_SECRET")).
func Secret(key string) slog.Attr { return slog.String(key, "[redacted]") }

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
