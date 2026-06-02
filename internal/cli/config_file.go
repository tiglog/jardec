package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFileName = "config.yaml"

type ProjectConfig struct {
	JadxPath                string   `yaml:"jadx_path"`
	VineflowerPath          string   `yaml:"vineflower_path"`
	JavacPath               string   `yaml:"javac_path"`
	DecompileClasspath      []string `yaml:"decompile_classpath"`
	PatchSourcesClasspath   []string `yaml:"patch_sources_classpath"`
	DefaultRetryConcurrency int      `yaml:"default_retry_concurrency"`
	ConfigDir               string   `yaml:"-"`
}

func loadProjectConfigFromWorkingDir() (ProjectConfig, error) {
	rootDir, err := os.Getwd()
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("get working directory: %w", err)
	}

	return LoadProjectConfig(rootDir)
}

func LoadProjectConfigFromPath(configPath string) (ProjectConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProjectConfig{}, fmt.Errorf("config file not found: %s", configPath)
		}
		return ProjectConfig{}, fmt.Errorf("read config file: %w", err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, fmt.Errorf("parse config file: %w", err)
	}
	cfg.ConfigDir = filepath.Dir(configPath)
	return cfg, nil
}

func LoadProjectConfig(rootDir string) (ProjectConfig, error) {
	currentDir := rootDir
	for {
		path := filepath.Join(currentDir, DefaultConfigFileName)
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg ProjectConfig
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return ProjectConfig{}, fmt.Errorf("parse config file: %w", err)
			}
			cfg.ConfigDir = currentDir
			return cfg, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return ProjectConfig{}, fmt.Errorf("read config file: %w", err)
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return ProjectConfig{}, nil
		}
		currentDir = parentDir
	}
}
