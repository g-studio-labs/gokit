package banner

import (
	"strings"
	"testing"
)

func TestWordmarkSeparaLetras(t *testing.T) {
	rows := Wordmark("CAC")
	if len(rows) != 6 {
		t.Fatalf("filas = %d, quiero 6", len(rows))
	}
	// Las tres letras quedan en la misma fila y en orden, sin solaparse.
	if !strings.Contains(rows[0], "██████╗") || strings.Count(rows[0], "██████╗") != 2 {
		t.Fatalf("primera fila inesperada: %q", rows[0])
	}
	if strings.HasSuffix(rows[0], " ") {
		t.Fatal("no debe dejar espacios al final")
	}
}

func TestWordmarkRunaDesconocida(t *testing.T) {
	rows := Wordmark("Ñ")
	for _, r := range rows {
		if strings.TrimSpace(r) != "" {
			t.Fatalf("una runa sin glifo se dibuja vacía, no %q", r)
		}
	}
}

func TestRenderSinColor(t *testing.T) {
	out := Render(Options{Name: "DEMO", Subtitle: "API", Env: "test", Addr: ":9090",
		Rows: []Row{{"db", "postgres"}}, NoColor: true})
	for _, want := range []string{"[+] env       test", "[+] addr      :9090", "http://localhost:9090/health", "[+] db        postgres", ">> listening on :9090", "  API"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\033[") {
		t.Error("NoColor no debe emitir ANSI")
	}
}

func TestRenderOcultaHealth(t *testing.T) {
	out := Render(Options{Name: "X", Health: "-", NoColor: true})
	if strings.Contains(out, "health") {
		t.Error("Health \"-\" oculta la fila")
	}
}

func TestHex(t *testing.T) {
	if Hex("#D32F2F") != "\033[38;2;211;47;47m" {
		t.Errorf("Hex = %q", Hex("#D32F2F"))
	}
	if Hex("rojo") != cyan {
		t.Error("hex inválido cae a cian")
	}
}
