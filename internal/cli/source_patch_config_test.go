package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestApplySourcePatchProjectConfigUsesDefaultClasspath(t *testing.T) {
	t.Parallel()

	cfg := ApplySourcePatchProjectConfig(SourcePatchConfig{}, ProjectConfig{
		JavacPath:             "/config/javac",
		DecompileClasspath: []string{"/deps/base.jar", "/deps/shared.jar"},
	})

	if cfg.JavacPath != "/config/javac" {
		t.Fatalf("JavacPath = %q, want /config/javac", cfg.JavacPath)
	}
	if want := []string{"/deps/base.jar", "/deps/shared.jar"}; !slices.Equal(cfg.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", cfg.ExtraClasspath, want)
	}
}

func TestApplySourcePatchProjectConfigRebasesRelativeConfigEntries(t *testing.T) {
	t.Parallel()

	cfgDir := filepath.Join(string(filepath.Separator), "repo")
	cfg := ApplySourcePatchProjectConfig(SourcePatchConfig{}, ProjectConfig{
		JavacPath:             filepath.Join("tools", "javac-wrapper"),
		DecompileClasspath: []string{filepath.Join("libs", "base.jar"), filepath.Join("libs", "shared.jar")},
		ConfigDir:             cfgDir,
	})

	if want := filepath.Join(cfgDir, "tools", "javac-wrapper"); cfg.JavacPath != want {
		t.Fatalf("JavacPath = %q, want %q", cfg.JavacPath, want)
	}
	if want := []string{
		filepath.Join(cfgDir, "libs", "base.jar"),
		filepath.Join(cfgDir, "libs", "shared.jar"),
	}; !slices.Equal(cfg.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", cfg.ExtraClasspath, want)
	}
}

func TestApplySourcePatchProjectConfigAppendsCLIClasspathAfterConfig(t *testing.T) {
	t.Parallel()

	cfg := ApplySourcePatchProjectConfig(SourcePatchConfig{
		JavacPath:      "/cli/javac",
		ExtraClasspath: []string{"/deps/shared.jar", "/deps/cli.jar"},
	}, ProjectConfig{
		JavacPath:             "/config/javac",
		DecompileClasspath: []string{"/deps/base.jar", "/deps/shared.jar"},
	})

	if cfg.JavacPath != "/cli/javac" {
		t.Fatalf("JavacPath = %q, want /cli/javac", cfg.JavacPath)
	}
	if want := []string{"/deps/base.jar", "/deps/shared.jar", "/deps/cli.jar"}; !slices.Equal(cfg.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", cfg.ExtraClasspath, want)
	}
}

func TestValidateSourcePatchConfigRejectsNestedTargetClass(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	_, err := ValidateSourcePatchConfig(SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    sourcesDir,
		OutputJarPath: filepath.Join(dir, "patched.jar"),
		TargetClasses: []string{"com.example.Foo$Inner"},
		JavacPath:     "/tools/javac",
	}, func(string) (string, error) {
		return "/tools/javac", nil
	})
	if err == nil {
		t.Fatal("ValidateSourcePatchConfig() error = nil, want nested target rejection")
	}
	if got := err.Error(); got != "class target must be a top-level binary name: com.example.Foo$Inner" {
		t.Fatalf("ValidateSourcePatchConfig() error = %q, want explicit top-level error", got)
	}
}
