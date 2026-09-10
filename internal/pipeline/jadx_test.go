package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"jardec/internal/decompiler"
)

func TestExecuteJadxCreatesWorkspaceAndCapturesDiagnostics(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	fake := &fakeJadxRunner{
		run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
			if spec.Path != "/tools/jadx" {
				t.Fatalf("Path = %q, want /tools/jadx", spec.Path)
			}
			if len(spec.Args) != 3 || spec.Args[0] != "-d" || spec.Args[2] != "input.jar" {
				t.Fatalf("Args = %v, want ['-d' <out> 'input.jar']", spec.Args)
			}
			workspaceRoot := filepath.Dir(spec.Args[1])
			wantEnv := []string{
				"XDG_CONFIG_HOME=" + filepath.Join(workspaceRoot, "xdg-config"),
				"XDG_CACHE_HOME=" + filepath.Join(workspaceRoot, "xdg-cache"),
			}
			if !slices.Equal(spec.Env, wantEnv) {
				t.Fatalf("Env = %v, want %v", spec.Env, wantEnv)
			}
			for _, dir := range []string{filepath.Join(workspaceRoot, "xdg-config"), filepath.Join(workspaceRoot, "xdg-cache")} {
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					t.Fatalf("XDG directory %q = %v, want existing directory", dir, err)
				}
			}
			outputDir := spec.Args[1]
			if err := os.WriteFile(filepath.Join(outputDir, "sources", "marker.txt"), []byte("ok"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			if err := os.WriteFile(filepath.Join(outputDir, "resources", "config.properties"), []byte("k=v"), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			return decompiler.RunResult{Stdout: "jadx ok"}, nil
		},
	}

	workspace, err := ExecuteJadx(context.Background(), fake, JadxWorkspaceConfig{
		BaseTempDir: baseDir,
		JadxPath:    "/tools/jadx",
		InputJar:    "input.jar",
	})
	if err != nil {
		t.Fatalf("ExecuteJadx() error = %v", err)
	}

	if workspace.RootDir == "" {
		t.Fatal("RootDir is empty")
	}
	if workspace.OutputDir == "" {
		t.Fatal("OutputDir is empty")
	}
	if workspace.SourcesDir == "" {
		t.Fatal("SourcesDir is empty")
	}
	if workspace.ResourcesDir == "" {
		t.Fatal("ResourcesDir is empty")
	}
	if workspace.Diagnostics.Stdout != "jadx ok" {
		t.Fatalf("Diagnostics.Stdout = %q, want 'jadx ok'", workspace.Diagnostics.Stdout)
	}
	if _, err := os.Stat(filepath.Join(workspace.SourcesDir, "marker.txt")); err != nil {
		t.Fatalf("expected sources marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.ResourcesDir, "config.properties")); err != nil {
		t.Fatalf("expected resources marker: %v", err)
	}
}

type fakeJadxRunner struct {
	run func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (f *fakeJadxRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	return f.run(spec)
}
