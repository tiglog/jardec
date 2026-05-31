package pipeline

import (
	"context"
	"os"
	"path/filepath"

	"jardec/internal/decompiler"
)

type JadxWorkspaceConfig struct {
	BaseTempDir string
	JadxPath    string
	InputJar    string
}

type JadxWorkspace struct {
	RootDir      string
	OutputDir    string
	SourcesDir   string
	ResourcesDir string
	Diagnostics  decompiler.RunResult
}

func ExecuteJadx(ctx context.Context, runner decompiler.Runner, cfg JadxWorkspaceConfig) (JadxWorkspace, error) {
	rootDir, err := os.MkdirTemp(cfg.BaseTempDir, "jadx-*")
	if err != nil {
		return JadxWorkspace{}, err
	}

	outputDir := filepath.Join(rootDir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return JadxWorkspace{}, err
	}
	sourcesDir := filepath.Join(outputDir, "sources")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		return JadxWorkspace{}, err
	}
	resourcesDir := filepath.Join(outputDir, "resources")
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		return JadxWorkspace{}, err
	}

	result, err := decompiler.RunJadx(ctx, runner, decompiler.JadxConfig{
		BinaryPath: cfg.JadxPath,
		InputJar:   cfg.InputJar,
		OutputDir:  outputDir,
	})

	return JadxWorkspace{
		RootDir:      rootDir,
		OutputDir:    outputDir,
		SourcesDir:   sourcesDir,
		ResourcesDir: resourcesDir,
		Diagnostics:  result,
	}, err
}
