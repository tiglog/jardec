package sourcepatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	appcli "jardec/internal/cli"
	"jardec/internal/decompiler"
)

func TestCompilerBuildsExpectedCommandAndClasspath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	writeJavaSource(t, sourcesDir, "com.example.Foo")

	fake := &fakeRunner{
		run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
			if err := os.MkdirAll(filepath.Join(spec.Args[3], "com", "example"), 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(spec.Args[3], "com", "example", "Foo.class"), []byte("compiled"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			return decompiler.RunResult{}, nil
		},
	}

	result, err := Compiler{Runner: fake}.Compile(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:   inputJar,
		SourcesDir:     sourcesDir,
		OutputJarPath:  filepath.Join(dir, "patched.jar"),
		TargetClasses:  []string{"com.example.Foo"},
		JavacPath:      "/tools/javac",
		ExtraClasspath: []string{"/deps/a.jar", "/deps/b.jar"},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	defer os.RemoveAll(result.ClassesDir)

	if fake.spec.Path != "/tools/javac" {
		t.Fatalf("Path = %q, want /tools/javac", fake.spec.Path)
	}
	if len(fake.spec.Args) != 5 {
		t.Fatalf("len(Args) = %d, want 5", len(fake.spec.Args))
	}
	if !slices.Equal(fake.spec.Args[:3], []string{"-cp", strings.Join([]string{inputJar, "/deps/a.jar", "/deps/b.jar"}, string(os.PathListSeparator)), "-d"}) {
		t.Fatalf("Args prefix = %v, want classpath and output flags", fake.spec.Args[:3])
	}
	if !strings.HasSuffix(fake.spec.Args[4], filepath.Join("com", "example", "Foo.java")) {
		t.Fatalf("source arg = %q, want Foo.java path", fake.spec.Args[4])
	}
	if want := []string{inputJar, "/deps/a.jar", "/deps/b.jar"}; !slices.Equal(result.Classpath, want) {
		t.Fatalf("Classpath = %v, want %v", result.Classpath, want)
	}
}

func TestCompilerRejectsMissingTargetSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Compiler{Runner: &fakeRunner{}}.Compile(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    filepath.Join(dir, "src"),
		OutputJarPath: filepath.Join(dir, "patched.jar"),
		TargetClasses: []string{"com.example.Missing"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Compile() error = nil, want missing source error")
	}
	if !strings.Contains(err.Error(), "com.example.Missing") || !strings.Contains(err.Error(), filepath.Join("com", "example", "Missing.java")) {
		t.Fatalf("Compile() error = %v, want target class and expected source path", err)
	}
}

func TestCompilerReturnsDiagnosticsOnFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	writeJavaSource(t, sourcesDir, "com.example.Foo")

	fake := &fakeRunner{
		run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
			return decompiler.RunResult{
				Stdout:   "stdout note",
				Stderr:   "compile error",
				ExitCode: 1,
			}, errors.New("exit status 1")
		},
	}

	result, err := Compiler{Runner: fake}.Compile(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    sourcesDir,
		OutputJarPath: filepath.Join(dir, "patched.jar"),
		TargetClasses: []string{"com.example.Foo"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Compile() error = nil, want compile failure")
	}
	if !strings.Contains(err.Error(), "compile error") {
		t.Fatalf("Compile() error = %v, want diagnostics", err)
	}
	if result.Diagnostics != "compile error\nstdout note" && result.Diagnostics != "stdout note\ncompile error" {
		t.Fatalf("Diagnostics = %q, want combined output", result.Diagnostics)
	}
}

func TestCompilerRejectsMissingTopLevelClassOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	writeJavaSource(t, sourcesDir, "com.example.Foo")

	_, err := Compiler{Runner: &fakeRunner{}}.Compile(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    sourcesDir,
		OutputJarPath: filepath.Join(dir, "patched.jar"),
		TargetClasses: []string{"com.example.Foo"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Compile() error = nil, want missing class output error")
	}
	if !strings.Contains(err.Error(), "com.example.Foo") || !strings.Contains(err.Error(), filepath.Join("com", "example", "Foo.class")) {
		t.Fatalf("Compile() error = %v, want target and missing class output path", err)
	}
}

type fakeRunner struct {
	spec decompiler.CommandSpec
	run  func(decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (f *fakeRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	f.spec = spec
	if f.run == nil {
		return decompiler.RunResult{}, nil
	}
	return f.run(spec)
}

func writeJavaSource(t *testing.T, sourcesDir, binaryName string) {
	t.Helper()

	relativePath := strings.ReplaceAll(binaryName, ".", string(filepath.Separator)) + ".java"
	path := filepath.Join(sourcesDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	packageName := ""
	className := binaryName
	if idx := strings.LastIndex(binaryName, "."); idx >= 0 {
		packageName = binaryName[:idx]
		className = binaryName[idx+1:]
	}
	var source strings.Builder
	if packageName != "" {
		source.WriteString("package ")
		source.WriteString(packageName)
		source.WriteString(";\n")
	}
	source.WriteString("public class ")
	source.WriteString(className)
	source.WriteString(" {}\n")
	if err := os.WriteFile(path, []byte(source.String()), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func TestPathSeparatorExpectation(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" && os.PathListSeparator != ';' {
		t.Fatalf("PathListSeparator = %q, want ';' on windows", os.PathListSeparator)
	}
}
