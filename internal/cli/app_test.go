package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNewAppParsesOptionsIntoConfig(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/tools/jadx",
		"--cfr-path", "/tools/cfr",
		"--temp-dir", "/tmp/jardec",
		"--keep-temp",
		"--retry-concurrency", "5",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.InputPath != "sample.jar" {
		t.Fatalf("InputPath = %q, want sample.jar", got.InputPath)
	}
	if got.OutputDir != "out" {
		t.Fatalf("OutputDir = %q, want out", got.OutputDir)
	}
	if got.JadxPath != "/tools/jadx" {
		t.Fatalf("JadxPath = %q, want /tools/jadx", got.JadxPath)
	}
	if got.CfrPath != "/tools/cfr" {
		t.Fatalf("CfrPath = %q, want /tools/cfr", got.CfrPath)
	}
	if got.TempDir != "/tmp/jardec" {
		t.Fatalf("TempDir = %q, want /tmp/jardec", got.TempDir)
	}
	if !got.KeepTemp {
		t.Fatal("KeepTemp = false, want true")
	}
	if got.RetryConcurrency != 5 {
		t.Fatalf("RetryConcurrency = %d, want 5", got.RetryConcurrency)
	}
}

func TestNewAppRejectsMissingRequiredOptions(t *testing.T) {
	t.Parallel()

	called := false
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		called = true
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{"jardec", "--output", "out"})
	if err == nil {
		t.Fatal("RunContext() error = nil, want validation error")
	}
	if called {
		t.Fatal("run callback was called despite validation failure")
	}
}

func TestRootCommandDoesNotRunDecompile(t *testing.T) {
	t.Parallel()

	called := false
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		called = true
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{"jardec", "--input", "sample.jar", "--output", "out"})
	if err == nil {
		t.Fatal("RunContext() error = nil, want root command rejection")
	}
	if called {
		t.Fatal("run callback was called from root command")
	}
}

func TestNewAppUsesExplicitBinaryOverrides(t *testing.T) {
	t.Parallel()

	var lookedUp []string
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		return nil
	}, nil, func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/custom/jadx",
		"--cfr-path", "/custom/cfr",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	want := []string{"/custom/jadx", "/custom/cfr"}
	if !slices.Equal(lookedUp, want) {
		t.Fatalf("lookup calls = %v, want %v", lookedUp, want)
	}
}

func TestNewAppReportsBinaryLookupFailures(t *testing.T) {
	t.Parallel()

	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		return nil
	}, nil, func(name string) (string, error) {
		if name == "jadx" {
			return "", errors.New("not found")
		}
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err == nil {
		t.Fatal("RunContext() error = nil, want lookup error")
	}
}

func TestNewAppUsesConfigFileDefaults(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			CfrPath:                 "/config/cfr",
			DefaultRetryConcurrency: 7,
		}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/config/jadx" {
		t.Fatalf("JadxPath = %q, want /config/jadx", got.JadxPath)
	}
	if got.CfrPath != "/config/cfr" {
		t.Fatalf("CfrPath = %q, want /config/cfr", got.CfrPath)
	}
	if got.RetryConcurrency != 7 {
		t.Fatalf("RetryConcurrency = %d, want 7", got.RetryConcurrency)
	}
}

func TestNewAppFlagsOverrideConfigFileDefaults(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			CfrPath:                 "/config/cfr",
			DefaultRetryConcurrency: 7,
		}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/flag/jadx",
		"--cfr-path", "/flag/cfr",
		"--retry-concurrency", "3",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/flag/jadx" {
		t.Fatalf("JadxPath = %q, want /flag/jadx", got.JadxPath)
	}
	if got.CfrPath != "/flag/cfr" {
		t.Fatalf("CfrPath = %q, want /flag/cfr", got.CfrPath)
	}
	if got.RetryConcurrency != 3 {
		t.Fatalf("RetryConcurrency = %d, want 3", got.RetryConcurrency)
	}
}

func TestNewAppSupportsDirectCFRJarPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfrJar := filepath.Join(dir, "cfr.jar")
	if err := os.WriteFile(cfrJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		if name == "java" {
			return "/usr/bin/java", nil
		}
		if name == "jadx" {
			return "/usr/bin/jadx", nil
		}
		return "", errors.New("unexpected lookup")
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
		"--cfr-path", cfrJar,
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}
	if got.CfrPath != cfrJar {
		t.Fatalf("CfrPath = %q, want %q", got.CfrPath, cfrJar)
	}
}

func TestPatchClassesCommandParsesOptionsIntoPatchConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	classesDir := filepath.Join(dir, "classes")
	if err := os.MkdirAll(classesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	var got PatchConfig
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-classes")
		return nil
	}, func(_ context.Context, cfg PatchConfig) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"patch-classes",
		"--input-jar", inputJar,
		"--classes-dir", classesDir,
		"--output-jar", filepath.Join(dir, "patched.jar"),
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.InputJarPath != inputJar {
		t.Fatalf("InputJarPath = %q, want %q", got.InputJarPath, inputJar)
	}
	if got.ClassesDir != classesDir {
		t.Fatalf("ClassesDir = %q, want %q", got.ClassesDir, classesDir)
	}
	if got.OutputJarPath != filepath.Join(dir, "patched.jar") {
		t.Fatalf("OutputJarPath = %q, want %q", got.OutputJarPath, filepath.Join(dir, "patched.jar"))
	}
}

func TestPatchClassesCommandParsesPlanningOptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	classesDir := filepath.Join(dir, "classes")
	if err := os.MkdirAll(classesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	var got PatchConfig
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-classes")
		return nil
	}, func(_ context.Context, cfg PatchConfig) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"patch-classes",
		"--input-jar", inputJar,
		"--classes-dir", classesDir,
		"--output-jar", filepath.Join(dir, "patched.jar"),
		"--dry-run",
		"--class", "com.example.Foo",
		"--class", "com.example.Bar",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if !got.DryRun {
		t.Fatal("DryRun = false, want true")
	}
	if want := []string{"com.example.Foo", "com.example.Bar"}; !slices.Equal(got.TargetClasses, want) {
		t.Fatalf("TargetClasses = %v, want %v", got.TargetClasses, want)
	}
}

func TestPatchClassesCommandRejectsMissingRequiredOptions(t *testing.T) {
	t.Parallel()

	called := false
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-classes")
		return nil
	}, func(_ context.Context, _ PatchConfig) error {
		called = true
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{"jardec", "patch-classes", "--input-jar", "sample.jar"})
	if err == nil {
		t.Fatal("RunContext() error = nil, want validation error")
	}
	if called {
		t.Fatal("patch callback was called despite validation failure")
	}
}

func TestPatchClassesCommandRejectsInvalidInputPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	classesDir := filepath.Join(dir, "classes")
	if err := os.MkdirAll(classesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	outputDir := filepath.Join(dir, "patched.jar")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-classes")
		return nil
	}, func(_ context.Context, _ PatchConfig) error {
		t.Fatal("patch callback should not be called on invalid paths")
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing input jar",
			args: []string{"jardec", "patch-classes", "--input-jar", filepath.Join(dir, "missing.jar"), "--classes-dir", classesDir, "--output-jar", filepath.Join(dir, "out.jar")},
			want: "input jar does not exist",
		},
		{
			name: "classes dir is file",
			args: []string{"jardec", "patch-classes", "--input-jar", inputJar, "--classes-dir", inputJar, "--output-jar", filepath.Join(dir, "out.jar")},
			want: "classes directory is not a directory",
		},
		{
			name: "output path is directory",
			args: []string{"jardec", "patch-classes", "--input-jar", inputJar, "--classes-dir", classesDir, "--output-jar", outputDir},
			want: "output jar path must be a file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := app.RunContext(context.Background(), tt.args)
			if err == nil {
				t.Fatal("RunContext() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunContext() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestPatchSourcesCommandParsesOptionsIntoSourcePatchConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(sourcesDir, "com", "example"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcesDir, "com", "example", "Foo.java"), []byte("class Foo {}"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got SourcePatchConfig
	app := newSourcePatchAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-sources")
		return nil
	}, nil, func(_ context.Context, cfg SourcePatchConfig) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"patch-sources",
		"--input-jar", inputJar,
		"--sources-dir", sourcesDir,
		"--output-jar", filepath.Join(dir, "patched.jar"),
		"--class", "com.example.Foo",
		"--javac-path", "/tools/javac",
		"--classpath", "/deps/a.jar",
		"--classpath", "/deps/b.jar",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.InputJarPath != inputJar {
		t.Fatalf("InputJarPath = %q, want %q", got.InputJarPath, inputJar)
	}
	if got.SourcesDir != sourcesDir {
		t.Fatalf("SourcesDir = %q, want %q", got.SourcesDir, sourcesDir)
	}
	if got.OutputJarPath != filepath.Join(dir, "patched.jar") {
		t.Fatalf("OutputJarPath = %q, want %q", got.OutputJarPath, filepath.Join(dir, "patched.jar"))
	}
	if want := []string{"com.example.Foo"}; !slices.Equal(got.TargetClasses, want) {
		t.Fatalf("TargetClasses = %v, want %v", got.TargetClasses, want)
	}
	if got.JavacPath != "/tools/javac" {
		t.Fatalf("JavacPath = %q, want /tools/javac", got.JavacPath)
	}
	if want := []string{"/deps/a.jar", "/deps/b.jar"}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestPatchSourcesCommandRejectsMissingTargetClass(t *testing.T) {
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

	called := false
	app := newSourcePatchAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-sources")
		return nil
	}, nil, func(_ context.Context, _ SourcePatchConfig) error {
		called = true
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"patch-sources",
		"--input-jar", inputJar,
		"--sources-dir", sourcesDir,
		"--output-jar", filepath.Join(dir, "patched.jar"),
	})
	if err == nil {
		t.Fatal("RunContext() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "at least one class target is required") {
		t.Fatalf("RunContext() error = %v, want class target validation", err)
	}
	if called {
		t.Fatal("source patch callback was called despite validation failure")
	}
}

func TestPatchSourcesCommandUsesConfigFileJavacDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "sample.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(sourcesDir, "com", "example"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcesDir, "com", "example", "Foo.java"), []byte("class Foo {}"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got SourcePatchConfig
	app := newSourcePatchAppWithDeps(func(_ context.Context, _ Config) error {
		t.Fatal("decompile callback should not be called for patch-sources")
		return nil
	}, nil, func(_ context.Context, cfg SourcePatchConfig) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{JavacPath: "/config/javac"}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"patch-sources",
		"--input-jar", inputJar,
		"--sources-dir", sourcesDir,
		"--output-jar", filepath.Join(dir, "patched.jar"),
		"--class", "com.example.Foo",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}
	if got.JavacPath != "/config/javac" {
		t.Fatalf("JavacPath = %q, want /config/javac", got.JavacPath)
	}
}
