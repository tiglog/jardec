package jar

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ExtractEntry(jarPath, entryPath, destPath string) error {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != entryPath {
			continue
		}

		src, err := file.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}

		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	}

	return fmt.Errorf("jar entry not found: %s", entryPath)
}
