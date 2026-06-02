package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"jardec/internal/decompiler"
	jarpkg "jardec/internal/jar"
)

var (
	ErrMissingRetryOutput   = errors.New("missing retry output")
	ErrAmbiguousRetryOutput = errors.New("ambiguous retry output")
	ErrInvalidRetryOutput   = errors.New("invalid retry output")
)

type ProcyonRetryConfig struct {
	BaseTempDir    string
	ProcyonPath string
	InputJar       string
	ExtraClasspath []string
	Concurrency    int
}

type RetryResult struct {
	Class       jarpkg.Class
	RootDir     string
	OutputDir   string
	Diagnostics decompiler.RunResult
	Err         error
}

func ExecuteProcyonRetries(ctx context.Context, runner decompiler.Runner, cfg ProcyonRetryConfig, classes []jarpkg.Class) ([]RetryResult, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}

	results := make([]RetryResult, len(classes))
	type job struct {
		index int
		class jarpkg.Class
	}

	jobs := make(chan job)
	var workers sync.WaitGroup
	for range cfg.Concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				results[job.index] = executeSingleRetry(ctx, runner, cfg, job.class)
			}
		}()
	}

	for index, class := range classes {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return nil, ctx.Err()
		case jobs <- job{index: index, class: class}:
		}
	}
	close(jobs)
	workers.Wait()

	return results, nil
}

func executeSingleRetry(ctx context.Context, runner decompiler.Runner, cfg ProcyonRetryConfig, class jarpkg.Class) RetryResult {
	rootDir, err := os.MkdirTemp(cfg.BaseTempDir, "procyon-*")
	if err != nil {
		return RetryResult{Class: class, Err: err}
	}

	classFile := filepath.Join(rootDir, "classes", filepath.FromSlash(class.EntryPath))
	if err := jarpkg.ExtractEntry(cfg.InputJar, class.EntryPath, classFile); err != nil {
		return RetryResult{Class: class, RootDir: rootDir, Err: err}
	}

	outputDir := filepath.Join(rootDir, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return RetryResult{Class: class, RootDir: rootDir, Err: err}
	}

	diagnostics, err := decompiler.RunProcyon(ctx, runner, decompiler.ProcyonConfig{
		JarPath:   cfg.ProcyonPath,
		ClassFile: classFile,
		OutputDir: outputDir,
		Classpath: buildRetryClasspath(cfg.InputJar, cfg.ExtraClasspath),
	})

	return RetryResult{
		Class:       class,
		RootDir:     rootDir,
		OutputDir:   outputDir,
		Diagnostics: diagnostics,
		Err:         err,
	}
}

func buildRetryClasspath(inputJar string, extraClasspath []string) []string {
	classpath := make([]string, 0, 1+len(extraClasspath))
	if inputJar != "" {
		classpath = append(classpath, inputJar)
	}
	for _, entry := range extraClasspath {
		entry = strings.TrimSpace(entry)
		if entry != "" && !slices.Contains(classpath, entry) {
			classpath = append(classpath, entry)
		}
	}
	return classpath
}

func ValidateRetryOutput(class jarpkg.Class, outputDir string) error {
	expectedPath := filepath.Join(outputDir, filepath.FromSlash(class.SourcePath))
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrMissingRetryOutput
		}
		return err
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" || hasRetryPlaceholderFailure(trimmed) {
		return ErrInvalidRetryOutput
	}

	javaFiles := make([]string, 0)
	err = filepath.WalkDir(outputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".java" {
			javaFiles = append(javaFiles, filepath.Clean(path))
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, jf := range javaFiles {
		if jf != filepath.Clean(expectedPath) {
			return ErrAmbiguousRetryOutput
		}
	}

	return nil
}

func hasRetryPlaceholderFailure(content string) bool {
	placeholders := []string{
		"JADX ERROR",
		"Method not decompiled",
		"Code decompiled incorrectly",
		"/* Could not decompile. */",
	}
	for _, marker := range placeholders {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}
