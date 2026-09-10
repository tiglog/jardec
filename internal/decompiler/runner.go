package decompiler

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CommandSpec struct {
	Path string
	Args []string
	Dir  string
	Env  []string
}

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

const maxDiagnosticBytes = 4096

func TruncateDiagnostic(text string) string {
	if len(text) <= maxDiagnosticBytes {
		return text
	}
	return text[:maxDiagnosticBytes] + "\n[truncated]"
}

type Runner interface {
	Run(context.Context, CommandSpec) (RunResult, error)
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, spec CommandSpec) (RunResult, error) {
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = os.Environ()
	if len(spec.Env) > 0 {
		cmd.Env = append(cmd.Env, spec.Env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}

	if err == nil {
		return result, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		result.ExitCode = -1
		return result, ctxErr
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, err
	}

	result.ExitCode = -1
	return result, err
}

type JadxConfig struct {
	BinaryPath string
	InputJar   string
	OutputDir  string
	Env        []string
}

type ProcyonConfig struct {
	JarPath   string
	ClassFile string
	OutputDir string
	Classpath []string
}

func RunJadx(ctx context.Context, runner Runner, cfg JadxConfig) (RunResult, error) {
	return runner.Run(ctx, JadxCommand(cfg))
}

func JadxCommand(cfg JadxConfig) CommandSpec {
	return CommandSpec{
		Path: cfg.BinaryPath,
		Args: []string{"-d", cfg.OutputDir, cfg.InputJar},
		Env:  cfg.Env,
	}
}

func RunProcyon(ctx context.Context, runner Runner, cfg ProcyonConfig) (RunResult, error) {
	return runner.Run(ctx, ProcyonCommand(cfg))
}

func ProcyonCommand(cfg ProcyonConfig) CommandSpec {
	args := []string{"-jar", cfg.JarPath, "-o", cfg.OutputDir}
	if len(cfg.Classpath) > 0 {
		args = append(args, "--classpath", strings.Join(cfg.Classpath, string(filepath.ListSeparator)))
	}
	args = append(args, cfg.ClassFile)
	return CommandSpec{
		Path: "java",
		Args: args,
	}
}

func RunProcyonPreflight(ctx context.Context, runner Runner, jarPath string) (RunResult, error) {
	return runner.Run(ctx, CommandSpec{
		Path: "java",
		Args: []string{"-jar", jarPath, "--help"},
	})
}

func DescribeCommand(spec CommandSpec) string {
	return strings.Join(append([]string{spec.Path}, spec.Args...), " ")
}
