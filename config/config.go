// Package config lee la configuración del servicio desde el entorno, con las
// reglas que ya seguían crm, eleva y santa-travel:
//
//   - una variable se resuelve como env → NAME_FILE → /run/secrets/NAME, así la
//     imagen distroless no necesita un entrypoint que exporte los secrets;
//   - una referencia de 1Password sin resolver (op://...) cuenta como faltante,
//     porque `op run` no la sustituyó;
//   - .env nunca pisa lo que ya venga del entorno real;
//   - lo que falta se acumula y se reporta junto, por nombre, nunca por valor.
//
// Uso:
//
//	cfg := struct{ DSN, Port string }{
//	    DSN:  config.Required("DATABASE_URL"),
//	    Port: config.Default("PORT", "8080"),
//	}
//	config.Must() // sale con la lista de faltantes si hay
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/RomanshkVolkov/gokit/lg"
)

// SecretsDir es donde Swarm (y k8s con un volumen) monta los secrets.
var SecretsDir = "/run/secrets"

var (
	mu      sync.Mutex
	missing []string
	invalid []string
)

// Get devuelve el valor de name (env → NAME_FILE → /run/secrets/name) o "".
// Una referencia op:// sin resolver se trata como vacío.
func Get(name string) string {
	v := os.Getenv(name)
	if v == "" {
		if path := os.Getenv(name + "_FILE"); path != "" {
			v = readFile(path)
		}
	}
	if v == "" {
		v = readFile(SecretsDir + "/" + name)
	}
	if strings.HasPrefix(v, "op://") {
		return ""
	}
	return strings.TrimSpace(v)
}

// Required registra name como faltante si no tiene valor. Must() reporta todos.
func Required(name string) string {
	v := Get(name)
	if v == "" {
		mu.Lock()
		missing = append(missing, name)
		mu.Unlock()
	}
	return v
}

// Default devuelve el valor o def si no está.
func Default(name, def string) string {
	if v := Get(name); v != "" {
		return v
	}
	return def
}

// Bool acepta 1/true/yes/on (sin importar mayúsculas); otro valor es false.
func Bool(name string, def bool) bool {
	v := Get(name)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	markInvalid(name)
	return def
}

// Int parsea un entero; inválido cuenta para Must().
func Int(name string, def int) int {
	v := Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		markInvalid(name)
		return def
	}
	return n
}

// Duration parsea "15s", "2h"; inválido o <= 0 cuenta para Must().
func Duration(name string, def time.Duration) time.Duration {
	v := Get(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		markInvalid(name)
		return def
	}
	return d
}

// List parte una lista separada por comas, sin vacíos ni espacios.
func List(name string) []string {
	var out []string
	for _, p := range strings.Split(Get(name), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Env devuelve APP_ENV (o GO_ENV, NODE_ENV) con "development" por defecto.
func Env() string {
	for _, k := range []string{"APP_ENV", "GO_ENV", "NODE_ENV"} {
		if v := Get(k); v != "" {
			return v
		}
	}
	return "development"
}

// IsProduction es Env() == "production".
func IsProduction() bool { return Env() == "production" }

// Check devuelve los nombres faltantes e inválidos acumulados, o nil.
func Check() (missingNames, invalidNames []string) {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), missing...), append([]string(nil), invalid...)
}

// Must termina el proceso si algo requerido falta o no parsea. Lista nombres,
// nunca valores.
func Must() {
	m, i := Check()
	if len(m) == 0 && len(i) == 0 {
		return
	}
	if len(m) > 0 {
		lg.Error("faltan variables de entorno: " + strings.Join(m, ", "))
	}
	if len(i) > 0 {
		lg.Error("variables de entorno con valor inválido: " + strings.Join(i, ", "))
	}
	os.Exit(1)
}

// Reset olvida lo acumulado (para pruebas).
func Reset() {
	mu.Lock()
	missing, invalid = nil, nil
	mu.Unlock()
}

// Mask deja ver solo los últimos 4 caracteres: "****abcd".
func Mask(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", 4) + s[len(s)-4:]
}

// LoadDotEnv carga los archivos (p. ej. ".env", ".env.local") sin pisar lo que
// ya exista en el entorno. Un archivo que no existe se ignora. Soporta
// comentarios, `export KEY=val`, y valores entre comillas simples o dobles.
func LoadDotEnv(paths ...string) {
	if len(paths) == 0 {
		paths = []string{".env"}
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			} else if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
			if _, exists := os.LookupEnv(k); !exists {
				os.Setenv(k, v)
			}
		}
		f.Close()
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\r\n")
}

func markInvalid(name string) {
	mu.Lock()
	invalid = append(invalid, name)
	mu.Unlock()
}
