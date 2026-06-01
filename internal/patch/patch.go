package patch

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	appcli "jardec/internal/cli"
	ireport "jardec/internal/report"
)

type ClassFile struct {
	EntryPath string
	DiskPath  string
}

type ReplacementGroup struct {
	BinaryName    string
	TopLevelEntry string
	Files         []ClassFile
}

type GroupStatus string

const (
	GroupStatusChanged   GroupStatus = "changed"
	GroupStatusUnchanged GroupStatus = "unchanged"
)

type PlannedGroup struct {
	BinaryName       string
	TopLevelEntry    string
	ReplacementFiles []ClassFile
	OriginalEntries  []string
	StaleEntries     []string
	Status           GroupStatus
}

type Plan struct {
	Groups             []PlannedGroup
	SignatureFiles     []string
	DetectedSignatures []string
}

type Result struct {
	Groups                []PlannedGroup
	RemovedStaleEntries   []string
	RemovedSignatureFiles []string
}

type Engine struct{}

func (Engine) Run(ctx context.Context, cfg appcli.PatchConfig) (ireport.PatchReport, error) {
	startedAt := time.Now()
	if err := ctx.Err(); err != nil {
		return ireport.PatchReport{}, err
	}

	groups, err := DiscoverReplacementGroups(cfg.ClassesDir)
	if err != nil {
		return ireport.PatchReport{}, err
	}
	groups, err = FilterGroups(groups, cfg.TargetClasses)
	if err != nil {
		return ireport.PatchReport{}, err
	}
	plan, err := PlanReplacement(cfg.InputJarPath, groups)
	if err != nil {
		return ireport.PatchReport{}, err
	}
	result := Result{
		Groups:                changedGroups(plan.Groups),
		RemovedSignatureFiles: append([]string(nil), plan.SignatureFiles...),
	}
	for _, group := range plan.Groups {
		if group.Status == GroupStatusChanged {
			result.RemovedStaleEntries = append(result.RemovedStaleEntries, group.StaleEntries...)
		}
	}
	if !cfg.DryRun {
		result, err = ApplyPatch(cfg.InputJarPath, cfg.OutputJarPath, plan)
		if err != nil {
			return ireport.PatchReport{}, err
		}
	}

	groupResults := make([]ireport.PatchGroupResult, 0, len(plan.Groups))
	unchangedGroups := 0
	for _, group := range plan.Groups {
		replacedEntries := make([]string, 0, len(group.ReplacementFiles))
		if group.Status == GroupStatusChanged {
			for _, file := range group.ReplacementFiles {
				replacedEntries = append(replacedEntries, file.EntryPath)
			}
		} else {
			unchangedGroups++
		}
		groupResults = append(groupResults, ireport.PatchGroupResult{
			BinaryName:          group.BinaryName,
			Status:              string(group.Status),
			ReplacedEntries:     replacedEntries,
			RemovedStaleEntries: append([]string(nil), group.StaleEntries...),
		})
	}
	noop := !cfg.DryRun && len(result.Groups) == 0
	preservedSignatures := []string(nil)
	if noop {
		preservedSignatures = append([]string(nil), plan.DetectedSignatures...)
	}

	rep := ireport.PatchReport{
		InputJar:              cfg.InputJarPath,
		OutputJar:             cfg.OutputJarPath,
		DryRun:                cfg.DryRun,
		Noop:                  noop,
		ReplacedGroups:        len(result.Groups),
		UnchangedGroups:       unchangedGroups,
		RemovedStaleEntries:   len(result.RemovedStaleEntries),
		RemovedSignatureFiles: len(result.RemovedSignatureFiles),
		ElapsedMillis:         time.Since(startedAt).Milliseconds(),
		Groups:                groupResults,
		SignatureFiles:        append([]string(nil), result.RemovedSignatureFiles...),
		PreservedSignatures:   preservedSignatures,
	}
	jsonPath, textPath := ireport.PatchReportPaths(cfg.OutputJarPath)
	if err := ireport.WritePatchJSON(jsonPath, rep); err != nil {
		return ireport.PatchReport{}, err
	}
	if err := ireport.WritePatchText(textPath, rep); err != nil {
		return ireport.PatchReport{}, err
	}

	return rep, nil
}

func DiscoverReplacementGroups(classesDir string) ([]ReplacementGroup, error) {
	groupsByTopLevel := map[string]*ReplacementGroup{}

	err := filepath.WalkDir(classesDir, func(filePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(filePath) != ".class" {
			return nil
		}

		relPath, err := filepath.Rel(classesDir, filePath)
		if err != nil {
			return fmt.Errorf("relative class path: %w", err)
		}
		entryPath := filepath.ToSlash(relPath)
		topLevelEntry, binaryName, include := classifyEntry(entryPath)
		if !include {
			return nil
		}
		declaredBinaryName, err := readDeclaredBinaryName(filePath)
		if err != nil {
			return fmt.Errorf("read class identity %s: %w", entryPath, err)
		}
		expectedBinaryName := entryBinaryName(entryPath)
		if declaredBinaryName != expectedBinaryName {
			return fmt.Errorf("class file %s declares %s, want %s", entryPath, declaredBinaryName, expectedBinaryName)
		}

		group := groupsByTopLevel[topLevelEntry]
		if group == nil {
			group = &ReplacementGroup{
				BinaryName:    binaryName,
				TopLevelEntry: topLevelEntry,
			}
			groupsByTopLevel[topLevelEntry] = group
		}
		group.Files = append(group.Files, ClassFile{
			EntryPath: entryPath,
			DiskPath:  filePath,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk classes directory: %w", err)
	}

	if len(groupsByTopLevel) == 0 {
		return nil, errors.New("no replacement class groups found")
	}

	groups := make([]ReplacementGroup, 0, len(groupsByTopLevel))
	for _, group := range groupsByTopLevel {
		slices.SortFunc(group.Files, func(a, b ClassFile) int {
			if a.EntryPath == group.TopLevelEntry {
				return -1
			}
			if b.EntryPath == group.TopLevelEntry {
				return 1
			}
			return strings.Compare(a.EntryPath, b.EntryPath)
		})
		hasTopLevel := false
		for _, file := range group.Files {
			if file.EntryPath == group.TopLevelEntry {
				hasTopLevel = true
				break
			}
		}
		if !hasTopLevel {
			return nil, fmt.Errorf("replacement group %s is missing top-level class file %s", group.BinaryName, group.TopLevelEntry)
		}
		groups = append(groups, *group)
	}

	slices.SortFunc(groups, func(a, b ReplacementGroup) int {
		return strings.Compare(a.BinaryName, b.BinaryName)
	})

	return groups, nil
}

func PlanReplacement(jarPath string, groups []ReplacementGroup) (Plan, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return Plan{}, fmt.Errorf("open input jar: %w", err)
	}
	defer reader.Close()

	entries := make(map[string]*zip.File, len(reader.File))
	signatureFiles := make([]string, 0)
	for _, file := range reader.File {
		entries[file.Name] = file
		if isInvalidatedSignature(file.Name) {
			signatureFiles = append(signatureFiles, file.Name)
		}
	}
	slices.Sort(signatureFiles)

	planned := make([]PlannedGroup, 0, len(groups))
	for _, group := range groups {
		if _, ok := entries[group.TopLevelEntry]; !ok {
			return Plan{}, fmt.Errorf("target class %s does not exist in input jar", group.TopLevelEntry)
		}

		originalEntries := make([]string, 0)
		for _, file := range reader.File {
			if belongsToGroup(file.Name, group.TopLevelEntry) {
				originalEntries = append(originalEntries, file.Name)
			}
		}

		replacementSet := make(map[string]struct{}, len(group.Files))
		for _, file := range group.Files {
			replacementSet[file.EntryPath] = struct{}{}
		}

		staleEntries := make([]string, 0)
		for _, entryName := range originalEntries {
			if _, ok := replacementSet[entryName]; !ok {
				staleEntries = append(staleEntries, entryName)
			}
		}

		status, err := classifyGroupStatus(entries, group, staleEntries)
		if err != nil {
			return Plan{}, err
		}

		planned = append(planned, PlannedGroup{
			BinaryName:       group.BinaryName,
			TopLevelEntry:    group.TopLevelEntry,
			ReplacementFiles: append([]ClassFile(nil), group.Files...),
			OriginalEntries:  originalEntries,
			StaleEntries:     staleEntries,
			Status:           status,
		})
	}

	slices.SortFunc(planned, func(a, b PlannedGroup) int {
		return strings.Compare(a.BinaryName, b.BinaryName)
	})

	plan := Plan{
		Groups:             planned,
		DetectedSignatures: append([]string(nil), signatureFiles...),
	}
	if hasChangedGroups(planned) {
		plan.SignatureFiles = signatureFiles
	}

	return plan, nil
}

func ApplyPatch(jarPath, outputJar string, plan Plan) (Result, error) {
	if !hasChangedGroups(plan.Groups) {
		if err := copyFile(jarPath, outputJar); err != nil {
			return Result{}, err
		}
		return Result{}, nil
	}

	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return Result{}, fmt.Errorf("open input jar: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(filepath.Dir(outputJar), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output jar directory: %w", err)
	}
	outputFile, err := os.Create(outputJar)
	if err != nil {
		return Result{}, fmt.Errorf("create output jar: %w", err)
	}
	defer outputFile.Close()

	writer := zip.NewWriter(outputFile)

	changedGroups := make([]PlannedGroup, 0, len(plan.Groups))
	groupByOriginalEntry := make(map[string]PlannedGroup)
	firstOriginalEntryByGroup := make(map[string]string)
	originalByName := make(map[string]*zip.File, len(reader.File))
	for _, group := range plan.Groups {
		if group.Status != GroupStatusChanged {
			continue
		}
		changedGroups = append(changedGroups, group)
		for idx, entry := range group.OriginalEntries {
			groupByOriginalEntry[entry] = group
			if idx == 0 {
				firstOriginalEntryByGroup[group.TopLevelEntry] = entry
			}
		}
	}
	for _, file := range reader.File {
		originalByName[file.Name] = file
	}

	removedStaleEntries := make([]string, 0)
	for _, group := range changedGroups {
		removedStaleEntries = append(removedStaleEntries, group.StaleEntries...)
	}
	removedSignatureFiles := append([]string(nil), plan.SignatureFiles...)
	writtenGroups := map[string]struct{}{}

	for _, file := range reader.File {
		if isInvalidatedSignature(file.Name) {
			continue
		}

		if group, ok := groupByOriginalEntry[file.Name]; ok {
			if _, written := writtenGroups[group.TopLevelEntry]; !written && firstOriginalEntryByGroup[group.TopLevelEntry] == file.Name {
				if err := writeReplacementGroup(writer, group, originalByName); err != nil {
					return Result{}, fmt.Errorf("write replacement group %s: %w", group.BinaryName, err)
				}
				writtenGroups[group.TopLevelEntry] = struct{}{}
			}
			continue
		}

		if err := copyZipEntry(writer, file); err != nil {
			return Result{}, fmt.Errorf("copy archive entry %s: %w", file.Name, err)
		}
	}

	if err := writer.Close(); err != nil {
		return Result{}, fmt.Errorf("close output jar writer: %w", err)
	}

	slices.Sort(removedStaleEntries)
	slices.Sort(removedSignatureFiles)

	return Result{
		Groups:                changedGroups,
		RemovedStaleEntries:   removedStaleEntries,
		RemovedSignatureFiles: removedSignatureFiles,
	}, nil
}

func FilterGroups(groups []ReplacementGroup, targets []string) ([]ReplacementGroup, error) {
	if len(targets) == 0 {
		return groups, nil
	}

	groupByName := make(map[string]ReplacementGroup, len(groups))
	for _, group := range groups {
		groupByName[group.BinaryName] = group
	}

	filtered := make([]ReplacementGroup, 0, len(targets))
	missing := make([]string, 0)
	for _, target := range targets {
		group, ok := groupByName[target]
		if !ok {
			missing = append(missing, target)
			continue
		}
		filtered = append(filtered, group)
	}

	if len(missing) > 0 {
		slices.Sort(missing)
		return nil, fmt.Errorf("unknown class target(s): %s", strings.Join(missing, ", "))
	}

	return filtered, nil
}

func classifyEntry(entryPath string) (string, string, bool) {
	baseName := path.Base(entryPath)
	if baseName == "module-info.class" || baseName == "package-info.class" {
		return "", "", false
	}

	className := strings.TrimSuffix(baseName, ".class")
	topLevelName := strings.SplitN(className, "$", 2)[0] + ".class"
	topLevelEntry := path.Join(path.Dir(entryPath), topLevelName)
	if path.Dir(entryPath) == "." {
		topLevelEntry = topLevelName
	}

	binaryName := strings.TrimSuffix(topLevelEntry, ".class")
	binaryName = strings.ReplaceAll(binaryName, "/", ".")
	return topLevelEntry, binaryName, true
}

func belongsToGroup(entryName, topLevelEntry string) bool {
	if entryName == topLevelEntry {
		return true
	}

	prefix := strings.TrimSuffix(topLevelEntry, ".class")
	return strings.HasPrefix(entryName, prefix+"$") && strings.HasSuffix(entryName, ".class")
}

func isInvalidatedSignature(entryName string) bool {
	upper := strings.ToUpper(entryName)
	if !strings.HasPrefix(upper, "META-INF/") {
		return false
	}

	return strings.HasSuffix(upper, ".SF") || strings.HasSuffix(upper, ".RSA") || strings.HasSuffix(upper, ".DSA")
}

func copyZipEntry(writer *zip.Writer, file *zip.File) error {
	header := file.FileHeader
	w, err := writer.CreateHeader(&header)
	if err != nil {
		return err
	}
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	return err
}

func writeReplacementEntry(writer *zip.Writer, file ClassFile) error {
	return writeReplacementEntryWithHeader(writer, file, nil)
}

func writeReplacementGroup(writer *zip.Writer, group PlannedGroup, originalByName map[string]*zip.File) error {
	for _, replacement := range group.ReplacementFiles {
		if err := writeReplacementEntryWithHeader(writer, replacement, originalByName[replacement.EntryPath]); err != nil {
			return err
		}
	}
	return nil
}

func writeReplacementEntryWithHeader(writer *zip.Writer, file ClassFile, original *zip.File) error {
	data, err := os.ReadFile(file.DiskPath)
	if err != nil {
		return err
	}

	header := &zip.FileHeader{
		Name:   file.EntryPath,
		Method: zip.Deflate,
	}
	if original != nil {
		clone := original.FileHeader
		clone.Name = file.EntryPath
		header = &clone
	}
	w, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func classifyGroupStatus(originalEntries map[string]*zip.File, group ReplacementGroup, staleEntries []string) (GroupStatus, error) {
	if len(staleEntries) > 0 {
		return GroupStatusChanged, nil
	}
	for _, file := range group.Files {
		if _, ok := originalEntries[file.EntryPath]; !ok {
			return GroupStatusChanged, nil
		}
	}

	for _, file := range group.Files {
		matches, err := replacementMatchesOriginal(file, originalEntries)
		if err != nil {
			return "", err
		}
		if !matches {
			return GroupStatusChanged, nil
		}
	}
	return GroupStatusUnchanged, nil
}

func replacementMatchesOriginal(file ClassFile, originalEntries map[string]*zip.File) (bool, error) {
	original, ok := originalEntries[file.EntryPath]
	if !ok {
		return false, nil
	}

	replacementBytes, err := os.ReadFile(file.DiskPath)
	if err != nil {
		return false, fmt.Errorf("read replacement class %s: %w", file.EntryPath, err)
	}
	originalReader, err := original.Open()
	if err != nil {
		return false, fmt.Errorf("open original class %s: %w", file.EntryPath, err)
	}
	defer originalReader.Close()

	originalBytes, err := io.ReadAll(originalReader)
	if err != nil {
		return false, fmt.Errorf("read original class %s: %w", file.EntryPath, err)
	}

	return slices.Equal(replacementBytes, originalBytes), nil
}

func hasChangedGroups(groups []PlannedGroup) bool {
	for _, group := range groups {
		if group.Status == GroupStatusChanged {
			return true
		}
	}
	return false
}

func changedGroups(groups []PlannedGroup) []PlannedGroup {
	filtered := make([]PlannedGroup, 0, len(groups))
	for _, group := range groups {
		if group.Status == GroupStatusChanged {
			filtered = append(filtered, group)
		}
	}
	return filtered
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create output jar directory: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open input jar: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create output jar: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy output jar: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close output jar: %w", err)
	}
	return nil
}

func readDeclaredBinaryName(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return parseClassBinaryName(data)
}

func parseClassBinaryName(data []byte) (string, error) {
	reader := &classReader{data: data}
	magic, err := reader.u4()
	if err != nil {
		return "", err
	}
	if magic != 0xCAFEBABE {
		return "", errors.New("invalid class file magic")
	}
	if _, err := reader.u2(); err != nil {
		return "", err
	}
	if _, err := reader.u2(); err != nil {
		return "", err
	}

	cpCount, err := reader.u2()
	if err != nil {
		return "", err
	}
	utf8Values := map[uint16]string{}
	classNameRefs := map[uint16]uint16{}
	for idx := uint16(1); idx < cpCount; idx++ {
		tag, err := reader.u1()
		if err != nil {
			return "", err
		}
		switch tag {
		case 1:
			size, err := reader.u2()
			if err != nil {
				return "", err
			}
			raw, err := reader.bytes(int(size))
			if err != nil {
				return "", err
			}
			utf8Values[idx] = string(raw)
		case 3, 4:
			if _, err := reader.bytes(4); err != nil {
				return "", err
			}
		case 5, 6:
			if _, err := reader.bytes(8); err != nil {
				return "", err
			}
			idx++
		case 7:
			nameIndex, err := reader.u2()
			if err != nil {
				return "", err
			}
			classNameRefs[idx] = nameIndex
		case 8, 16, 19, 20:
			if _, err := reader.bytes(2); err != nil {
				return "", err
			}
		case 9, 10, 11, 12, 17, 18:
			if _, err := reader.bytes(4); err != nil {
				return "", err
			}
		case 15:
			if _, err := reader.bytes(3); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unsupported constant pool tag %d", tag)
		}
	}

	if _, err := reader.u2(); err != nil {
		return "", err
	}
	thisClassIndex, err := reader.u2()
	if err != nil {
		return "", err
	}
	nameIndex, ok := classNameRefs[thisClassIndex]
	if !ok {
		return "", fmt.Errorf("missing class entry for index %d", thisClassIndex)
	}
	internalName, ok := utf8Values[nameIndex]
	if !ok {
		return "", fmt.Errorf("missing utf8 entry for class name index %d", nameIndex)
	}
	return strings.ReplaceAll(internalName, "/", "."), nil
}

func entryBinaryName(entryPath string) string {
	return strings.ReplaceAll(strings.TrimSuffix(entryPath, ".class"), "/", ".")
}

type classReader struct {
	data []byte
	pos  int
}

func (r *classReader) u1() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	value := r.data[r.pos]
	r.pos++
	return value, nil
}

func (r *classReader) u2() (uint16, error) {
	raw, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(raw), nil
}

func (r *classReader) u4() (uint32, error) {
	raw, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(raw), nil
}

func (r *classReader) bytes(count int) ([]byte, error) {
	if r.pos+count > len(r.data) {
		return nil, io.ErrUnexpectedEOF
	}
	value := r.data[r.pos : r.pos+count]
	r.pos += count
	return value, nil
}
