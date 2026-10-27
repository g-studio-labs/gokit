// Package banner prints the house's startup splash: the service name in ANSI
// Shadow with the brand color, a gray subtitle and [+] rows with the
// environment, the address, the version and whatever each project wants to
// add. It is decoration only: structured logs still go through lg.
//
// It is the same banner several services already booted with, plus two
// things none of them had: the wordmark is rendered from the embedded font
// (no pasting the art by hand) and the version row comes on its own from the
// binary (debug.ReadBuildInfo) or from -ldflags.
package banner

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// Version and Commit can be set at build time:
//
//	go build -ldflags "-X github.com/g-studio-labs/gokit/banner.Version=1.4.0"
//
// If left empty they are read from the module's build info (vcs.revision).
var (
	Version string
	Commit  string
)

// Row is an extra [+] row of the banner (e.g. {"contract", "/openapi.json"}).
type Row struct {
	Key   string
	Value string
}

// Options describes the banner. Only Name is required.
type Options struct {
	Name      string // wordmark text, e.g. "ACME" (uppercase, A-Z 0-9)
	Subtitle  string // gray line under the wordmark, e.g. "STUDIO · API"
	Accent    Color  // wordmark color; empty = cyan
	Env       string // development | production | ...
	Addr      string // ":8080" or "0.0.0.0:8080"
	Health    string // health path; empty = "/health"; "-" hides it
	Rows      []Row  // extra rows, after the fixed ones
	Out       io.Writer
	NoColor   bool // forces output without ANSI (also NO_COLOR in the environment)
	noVersion bool
}

// Color is an ANSI foreground color sequence.
type Color string

const (
	cyan   Color = "\033[38;2;94;211;235m"
	green  Color = "\033[38;2;31;184;116m"
	yellow Color = "\033[38;2;255;210;26m"
	gray   Color = "\033[90m"
	bold   Color = "\033[1m"
	reset  Color = "\033[0m"
)

// Hex converts "#D32F2F" (or "D32F2F") into a truecolor color. Terminals
// without truecolor fall back to the nearest color. An invalid hex returns cyan.
func Hex(hex string) Color {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return cyan
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b); err != nil {
		return cyan
	}
	return Color(fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b))
}

// Wordmark renders text in ANSI Shadow, with letters joined the way figlet
// does it (fitting). Runes the font lacks are drawn as a space.
func Wordmark(text string) []string {
	var out []string
	for _, r := range text {
		g, ok := glyphs[r]
		if !ok {
			g = glyphs[' ']
		}
		out = fit(out, g)
	}
	return trimRight(out)
}

// fit appends the glyph to the right of the rows removing the extra space
// between them (without overlapping), like figlet's "fitting" layout.
func fit(left, right []string) []string {
	if len(left) == 0 {
		return append([]string(nil), right...)
	}
	overlap := -1
	for i := range left {
		gap := trailing(left[i]) + leading(right[i])
		if overlap < 0 || gap < overlap {
			overlap = gap
		}
	}
	if overlap < 0 {
		overlap = 0
	}
	out := make([]string, len(left))
	for i := range left {
		l := []rune(left[i])
		r := []rune(right[i])
		cut := min(overlap, trailing(left[i]))
		l = l[:len(l)-cut]
		r = r[min(overlap-cut, leading(right[i])):]
		out[i] = string(l) + string(r)
	}
	return out
}

func trailing(s string) int {
	n := 0
	rs := []rune(s)
	for i := len(rs) - 1; i >= 0 && rs[i] == ' '; i-- {
		n++
	}
	return n
}

func leading(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

func trimRight(rows []string) []string {
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// Print writes the banner. Call it right before ListenAndServe. Without a
// terminal (Docker, a file) it comes out without colors.
func Print(o Options) {
	out := orStdout(o.Out)
	if !isTerminal(out) {
		o.NoColor = true
	}
	fmt.Fprint(out, Render(o))
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// Render returns the banner as text, to print or test it.
func Render(o Options) string {
	color := !o.NoColor && os.Getenv("NO_COLOR") == ""
	paint := func(c Color, s string) string {
		if !color {
			return s
		}
		return string(c) + s + string(reset)
	}
	accent := o.Accent
	if accent == "" {
		accent = cyan
	}
	env := o.Env
	if env == "" {
		env = "development"
	}
	addr := o.Addr
	if addr == "" {
		addr = ":8080"
	}
	port := addr[strings.LastIndex(addr, ":")+1:]

	var b strings.Builder
	b.WriteString("\n")
	for _, row := range Wordmark(o.Name) {
		b.WriteString("  " + paint(accent, row) + "\n")
	}
	if o.Subtitle != "" {
		b.WriteString("  " + paint(gray, "  "+o.Subtitle) + "\n")
	}
	b.WriteString("\n")

	row := func(key, value string, c Color) {
		fmt.Fprintf(&b, "    %s %-9s %s\n", paint(cyan, "[+]"), key, paint(c, value))
	}
	row("env", env, yellow)
	row("addr", addr, yellow)
	if o.Health != "-" {
		h := o.Health
		if h == "" {
			h = "/health"
		}
		row("health", "http://localhost:"+port+h, gray)
	}
	if v := version(); v != "" && !o.noVersion {
		row("version", v, gray)
	}
	for _, r := range o.Rows {
		row(r.Key, r.Value, gray)
	}
	row("booted", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"), gray)
	b.WriteString("\n")
	fmt.Fprintf(&b, "    %s listening on %s\n\n", paint(green, ">>"), paint(bold, addr))
	return b.String()
}

// version builds "1.4.0 (a1b2c3d)" from whatever is available: the -ldflags
// variables, or else the module's build info (Go records it when building
// from a git checkout; with -buildvcs=false there is nothing).
func version() string {
	v, c := Version, Commit
	if info, ok := debug.ReadBuildInfo(); ok {
		if v == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
		dirty := false
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if c == "" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
		if dirty && c != "" {
			c += "*"
		}
	}
	switch {
	case v != "" && c != "":
		return v + " (" + c + ")"
	case v != "":
		return v
	default:
		return c
	}
}

func orStdout(w io.Writer) io.Writer {
	if w == nil {
		return os.Stdout
	}
	return w
}
