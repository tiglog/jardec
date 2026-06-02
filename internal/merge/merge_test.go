package merge

import (
	"os"
	"path/filepath"
	"testing"

	jarpkg "jardec/internal/jar"
)

func TestApplyRecoveryReplacesExistingOutput(t *testing.T) {
	t.Parallel()

	finalDir := t.TempDir()
	retryDir := t.TempDir()

	writeMergeFile(t, finalDir, "sources/com/example/Foo.java", "jadx\n")
	writeMergeFile(t, retryDir, "com/example/Foo.java", "procyon\n")

	err := ApplyRecovery(filepath.Join(finalDir, "sources"), jarpkg.Class{
		BinaryName: "com.example.Foo",
		SourcePath: "com/example/Foo.java",
	}, retryDir)
	if err != nil {
		t.Fatalf("ApplyRecovery() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(finalDir, "sources/com/example/Foo.java"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "procyon\n" {
		t.Fatalf("content = %q, want procyon", string(content))
	}
}

func writeMergeFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
