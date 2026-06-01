package patch

import (
	"archive/zip"
	"context"
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

type PlannedGroup struct {
	BinaryName       string
	TopLevelEntry    string
	ReplacementFiles []ClassFile
	OriginalEntries  []string
	StaleEntries     []string
}

type Plan struct {
	Groups         []PlannedGroup
	SignatureFiles []string
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
		Groups:                append([]PlannedGroup(nil), plan.Groups...),
		RemovedSignatureFiles: append([]string(nil), plan.SignatureFiles...),
	}
	for _, group := range plan.Groups {
		result.RemovedStaleEntries = append(result.RemovedStaleEntries, group.StaleEntries...)
	}
	if !cfg.DryRun {
		result, err = ApplyPatch(cfg.InputJarPath, cfg.OutputJarPath, plan)
		if err != nil {
			return ireport.PatchReport{}, err
		}
	}

	groupResults := make([]ireport.PatchGroupResult, 0, len(result.Groups))
	for _, group := range result.Groups {
		replacedEntries := make([]string, 0, len(group.ReplacementFiles))
		for _, file := range group.ReplacementFiles {
			replacedEntries = append(replacedEntries, file.EntryPath)
		}
		groupResults = append(groupResults, ireport.PatchGroupResult{
			BinaryName:          group.BinaryName,
			ReplacedEntries:     replacedEntries,
			RemovedStaleEntries: append([]string(nil), group.StaleEntries...),
		})
	}

	rep := ireport.PatchReport{
		InputJar:              cfg.InputJarPath,
		OutputJar:             cfg.OutputJarPath,
		DryRun:                cfg.DryRun,
		ReplacedGroups:        len(groupResults),
		RemovedStaleEntries:   len(result.RemovedStaleEntries),
		RemovedSignatureFiles: len(result.RemovedSignatureFiles),
		ElapsedMillis:         time.Since(startedAt).Milliseconds(),
		Groups:                groupResults,
		SignatureFiles:        append([]string(nil), result.RemovedSignatureFiles...),
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

	entries := make(map[string]struct{}, len(reader.File))
	signatureFiles := make([]string, 0)
	for _, file := range reader.File {
		entries[file.Name] = struct{}{}
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
		for entryName := range entries {
			if belongsToGroup(entryName, group.TopLevelEntry) {
				originalEntries = append(originalEntries, entryName)
			}
		}
		slices.Sort(originalEntries)

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

		planned = append(planned, PlannedGroup{
			BinaryName:       group.BinaryName,
			TopLevelEntry:    group.TopLevelEntry,
			ReplacementFiles: append([]ClassFile(nil), group.Files...),
			OriginalEntries:  originalEntries,
			StaleEntries:     staleEntries,
		})
	}

	slices.SortFunc(planned, func(a, b PlannedGroup) int {
		return strings.Compare(a.BinaryName, b.BinaryName)
	})

	return Plan{
		Groups:         planned,
		SignatureFiles: signatureFiles,
	}, nil
}

func ApplyPatch(jarPath, outputJar string, plan Plan) (Result, error) {
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

	replacementFiles := make(map[string]ClassFile)
	staleEntries := make(map[string]struct{})
	for _, group := range plan.Groups {
		for _, file := range group.ReplacementFiles {
			replacementFiles[file.EntryPath] = file
		}
		for _, entry := range group.StaleEntries {
			staleEntries[entry] = struct{}{}
		}
	}

	removedStaleEntries := make([]string, 0)
	removedSignatureFiles := append([]string(nil), plan.SignatureFiles...)
	seenRemovedStale := map[string]struct{}{}

	for _, file := range reader.File {
		if isInvalidatedSignature(file.Name) {
			continue
		}

		if _, ok := staleEntries[file.Name]; ok {
			if _, seen := seenRemovedStale[file.Name]; !seen {
				removedStaleEntries = append(removedStaleEntries, file.Name)
				seenRemovedStale[file.Name] = struct{}{}
			}
			continue
		}

		if _, ok := replacementFiles[file.Name]; ok {
			continue
		}

		if err := copyZipEntry(writer, file); err != nil {
			return Result{}, fmt.Errorf("copy archive entry %s: %w", file.Name, err)
		}
	}

	for _, group := range plan.Groups {
		for _, replacement := range group.ReplacementFiles {
			if err := writeReplacementEntry(writer, replacement); err != nil {
				return Result{}, fmt.Errorf("write replacement entry %s: %w", replacement.EntryPath, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return Result{}, fmt.Errorf("close output jar writer: %w", err)
	}

	slices.Sort(removedStaleEntries)
	slices.Sort(removedSignatureFiles)

	return Result{
		Groups:                append([]PlannedGroup(nil), plan.Groups...),
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
	data, err := os.ReadFile(file.DiskPath)
	if err != nil {
		return err
	}

	header := &zip.FileHeader{
		Name:   file.EntryPath,
		Method: zip.Deflate,
	}
	w, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
