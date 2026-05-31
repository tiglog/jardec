package merge

import (
	"io"
	"os"
	"path/filepath"

	jarpkg "jardec/internal/jar"
)

func ApplyRecovery(finalDir string, class jarpkg.Class, retryOutputDir string) error {
	srcPath := filepath.Join(retryOutputDir, filepath.FromSlash(class.SourcePath))
	dstPath := filepath.Join(finalDir, filepath.FromSlash(class.SourcePath))

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}
