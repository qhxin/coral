package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotenvFileIfPresent(t *testing.T) {
	const kLoad = "CORAL_TEST_DOTENV_LOAD"
	const kKeep = "CORAL_TEST_DOTENV_KEEP"
	const kBad = "coral_test_bad_key"

	_ = os.Unsetenv(kLoad)
	_ = os.Unsetenv(kKeep)
	_ = os.Unsetenv(kBad)
	t.Setenv(kKeep, "already-set")

	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := "# comment\n" +
		"\n" +
		kLoad + "=loaded\n" +
		kKeep + "=from-file\n" +
		"BAD-LINE\n" +
		kBad + "=ignored\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := loadDotenvFileIfPresent(p); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv(kLoad); got != "loaded" {
		t.Fatalf("expected %s=loaded, got %q", kLoad, got)
	}
	if got := os.Getenv(kKeep); got != "already-set" {
		t.Fatalf("existing env should not be overwritten, got %q", got)
	}
	if got := os.Getenv(kBad); got != "" {
		t.Fatalf("invalid key should be ignored, got %q", got)
	}
}

func TestLoadDotenvFileIfPresent_missingFile(t *testing.T) {
	if err := loadDotenvFileIfPresent(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDotenvFromExecutableDir(t *testing.T) {
	const key = "CORAL_TEST_DOTENV_EXE"
	_ = os.Unsetenv(key)

	dir := t.TempDir()
	exePath := filepath.Join(dir, "coral.exe")
	dotenvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(dotenvPath, []byte(key+"=from-exe-dir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := osExecutableFn
	osExecutableFn = func() (string, error) { return exePath, nil }
	t.Cleanup(func() { osExecutableFn = old })

	if err := loadDotenvFromExecutableDir(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "from-exe-dir" {
		t.Fatalf("expected %s=from-exe-dir, got %q", key, got)
	}
}

