package lg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// textHandler reproduces the format every house service already prints:
// `2006-01-02 15:04:05 (utc) - [Info]: message key=val`, with date and level
// in the level's color and the message in the usual color. Always in UTC
// (several copies said "(utc)" without being so) and a single line per event.
type textHandler struct {
	mu    *sync.Mutex
	out   io.Writer
	level slog.Leveler
	color bool
	attrs []slog.Attr
	group string
}

const (
	reset   = "\033[0m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	gray    = "\033[90m"
	uGreen  = "\033[4;32m"
	uYellow = "\033[4;33m"
	uRed    = "\033[4;31m"
)

func newTextHandler(out io.Writer, level slog.Leveler, color bool) *textHandler {
	return &textHandler{mu: &sync.Mutex{}, out: out, level: level, color: color}
}

func (h *textHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr(nil), h.attrs...), h.prefixed(attrs)...)
	return &c
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	if h.group != "" {
		c.group = h.group + "." + name
	} else {
		c.group = name
	}
	return &c
}

func (h *textHandler) prefixed(attrs []slog.Attr) []slog.Attr {
	if h.group == "" {
		return attrs
	}
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = slog.Attr{Key: h.group + "." + a.Key, Value: a.Value}
	}
	return out
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	levelColor, msgColor, label := h.palette(r.Level)
	ts := r.Time
	if ts.IsZero() {
		ts = time.Now()
	}
	var b bytes.Buffer
	h.paint(&b, levelColor, ts.UTC().Format("2006-01-02 15:04:05 (utc)")+" - "+label+":")
	b.WriteByte(' ')
	h.paint(&b, msgColor, r.Message)
	write := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			return
		}
		b.WriteByte(' ')
		h.paint(&b, gray, a.Key+"="+formatValue(a.Value))
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		for _, p := range h.prefixed([]slog.Attr{a}) {
			write(p)
		}
		return true
	})
	b.WriteByte('\n')
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(b.Bytes())
	return err
}

func (h *textHandler) palette(l slog.Level) (levelColor, msgColor, label string) {
	switch {
	case l >= slog.LevelError:
		return red, uRed, "[Error]"
	case l >= slog.LevelWarn:
		return yellow, uYellow, "[Warn]"
	case l >= slog.LevelInfo:
		return blue, uGreen, "[Info]"
	default:
		return magenta, gray, "[Debug]"
	}
}

func (h *textHandler) paint(b *bytes.Buffer, color, s string) {
	if h.color {
		b.WriteString(color)
		b.WriteString(s)
		b.WriteString(reset)
		return
	}
	b.WriteString(s)
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if strings.ContainsAny(s, " \t\"") {
			return fmt.Sprintf("%q", s)
		}
		return s
	case slog.KindTime:
		return v.Time().UTC().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	default:
		return v.String()
	}
}
