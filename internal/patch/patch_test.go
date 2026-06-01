package patch

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	appcli "jardec/internal/cli"
)

func TestDiscoverReplacementGroupsGroupsTopLevelAndNestedClasses(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class")
	writeClassFile(t, classesDir, "com/example/Foo$Inner.class")
	writeClassFile(t, classesDir, "com/example/Bar.class")
	writeClassFile(t, classesDir, "com/example/package-info.class")

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
	if want := []string{"com/example/Foo.class", "com/example/Foo$Inner.class"}; !slices.Equal(gotEntries, want) {
		t.Fatalf("Foo group entries = %v, want %v", gotEntries, want)
	}
}

func TestDiscoverReplacementGroupsRejectsIncompleteGroup(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo$Inner.class")

	_, err := DiscoverReplacementGroups(classesDir)
	if err == nil {
		t.Fatal("DiscoverReplacementGroups() error = nil, want validation error")
	}
}

func TestDiscoverReplacementGroupsRejectsMismatchedTopLevelClassName(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Bar")

	_, err := DiscoverReplacementGroups(classesDir)
	if err == nil {
		t.Fatal("DiscoverReplacementGroups() error = nil, want mismatch error")
	}
	if !strings.Contains(err.Error(), "com/example/Foo.class") || !strings.Contains(err.Error(), "com.example.Bar") {
		t.Fatalf("DiscoverReplacementGroups() error = %v, want path and declared class name", err)
	}
}

func TestDiscoverReplacementGroupsRejectsMismatchedNestedClassName(t *testing.T) {
	t.Parallel()

	classesDir := t.TempDir()
	writeClassFile(t, classesDir, "com/example/Foo.class")
	writeDeclaredClassFile(t, classesDir, "com/example/Foo$Inner.class", "com.example.Other$Inner")

	_, err := DiscoverReplacementGroups(classesDir)
	if err == nil {
		t.Fatal("DiscoverReplacementGroups() error = nil, want mismatch error")
	}
	if !strings.Contains(err.Error(), "com/example/Foo$Inner.class") || !strings.Contains(err.Error(), "com.example.Other$Inner") {
		t.Fatalf("DiscoverReplacementGroups() error = %v, want path and declared class name", err)
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
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")
	writeDeclaredClassFile(t, classesDir, "com/example/Foo$Keep.class", "com.example.Foo$Keep")
	writeDeclaredClassFile(t, classesDir, "com/example/Foo$New.class", "com.example.Foo$New")

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
	if plan.Groups[0].Status != GroupStatusChanged {
		t.Fatalf("Status = %q, want %q", plan.Groups[0].Status, GroupStatusChanged)
	}
}

func TestPlanReplacementMarksUnchangedGroupAndSkipsSignatureCleanup(t *testing.T) {
	t.Parallel()

	fooBytes := classFileBytes(t, "com.example.Foo")
	jarPath := writePatchJarEntries(t, []patchJarEntry{
		{Name: "META-INF/SIGNER.SF", Content: []byte("sig")},
		{Name: "com/example/Foo.class", Content: fooBytes},
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")

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
	if plan.Groups[0].Status != GroupStatusUnchanged {
		t.Fatalf("Status = %q, want %q", plan.Groups[0].Status, GroupStatusUnchanged)
	}
	if len(plan.SignatureFiles) != 0 {
		t.Fatalf("SignatureFiles = %v, want none for unchanged patch plan", plan.SignatureFiles)
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
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")
	writeDeclaredClassFile(t, classesDir, "com/example/Foo$New.class", "com.example.Foo$New")

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
	if entries["com/example/Foo.class"] != string(classFileBytes(t, "com.example.Foo")) {
		t.Fatalf("patched Foo.class did not contain replacement bytes")
	}
	if entries["com/example/Foo$New.class"] != string(classFileBytes(t, "com.example.Foo$New")) {
		t.Fatalf("patched Foo$New.class did not contain replacement bytes")
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

func TestApplyPatchPreservesChangedGroupPositionAndExistingHeaderMetadata(t *testing.T) {
	t.Parallel()

	modifiedAt := time.Date(2024, time.January, 2, 3, 4, 6, 0, time.UTC)
	jarPath := writePatchJarEntries(t, []patchJarEntry{
		{Name: "a.txt", Content: []byte("a"), Method: zip.Store, Modified: modifiedAt},
		{Name: "com/example/Foo.class", Content: []byte("old-foo"), Method: zip.Store, Modified: modifiedAt},
		{Name: "z.txt", Content: []byte("z"), Method: zip.Deflate, Modified: modifiedAt.Add(2 * time.Hour)},
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")
	writeDeclaredClassFile(t, classesDir, "com/example/Foo$Inner.class", "com.example.Foo$Inner")

	groups, err := DiscoverReplacementGroups(classesDir)
	if err != nil {
		t.Fatalf("DiscoverReplacementGroups() error = %v", err)
	}
	plan, err := PlanReplacement(jarPath, groups)
	if err != nil {
		t.Fatalf("PlanReplacement() error = %v", err)
	}

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	if _, err := ApplyPatch(jarPath, outputJar, plan); err != nil {
		t.Fatalf("ApplyPatch() error = %v", err)
	}

	gotEntries := readPatchJarEntries(t, outputJar)
	gotNames := make([]string, 0, len(gotEntries))
	for _, entry := range gotEntries {
		gotNames = append(gotNames, entry.Name)
	}
	wantNames := []string{"a.txt", "com/example/Foo.class", "com/example/Foo$Inner.class", "z.txt"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("entry order = %v, want %v", gotNames, wantNames)
	}

	fooEntry := gotEntries[1]
	if fooEntry.Method != zip.Store {
		t.Fatalf("Foo.class method = %d, want %d", fooEntry.Method, zip.Store)
	}
	if !fooEntry.Modified.Equal(modifiedAt) {
		t.Fatalf("Foo.class modified = %v, want %v", fooEntry.Modified, modifiedAt)
	}
}

func TestEngineRunFiltersToExplicitTargetClasses(t *testing.T) {
	t.Parallel()

	inputJar := writePatchJar(t, map[string]string{
		"com/example/Foo.class": "old-foo",
		"com/example/Bar.class": "old-bar",
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")
	writeDeclaredClassFile(t, classesDir, "com/example/Bar.class", "com.example.Bar")

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
	if entries["com/example/Bar.class"] != string(classFileBytes(t, "com.example.Bar")) {
		t.Fatalf("Bar.class did not contain replacement bytes")
	}
}

func TestEngineRunRejectsUnknownTargetClasses(t *testing.T) {
	t.Parallel()

	inputJar := writePatchJar(t, map[string]string{
		"com/example/Foo.class": "old-foo",
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")

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
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")

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

func TestEngineRunNoOpPreservesSignedJarBytes(t *testing.T) {
	t.Parallel()

	fooBytes := classFileBytes(t, "com.example.Foo")
	inputJar := writePatchJarEntries(t, []patchJarEntry{
		{Name: "META-INF/SIGNER.SF", Content: []byte("sig")},
		{Name: "com/example/Foo.class", Content: fooBytes},
	})
	classesDir := t.TempDir()
	writeDeclaredClassFile(t, classesDir, "com/example/Foo.class", "com.example.Foo")

	outputJar := filepath.Join(t.TempDir(), "patched.jar")
	rep, err := (Engine{}).Run(context.Background(), appcli.PatchConfig{
		InputJarPath:  inputJar,
		ClassesDir:    classesDir,
		OutputJarPath: outputJar,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if rep.ReplacedGroups != 0 {
		t.Fatalf("ReplacedGroups = %d, want 0 for no-op patch", rep.ReplacedGroups)
	}
	if !rep.Noop {
		t.Fatal("Noop = false, want true for no-op patch")
	}
	if rep.UnchangedGroups != 1 {
		t.Fatalf("UnchangedGroups = %d, want 1", rep.UnchangedGroups)
	}
	if !slices.Equal(rep.PreservedSignatures, []string{"META-INF/SIGNER.SF"}) {
		t.Fatalf("PreservedSignatures = %v, want preserved signature list", rep.PreservedSignatures)
	}
	inputBytes, err := os.ReadFile(inputJar)
	if err != nil {
		t.Fatalf("ReadFile(inputJar) error = %v", err)
	}
	outputBytes, err := os.ReadFile(outputJar)
	if err != nil {
		t.Fatalf("ReadFile(outputJar) error = %v", err)
	}
	if !bytes.Equal(outputBytes, inputBytes) {
		t.Fatal("patched jar bytes changed for no-op patch")
	}
	if entries := readPatchJar(t, outputJar); entries["META-INF/SIGNER.SF"] != "sig" {
		t.Fatalf("META-INF/SIGNER.SF = %q, want sig", entries["META-INF/SIGNER.SF"])
	}
}

func writeClassFile(t *testing.T, root, relativePath string) {
	t.Helper()

	writeDeclaredClassFile(t, root, relativePath, binaryNameFromEntry(relativePath))
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

type patchJarEntry struct {
	Name     string
	Content  []byte
	Method   uint16
	Modified time.Time
}

func writePatchJarEntries(t *testing.T, entries []patchJarEntry) string {
	t.Helper()

	jarPath := filepath.Join(t.TempDir(), "input.jar")
	file, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:     entry.Name,
			Method:   entry.Method,
			Modified: entry.Modified,
		}
		w, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("CreateHeader(%q) error = %v", entry.Name, err)
		}
		if _, err := w.Write(entry.Content); err != nil {
			t.Fatalf("Write(%q) error = %v", entry.Name, err)
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

type jarEntrySnapshot struct {
	Name     string
	Method   uint16
	Modified time.Time
}

func readPatchJarEntries(t *testing.T, jarPath string) []jarEntrySnapshot {
	t.Helper()

	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer reader.Close()

	entries := make([]jarEntrySnapshot, 0, len(reader.File))
	for _, file := range reader.File {
		entries = append(entries, jarEntrySnapshot{
			Name:     file.Name,
			Method:   file.Method,
			Modified: file.Modified,
		})
	}
	return entries
}

func binaryNameFromEntry(entryPath string) string {
	return strings.ReplaceAll(strings.TrimSuffix(path.Clean(entryPath), ".class"), "/", ".")
}

func writeU2(t *testing.T, buf *bytes.Buffer, value uint16) {
	t.Helper()
	if err := binary.Write(buf, binary.BigEndian, value); err != nil {
		t.Fatalf("binary.Write(uint16) error = %v", err)
	}
}

func writeU4(t *testing.T, buf *bytes.Buffer, value uint32) {
	t.Helper()
	if err := binary.Write(buf, binary.BigEndian, value); err != nil {
		t.Fatalf("binary.Write(uint32) error = %v", err)
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
