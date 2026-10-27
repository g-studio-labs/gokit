package lg

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTextHouseFormat(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "text", Level: "debug"})
	t.Setenv("NO_COLOR", "1")
	Setup(Options{Out: &buf, Format: "text", Level: "debug"})
	Info("db ready", "pool", 10, "dsn", "with spaces")
	Warnf("%d left", 3)
	Debug("detail")
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("one line per event, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "(utc) - [Info]: db ready pool=10 dsn=\"with spaces\"") {
		t.Errorf("unexpected format: %q", lines[0])
	}
	if !strings.Contains(lines[1], "[Warn]: 3 left") || !strings.Contains(lines[2], "[Debug]: detail") {
		t.Errorf("wrong levels: %q %q", lines[1], lines[2])
	}
}

func TestLevelFilters(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "text", Level: "warn"})
	Info("hidden")
	Warn("shown")
	if strings.Contains(buf.String(), "hidden") || !strings.Contains(buf.String(), "shown") {
		t.Errorf("level filter broken: %q", buf.String())
	}
}

func TestJSONWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "json", Service: "demo"})
	ctx := WithRequestID(context.Background(), "abc123")
	Ctx(ctx).Info("hello", "k", "v")
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if rec["request_id"] != "abc123" || rec["service"] != "demo" || rec["k"] != "v" || rec["msg"] != "hello" {
		t.Errorf("fields: %v", rec)
	}
}

func TestSecretHidesValue(t *testing.T) {
	var buf bytes.Buffer
	Setup(Options{Out: &buf, Format: "json"})
	Info("config", Secret("JWT_SECRET"))
	if !strings.Contains(buf.String(), `"JWT_SECRET":"[redacted]"`) {
		t.Errorf("Secret must hide the value: %s", buf.String())
	}
}
