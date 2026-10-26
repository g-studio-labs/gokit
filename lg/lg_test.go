package lg

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTextoFormatoDeLaCasa(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "text", Level: "debug"})
	t.Setenv("NO_COLOR", "1")
	Setup(Options{Out: &buf, Format: "text", Level: "debug"})
	Info("db lista", "pool", 10, "dsn", "con espacios")
	Warnf("quedan %d", 3)
	Debug("detalle")
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("una línea por evento, salieron %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "(utc) - [Info]: db lista pool=10 dsn=\"con espacios\"") {
		t.Errorf("formato inesperado: %q", lines[0])
	}
	if !strings.Contains(lines[1], "[Warn]: quedan 3") || !strings.Contains(lines[2], "[Debug]: detalle") {
		t.Errorf("niveles mal: %q %q", lines[1], lines[2])
	}
}

func TestNivelFiltra(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "text", Level: "warn"})
	Info("no sale")
	Warn("sale")
	if strings.Contains(buf.String(), "no sale") || !strings.Contains(buf.String(), "sale") {
		t.Errorf("filtro de nivel roto: %q", buf.String())
	}
}

func TestJSONConRequestID(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "json", Service: "demo"})
	ctx := WithRequestID(context.Background(), "abc123")
	Ctx(ctx).Info("hola", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("no es JSON: %v\n%s", err, buf.String())
	}
	if rec["request_id"] != "abc123" || rec["service"] != "demo" || rec["k"] != "v" || rec["msg"] != "hola" {
		t.Errorf("campos: %v", rec)
	}
}

func TestSecretNoMuestraValor(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "json"})
	Info("config", Secret("JWT_SECRET"))
	if !strings.Contains(buf.String(), `"JWT_SECRET":"[redacted]"`) {
		t.Errorf("Secret debe tapar el valor: %s", buf.String())
	}
}
