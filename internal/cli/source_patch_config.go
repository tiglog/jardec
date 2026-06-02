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

type SourcePatchConfig struct {
	InputJarPath   string
	SourcesDir     string
	OutputJarPath  string
	TargetClasses  []string
	JavacPath      string
	ExtraClasspath []string
}

func SourcePatchConfigFromContext(ctx *urfavecli.Context) (SourcePatchConfig, error) {
	return SourcePatchConfig{
		InputJarPath:   ctx.String("input-jar"),
		SourcesDir:     ctx.String("sources-dir"),
		OutputJarPath:  ctx.String("output-jar"),
		TargetClasses:  ctx.StringSlice("class"),
		JavacPath:      ctx.String("javac-path"),
		ExtraClasspath: ctx.StringSlice("classpath"),
	}, nil
}

func ApplySourcePatchProjectConfig(cfg SourcePatchConfig, projectCfg ProjectConfig) SourcePatchConfig {
	if cfg.JavacPath == "" {
		cfg.JavacPath = resolveProjectConfigExecutable(projectCfg.ConfigDir, projectCfg.JavacPath)
	}
	classpath := make([]string, 0, len(projectCfg.DecompileClasspath)+len(cfg.ExtraClasspath))
	for _, entry := range projectCfg.DecompileClasspath {
		entry = resolveProjectConfigPath(projectCfg.ConfigDir, entry)
		if entry != "" && !slices.Contains(classpath, entry) {
			classpath = append(classpath, entry)
		}
	}
	for _, entry := range cfg.ExtraClasspath {
		entry = strings.TrimSpace(entry)
		if entry != "" && !slices.Contains(classpath, entry) {
			classpath = append(classpath, entry)
		}
	}
	cfg.ExtraClasspath = classpath
	return cfg
}

func ValidateSourcePatchConfig(cfg SourcePatchConfig, lookup LookupFunc) (SourcePatchConfig, error) {
	if cfg.InputJarPath == "" {
		return SourcePatchConfig{}, errors.New("input jar is required")
	}
	inputInfo, err := os.Stat(cfg.InputJarPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SourcePatchConfig{}, fmt.Errorf("input jar does not exist: %s", cfg.InputJarPath)
		}
		return SourcePatchConfig{}, fmt.Errorf("stat input jar: %w", err)
	}
	if inputInfo.IsDir() {
		return SourcePatchConfig{}, errors.New("input jar must be a file")
	}

	if cfg.SourcesDir == "" {
		return SourcePatchConfig{}, errors.New("sources directory is required")
	}
	sourcesInfo, err := os.Stat(cfg.SourcesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SourcePatchConfig{}, fmt.Errorf("sources directory does not exist: %s", cfg.SourcesDir)
		}
		return SourcePatchConfig{}, fmt.Errorf("stat sources directory: %w", err)
	}
	if !sourcesInfo.IsDir() {
		return SourcePatchConfig{}, errors.New("sources directory is not a directory")
	}
	if _, err := os.ReadDir(cfg.SourcesDir); err != nil {
		return SourcePatchConfig{}, fmt.Errorf("read sources directory: %w", err)
	}

	if cfg.OutputJarPath == "" {
		return SourcePatchConfig{}, errors.New("output jar path is required")
	}
	if err := validateOutputJarPath(cfg.OutputJarPath); err != nil {
		return SourcePatchConfig{}, err
	}

	targets := make([]string, 0, len(cfg.TargetClasses))
	for _, target := range cfg.TargetClasses {
		target = strings.TrimSpace(target)
		if target == "" {
			return SourcePatchConfig{}, errors.New("class target must not be empty")
		}
		if strings.Contains(target, "$") {
			return SourcePatchConfig{}, fmt.Errorf("class target must be a top-level binary name: %s", target)
		}
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return SourcePatchConfig{}, errors.New("at least one class target is required")
	}
	cfg.TargetClasses = targets

	classpath := make([]string, 0, len(cfg.ExtraClasspath))
	for _, entry := range cfg.ExtraClasspath {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return SourcePatchConfig{}, errors.New("classpath entry must not be empty")
		}
		expandedEntries, err := expandClasspathEntry(entry)
		if err != nil {
			return SourcePatchConfig{}, err
		}
		for _, expanded := range expandedEntries {
			if !slices.Contains(classpath, expanded) {
				classpath = append(classpath, expanded)
			}
		}
	}
	cfg.ExtraClasspath = classpath

	if lookup == nil {
		return SourcePatchConfig{}, errors.New("lookup function is required")
	}
	javacTarget := cfg.JavacPath
	if javacTarget == "" {
		javacTarget = defaultJavacBinary
	}
	if _, err := lookup(javacTarget); err != nil {
		return SourcePatchConfig{}, fmt.Errorf("resolve javac binary: %w", err)
	}
	if cfg.JavacPath == "" {
		cfg.JavacPath = javacTarget
	}

	return cfg, nil
}

func resolveProjectConfigExecutable(configDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || configDir == "" || filepath.IsAbs(value) || !strings.ContainsAny(value, `/\`) {
		return value
	}
	return filepath.Clean(filepath.Join(configDir, value))
}

func resolveProjectConfigPath(configDir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || configDir == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Clean(filepath.Join(configDir, value))
}
