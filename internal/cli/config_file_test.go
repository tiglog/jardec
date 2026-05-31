package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfigReadsYAMLFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, DefaultConfigFileName)
	err := os.WriteFile(path, []byte("jadx_path: /tools/jadx\ncfr_path: /tools/cfr\ndefault_retry_concurrency: 9\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadProjectConfig(dir)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}

	if cfg.JadxPath != "/tools/jadx" {
		t.Fatalf("JadxPath = %q, want /tools/jadx", cfg.JadxPath)
	}
	if cfg.CfrPath != "/tools/cfr" {
		t.Fatalf("CfrPath = %q, want /tools/cfr", cfg.CfrPath)
	}
	if cfg.DefaultRetryConcurrency != 9 {
		t.Fatalf("DefaultRetryConcurrency = %d, want 9", cfg.DefaultRetryConcurrency)
	}
}

func TestLoadProjectConfigMissingFileUsesEmptyConfig(t *testing.T) {
	t.Parallel()

	cfg, err := LoadProjectConfig(t.TempDir())
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}
	if cfg != (ProjectConfig{}) {
		t.Fatalf("config = %#v, want zero value", cfg)
	}
}

func TestLoadProjectConfigFindsNearestParentConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(root, DefaultConfigFileName)
	err := os.WriteFile(path, []byte("jadx_path: /tools/jadx\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadProjectConfig(nested)
	if err != nil {
		t.Fatalf("LoadProjectConfig() error = %v", err)
	}
	if cfg.JadxPath != "/tools/jadx" {
		t.Fatalf("JadxPath = %q, want /tools/jadx", cfg.JadxPath)
	}
}
