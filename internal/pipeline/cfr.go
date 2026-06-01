package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

type CfrRetryConfig struct {
	BaseTempDir string
	CfrPath     string
	InputJar    string
	Concurrency int
}

type RetryResult struct {
	Class       jarpkg.Class
	RootDir     string
	OutputDir   string
	Diagnostics decompiler.RunResult
	Err         error
}

func ExecuteCFRRetries(ctx context.Context, runner decompiler.Runner, cfg CfrRetryConfig, classes []jarpkg.Class) ([]RetryResult, error) {
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

func executeSingleRetry(ctx context.Context, runner decompiler.Runner, cfg CfrRetryConfig, class jarpkg.Class) RetryResult {
	rootDir, err := os.MkdirTemp(cfg.BaseTempDir, "cfr-*")
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

	diagnostics, err := decompiler.RunCFR(ctx, runner, decompiler.CfrConfig{
		BinaryPath: cfg.CfrPath,
		ClassFile:  classFile,
		OutputDir:  outputDir,
	})

	return RetryResult{
		Class:       class,
		RootDir:     rootDir,
		OutputDir:   outputDir,
		Diagnostics: diagnostics,
		Err:         err,
	}
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
	if trimmed == "" || hasPlaceholderFailure(trimmed) {
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

	if len(javaFiles) != 1 || javaFiles[0] != filepath.Clean(expectedPath) {
		return ErrAmbiguousRetryOutput
	}

	return nil
}
