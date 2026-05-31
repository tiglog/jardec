package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"jardec/internal/decompiler"
	jarpkg "jardec/internal/jar"
)

func TestClassifyJadxResultMarksMissingOutputForRetry(t *testing.T) {
	t.Parallel()

	sourcesDir := filepath.Join(t.TempDir(), "sources")
	classification, err := ClassifyJadxResult(jarpkg.Class{
		BinaryName: "com.example.Foo",
		SourcePath: "com/example/Foo.java",
	}, sourcesDir, decompiler.RunResult{})
	if err != nil {
		t.Fatalf("ClassifyJadxResult() error = %v", err)
	}

	if !classification.NeedsRetry {
		t.Fatal("NeedsRetry = false, want true")
	}
	if !slices.Equal(classification.Reasons, []RetryReason{RetryReasonMissingOutput}) {
		t.Fatalf("Reasons = %v, want [%s]", classification.Reasons, RetryReasonMissingOutput)
	}
}

func TestClassifyJadxResultMarksJadxWarnForRetry(t *testing.T) {
	t.Parallel()

	sourcesDir := filepath.Join(t.TempDir(), "sources")
	writeJavaFile(t, sourcesDir, "com/example/Foo.java", "class Foo {\n// JADX WARN: decode failed\n}\n")

	classification, err := ClassifyJadxResult(jarpkg.Class{
		BinaryName: "com.example.Foo",
		SourcePath: "com/example/Foo.java",
	}, sourcesDir, decompiler.RunResult{})
	if err != nil {
		t.Fatalf("ClassifyJadxResult() error = %v", err)
	}

	if !classification.NeedsRetry {
		t.Fatal("NeedsRetry = false, want true")
	}
	if !slices.Equal(classification.Reasons, []RetryReason{RetryReasonJADXWarn}) {
		t.Fatalf("Reasons = %v, want [%s]", classification.Reasons, RetryReasonJADXWarn)
	}
}

func TestClassifyJadxResultMarksLoggedFailuresForRetry(t *testing.T) {
	t.Parallel()

	sourcesDir := filepath.Join(t.TempDir(), "sources")
	writeJavaFile(t, sourcesDir, "com/example/Foo.java", "class Foo {}\n")

	classification, err := ClassifyJadxResult(jarpkg.Class{
		BinaryName: "com.example.Foo",
		EntryPath:  "com/example/Foo.class",
		SourcePath: "com/example/Foo.java",
	}, sourcesDir, decompiler.RunResult{
		Stderr: "ERROR: failed to process com.example.Foo",
	})
	if err != nil {
		t.Fatalf("ClassifyJadxResult() error = %v", err)
	}

	if !classification.NeedsRetry {
		t.Fatal("NeedsRetry = false, want true")
	}
	if !slices.Equal(classification.Reasons, []RetryReason{RetryReasonLoggedFailure}) {
		t.Fatalf("Reasons = %v, want [%s]", classification.Reasons, RetryReasonLoggedFailure)
	}
}

func TestClassifyJadxResultKeepsCleanOutput(t *testing.T) {
	t.Parallel()

	sourcesDir := filepath.Join(t.TempDir(), "sources")
	writeJavaFile(t, sourcesDir, "com/example/Foo.java", "class Foo {}\n")

	classification, err := ClassifyJadxResult(jarpkg.Class{
		BinaryName: "com.example.Foo",
		SourcePath: "com/example/Foo.java",
	}, sourcesDir, decompiler.RunResult{
		Stdout: "processed 1 classes",
	})
	if err != nil {
		t.Fatalf("ClassifyJadxResult() error = %v", err)
	}

	if classification.NeedsRetry {
		t.Fatalf("NeedsRetry = true, want false with reasons %v", classification.Reasons)
	}
}

func writeJavaFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
