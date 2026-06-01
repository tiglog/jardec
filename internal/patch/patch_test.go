package patch

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	appcli "jardec/internal/cli"
)

func TestDiscoverReplacementGroupsGroupsTopLevelAndNestedClasses(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "foo")
	writeClassFile(t, classesDir, "com/example/Foo$Inner.class", "inner")
	writeClassFile(t, classesDir, "com/example/Bar.class", "bar")
	writeClassFile(t, classesDir, "com/example/package-info.class", "pkg")

	groups, err := DiscoverReplacementGroups(classesDir)
	if err != nil {
		t.Fatalf("DiscoverReplacementGroups() error = %v", err)
	}

	gotNames := make([]string, 0, len(groups))
	for _, group := range groups {
		gotNames = append(gotNames, group.BinaryName)
	}
	if want := []string{"com.example.Bar", "com.example.Foo"}; !slices.Equal(gotNames, want) {
		t.Fatalf("group names = %v, want %v", gotNames, want)
	}

	foo := groups[1]
	gotEntries := make([]string, 0, len(foo.Files))
	for _, file := range foo.Files {
		gotEntries = append(gotEntries, file.EntryPath)
	}
	if want := []string{"com/example/Foo$Inner.class", "com/example/Foo.class"}; !slices.Equal(gotEntries, want) {
		t.Fatalf("Foo group entries = %v, want %v", gotEntries, want)
	}
}

func TestDiscoverReplacementGroupsRejectsIncompleteGroup(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo$Inner.class", "inner")

	_, err := DiscoverReplacementGroups(classesDir)
	if err == nil {
		t.Fatal("DiscoverReplacementGroups() error = nil, want validation error")
	}
}

func TestPlanReplacementMarksStaleNestedEntries(t *testing.T) {
	t.Parallel()

	jarPath := writePatchJar(t, map[string]string{
		"com/example/Foo.class":      "old-foo",
		"com/example/Foo$Keep.class": "old-keep",
		"com/example/Foo$Old.class":  "old-old",
		"com/example/Other.class":    "other",
	})
	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "new-foo")
	writeClassFile(t, classesDir, "com/example/Foo$Keep.class", "new-keep")
	writeClassFile(t, classesDir, "com/example/Foo$New.class", "new-new")

	groups, err := DiscoverReplacementGroups(classesDir)
	if err != nil {
		t.Fatalf("DiscoverReplacementGroups() error = %v", err)
	}
	plan, err := PlanReplacement(jarPath, groups)
	if err != nil {
		t.Fatalf("PlanReplacement() error = %v", err)
	}

	if len(plan.Groups) != 1 {
		t.Fatalf("len(plan.Groups) = %d, want 1", len(plan.Groups))
	}
	got := plan.Groups[0].StaleEntries
	want := []string{"com/example/Foo$Old.class"}
	if !slices.Equal(got, want) {
		t.Fatalf("StaleEntries = %v, want %v", got, want)
	}
}

func TestApplyPatchRewritesJarPreservesResourcesAndRemovesSignatures(t *testing.T) {
	t.Parallel()

	jarPath := writePatchJar(t, map[string]string{
		"com/example/Foo.class":         "old-foo",
		"com/example/Foo$Old.class":     "old-old",
		"com/example/Other.class":       "other",
		"META-INF/MANIFEST.MF":          "manifest",
		"META-INF/SIGNER.SF":            "sig",
		"META-INF/SIGNER.RSA":           "rsa",
		"config/application.properties": "value=true",
	})
	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "new-foo")
	writeClassFile(t, classesDir, "com/example/Foo$New.class", "new-new")

	groups, err := DiscoverReplacementGroups(classesDir)
	if err != nil {
		t.Fatalf("DiscoverReplacementGroups() error = %v", err)
	}
	plan, err := PlanReplacement(jarPath, groups)
	if err != nil {
		t.Fatalf("PlanReplacement() error = %v", err)
	}

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	result, err := ApplyPatch(jarPath, outputJar, plan)
	if err != nil {
		t.Fatalf("ApplyPatch() error = %v", err)
	}

	if !slices.Equal(result.RemovedSignatureFiles, []string{"META-INF/SIGNER.RSA", "META-INF/SIGNER.SF"}) {
		t.Fatalf("RemovedSignatureFiles = %v, want signature removals", result.RemovedSignatureFiles)
	}
	if !slices.Equal(result.RemovedStaleEntries, []string{"com/example/Foo$Old.class"}) {
		t.Fatalf("RemovedStaleEntries = %v, want stale entry removal", result.RemovedStaleEntries)
	}

	entries := readPatchJar(t, outputJar)
	if entries["com/example/Foo.class"] != "new-foo" {
		t.Fatalf("patched Foo.class = %q, want new-foo", entries["com/example/Foo.class"])
	}
	if entries["com/example/Foo$New.class"] != "new-new" {
		t.Fatalf("patched Foo$New.class = %q, want new-new", entries["com/example/Foo$New.class"])
	}
	if _, ok := entries["com/example/Foo$Old.class"]; ok {
		t.Fatal("patched jar still contains stale Foo$Old.class")
	}
	if entries["com/example/Other.class"] != "other" {
		t.Fatalf("Other.class = %q, want other", entries["com/example/Other.class"])
	}
	if entries["config/application.properties"] != "value=true" {
		t.Fatalf("application.properties = %q, want value=true", entries["config/application.properties"])
	}
	if _, ok := entries["META-INF/SIGNER.SF"]; ok {
		t.Fatal("patched jar still contains signature file")
	}
	if _, ok := entries["META-INF/SIGNER.RSA"]; ok {
		t.Fatal("patched jar still contains signature block")
	}
	if entries["META-INF/MANIFEST.MF"] != "manifest" {
		t.Fatalf("MANIFEST.MF = %q, want manifest", entries["META-INF/MANIFEST.MF"])
	}
}

func TestEngineRunFiltersToExplicitTargetClasses(t *testing.T) {
	t.Parallel()

	inputJar := writePatchJar(t, map[string]string{
		"com/example/Foo.class": "old-foo",
		"com/example/Bar.class": "old-bar",
	})
	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "new-foo")
	writeClassFile(t, classesDir, "com/example/Bar.class", "new-bar")

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	rep, err := (Engine{}).Run(context.Background(), appcli.PatchConfig{
		InputJarPath:  inputJar,
		ClassesDir:    classesDir,
		OutputJarPath: outputJar,
		TargetClasses: []string{"com.example.Bar"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if rep.ReplacedGroups != 1 {
		t.Fatalf("ReplacedGroups = %d, want 1", rep.ReplacedGroups)
	}
	if len(rep.Groups) != 1 || rep.Groups[0].BinaryName != "com.example.Bar" {
		t.Fatalf("Groups = %+v, want only com.example.Bar", rep.Groups)
	}

	entries := readPatchJar(t, outputJar)
	if entries["com/example/Foo.class"] != "old-foo" {
		t.Fatalf("Foo.class = %q, want old-foo", entries["com/example/Foo.class"])
	}
	if entries["com/example/Bar.class"] != "new-bar" {
		t.Fatalf("Bar.class = %q, want new-bar", entries["com/example/Bar.class"])
	}
}

func TestEngineRunRejectsUnknownTargetClasses(t *testing.T) {
	t.Parallel()

	inputJar := writePatchJar(t, map[string]string{
		"com/example/Foo.class": "old-foo",
	})
	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "new-foo")

	_, err := (Engine{}).Run(context.Background(), appcli.PatchConfig{
		InputJarPath:  inputJar,
		ClassesDir:    classesDir,
		OutputJarPath: filepath.Join(t.TempDir(), "patched.jar"),
		TargetClasses: []string{"com.example.Missing"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want unknown target error")
	}
}

func TestEngineRunDryRunWritesReportsWithoutWritingJar(t *testing.T) {
	t.Parallel()

	inputJar := writePatchJar(t, map[string]string{
		"com/example/Foo.class":     "old-foo",
		"com/example/Foo$Old.class": "old-old",
		"META-INF/SIGNER.SF":        "sig",
	})
	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class", "new-foo")

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	rep, err := (Engine{}).Run(context.Background(), appcli.PatchConfig{
		InputJarPath:  inputJar,
		ClassesDir:    classesDir,
		OutputJarPath: outputJar,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !rep.DryRun {
		t.Fatal("DryRun = false, want true")
	}
	if _, err := os.Stat(outputJar); !os.IsNotExist(err) {
		t.Fatalf("expected no output jar for dry-run, got err=%v", err)
	}
	for _, reportPath := range []string{outputJar + ".report.json", outputJar + ".report.txt"} {
		if _, err := os.Stat(reportPath); err != nil {
			t.Fatalf("expected report file %s: %v", reportPath, err)
		}
	}
	if !slices.Equal(rep.SignatureFiles, []string{"META-INF/SIGNER.SF"}) {
		t.Fatalf("SignatureFiles = %v, want signature preview", rep.SignatureFiles)
	}
}

func writeClassFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writePatchJar(t *testing.T, entries map[string]string) string {
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
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	return jarPath
}

func readPatchJar(t *testing.T, jarPath string) map[string]string {
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
