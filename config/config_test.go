package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestResolveEnvFileSecrets(t *testing.T) {
	dir := t.TempDir()
	SecretsDir = dir
	t.Cleanup(func() { SecretsDir = "/run/secrets" })
	os.WriteFile(filepath.Join(dir, "FROM_SECRET"), []byte("s3cr3t\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("from-file"), 0o600)
	t.Setenv("FROM_FILE_FILE", filepath.Join(dir, "file.txt"))
	t.Setenv("DIRECT", "env")
	t.Setenv("OP_UNRESOLVED", "op://vault/item/field")

	if Get("DIRECT") != "env" || Get("FROM_FILE") != "from-file" || Get("FROM_SECRET") != "s3cr3t" {
		t.Errorf("resolution: %q %q %q", Get("DIRECT"), Get("FROM_FILE"), Get("FROM_SECRET"))
	}
	if Get("OP_UNRESOLVED") != "" {
		t.Error("an unresolved op:// reference is empty")
	}
}

func TestRequiredAccumulates(t *testing.T) {
	Reset()
	t.Setenv("BAD_DUR", "abc")
	Required("MISSING_1")
	Required("MISSING_2")
	Duration("BAD_DUR", time.Second)
	m, i := Check()
	if !slices.Equal(m, []string{"MISSING_1", "MISSING_2"}) || !slices.Equal(i, []string{"BAD_DUR"}) {
		t.Errorf("missing=%v invalid=%v", m, i)
	}
}

func TestTypes(t *testing.T) {
	Reset()
	t.Setenv("B", "yes")
	t.Setenv("N", "42")
	t.Setenv("D", "90s")
	t.Setenv("L", "a, b,,c ")
	if !Bool("B", false) || Int("N", 0) != 42 || Duration("D", 0) != 90*time.Second || !slices.Equal(List("L"), []string{"a", "b", "c"}) {
		t.Error("type parsing")
	}
}

func TestDotEnvDoesNotOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("# comment\nexport A=\"with spaces\"\nB=value # inline\nALREADY_SET=new\n"), 0o600)
	t.Setenv("ALREADY_SET", "old")
	os.Unsetenv("A")
	os.Unsetenv("B")
	LoadDotEnv(p, "does-not-exist.env")
	if os.Getenv("A") != "with spaces" || os.Getenv("B") != "value" || os.Getenv("ALREADY_SET") != "old" {
		t.Errorf("A=%q B=%q ALREADY=%q", os.Getenv("A"), os.Getenv("B"), os.Getenv("ALREADY_SET"))
	}
}

func TestMask(t *testing.T) {
	if Mask("abcdefgh") != "****efgh" || Mask("ab") != "**" {
		t.Error("Mask")
	}
}
