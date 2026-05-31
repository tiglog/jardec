package jar

import (
	"archive/zip"
	"path"
	"slices"
	"strings"
)

type Class struct {
	BinaryName string
	EntryPath  string
	SourcePath string
}

func EnumerateTopLevelClasses(jarPath string) ([]Class, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	classes := make([]Class, 0)
	for _, file := range reader.File {
		if !isTopLevelClassEntry(file.Name) {
			continue
		}

		sourcePath := strings.TrimSuffix(file.Name, ".class") + ".java"
		binaryName := strings.ReplaceAll(strings.TrimSuffix(file.Name, ".class"), "/", ".")
		classes = append(classes, Class{
			BinaryName: binaryName,
			EntryPath:  file.Name,
			SourcePath: sourcePath,
		})
	}

	slices.SortFunc(classes, func(a, b Class) int {
		return strings.Compare(a.BinaryName, b.BinaryName)
	})

	return classes, nil
}

func isTopLevelClassEntry(name string) bool {
	if !strings.HasSuffix(name, ".class") {
		return false
	}

	base := path.Base(name)
	if base == "module-info.class" || base == "package-info.class" {
		return false
	}

	return !strings.Contains(base, "$")
}
