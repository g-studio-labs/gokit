package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestResolucionEnvFileSecrets(t *testing.T) {
	dir := t.TempDir()
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = "/run/secrets" })
	os.WriteFile(filepath.Join(dir, "DESDE_SECRET"), []byte("s3cr3t\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "archivo.txt"), []byte("de-archivo"), 0o600)
	t.Setenv("DESDE_FILE_FILE", filepath.Join(dir, "archivo.txt"))
	t.Setenv("DIRECTA", "env")
	t.Setenv("OP_SIN_RESOLVER", "op://boveda/item/campo")

	if Get("DIRECTA") != "env" || Get("DESDE_FILE") != "de-archivo" || Get("DESDE_SECRET") != "s3cr3t" {
		t.Errorf("resolución: %q %q %q", Get("DIRECTA"), Get("DESDE_FILE"), Get("DESDE_SECRET"))
	}
	if Get("OP_SIN_RESOLVER") != "" {
		t.Error("una referencia op:// sin resolver es vacío")
	}
}

func TestRequiredAcumula(t *testing.T) {
	Reset()
	t.Setenv("DUR_MALA", "abc")
	Required("NO_EXISTE_1")
	Required("NO_EXISTE_2")
	Duration("DUR_MALA", time.Second)
	m, i := Check()
	if !slices.Equal(m, []string{"NO_EXISTE_1", "NO_EXISTE_2"}) || !slices.Equal(i, []string{"DUR_MALA"}) {
		t.Errorf("faltantes=%v inválidos=%v", m, i)
	}
}

func TestTipos(t *testing.T) {
	Reset()
	t.Setenv("B", "yes")
	t.Setenv("N", "42")
	t.Setenv("D", "90s")
	t.Setenv("L", "a, b,,c ")
	if !Bool("B", false) || Int("N", 0) != 42 || Duration("D", 0) != 90*time.Second || !slices.Equal(List("L"), []string{"a", "b", "c"}) {
		t.Error("parseo de tipos")
	}
}

func TestDotEnvNoPisa(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("# comentario\nexport A=\"con espacios\"\nB=valor # inline\nYA_ESTABA=nuevo\n"), 0o600)
	t.Setenv("YA_ESTABA", "viejo")
	os.Unsetenv("A")
	os.Unsetenv("B")
	LoadDotEnv(p, "no-existe.env")
	if os.Getenv("A") != "con espacios" || os.Getenv("B") != "valor" || os.Getenv("YA_ESTABA") != "viejo" {
		t.Errorf("A=%q B=%q YA=%q", os.Getenv("A"), os.Getenv("B"), os.Getenv("YA_ESTABA"))
	}
}

func TestMask(t *testing.T) {
	if Mask("abcdefgh") != "****efgh" || Mask("ab") != "**" {
		t.Error("Mask")
	}
}
