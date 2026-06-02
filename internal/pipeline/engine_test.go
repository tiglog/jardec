package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
		ProcyonPath:          "/tools/procyon.jar",
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
		ProcyonPath:          "/tools/procyon.jar",
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
				return decompiler.RunResult{}, errors.New("procyon failed")
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		ProcyonPath:          "/tools/procyon.jar",
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
		ProcyonPath:          "/tools/procyon.jar",
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
		ProcyonPath:          "/tools/procyon.jar",
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
	run func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (s *scriptedRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	return s.run(spec)
}
