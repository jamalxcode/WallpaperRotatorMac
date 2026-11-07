package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "deviantart-secret")
	if got := readSecretFile(path); got != "" {
		t.Fatalf("missing file read as %q", got)
	}
	if err := writeSecretFile(path, "abc123"); err != nil {
		t.Fatal(err)
	}
	if got := readSecretFile(path); got != "abc123" {
		t.Fatalf("read %q, want abc123", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	if err := writeSecretFile(path, "new"); err != nil || readSecretFile(path) != "new" {
		t.Errorf("overwrite failed: %v", err)
	}
}
