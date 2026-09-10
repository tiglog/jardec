package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"jardec/internal/decompiler"
	ireport "jardec/internal/report"
)

func TestEngineRecoversJadxWarnAndWritesReports(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
		"com/example/Bar.class": "bar",
	})

	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[1]
				writePipelineFile(t, outputDir, "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				writePipelineFile(t, outputDir, "sources/com/example/Bar.java", "class Bar {}\n")
				writePipelineFile(t, outputDir, "resources/app.properties", "k=v\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[3]
				writePipelineFile(t, outputDir, "com/example/Foo.java", "class Foo { int recovered = 1; }\n")
				return decompiler.RunResult{}, nil
			},
		},
	}

	outputDir := t.TempDir()
	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        outputDir,
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		RetryConcurrency: 2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.JadxSucceeded != 1 || rep.ProcyonRecovered != 1 || rep.FinalFailed != 0 {
		t.Fatalf("report counts = %+v", rep)
	}
	if rep.RetryCandidates != 1 {
		t.Fatalf("RetryCandidates = %d, want 1", rep.RetryCandidates)
	}
	if rep.TotalElapsedMillis < 0 || rep.RetryElapsedMillis < 0 {
		t.Fatalf("elapsed millis should be non-negative: %+v", rep)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "sources/com/example/Foo.java"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "recovered = 1") {
		t.Fatalf("final Foo.java = %q, want procyon content", string(content))
	}
	if _, err := os.Stat(filepath.Join(outputDir, "com/example/Foo.java")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no top-level java output outside sources/, got err=%v", err)
	}

	var written ireport.Report
	reportData, err := os.ReadFile(filepath.Join(outputDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadFile(report.json) error = %v", err)
	}
	if err := json.Unmarshal(reportData, &written); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if written.ProcyonRecovered != 1 {
		t.Fatalf("written report = %+v, want ProcyonRecovered=1", written)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "resources/app.properties")); err != nil {
		t.Fatalf("expected resources to be preserved: %v", err)
	}
}

func TestEngineStopsBeforeJadxWhenProcyonPreflightFails(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})
	outputDir := filepath.Join(t.TempDir(), "output")
	jadxCalled := false
	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
				jadxCalled = true
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			preflight: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				if want := []string{"-jar", "/tools/procyon.jar", "--help"}; !slices.Equal(spec.Args, want) {
					t.Fatalf("preflight args = %v, want %v", spec.Args, want)
				}
				return decompiler.RunResult{Stderr: "procyon stderr", ExitCode: 9}, errors.New("exit status 9")
			},
		},
	}

	_, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        outputDir,
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		RetryConcurrency: 1,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failed Procyon preflight")
	}
	for _, want := range []string{"exit code 9", "procyon stderr"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run() error = %q, want substring %q", err, want)
		}
	}
	if jadxCalled {
		t.Fatal("jadx ran after Procyon preflight failed")
	}
	if _, statErr := os.Stat(outputDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output directory stat error = %v, want not exist", statErr)
	}
}

func TestEngineCreatesMissingTempRootBeforeRunningTools(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{"com/example/Foo.class": "foo"})
	tempDir := filepath.Join(t.TempDir(), "new", "work")
	preflightCalled := false
	jadxCalled := false
	engine := Engine{
		JadxRunner: &scriptedRunner{run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
			jadxCalled = true
			writePipelineFile(t, spec.Args[1], "sources/com/example/Foo.java", "class Foo {}\n")
			return decompiler.RunResult{}, nil
		}},
		ProcyonRunner: &scriptedRunner{preflight: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
			preflightCalled = true
			return decompiler.RunResult{}, nil
		}},
	}
	_, err := engine.Run(context.Background(), Config{InputPath: jarPath, OutputDir: t.TempDir(), JadxPath: "/tools/jadx", ProcyonPath: "/tools/procyon.jar", TempDir: tempDir, RetryConcurrency: 1})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !preflightCalled || !jadxCalled {
		t.Fatalf("tool calls: preflight=%t, jadx=%t, want both true", preflightCalled, jadxCalled)
	}
	if info, err := os.Stat(tempDir); err != nil || !info.IsDir() {
		t.Fatalf("TempDir %q = %v, want existing directory", tempDir, err)
	}
}

func TestEngineRejectsUncreatableTempRootBeforeRunningTools(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{"com/example/Foo.class": "foo"})
	tempRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(tempRoot, []byte("file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	called := false
	runner := &scriptedRunner{run: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
		called = true
		return decompiler.RunResult{}, nil
	}, preflight: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
		called = true
		return decompiler.RunResult{}, nil
	}}
	_, err := (Engine{JadxRunner: runner, ProcyonRunner: runner}).Run(context.Background(), Config{InputPath: jarPath, OutputDir: t.TempDir(), JadxPath: "/tools/jadx", ProcyonPath: "/tools/procyon.jar", TempDir: tempRoot, RetryConcurrency: 1})
	if err == nil || !strings.Contains(err.Error(), tempRoot) {
		t.Fatalf("Run() error = %v, want temp root path", err)
	}
	if called {
		t.Fatal("a tool ran despite an uncreatable temporary root")
	}
}

func TestEngineReportsJadxStartupDiagnostics(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{"com/example/Foo.class": "foo"})
	engine := Engine{
		JadxRunner: &scriptedRunner{run: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
			return decompiler.RunResult{Stdout: "jadx stdout", Stderr: "jadx stderr", ExitCode: 12}, errors.New("jadx failed")
		}},
		ProcyonRunner: &scriptedRunner{},
	}
	_, err := engine.Run(context.Background(), Config{InputPath: jarPath, OutputDir: t.TempDir(), JadxPath: "/tools/jadx", ProcyonPath: "/tools/procyon.jar", RetryConcurrency: 1})
	if err == nil {
		t.Fatal("Run() error = nil, want JADX startup failure")
	}
	for _, want := range []string{"exit code 12", "jadx stdout", "jadx stderr", "/tools/jadx -d"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run() error = %q, want substring %q", err, want)
		}
	}
}

func TestEngineMarksAmbiguousRetryOutputAsFailure(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})

	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[1]
				writePipelineFile(t, outputDir, "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[3]
				writePipelineFile(t, outputDir, "com/example/Foo.java", "class Foo {}\n")
				writePipelineFile(t, outputDir, "com/example/Extra.java", "class Extra {}\n")
				return decompiler.RunResult{}, nil
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.FinalFailed != 1 {
		t.Fatalf("FinalFailed = %d, want 1", rep.FinalFailed)
	}
	if rep.Classes[0].RetryOutcome != "ambiguous_retry_output" {
		t.Fatalf("RetryOutcome = %q, want ambiguous_retry_output", rep.Classes[0].RetryOutcome)
	}
	if !strings.Contains(ireport.RenderText(rep), "Retry candidates: 1") {
		t.Fatalf("RenderText() missing retry summary: %q", ireport.RenderText(rep))
	}
}

func TestEngineMarksUnrecoverableRetryOutputAsFailure(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})
	tempDir := t.TempDir()

	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[1]
				writePipelineFile(t, outputDir, "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				return decompiler.RunResult{
					Stdout:   "procyon stdout",
					Stderr:   "procyon stderr",
					ExitCode: 17,
				}, errors.New("procyon failed")
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		TempDir:          tempDir,
		KeepTemp:         true,
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.FinalFailed != 1 {
		t.Fatalf("FinalFailed = %d, want 1", rep.FinalFailed)
	}
	if rep.Classes[0].RetryOutcome != "procyon_execution_failed" {
		t.Fatalf("RetryOutcome = %q, want procyon_execution_failed", rep.Classes[0].RetryOutcome)
	}
	diagnostics := rep.Classes[0].ProcyonDiagnostics
	if diagnostics == nil {
		t.Fatal("ProcyonDiagnostics = nil, want execution diagnostics")
	}
	if diagnostics.ExitCode != 17 || diagnostics.Stdout != "procyon stdout" || diagnostics.Stderr != "procyon stderr" {
		t.Fatalf("ProcyonDiagnostics = %+v, want exit code and tool streams", diagnostics)
	}
	if !strings.Contains(diagnostics.Command, "java -jar /tools/procyon.jar") {
		t.Fatalf("ProcyonDiagnostics.Command = %q, want Procyon command", diagnostics.Command)
	}
	if diagnostics.WorkspaceDisposition != "retained" || diagnostics.WorkspacePath == "" {
		t.Fatalf("ProcyonDiagnostics workspace = %+v, want retained workspace path", diagnostics)
	}
}

func TestEngineReportsCleanedWorkspaceAfterProcyonFailure(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})
	tempDir := t.TempDir()
	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				writePipelineFile(t, spec.Args[1], "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(decompiler.CommandSpec) (decompiler.RunResult, error) {
				return decompiler.RunResult{ExitCode: 4}, errors.New("procyon failed")
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		TempDir:          tempDir,
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	diagnostics := rep.Classes[0].ProcyonDiagnostics
	if diagnostics == nil {
		t.Fatal("ProcyonDiagnostics = nil, want diagnostics")
	}
	if diagnostics.WorkspaceDisposition != "cleaned" || diagnostics.WorkspacePath != "" {
		t.Fatalf("workspace diagnostics = %+v, want cleaned with no path", diagnostics)
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", tempDir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary workspaces = %v, want all cleaned", entries)
	}
}

func TestEnginePassesDecompileClasspathToProcyonRetries(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})

	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[1]
				writePipelineFile(t, outputDir, "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				if got, want := spec.Args[5], strings.Join([]string{jarPath, "/deps/base.jar", "/deps/cli.jar"}, string(os.PathListSeparator)); got != want {
					t.Fatalf("extraclasspath = %q, want %q", got, want)
				}
				outputDir := spec.Args[3]
				writePipelineFile(t, outputDir, "com/example/Foo.java", "class Foo { int recovered = 1; }\n")
				return decompiler.RunResult{}, nil
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		ExtraClasspath:   []string{"/deps/base.jar", jarPath, "/deps/cli.jar"},
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.ProcyonRecovered != 1 {
		t.Fatalf("ProcyonRecovered = %d, want 1", rep.ProcyonRecovered)
	}
}

func TestEnginePreservesDependencyWarningsSeparately(t *testing.T) {
	t.Parallel()

	jarPath := writePipelineJar(t, map[string]string{
		"com/example/Foo.class": "foo",
	})

	engine := Engine{
		JadxRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[1]
				writePipelineFile(t, outputDir, "sources/com/example/Foo.java", "class Foo {\n// JADX WARN: fallback\n}\n")
				return decompiler.RunResult{}, nil
			},
		},
		ProcyonRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[3]
				writePipelineFile(t, outputDir, "com/example/Foo.java", "/* Could not load the following classes */\nclass Foo {}\n")
				return decompiler.RunResult{}, nil
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:      "/tools/procyon.jar",
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	classRep := rep.Classes[0]
	if classRep.Status != ireport.StatusSucceeded || classRep.Origin != ireport.OriginProcyon {
		t.Fatalf("class report = %+v, want successful Procyon recovery", classRep)
	}
	if got := classRep.RetryReasons; len(got) != 1 || got[0] != "jadx_warn" {
		t.Fatalf("RetryReasons = %v, want [jadx_warn]", got)
	}
	if got := classRep.DependencyWarnings; len(got) != 1 || got[0] != "Could not load the following classes" {
		t.Fatalf("DependencyWarnings = %v, want unresolved class warning", got)
	}
	if !strings.Contains(ireport.RenderText(rep), "dependencyWarnings=Could not load the following classes") {
		t.Fatalf("RenderText() missing dependency warning: %q", ireport.RenderText(rep))
	}
}

type scriptedRunner struct {
	run       func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
	preflight func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (s *scriptedRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	if slices.Equal(spec.Args[2:], []string{"--help"}) {
		if s.preflight != nil {
			return s.preflight(spec)
		}
		return decompiler.RunResult{}, nil
	}
	return s.run(spec)
}
