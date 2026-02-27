package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_SetsMissingVariables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "FOO=bar\nEMPTY=\nQUOTED=\"hello world\"\nSINGLE='x y'\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("FOO", "")
	os.Unsetenv("FOO")
	os.Unsetenv("EMPTY")
	os.Unsetenv("QUOTED")
	os.Unsetenv("SINGLE")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv error: %v", err)
	}

	if got := os.Getenv("FOO"); got != "bar" {
		t.Fatalf("FOO = %q, want %q", got, "bar")
	}
	if got := os.Getenv("EMPTY"); got != "" {
		t.Fatalf("EMPTY = %q, want empty", got)
	}
	if got := os.Getenv("QUOTED"); got != "hello world" {
		t.Fatalf("QUOTED = %q, want %q", got, "hello world")
	}
	if got := os.Getenv("SINGLE"); got != "x y" {
		t.Fatalf("SINGLE = %q, want %q", got, "x y")
	}
}

func TestLoadDotEnv_DoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=from_file\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	t.Setenv("FOO", "from_env")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv error: %v", err)
	}
	if got := os.Getenv("FOO"); got != "from_env" {
		t.Fatalf("FOO = %q, want %q", got, "from_env")
	}
}
