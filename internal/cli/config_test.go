package cli

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestApplyProjectConfigRebasesRelativeDecompileClasspath(t *testing.T) {
	t.Parallel()

	configDir := filepath.Join(string(filepath.Separator), "repo")
	cfg := ApplyProjectConfig(Config{}, ProjectConfig{
		DecompileClasspath: []string{filepath.Join("libs", "base.jar"), filepath.Join("libs", "shared.jar")},
		ConfigDir:          configDir,
	})

	if want := []string{
		filepath.Join(configDir, "libs", "base.jar"),
		filepath.Join(configDir, "libs", "shared.jar"),
	}; !slices.Equal(cfg.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", cfg.ExtraClasspath, want)
	}
}

func TestApplyProjectConfigAppendsCLIClasspathAfterConfig(t *testing.T) {
	t.Parallel()

	cfg := ApplyProjectConfig(Config{
		ExtraClasspath: []string{"/deps/shared.jar", "/deps/cli.jar"},
	}, ProjectConfig{
		DecompileClasspath: []string{"/deps/base.jar", "/deps/shared.jar"},
	})

	if want := []string{"/deps/base.jar", "/deps/shared.jar", "/deps/cli.jar"}; !slices.Equal(cfg.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", cfg.ExtraClasspath, want)
	}
}
