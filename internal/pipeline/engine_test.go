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
		CfrRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[2]
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
		CfrPath:          "/tools/cfr",
		RetryConcurrency: 2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.JadxSucceeded != 1 || rep.CfrRecovered != 1 || rep.FinalFailed != 0 {
		t.Fatalf("report counts = %+v", rep)
	}

	content, err := os.ReadFile(filepath.Join(outputDir, "sources/com/example/Foo.java"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "recovered = 1") {
		t.Fatalf("final Foo.java = %q, want cfr content", string(content))
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
	if written.CfrRecovered != 1 {
		t.Fatalf("written report = %+v, want CfrRecovered=1", written)
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
		CfrRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				outputDir := spec.Args[2]
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
		CfrPath:          "/tools/cfr",
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.FinalFailed != 1 {
		t.Fatalf("FinalFailed = %d, want 1", rep.FinalFailed)
	}
	if rep.Classes[0].FailureReason != "ambiguous_retry_output" {
		t.Fatalf("FailureReason = %q, want ambiguous_retry_output", rep.Classes[0].FailureReason)
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
		CfrRunner: &scriptedRunner{
			run: func(spec decompiler.CommandSpec) (decompiler.RunResult, error) {
				return decompiler.RunResult{}, errors.New("cfr failed")
			},
		},
	}

	rep, err := engine.Run(context.Background(), Config{
		InputPath:        jarPath,
		OutputDir:        t.TempDir(),
		JadxPath:         "/tools/jadx",
		CfrPath:          "/tools/cfr",
		RetryConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.FinalFailed != 1 {
		t.Fatalf("FinalFailed = %d, want 1", rep.FinalFailed)
	}
	if rep.Classes[0].FailureReason != "cfr_execution_failed" {
		t.Fatalf("FailureReason = %q, want cfr_execution_failed", rep.Classes[0].FailureReason)
	}
}

type scriptedRunner struct {
	run func(spec decompiler.CommandSpec) (decompiler.RunResult, error)
}

func (s *scriptedRunner) Run(_ context.Context, spec decompiler.CommandSpec) (decompiler.RunResult, error) {
	return s.run(spec)
}
