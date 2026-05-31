package pipeline

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"jardec/internal/decompiler"
	jarpkg "jardec/internal/jar"
)

func TestExecuteCFRRetriesCreatesIsolatedWorkspaces(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
		"com/example/Bar.class": "bar",
	})

	fake := &fakeCfrRunner{
		run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
			classFile := spec.Args[0]
			outputDir := spec.Args[2]
			if _, err := os.Stat(classFile); err != nil {
				t.Fatalf("expected extracted class file: %v", err)
			}
			relativeClass, err := filepath.Rel(filepath.Dir(filepath.Dir(classFile)), classFile)
			if err != nil {
				t.Fatalf("Rel() error = %v", err)
			}
			javaPath := filepath.Join(outputDir, relativeClass[:len(relativeClass)-len(".class")]+".java")
			if err := os.MkdirAll(filepath.Dir(javaPath), 0o755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(javaPath, []byte("class ok {}\n"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			return decompiler.RunResult{Stdout: "cfr ok"}, nil
		},
	}

	results, err := ExecuteCFRRetries(context.Background(), fake, CfrRetryConfig{
		BaseTempDir: t.TempDir(),
		CfrPath:     "/tools/cfr",
		InputJar:    jarPath,
		Concurrency: 2,
	}, []jarpkg.Class{
		{BinaryName: "com.example.Bar", EntryPath: "com/example/Bar.class", SourcePath: "com/example/Bar.java"},
		{BinaryName: "com.example.Foo", EntryPath: "com/example/Foo.class", SourcePath: "com/example/Foo.java"},
	})
	if err != nil {
		t.Fatalf("ExecuteCFRRetries() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].RootDir == results[1].RootDir {
		t.Fatal("expected isolated retry workspaces, got shared root directory")
	}
}

func TestValidateRetryOutputRejectsAmbiguousJavaFiles(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	writePipelineFile(t, outputDir, "com/example/Foo.java", "class Foo {}\n")
	writePipelineFile(t, outputDir, "com/example/Bar.java", "class Bar {}\n")

	err := ValidateRetryOutput(jarpkg.Class{
		BinaryName: "com.example.Foo",
		SourcePath: "com/example/Foo.java",
	}, outputDir)
	if !errors.Is(err, ErrAmbiguousRetryOutput) {
		t.Fatalf("ValidateRetryOutput() error = %v, want ErrAmbiguousRetryOutput", err)
	}
}

type fakeCfrRunner struct {
	run func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (f *fakeCfrRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	return f.run(spec)
}

func writePipelineJar(t *testing.T, entries map[string]string) string {
	t.Helper()

	jarPath := filepath.Join(t.TempDir(), "sample.jar")
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
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	return jarPath
}

func writePipelineFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
