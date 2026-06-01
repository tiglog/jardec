package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	urfavecli "github.com/urfave/cli/v2"
)

const (
	defaultJadxBinary  = "jadx"
	defaultCfrBinary   = "cfr"
	defaultJavacBinary = "javac"
)

type Config struct {
	InputPath        string
	OutputDir        string
	JadxPath         string
	CfrPath          string
	ExtraClasspath   []string
	TempDir          string
	KeepTemp         bool
	RetryConcurrency int
}

func ConfigFromContext(ctx *urfavecli.Context) (Config, error) {
	cfg := Config{
		InputPath:        ctx.String("input"),
		OutputDir:        ctx.String("output"),
		JadxPath:         ctx.String("jadx-path"),
		CfrPath:          ctx.String("cfr-path"),
		ExtraClasspath:   ctx.StringSlice("classpath"),
		TempDir:          ctx.String("temp-dir"),
		KeepTemp:         ctx.Bool("keep-temp"),
		RetryConcurrency: ctx.Int("retry-concurrency"),
	}

	return cfg, nil
}

func ApplyProjectConfig(cfg Config, projectCfg ProjectConfig) Config {
	if cfg.JadxPath == "" {
		cfg.JadxPath = projectCfg.JadxPath
	}
	if cfg.CfrPath == "" {
		cfg.CfrPath = projectCfg.CfrPath
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
		if !slices.Contains(classpath, entry) {
			classpath = append(classpath, entry)
		}
	}
	cfg.ExtraClasspath = classpath
	if cfg.RetryConcurrency == 0 {
		cfg.RetryConcurrency = runtime.NumCPU()
	}
	if cfg.RetryConcurrency <= 0 {
		return Config{}, errors.New("retry concurrency must be greater than zero")
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

	cfrTarget := cfg.CfrPath
	if cfrTarget == "" {
		cfrTarget = defaultCfrBinary
	}
	if isJarPath(cfrTarget) {
		if _, err := os.Stat(cfrTarget); err != nil {
			return Config{}, fmt.Errorf("resolve cfr jar: %w", err)
		}
		if _, err := lookup("java"); err != nil {
			return Config{}, fmt.Errorf("resolve java runtime: %w", err)
		}
	} else if _, err := lookup(cfrTarget); err != nil {
		return Config{}, fmt.Errorf("resolve cfr binary: %w", err)
	}
	if cfg.CfrPath == "" {
		cfg.CfrPath = cfrTarget
	}

	return cfg, nil
}

func isJarPath(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".jar")
}
