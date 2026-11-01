package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsSavedTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "yx-config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := loadConfigFrom(path)
	if got.Theme != "light" {
		t.Fatalf("theme = %q, want light", got.Theme)
	}
}
