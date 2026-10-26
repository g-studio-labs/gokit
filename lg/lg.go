// Package lg es el logger de la casa. Mantiene la API de siempre
// (lg.Info("texto"), lg.Errorf("%v", err)) y por debajo es log/slog: texto a
// color cuando la salida es una terminal, JSON cuando no (Docker, Swarm, k8s),
// nivel por LOG_LEVEL y request_id en cada línea cuando el contexto lo trae.
//
// Sin llamar a Setup funciona con los defaults; Setup se llama una vez en main
// para fijar servicio, nivel o formato.
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

// Options configura el logger raíz. Todo es opcional.
type Options struct {
	// Level: "debug" | "info" | "warn" | "error". Vacío = LOG_LEVEL o info.
	Level string
	// Format: "auto" | "text" | "json". Vacío = LOG_FORMAT o auto (texto si Out
	// es una terminal, JSON si no).
	Format string
	// Out: destino; nil = os.Stdout.
	Out io.Writer
	// Service se añade como atributo "service" en cada línea (útil cuando varios
	// servicios comparten el agregador de logs). Vacío = no se añade.
	Service string
	// AddSource añade archivo:línea. Caro; solo para depurar.
	AddSource bool
}

var (
	root  atomic.Pointer[slog.Logger]
	level = new(slog.LevelVar)
	once  sync.Once
)

// Setup construye el logger raíz. Llamarla más de una vez reemplaza el anterior.
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
	slog.SetDefault(l) // lo que loguee la stdlib (http.Server.ErrorLog) sale igual
	return l
}

// Logger devuelve el *slog.Logger raíz (para pasárselo a librerías).
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

// SetLevel cambia el nivel en caliente ("debug", "info", "warn", "error").
func SetLevel(s string) { level.Set(parseLevel(s)) }

type ctxKey struct{}

// WithRequestID guarda el id de la petición en el contexto; Ctx lo añade a
// cada línea como request_id. Lo usa middleware.RequestID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID devuelve el id guardado por WithRequestID, o "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Ctx devuelve el logger con el request_id del contexto (si hay). Úsalo dentro
// de handlers y services: lg.Ctx(ctx).Warn("stock agotado", "sku", sku).
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

// Debug, Info, Warn y Error aceptan atributos como slog: lg.Info("db lista", "pool", 10).
func Debug(msg string, attrs ...any) { Logger().Debug(msg, attrs...) }
func Info(msg string, attrs ...any)  { Logger().Info(msg, attrs...) }
func Warn(msg string, attrs ...any)  { Logger().Warn(msg, attrs...) }
func Error(msg string, attrs ...any) { Logger().Error(msg, attrs...) }

// Debugf, Infof, Warnf y Errorf formatean como fmt.Sprintf.
func Debugf(format string, a ...any) { Logger().Debug(fmt.Sprintf(format, a...)) }
func Infof(format string, a ...any)  { Logger().Info(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)  { Logger().Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any) { Logger().Error(fmt.Sprintf(format, a...)) }

// Err es el atributo estándar para errores: lg.Error("fallo", lg.Err(err)).
func Err(err error) slog.Attr {
	if err == nil {
		return slog.Attr{}
	}
	return slog.String("error", err.Error())
}

// Secret es un atributo que nunca muestra el valor: lg.Info("config", lg.Secret("JWT_SECRET")).
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
