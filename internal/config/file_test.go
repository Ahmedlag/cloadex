package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFileMigratesLegacyConfig(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	_ = os.Chdir(tmp)
	defer os.Chdir(orig)

	legacyDir := filepath.Join(".wizdo")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "config.yaml"), []byte("rounds: 7\nverbose: true\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	opts := DefaultOptions()
	if err := LoadFile(&opts, map[string]bool{}); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if opts.MaxRounds != 7 {
		t.Fatalf("MaxRounds = %d, want 7", opts.MaxRounds)
	}
	if !opts.Verbose {
		t.Fatal("expected verbose to be loaded from migrated config")
	}
	if _, err := os.Stat(filepath.Join(".cloadex", "config.yaml")); err != nil {
		t.Fatalf("expected migrated config under .cloadex: %v", err)
	}
	if _, err := os.Stat(".wizdo"); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .wizdo dir removed after migration, got %v", err)
	}
}
