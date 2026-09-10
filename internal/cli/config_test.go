package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestValidateConfigExpandsConfigRelativeClasspathDirectory(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	depsDir := filepath.Join(configDir, "libs")
	mustMkdirAll(t, depsDir)
	mustWriteFile(t, filepath.Join(depsDir, "b.jar"))
	mustWriteFile(t, filepath.Join(depsDir, "a.JAR"))
	mustWriteFile(t, filepath.Join(depsDir, "notes.txt"))
	mustMkdirAll(t, filepath.Join(depsDir, "nested"))
	mustWriteFile(t, filepath.Join(depsDir, "nested", "ignored.jar"))

	cfg := ApplyProjectConfig(Config{
		InputPath:      "sample.jar",
		OutputDir:      "out",
		JadxPath:       "/tools/jadx",
		ProcyonPath:    "/tools/procyon",
		TempDir:        "/tmp/jardec",
		KeepTemp:       true,
		ExtraClasspath: []string{filepath.Join(configDir, "cli.jar")},
	}, ProjectConfig{
		DecompileClasspath: []string{"libs"},
		ConfigDir:          configDir,
	})
	mustWriteFile(t, filepath.Join(configDir, "cli.jar"))

	validated, err := ValidateConfig(cfg, func(name string) (string, error) { return name, nil })
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}

	if want := []string{
		filepath.Join(configDir, "libs", "a.JAR"),
		filepath.Join(configDir, "libs", "b.jar"),
		filepath.Join(configDir, "cli.jar"),
	}; !slices.Equal(validated.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", validated.ExtraClasspath, want)
	}
}

func TestValidateConfigRejectsClasspathDirectoryWithoutJars(t *testing.T) {
	t.Parallel()

	emptyDir := filepath.Join(t.TempDir(), "empty")
	mustMkdirAll(t, emptyDir)

	_, err := ValidateConfig(Config{
		InputPath:      "sample.jar",
		OutputDir:      "out",
		JadxPath:       "/tools/jadx",
		ProcyonPath:    "/tools/procyon",
		ExtraClasspath: []string{emptyDir},
	}, func(name string) (string, error) { return name, nil })
	if err == nil {
		t.Fatal("ValidateConfig() error = nil, want empty classpath directory error")
	}
	if !strings.Contains(err.Error(), "classpath directory contains no jar files") {
		t.Fatalf("error = %q, want empty classpath directory message", err.Error())
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestValidateConfigRejectsInvalidClasspathFileEntry(t *testing.T) {
	t.Parallel()

	nonJar := filepath.Join(t.TempDir(), "notes.txt")
	mustWriteFile(t, nonJar)
	for _, entry := range []string{filepath.Join(t.TempDir(), "missing.jar"), nonJar} {
		_, err := ValidateConfig(Config{
			InputPath: "sample.jar", OutputDir: "out", JadxPath: "/tools/jadx", ProcyonPath: "/tools/procyon", ExtraClasspath: []string{entry},
		}, func(name string) (string, error) { return name, nil })
		if err == nil || !strings.Contains(err.Error(), entry) {
			t.Fatalf("ValidateConfig(%q) error = %v, want error containing entry", entry, err)
		}
	}
}

func TestValidateConfigAcceptsSingleJarClasspathEntry(t *testing.T) {
	t.Parallel()

	entry := filepath.Join(t.TempDir(), "valid.JAR")
	mustWriteFile(t, entry)
	cfg, err := ValidateConfig(Config{
		InputPath: "sample.jar", OutputDir: "out", JadxPath: "/tools/jadx", ProcyonPath: "/tools/procyon", ExtraClasspath: []string{entry},
	}, func(name string) (string, error) { return name, nil })
	if err != nil {
		t.Fatalf("ValidateConfig() error = %v", err)
	}
	if !slices.Equal(cfg.ExtraClasspath, []string{entry}) {
		t.Fatalf("ExtraClasspath = %v, want [%s]", cfg.ExtraClasspath, entry)
	}
}
