package sourcepatch

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appcli "jardec/internal/cli"
	"jardec/internal/patch"
	ireport "jardec/internal/report"
)

func TestEngineRunPatchesJarFromCompiledSources(t *testing.T) {
	t.Parallel()

	inputJar := writeJar(t, map[string][]byte{
		"com/example/Foo.class": []byte("old-foo"),
		"config/app.properties": []byte("name=test"),
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	rep, err := (Engine{
		Compiler: fakeCompiler{
			result: CompileResult{
				ClassesDir: classesDir,
				Classpath:  []string{inputJar, "/deps/a.jar"},
			},
		},
		PatchRunner: patch.Engine{},
	}).Run(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:   inputJar,
		SourcesDir:     "src",
		OutputJarPath:  outputJar,
		TargetClasses:  []string{"com.example.Foo"},
		JavacPath:      "/tools/javac",
		ExtraClasspath: []string{"/deps/a.jar"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if rep.Compile == nil || rep.Compile.Status != ireport.StatusSucceeded {
		t.Fatalf("Compile report = %+v, want succeeded", rep.Compile)
	}
	if rep.ReplacedGroups != 1 {
		t.Fatalf("ReplacedGroups = %d, want 1", rep.ReplacedGroups)
	}
	if got := readJar(t, outputJar)["com/example/Foo.class"]; got != string(classFileBytes(t, "com.example.Foo")) {
		t.Fatalf("Foo.class = %q, want replacement bytes", got)
	}
	if got := readJar(t, outputJar)["config/app.properties"]; got != "name=test" {
		t.Fatalf("config/app.properties = %q, want preserved resource", got)
	}
	textReport, err := os.ReadFile(outputJar + ".report.txt")
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	if !strings.Contains(string(textReport), "Compile status: succeeded") {
		t.Fatalf("report text = %q, want compile summary", string(textReport))
	}
}

func TestEngineRunStopsBeforePatchOnCompileFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	outputJar := filepath.Join(dir, "patched.jar")
	patcher := &fakePatchRunner{}

	rep, err := (Engine{
		Compiler: fakeCompiler{
			result: CompileResult{
				Classpath:   []string{inputJar},
				Diagnostics: "Foo.java:1: error",
			},
			err: errors.New("compile Java sources: exit status 1: Foo.java:1: error"),
		},
		PatchRunner: patcher,
	}).Run(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    "src",
		OutputJarPath: outputJar,
		TargetClasses: []string{"com.example.Foo"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Run() error = nil, want compile failure")
	}
	if patcher.called {
		t.Fatal("patch runner was called despite compile failure")
	}
	if rep.Compile == nil || rep.Compile.Status != ireport.StatusFailed {
		t.Fatalf("Compile report = %+v, want failed", rep.Compile)
	}
	if _, statErr := os.Stat(outputJar); !os.IsNotExist(statErr) {
		t.Fatalf("expected no patched jar, got stat err=%v", statErr)
	}
	if _, statErr := os.Stat(outputJar + ".report.json"); statErr != nil {
		t.Fatalf("expected report file, got %v", statErr)
	}
}

func TestEngineRunStopsBeforePatchWhenCompiledOutputMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := filepath.Join(dir, "app.jar")
	if err := os.WriteFile(inputJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	sourcesDir := filepath.Join(dir, "src")
	writeJavaSource(t, sourcesDir, "com.example.Foo")
	patcher := &fakePatchRunner{}

	_, err := (Engine{
		Compiler:    Compiler{Runner: &fakeRunner{}},
		PatchRunner: patcher,
	}).Run(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    sourcesDir,
		OutputJarPath: filepath.Join(dir, "patched.jar"),
		TargetClasses: []string{"com.example.Foo"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Run() error = nil, want missing compiled output error")
	}
	if patcher.called {
		t.Fatal("patch runner was called despite missing compiled output")
	}
	if !strings.Contains(err.Error(), "compiled output for com.example.Foo is missing") {
		t.Fatalf("Run() error = %v, want missing compiled output message", err)
	}
}

func TestEngineRunWritesPatchFailureReportBesideRequestedOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputJar := writeJar(t, map[string][]byte{
		"com/example/Foo.class": []byte("old-foo"),
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Bar.class", "com.example.Bar")
	outputJar := filepath.Join(dir, "patched.jar")

	rep, err := (Engine{
		Compiler: fakeCompiler{
			result: CompileResult{
				ClassesDir: classesDir,
				Classpath:  []string{inputJar},
			},
		},
		PatchRunner: &fakePatchRunner{
			err: errors.New("target class does not exist in input jar: com.example.Bar"),
		},
	}).Run(context.Background(), appcli.SourcePatchConfig{
		InputJarPath:  inputJar,
		SourcesDir:    "src",
		OutputJarPath: outputJar,
		TargetClasses: []string{"com.example.Bar"},
		JavacPath:     "/tools/javac",
	})
	if err == nil {
		t.Fatal("Run() error = nil, want patch failure")
	}
	if rep.InputJar != inputJar {
		t.Fatalf("InputJar = %q, want %q", rep.InputJar, inputJar)
	}
	if rep.OutputJar != outputJar {
		t.Fatalf("OutputJar = %q, want %q", rep.OutputJar, outputJar)
	}
	if _, statErr := os.Stat(outputJar + ".report.json"); statErr != nil {
		t.Fatalf("expected report json beside output jar, got %v", statErr)
	}
	if _, statErr := os.Stat(outputJar + ".report.txt"); statErr != nil {
		t.Fatalf("expected report text beside output jar, got %v", statErr)
	}
}

type fakeCompiler struct {
	result CompileResult
	err    error
}

func (f fakeCompiler) Compile(context.Context, appcli.SourcePatchConfig) (CompileResult, error) {
	return f.result, f.err
}

type fakePatchRunner struct {
	called bool
	cfg    appcli.PatchConfig
	rep    ireport.PatchReport
	err    error
}

func (f *fakePatchRunner) Run(_ context.Context, cfg appcli.PatchConfig) (ireport.PatchReport, error) {
	f.called = true
	f.cfg = cfg
	return f.rep, f.err
}

func writeJar(t *testing.T, entries map[string][]byte) string {
	t.Helper()

	jarPath := filepath.Join(t.TempDir(), "input.jar")
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return jarPath
}

func readJar(t *testing.T, jarPath string) map[string]string {
	t.Helper()

	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer reader.Close()

	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("Open(%q) error = %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("ReadAll(%q) error = %v", file.Name, err)
		}
		entries[file.Name] = string(data)
	}
	return entries
}

func writeDeclaredClassFile(t *testing.T, root, relativePath, declaredBinaryName string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, classFileBytes(t, declaredBinaryName), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func classFileBytes(t *testing.T, binaryName string) []byte {
	t.Helper()

	internalName := strings.ReplaceAll(binaryName, ".", "/")
	superName := "java/lang/Object"

	var buf bytes.Buffer
	writeU4(t, &buf, 0xCAFEBABE)
	writeU2(t, &buf, 0)
	writeU2(t, &buf, 52)
	writeU2(t, &buf, 5)

	buf.WriteByte(7)
	writeU2(t, &buf, 3)
	buf.WriteByte(7)
	writeU2(t, &buf, 4)
	writeUTF8(t, &buf, internalName)
	writeUTF8(t, &buf, superName)

	writeU2(t, &buf, 0x0021)
	writeU2(t, &buf, 1)
	writeU2(t, &buf, 2)
	writeU2(t, &buf, 0)
	writeU2(t, &buf, 0)
	writeU2(t, &buf, 0)
	writeU2(t, &buf, 0)

	return buf.Bytes()
}

func writeU2(t *testing.T, buf *bytes.Buffer, value uint16) {
	t.Helper()
	var raw [2]byte
	binary.BigEndian.PutUint16(raw[:], value)
	if _, err := buf.Write(raw[:]); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func writeU4(t *testing.T, buf *bytes.Buffer, value uint32) {
	t.Helper()
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	if _, err := buf.Write(raw[:]); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func writeUTF8(t *testing.T, buf *bytes.Buffer, value string) {
	t.Helper()
	buf.WriteByte(1)
	writeU2(t, buf, uint16(len(value)))
	if _, err := buf.WriteString(value); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
}
