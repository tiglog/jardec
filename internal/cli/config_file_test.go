package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadProjectConfigReadsYAMLFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, DefaultConfigFileName)
	err := os.WriteFile(path, []byte("jadx_path: /tools/jadx\nprocyon_path: /tools/procyon.jar\njavac_path: /tools/javac\ndecompile_classpath:\n  - deps/runtime.jar\n  - /deps/external.jar\npatch_sources_classpath:\n  - /deps/base.jar\n  - /deps/extra.jar\ndefault_retry_concurrency: 9\n"), 0o644)
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
	if cfg.ProcyonPath != "/tools/procyon.jar" {
		t.Fatalf("ProcyonPath = %q, want /tools/procyon.jar", cfg.ProcyonPath)
	}
	if cfg.JavacPath != "/tools/javac" {
		t.Fatalf("JavacPath = %q, want /tools/javac", cfg.JavacPath)
	}
	if want := []string{"deps/runtime.jar", "/deps/external.jar"}; !slices.Equal(cfg.DecompileClasspath, want) {
		t.Fatalf("DecompileClasspath = %v, want %v", cfg.DecompileClasspath, want)
	}
	if want := []string{"/deps/base.jar", "/deps/extra.jar"}; !slices.Equal(cfg.PatchSourcesClasspath, want) {
		t.Fatalf("PatchSourcesClasspath = %v, want %v", cfg.PatchSourcesClasspath, want)
	}
	if cfg.ConfigDir != dir {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, dir)
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
	if cfg.JadxPath != "" || cfg.ProcyonPath != "" || cfg.JavacPath != "" || len(cfg.DecompileClasspath) != 0 || len(cfg.PatchSourcesClasspath) != 0 || cfg.DefaultRetryConcurrency != 0 || cfg.ConfigDir != "" {
		t.Fatalf("config = %#v, want zero-value fields", cfg)
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
	if cfg.ConfigDir != root {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, root)
	}
}

func TestLoadProjectConfigFromPathReadsExplicitFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "custom.yaml")
	err := os.WriteFile(configPath, []byte("jadx_path: /custom/jadx\nprocyon_path: /custom/procyon\ndecompile_classpath:\n  - /custom/lib.jar\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadProjectConfigFromPath(configPath)
	if err != nil {
		t.Fatalf("LoadProjectConfigFromPath() error = %v", err)
	}

	if cfg.JadxPath != "/custom/jadx" {
		t.Fatalf("JadxPath = %q, want /custom/jadx", cfg.JadxPath)
	}
	if cfg.ProcyonPath != "/custom/procyon" {
		t.Fatalf("ProcyonPath = %q, want /custom/procyon", cfg.ProcyonPath)
	}
	if want := []string{"/custom/lib.jar"}; !slices.Equal(cfg.DecompileClasspath, want) {
		t.Fatalf("DecompileClasspath = %v, want %v", cfg.DecompileClasspath, want)
	}
	if cfg.ConfigDir != dir {
		t.Fatalf("ConfigDir = %q, want %q", cfg.ConfigDir, dir)
	}
}

func TestLoadProjectConfigFromPathMissingFileReturnsError(t *testing.T) {
	t.Parallel()

	_, err := LoadProjectConfigFromPath(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("LoadProjectConfigFromPath() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("error = %q, want 'config file not found'", err.Error())
	}
}
