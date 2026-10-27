package banner

import (
	"strings"
	"testing"
)

func TestWordmarkSeparatesLetters(t *testing.T) {
	rows := Wordmark("CIC")
	if len(rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(rows))
	}
	// The three letters sit on the same row and in order, without overlapping.
	if !strings.Contains(rows[0], "██████╗") || strings.Count(rows[0], "██████╗") != 2 {
		t.Fatalf("unexpected first row: %q", rows[0])
	}
	if strings.HasSuffix(rows[0], " ") {
		t.Fatal("must not leave trailing spaces")
	}
}

func TestWordmarkUnknownRune(t *testing.T) {
	rows := Wordmark("Ñ")
	for _, r := range rows {
		if strings.TrimSpace(r) != "" {
			t.Fatalf("a rune without a glyph is drawn empty, not %q", r)
		}
	}
}

func TestRenderNoColor(t *testing.T) {
	out := Render(Options{Name: "DEMO", Subtitle: "API", Env: "test", Addr: ":9090",
		Rows: []Row{{"db", "postgres"}}, NoColor: true})
	for _, want := range []string{"[+] env       test", "[+] addr      :9090", "http://localhost:9090/health", "[+] db        postgres", ">> listening on :9090", "  API"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\033[") {
		t.Error("NoColor must not emit ANSI")
	}
}

func TestRenderHidesHealth(t *testing.T) {
	out := Render(Options{Name: "X", Health: "-", NoColor: true})
	if strings.Contains(out, "health") {
		t.Error("Health \"-\" hides the row")
	}
}

func TestHex(t *testing.T) {
	if Hex("#D32F2F") != "\033[38;2;211;47;47m" {
		t.Errorf("Hex = %q", Hex("#D32F2F"))
	}
	if Hex("red") != cyan {
		t.Error("invalid hex falls back to cyan")
	}
}
