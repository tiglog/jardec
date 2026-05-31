package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNewAppParsesOptionsIntoConfig(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/tools/jadx",
		"--cfr-path", "/tools/cfr",
		"--temp-dir", "/tmp/jardec",
		"--keep-temp",
		"--retry-concurrency", "5",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.InputPath != "sample.jar" {
		t.Fatalf("InputPath = %q, want sample.jar", got.InputPath)
	}
	if got.OutputDir != "out" {
		t.Fatalf("OutputDir = %q, want out", got.OutputDir)
	}
	if got.JadxPath != "/tools/jadx" {
		t.Fatalf("JadxPath = %q, want /tools/jadx", got.JadxPath)
	}
	if got.CfrPath != "/tools/cfr" {
		t.Fatalf("CfrPath = %q, want /tools/cfr", got.CfrPath)
	}
	if got.TempDir != "/tmp/jardec" {
		t.Fatalf("TempDir = %q, want /tmp/jardec", got.TempDir)
	}
	if !got.KeepTemp {
		t.Fatal("KeepTemp = false, want true")
	}
	if got.RetryConcurrency != 5 {
		t.Fatalf("RetryConcurrency = %d, want 5", got.RetryConcurrency)
	}
}

func TestNewAppRejectsMissingRequiredOptions(t *testing.T) {
	t.Parallel()

	called := false
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		called = true
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{"jardec", "--output", "out"})
	if err == nil {
		t.Fatal("RunContext() error = nil, want validation error")
	}
	if called {
		t.Fatal("run callback was called despite validation failure")
	}
}

func TestNewAppUsesExplicitBinaryOverrides(t *testing.T) {
	t.Parallel()

	var lookedUp []string
	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		return nil
	}, func(name string) (string, error) {
		lookedUp = append(lookedUp, name)
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/custom/jadx",
		"--cfr-path", "/custom/cfr",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	want := []string{"/custom/jadx", "/custom/cfr"}
	if !slices.Equal(lookedUp, want) {
		t.Fatalf("lookup calls = %v, want %v", lookedUp, want)
	}
}

func TestNewAppReportsBinaryLookupFailures(t *testing.T) {
	t.Parallel()

	app := newAppWithDeps(func(_ context.Context, _ Config) error {
		return nil
	}, func(name string) (string, error) {
		if name == "jadx" {
			return "", errors.New("not found")
		}
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err == nil {
		t.Fatal("RunContext() error = nil, want lookup error")
	}
}

func TestNewAppUsesConfigFileDefaults(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			CfrPath:                 "/config/cfr",
			DefaultRetryConcurrency: 7,
		}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/config/jadx" {
		t.Fatalf("JadxPath = %q, want /config/jadx", got.JadxPath)
	}
	if got.CfrPath != "/config/cfr" {
		t.Fatalf("CfrPath = %q, want /config/cfr", got.CfrPath)
	}
	if got.RetryConcurrency != 7 {
		t.Fatalf("RetryConcurrency = %d, want 7", got.RetryConcurrency)
	}
}

func TestNewAppFlagsOverrideConfigFileDefaults(t *testing.T) {
	t.Parallel()

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		return "/resolved/" + name, nil
	}, func() (ProjectConfig, error) {
		return ProjectConfig{
			JadxPath:                "/config/jadx",
			CfrPath:                 "/config/cfr",
			DefaultRetryConcurrency: 7,
		}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
		"--jadx-path", "/flag/jadx",
		"--cfr-path", "/flag/cfr",
		"--retry-concurrency", "3",
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}

	if got.JadxPath != "/flag/jadx" {
		t.Fatalf("JadxPath = %q, want /flag/jadx", got.JadxPath)
	}
	if got.CfrPath != "/flag/cfr" {
		t.Fatalf("CfrPath = %q, want /flag/cfr", got.CfrPath)
	}
	if got.RetryConcurrency != 3 {
		t.Fatalf("RetryConcurrency = %d, want 3", got.RetryConcurrency)
	}
}

func TestNewAppSupportsDirectCFRJarPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfrJar := filepath.Join(dir, "cfr.jar")
	if err := os.WriteFile(cfrJar, []byte("jar"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var got Config
	app := newAppWithDeps(func(_ context.Context, cfg Config) error {
		got = cfg
		return nil
	}, func(name string) (string, error) {
		if name == "java" {
			return "/usr/bin/java", nil
		}
		if name == "jadx" {
			return "/usr/bin/jadx", nil
		}
		return "", errors.New("unexpected lookup")
	}, func() (ProjectConfig, error) {
		return ProjectConfig{}, nil
	})

	err := app.RunContext(context.Background(), []string{
		"jardec",
		"--input", "sample.jar",
		"--output", "out",
		"--cfr-path", cfrJar,
	})
	if err != nil {
		t.Fatalf("RunContext() error = %v", err)
	}
	if got.CfrPath != cfrJar {
		t.Fatalf("CfrPath = %q, want %q", got.CfrPath, cfrJar)
	}
}
