package jar

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestEnumerateTopLevelClassesFiltersOutUnsupportedEntries(t *testing.T) {
	t.Parallel()

	jarPath := writeTestJar(t, map[string]string{
		"com/example/Foo.class":          "foo",
		"com/example/Foo$Inner.class":    "inner",
		"com/example/Foo$1.class":        "anon",
		"com/example/Bar.class":          "bar",
		"com/example/package-info.class": "pkg",
		"module-info.class":              "module",
		"META-INF/MANIFEST.MF":           "manifest",
	})

	classes, err := EnumerateTopLevelClasses(jarPath)
	if err != nil {
		t.Fatalf("EnumerateTopLevelClasses() error = %v", err)
	}

	got := make([]string, 0, len(classes))
	for _, class := range classes {
		got = append(got, class.BinaryName)
	}

	want := []string{"com.example.Bar", "com.example.Foo"}
	if !slices.Equal(got, want) {
		t.Fatalf("BinaryName list = %v, want %v", got, want)
	}
}

func TestEnumerateTopLevelClassesCalculatesStablePaths(t *testing.T) {
	t.Parallel()

	jarPath := writeTestJar(t, map[string]string{
		"org/acme/Foo.class": "foo",
	})

	classes, err := EnumerateTopLevelClasses(jarPath)
	if err != nil {
		t.Fatalf("EnumerateTopLevelClasses() error = %v", err)
	}
	if len(classes) != 1 {
		t.Fatalf("len(classes) = %d, want 1", len(classes))
	}

	got := classes[0]
	if got.BinaryName != "org.acme.Foo" {
		t.Fatalf("BinaryName = %q, want org.acme.Foo", got.BinaryName)
	}
	if got.EntryPath != "org/acme/Foo.class" {
		t.Fatalf("EntryPath = %q, want org/acme/Foo.class", got.EntryPath)
	}
	if got.SourcePath != "org/acme/Foo.java" {
		t.Fatalf("SourcePath = %q, want org/acme/Foo.java", got.SourcePath)
	}
}

func writeTestJar(t *testing.T, entries map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	jarPath := filepath.Join(dir, "sample.jar")
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
