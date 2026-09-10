package decompiler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
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

func TestCommandRunnerReturnsAfterCancellationWhenChildKeepsPipesOpen(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, err := (CommandRunner{}).Run(ctx, CommandSpec{Path: "sh", Args: []string{"-c", "sleep 5 & wait"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Fatalf("Run() elapsed = %s, want bounded cancellation", elapsed)
	}
}

func TestCommandRunnerCancelsLinuxProcessGroup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process-group assertion is Linux-specific")
	}
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := (CommandRunner{}).Run(ctx, CommandSpec{Path: "sh", Args: []string{"-c", "sleep 5 & echo $! > " + pidPath + "; wait"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context deadline", err)
	}
	pidText, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatalf("ReadFile(pid): %v", readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if parseErr != nil {
		t.Fatalf("Atoi(pid): %v", parseErr)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child pid %d is still alive: %v", pid, err)
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

func TestRunJadxPassesConfiguredEnvironment(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunJadx(context.Background(), fake, JadxConfig{
		BinaryPath: "/tools/jadx",
		InputJar:   "sample.jar",
		OutputDir:  "out",
		Env:        []string{"XDG_CONFIG_HOME=/tmp/config", "XDG_CACHE_HOME=/tmp/cache"},
	})
	if err != nil {
		t.Fatalf("RunJadx() error = %v", err)
	}
	want := []string{"XDG_CONFIG_HOME=/tmp/config", "XDG_CACHE_HOME=/tmp/cache"}
	if !slices.Equal(fake.spec.Env, want) {
		t.Fatalf("Env = %v, want %v", fake.spec.Env, want)
	}
}

func TestRunProcyonBuildsExpectedCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunProcyon(context.Background(), fake, ProcyonConfig{
		JarPath:   "/tools/procyon.jar",
		ClassFile: filepath.Join("tmp", "Foo.class"),
		OutputDir: "out",
		Classpath: []string{"input.jar", "/deps/a.jar"},
	})
	if err != nil {
		t.Fatalf("RunProcyon() error = %v", err)
	}

	if fake.spec.Path != "java" {
		t.Fatalf("Path = %q, want java", fake.spec.Path)
	}
	want := []string{
		"-jar", "/tools/procyon.jar",
		"-o", "out",
		"--classpath", strings.Join([]string{"input.jar", "/deps/a.jar"}, string(os.PathListSeparator)),
		filepath.Join("tmp", "Foo.class"),
	}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestRunProcyonOmitsClasspathWhenEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunProcyon(context.Background(), fake, ProcyonConfig{
		JarPath:   "/tools/procyon.jar",
		ClassFile: filepath.Join("tmp", "Foo.class"),
		OutputDir: "out",
	})
	if err != nil {
		t.Fatalf("RunProcyon() error = %v", err)
	}

	want := []string{
		"-jar", "/tools/procyon.jar",
		"-o", "out",
		filepath.Join("tmp", "Foo.class"),
	}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestRunProcyonPreflightBuildsHelpCommand(t *testing.T) {
	t.Parallel()

	fake := &fakeRunner{}
	_, err := RunProcyonPreflight(context.Background(), fake, "/tools/procyon.jar")
	if err != nil {
		t.Fatalf("RunProcyonPreflight() error = %v", err)
	}

	if fake.spec.Path != "java" {
		t.Fatalf("Path = %q, want java", fake.spec.Path)
	}
	want := []string{"-jar", "/tools/procyon.jar", "--help"}
	if !slices.Equal(fake.spec.Args, want) {
		t.Fatalf("Args = %v, want %v", fake.spec.Args, want)
	}
}

func TestTruncateDiagnosticBoundsToolOutput(t *testing.T) {
	t.Parallel()

	got := TruncateDiagnostic(strings.Repeat("x", maxDiagnosticBytes+1))
	if !strings.HasPrefix(got, strings.Repeat("x", maxDiagnosticBytes)) {
		t.Fatalf("TruncateDiagnostic() prefix = %q, want first %d bytes", got, maxDiagnosticBytes)
	}
	if !strings.HasSuffix(got, "\n[truncated]") {
		t.Fatalf("TruncateDiagnostic() = %q, want truncation marker", got)
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
