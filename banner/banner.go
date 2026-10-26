// Package banner imprime el splash de arranque de la casa: el nombre del
// servicio en ANSI Shadow con el color de la marca, un subtítulo gris y filas
// [+] con el entorno, la dirección, la versión y lo que cada proyecto quiera
// añadir. Es solo decoración: los logs estructurados siguen por lg.
//
// Es el mismo banner que ya arrancaba en varios servicios, con dos cosas
// que a ninguno le habían puesto: el wordmark se renderiza desde la fuente
// embebida (no hay que pegar el arte a mano) y la fila version sale sola del
// binario (debug.ReadBuildInfo) o de -ldflags.
package banner

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

// Version y Commit se pueden fijar al compilar:
//
//	go build -ldflags "-X github.com/g-studio-labs/gokit/banner.Version=1.4.0"
//
// Si quedan vacíos se leen de la info de build del módulo (vcs.revision).
var (
	Version string
	Commit  string
)

// Row es una fila [+] extra del banner (p. ej. {"contrato", "/openapi.json"}).
type Row struct {
	Key   string
	Value string
}

// Options describe el banner. Solo Name es obligatorio.
type Options struct {
	Name      string // texto del wordmark, p. ej. "SANTA" (mayúsculas, A-Z 0-9)
	Subtitle  string // línea gris bajo el wordmark, p. ej. "TRAVEL · API"
	Accent    Color  // color del wordmark; vacío = cian
	Env       string // development | production | ...
	Addr      string // ":8080" o "0.0.0.0:8080"
	Health    string // ruta del health; vacío = "/health"; "-" lo oculta
	Rows      []Row  // filas extra, después de las fijas
	Out       io.Writer
	NoColor   bool // fuerza salida sin ANSI (también NO_COLOR en el entorno)
	noVersion bool
}

// Color es una secuencia ANSI de color de primer plano.
type Color string

const (
	cyan   Color = "\033[38;2;94;211;235m"
	green  Color = "\033[38;2;31;184;116m"
	yellow Color = "\033[38;2;255;210;26m"
	gray   Color = "\033[90m"
	bold   Color = "\033[1m"
	reset  Color = "\033[0m"
)

// Hex convierte "#D32F2F" (o "D32F2F") en un color truecolor. Las terminales
// sin truecolor caen al color más cercano. Un hex inválido devuelve cian.
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

// Wordmark renderiza text en ANSI Shadow, con las letras pegadas como lo hace
// figlet (fitting). Las runas que la fuente no trae se dibujan como espacio.
func Wordmark(text string) []string {
	var out []string
	for _, r := range text {
		g, ok := glifos[r]
		if !ok {
			g = glifos[' ']
		}
		out = fit(out, g)
	}
	return trimRight(out)
}

// fit pega el glifo a la derecha de las filas quitando el espacio que sobra
// entre ambos (sin solaparlos), como el layout "fitting" de figlet.
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

// Print escribe el banner. Se llama justo antes de ListenAndServe. Sin
// terminal (Docker, un archivo) sale sin colores.
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

// Render devuelve el banner como texto, para imprimirlo o probarlo.
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

// version arma "1.4.0 (a1b2c3d)" con lo que haya: las variables de -ldflags,
// y si no, la info de build del módulo (Go la graba al compilar desde un
// checkout de git; con -buildvcs=false no hay nada).
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
