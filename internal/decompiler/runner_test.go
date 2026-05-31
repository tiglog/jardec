package decompiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestCommandRunnerCapturesOutputAndExitCode(t *testing.T) {
	t.Parallel()

	runner := CommandRunner{}
	result, err := runner.Run(context.Background(), CommandSpec{
		Path: "sh",
		Args: []string{"-c", "printf 'hello'; printf 'warn' >&2; exit 7"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit failure")
	}
	if result.Stdout != "hello" {
		t.Fatalf("Stdout = %q, want hello", result.Stdout)
	}
	if result.Stderr != "warn" {
		t.Fatalf("Stderr = %q, want warn", result.Stderr)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
}

func TestCommandRunnerPropagatesContextDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	runner := CommandRunner{}
	_, err := runner.Run(ctx, CommandSpec{
		Path: "sh",
		Args: []string{"-c", "sleep 5"},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded", err)
	}
}

func TestRunJadxBuildsExpectedCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunJadx(context.Background(), fake, JadxConfig{
		BinaryPath: "/tools/jadx",
		InputJar:   "sample.jar",
		OutputDir:  "out",
	})
	if err != nil {
		t.Fatalf("RunJadx() error = %v", err)
	}

	if fake.spec.Path != "/tools/jadx" {
		t.Fatalf("Path = %q, want /tools/jadx", fake.spec.Path)
	}
	want := []string{"-d", "out", "sample.jar"}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestRunCFRBuildsExpectedCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunCFR(context.Background(), fake, CfrConfig{
		BinaryPath: "/tools/cfr",
		ClassFile:  filepath.Join("tmp", "Foo.class"),
		OutputDir:  "out",
	})
	if err != nil {
		t.Fatalf("RunCFR() error = %v", err)
	}

	if fake.spec.Path != "/tools/cfr" {
		t.Fatalf("Path = %q, want /tools/cfr", fake.spec.Path)
	}
	want := []string{filepath.Join("tmp", "Foo.class"), "--outputdir", "out"}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestRunCFRSupportsDirectJarPath(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunCFR(context.Background(), fake, CfrConfig{
		BinaryPath: "/tools/cfr.jar",
		ClassFile:  filepath.Join("tmp", "Foo.class"),
		OutputDir:  "out",
	})
	if err != nil {
		t.Fatalf("RunCFR() error = %v", err)
	}

	if fake.spec.Path != "java" {
		t.Fatalf("Path = %q, want java", fake.spec.Path)
	}
	want := []string{"-jar", "/tools/cfr.jar", filepath.Join("tmp", "Foo.class"), "--outputdir", "out"}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestCommandRunnerPreservesParentEnvironment(t *testing.T) {
	t.Parallel()

	const key = "JARDEC_TEST_PARENT_ENV"
	const value = "present"
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Setenv() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(key)
	})

	runner := CommandRunner{}
	result, err := runner.Run(context.Background(), CommandSpec{
		Path: "sh",
		Args: []string{"-c", "printf %s \"$JARDEC_TEST_PARENT_ENV\""},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Stdout != value {
		t.Fatalf("Stdout = %q, want %q", result.Stdout, value)
	}
}

type fakeRunner struct {
	spec CommandSpec
}

func (f *fakeRunner) Run(_ context.Context, spec CommandSpec) (RunResult, error) {
	f.spec = spec
	return RunResult{}, nil
}
