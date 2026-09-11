package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNewAppDisplaysVersionWithoutRunningCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		version     string
		wantVersion string
	}{
		{
			name:        "long flag uses development fallback",
			args:        []string{"jardec", "--version"},
			wantVersion: "dev",
		},
		{
			name:        "short flag uses supplied version",
			args:        []string{"jardec", "-v"},
			version:     "v1.2.3",
			wantVersion: "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			app := newAppWithDeps(func(context.Context, Config) error {
				called = true
				return nil
			}, nil, func(name string) (string, error) {
				return "/resolved/" + name, nil
			}, func() (ProjectConfig, error) {
				return ProjectConfig{}, nil
			})
			if tt.version != "" {
				app.Version = tt.version
			}

			var output bytes.Buffer
			app.Writer = &output
			if err := app.RunContext(context.Background(), tt.args); err != nil {
				t.Fatalf("RunContext() error = %v", err)
			}
			if called {
				t.Fatal("decompile callback was called by version display")
			}
			if got := output.String(); !strings.Contains(got, tt.wantVersion) {
				t.Fatalf("version output = %q, want it to contain %q", got, tt.wantVersion)
			}
		})
	}
}

func TestNewAppParsesOptionsIntoConfig(t *testing.T) {
	t.Parallel()
	depsDir := t.TempDir()
	baseJar := filepath.Join(depsDir, "base.jar")
	extraJar := filepath.Join(depsDir, "extra.jar")
	procyonJar := filepath.Join(depsDir, "procyon.jar")
	for _, jar := range []string{baseJar, extraJar, procyonJar} {
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

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
		"--procyon-path", procyonJar,
		"--classpath", baseJar,
		"--classpath", extraJar,
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
	if got.ProcyonPath != procyonJar {
		t.Fatalf("ProcyonPath = %q, want %q", got.ProcyonPath, procyonJar)
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
	if want := []string{baseJar, extraJar}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestDecompileHelpDescribesProcyonJarRequirement(t *testing.T) {
	t.Parallel()

	app := newAppWithDeps(nil, nil, nil, nil)
	var output bytes.Buffer
	app.Writer = &output

	if err := app.RunContext(context.Background(), []string{"jardec", "decompile", "--help"}); err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}
	if !strings.Contains(output.String(), "Path to the readable Procyon JAR required for fallback") {
		t.Fatalf("help output = %q, want Procyon JAR requirement", output.String())
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

	procyonJar := filepath.Join(t.TempDir(), "procyon.jar")
	if err := os.WriteFile(procyonJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
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
		"--procyon-path", procyonJar,
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	want := []string{"/custom/jadx", "java"}
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
	depsDir := t.TempDir()
	baseJar := filepath.Join(depsDir, "base.jar")
	sharedJar := filepath.Join(depsDir, "shared.jar")
	procyonJar := filepath.Join(depsDir, "procyon.jar")
	for _, jar := range []string{baseJar, sharedJar, procyonJar} {
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			ProcyonPath:             procyonJar,
			DecompileClasspath:      []string{baseJar, sharedJar},
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
	if got.ProcyonPath != procyonJar {
		t.Fatalf("ProcyonPath = %q, want %q", got.ProcyonPath, procyonJar)
	}
	if got.RetryConcurrency != 7 {
		t.Fatalf("RetryConcurrency = %d, want 7", got.RetryConcurrency)
	}
	if want := []string{baseJar, sharedJar}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestNewAppFlagsOverrideConfigFileDefaults(t *testing.T) {
	t.Parallel()
	depsDir := t.TempDir()
	baseJar := filepath.Join(depsDir, "base.jar")
	sharedJar := filepath.Join(depsDir, "shared.jar")
	cliJar := filepath.Join(depsDir, "cli.jar")
	configProcyonJar := filepath.Join(depsDir, "config-procyon.jar")
	flagProcyonJar := filepath.Join(depsDir, "flag-procyon.jar")
	for _, jar := range []string{baseJar, sharedJar, cliJar, configProcyonJar, flagProcyonJar} {
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			ProcyonPath:             configProcyonJar,
			DecompileClasspath:      []string{baseJar, sharedJar},
			DefaultRetryConcurrency: 7,
		}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/flag/jadx",
		"--procyon-path", flagProcyonJar,
		"--classpath", sharedJar,
		"--classpath", cliJar,
		"--retry-concurrency", "3",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/flag/jadx" {
		t.Fatalf("JadxPath = %q, want /flag/jadx", got.JadxPath)
	}
	if got.ProcyonPath != flagProcyonJar {
		t.Fatalf("ProcyonPath = %q, want %q", got.ProcyonPath, flagProcyonJar)
	}
	if got.RetryConcurrency != 3 {
		t.Fatalf("RetryConcurrency = %d, want 3", got.RetryConcurrency)
	}
	if want := []string{baseJar, sharedJar, cliJar}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestNewAppExpandsClasspathDirectoryFlags(t *testing.T) {
	t.Parallel()

	depsDir := filepath.Join(t.TempDir(), "deps")
	if err := os.MkdirAll(filepath.Join(depsDir, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	for _, name := range []string{"b.jar", "a.jar", "nested/ignored.jar", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(depsDir, filepath.FromSlash(name)), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}
	explicitJar := filepath.Join(depsDir, "explicit.jar")
	if err := os.WriteFile(explicitJar, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	procyonJar := filepath.Join(t.TempDir(), "procyon.jar")
	if err := os.WriteFile(procyonJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

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
		"--procyon-path", procyonJar,
		"--classpath", depsDir,
		"--classpath", explicitJar,
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if want := []string{
		filepath.Join(depsDir, "a.jar"),
		filepath.Join(depsDir, "b.jar"),
		explicitJar,
	}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestNewAppSupportsDirectProcyonJarPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	vfJar := filepath.Join(dir, "procyon.jar")
	if err := os.WriteFile(vfJar, []byte("jar"), 0o644); err != nil {
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
		"--procyon-path", vfJar,
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}
	if got.ProcyonPath != vfJar {
		t.Fatalf("ProcyonPath = %q, want %q", got.ProcyonPath, vfJar)
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
	classpathA := filepath.Join(dir, "a.jar")
	classpathB := filepath.Join(dir, "b.jar")
	for _, jar := range []string{classpathA, classpathB} {
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
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
		"--classpath", classpathA,
		"--classpath", classpathB,
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
	if want := []string{classpathA, classpathB}; !slices.Equal(got.ExtraClasspath, want) {
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

func TestNewAppUsesExplicitConfigFlagForDecompile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	libJar := filepath.Join(dir, "lib.jar")
	procyonJar := filepath.Join(dir, "procyon.jar")
	if err := os.WriteFile(libJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(procyonJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	configPath := filepath.Join(dir, "prod.yaml")
	err := os.WriteFile(configPath, []byte(fmt.Sprintf("jadx_path: /explicit/jadx\nprocyon_path: %s\ndecompile_classpath:\n  - %s\n", procyonJar, libJar)), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err = app.RunContext(context.Background(), []string{
		"jardec",
		"--config", configPath,
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/explicit/jadx" {
		t.Fatalf("JadxPath = %q, want /explicit/jadx", got.JadxPath)
	}
	if got.ProcyonPath != procyonJar {
		t.Fatalf("ProcyonPath = %q, want %q", got.ProcyonPath, procyonJar)
	}
	if want := []string{libJar}; !slices.Equal(got.ExtraClasspath, want) {
		t.Fatalf("ExtraClasspath = %v, want %v", got.ExtraClasspath, want)
	}
}

func TestNewAppConfigFlagOverridesProjectConfigDiscovery(t *testing.T) {
	t.Parallel()

	// Create a config in a separate directory that should NOT be found by normal discovery.
	explicitDir := t.TempDir()
	procyonJar := filepath.Join(explicitDir, "procyon.jar")
	if err := os.WriteFile(procyonJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	configPath := filepath.Join(explicitDir, "explicit.yaml")
	err := os.WriteFile(configPath, []byte(fmt.Sprintf("jadx_path: /explicit/jadx\nprocyon_path: %s\n", procyonJar)), 0o644)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, nil, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		// Simulate the default loader returning empty config (no config.yaml found in cwd tree).
		return ProjectConfig{}, nil
	})

	// Run from a temp dir that has no config.yaml, but use --config to point to the explicit one.
	err = app.RunContext(context.Background(), []string{
		"jardec",
		"--config", configPath,
		"decompile",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/explicit/jadx" {
		t.Fatalf("JadxPath = %q, want /explicit/jadx", got.JadxPath)
	}
}
