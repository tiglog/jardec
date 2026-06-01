package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	urfavecli "github.com/urfave/cli/v2"
)

type PatchConfig struct {
	InputJarPath  string
	ClassesDir    string
	OutputJarPath string
	DryRun        bool
	TargetClasses []string
}

func PatchConfigFromContext(ctx *urfavecli.Context) (PatchConfig, error) {
	return PatchConfig{
		InputJarPath:  ctx.String("input-jar"),
		ClassesDir:    ctx.String("classes-dir"),
		OutputJarPath: ctx.String("output-jar"),
		DryRun:        ctx.Bool("dry-run"),
		TargetClasses: ctx.StringSlice("class"),
	}, nil
}

func ValidatePatchConfig(cfg PatchConfig) (PatchConfig, error) {
	if cfg.InputJarPath == "" {
		return PatchConfig{}, errors.New("input jar is required")
	}
	inputInfo, err := os.Stat(cfg.InputJarPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PatchConfig{}, fmt.Errorf("input jar does not exist: %s", cfg.InputJarPath)
		}
		return PatchConfig{}, fmt.Errorf("stat input jar: %w", err)
	}
	if inputInfo.IsDir() {
		return PatchConfig{}, errors.New("input jar must be a file")
	}

	if cfg.ClassesDir == "" {
		return PatchConfig{}, errors.New("classes directory is required")
	}
	classesInfo, err := os.Stat(cfg.ClassesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PatchConfig{}, fmt.Errorf("classes directory does not exist: %s", cfg.ClassesDir)
		}
		return PatchConfig{}, fmt.Errorf("stat classes directory: %w", err)
	}
	if !classesInfo.IsDir() {
		return PatchConfig{}, errors.New("classes directory is not a directory")
	}
	if _, err := os.ReadDir(cfg.ClassesDir); err != nil {
		return PatchConfig{}, fmt.Errorf("read classes directory: %w", err)
	}

	if cfg.OutputJarPath == "" {
		return PatchConfig{}, errors.New("output jar path is required")
	}
	if err := validateOutputJarPath(cfg.OutputJarPath); err != nil {
		return PatchConfig{}, err
	}

	normalizedTargets := make([]string, 0, len(cfg.TargetClasses))
	for _, target := range cfg.TargetClasses {
		target = strings.TrimSpace(target)
		if target == "" {
			return PatchConfig{}, errors.New("class target must not be empty")
		}
		if !slices.Contains(normalizedTargets, target) {
			normalizedTargets = append(normalizedTargets, target)
		}
	}
	cfg.TargetClasses = normalizedTargets

	return cfg, nil
}

func validateOutputJarPath(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		if info.IsDir() {
			return errors.New("output jar path must be a file")
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return fmt.Errorf("output jar path is not writable: %w", err)
		}
		return file.Close()
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("stat output jar path: %w", err)
	}

	parentDir := filepath.Dir(path)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	probe, err := os.CreateTemp(parentDir, ".jardec-write-check-*")
	if err != nil {
		return fmt.Errorf("output jar path is not writable: %w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("close output jar probe file: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("remove output jar probe file: %w", err)
	}

	return nil
}
