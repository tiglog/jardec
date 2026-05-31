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
	JadxPath                string `yaml:"jadx_path"`
	CfrPath                 string `yaml:"cfr_path"`
	DefaultRetryConcurrency int    `yaml:"default_retry_concurrency"`
}

func loadProjectConfigFromWorkingDir() (ProjectConfig, error) {
	rootDir, err := os.Getwd()
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("get working directory: %w", err)
	}

	return LoadProjectConfig(rootDir)
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
