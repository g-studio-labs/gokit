// Package config reads the service configuration from the environment, with
// the rules our services already followed:
//
//   - a variable resolves as env → NAME_FILE → /run/secrets/NAME, so the
//     distroless image needs no entrypoint that exports the secrets;
//   - an unresolved 1Password reference (op://...) counts as missing,
//     because `op run` did not substitute it;
//   - .env never overrides what already comes from the real environment;
//   - whatever is missing is accumulated and reported together, by name, never by value.
//
// Usage:
//
//	cfg := struct{ DSN, Port string }{
//	    DSN:  config.Required("DATABASE_URL"),
//	    Port: config.Default("PORT", "8080"),
//	}
//	config.Must() // exits with the list of missing ones, if any
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g-studio-labs/gokit/lg"
)

// SecretsDir is where Swarm (and k8s with a volume) mounts the secrets.
var SecretsDir = "/run/secrets"

var (
	mu      sync.Mutex
	missing []string
	invalid []string
)

// Get returns the value of name (env → NAME_FILE → /run/secrets/name) or "".
// An unresolved op:// reference is treated as empty.
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

// Required records name as missing if it has no value. Must() reports them all.
func Required(name string) string {
	v := Get(name)
	if v == "" {
		mu.Lock()
		missing = append(missing, name)
		mu.Unlock()
	}
	return v
}

// Default returns the value, or def if it is not set.
func Default(name, def string) string {
	if v := Get(name); v != "" {
		return v
	}
	return def
}

// Bool accepts 1/true/yes/on and 0/false/no/off (case-insensitive). Any other
// value returns def and is reported as invalid by Must.
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

// Int parses an integer; an invalid one counts for Must().
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

// Duration parses "15s", "2h"; invalid or <= 0 counts for Must().
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

// List splits a comma-separated list, without empties or spaces.
func List(name string) []string {
	var out []string
	for _, p := range strings.Split(Get(name), ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Env returns APP_ENV (or GO_ENV, NODE_ENV), "development" by default.
func Env() string {
	for _, k := range []string{"APP_ENV", "GO_ENV", "NODE_ENV"} {
		if v := Get(k); v != "" {
			return v
		}
	}
	return "development"
}

// IsProduction is Env() == "production".
func IsProduction() bool { return Env() == "production" }

// Check returns the accumulated missing and invalid names, or nil.
func Check() (missingNames, invalidNames []string) {
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), missing...), append([]string(nil), invalid...)
}

// Must exits the process if something required is missing or does not parse.
// It lists names, never values.
func Must() {
	m, i := Check()
	if len(m) == 0 && len(i) == 0 {
		return
	}
	if len(m) > 0 {
		lg.Error("missing environment variables: " + strings.Join(m, ", "))
	}
	if len(i) > 0 {
		lg.Error("environment variables with invalid value: " + strings.Join(i, ", "))
	}
	os.Exit(1)
}

// Reset forgets what was accumulated (for tests).
func Reset() {
	mu.Lock()
	missing, invalid = nil, nil
	mu.Unlock()
}

// Mask shows only the last 4 characters: "****abcd".
func Mask(s string) string {
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", 4) + s[len(s)-4:]
}

// LoadDotEnv loads the files (e.g. ".env", ".env.local") without overriding
// what already exists in the environment. A missing file is ignored. Supports
// comments, `export KEY=val`, and single- or double-quoted values.
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
