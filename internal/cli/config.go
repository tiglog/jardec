package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	urfavecli "github.com/urfave/cli/v2"
)

const (
	defaultJadxBinary  = "jadx"
	defaultJavacBinary = "javac"
)

type Config struct {
	InputPath        string
	OutputDir        string
	JadxPath         string
	ProcyonPath      string
	ExtraClasspath   []string
	TempDir          string
	KeepTemp         bool
	RetryConcurrency int
	ProcyonTimeout   time.Duration
}

func ConfigFromContext(ctx *urfavecli.Context) (Config, error) {
	if ctx.IsSet("procyon-timeout") && ctx.Duration("procyon-timeout") <= 0 {
		return Config{}, errors.New("procyon timeout must be greater than zero")
	}
	cfg := Config{
		InputPath:        ctx.String("input"),
		OutputDir:        ctx.String("output"),
		JadxPath:         ctx.String("jadx-path"),
		ProcyonPath:      ctx.String("procyon-path"),
		ExtraClasspath:   ctx.StringSlice("classpath"),
		TempDir:          ctx.String("temp-dir"),
		KeepTemp:         ctx.Bool("keep-temp"),
		RetryConcurrency: ctx.Int("retry-concurrency"),
		ProcyonTimeout:   ctx.Duration("procyon-timeout"),
	}

	return cfg, nil
}

func ApplyProjectConfig(cfg Config, projectCfg ProjectConfig) Config {
	if cfg.JadxPath == "" {
		cfg.JadxPath = projectCfg.JadxPath
	}
	if cfg.ProcyonPath == "" {
		cfg.ProcyonPath = projectCfg.ProcyonPath
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
	if cfg.RetryConcurrency == 0 {
		cfg.RetryConcurrency = projectCfg.DefaultRetryConcurrency
	}

	return cfg
}

func ValidateConfig(cfg Config, lookup LookupFunc) (Config, error) {
	if cfg.InputPath == "" {
		return Config{}, errors.New("input is required")
	}
	if cfg.OutputDir == "" {
		return Config{}, errors.New("output is required")
	}
	classpath := make([]string, 0, len(cfg.ExtraClasspath))
	for _, entry := range cfg.ExtraClasspath {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return Config{}, errors.New("classpath entry must not be empty")
		}
		expandedEntries, err := expandClasspathEntry(entry)
		if err != nil {
			return Config{}, err
		}
		for _, expanded := range expandedEntries {
			if !slices.Contains(classpath, expanded) {
				classpath = append(classpath, expanded)
			}
		}
	}
	cfg.ExtraClasspath = classpath
	if cfg.RetryConcurrency == 0 {
		cfg.RetryConcurrency = runtime.NumCPU()
	}
	if cfg.RetryConcurrency <= 0 {
		return Config{}, errors.New("retry concurrency must be greater than zero")
	}
	if cfg.ProcyonTimeout == 0 {
		cfg.ProcyonTimeout = 90 * time.Second
	}
	if cfg.ProcyonTimeout < 0 {
		return Config{}, errors.New("procyon timeout must be greater than zero")
	}
	if lookup == nil {
		return Config{}, errors.New("lookup function is required")
	}

	jadxTarget := cfg.JadxPath
	if jadxTarget == "" {
		jadxTarget = defaultJadxBinary
	}
	if _, err := lookup(jadxTarget); err != nil {
		return Config{}, fmt.Errorf("resolve jadx binary: %w", err)
	}
	if cfg.JadxPath == "" {
		cfg.JadxPath = jadxTarget
	}

	if cfg.ProcyonPath == "" {
		return Config{}, errors.New("procyon jar path is required; set --procyon-path or procyon_path in config.yaml")
	}
	if !isJarPath(cfg.ProcyonPath) {
		return Config{}, fmt.Errorf("procyon jar path %q must have a .jar extension", cfg.ProcyonPath)
	}
	procyonInfo, err := os.Stat(cfg.ProcyonPath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve procyon jar %q: %w", cfg.ProcyonPath, err)
	}
	if !procyonInfo.Mode().IsRegular() {
		return Config{}, fmt.Errorf("procyon jar path %q must be a regular file", cfg.ProcyonPath)
	}
	procyonFile, err := os.Open(cfg.ProcyonPath)
	if err != nil {
		return Config{}, fmt.Errorf("open procyon jar %q: %w", cfg.ProcyonPath, err)
	}
	if err := procyonFile.Close(); err != nil {
		return Config{}, fmt.Errorf("close procyon jar %q: %w", cfg.ProcyonPath, err)
	}
	if _, err := lookup("java"); err != nil {
		return Config{}, fmt.Errorf("resolve java runtime: %w", err)
	}

	return cfg, nil
}

func isJarPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".jar")
}

func expandClasspathEntry(entry string) ([]string, error) {
	info, err := os.Stat(entry)
	if err != nil {
		return nil, fmt.Errorf("stat classpath entry %q: %w", entry, err)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("classpath entry %q must be a regular jar file", entry)
		}
		if !isJarPath(entry) {
			return nil, fmt.Errorf("classpath entry %q must have a .jar extension", entry)
		}
		file, err := os.Open(entry)
		if err != nil {
			return nil, fmt.Errorf("open classpath entry %q: %w", entry, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close classpath entry %q: %w", entry, err)
		}
		return []string{entry}, nil
	}

	entries, err := os.ReadDir(entry)
	if err != nil {
		return nil, fmt.Errorf("read classpath directory: %w", err)
	}

	expanded := make([]string, 0, len(entries))
	for _, child := range entries {
		if child.IsDir() || !isJarPath(child.Name()) {
			continue
		}
		childPath := filepath.Join(entry, child.Name())
		childInfo, err := child.Info()
		if err != nil || !childInfo.Mode().IsRegular() {
			continue
		}
		expanded = append(expanded, childPath)
	}
	sort.Strings(expanded)
	if len(expanded) == 0 {
		return nil, fmt.Errorf("classpath directory contains no jar files: %s", entry)
	}

	return expanded, nil
}
