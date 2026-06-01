package sourcepatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	appcli "jardec/internal/cli"
	"jardec/internal/decompiler"
)

type CompileResult struct {
	ClassesDir  string
	SourceFiles []string
	Classpath   []string
	Diagnostics string
}

type Compiler struct {
	Runner decompiler.Runner
}

func (c Compiler) Compile(ctx context.Context, cfg appcli.SourcePatchConfig) (CompileResult, error) {
	sourceFiles := make([]string, 0, len(cfg.TargetClasses))
	for _, target := range cfg.TargetClasses {
		sourcePath := filepath.Join(cfg.SourcesDir, binaryNameToRelativePath(target, ".java"))
		info, err := os.Stat(sourcePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return CompileResult{}, fmt.Errorf("target source %s is missing at %s", target, sourcePath)
			}
			return CompileResult{}, fmt.Errorf("stat target source %s: %w", target, err)
		}
		if info.IsDir() {
			return CompileResult{}, fmt.Errorf("target source %s resolved to directory %s", target, sourcePath)
		}
		sourceFiles = append(sourceFiles, sourcePath)
	}

	classesDir, err := os.MkdirTemp("", "jardec-patch-sources-*")
	if err != nil {
		return CompileResult{}, fmt.Errorf("create compile workspace: %w", err)
	}

	classpath := append([]string{cfg.InputJarPath}, cfg.ExtraClasspath...)
	spec := decompiler.CommandSpec{
		Path: cfg.JavacPath,
		Args: append([]string{
			"-cp",
			strings.Join(classpath, string(os.PathListSeparator)),
			"-d",
			classesDir,
		}, sourceFiles...),
	}

	runner := c.Runner
	if runner == nil {
		runner = decompiler.CommandRunner{}
	}

	runResult, runErr := runner.Run(ctx, spec)
	result := CompileResult{
		ClassesDir:  classesDir,
		SourceFiles: append([]string(nil), sourceFiles...),
		Classpath:   append([]string(nil), classpath...),
		Diagnostics: combineDiagnostics(runResult),
	}
	if runErr != nil {
		_ = os.RemoveAll(classesDir)
		result.ClassesDir = ""
		return result, fmt.Errorf("compile Java sources: %w: %s", runErr, result.Diagnostics)
	}

	for _, target := range cfg.TargetClasses {
		classPath := filepath.Join(classesDir, binaryNameToRelativePath(target, ".class"))
		info, err := os.Stat(classPath)
		if err != nil {
			_ = os.RemoveAll(classesDir)
			result.ClassesDir = ""
			if errors.Is(err, os.ErrNotExist) {
				return result, fmt.Errorf("compiled output for %s is missing at %s", target, classPath)
			}
			return result, fmt.Errorf("stat compiled output for %s: %w", target, err)
		}
		if info.IsDir() {
			_ = os.RemoveAll(classesDir)
			result.ClassesDir = ""
			return result, fmt.Errorf("compiled output for %s resolved to directory %s", target, classPath)
		}
	}

	return result, nil
}

func binaryNameToRelativePath(binaryName, extension string) string {
	return filepath.Join(strings.Split(binaryName, ".")...) + extension
}

func combineDiagnostics(result decompiler.RunResult) string {
	parts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(result.Stderr); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(result.Stdout); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n")
}
