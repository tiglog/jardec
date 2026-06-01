package sourcepatch

import (
	"context"
	"fmt"
	"os"
	"time"

	appcli "jardec/internal/cli"
	"jardec/internal/patch"
	ireport "jardec/internal/report"
)

type compileRunner interface {
	Compile(context.Context, appcli.SourcePatchConfig) (CompileResult, error)
}

type patchRunner interface {
	Run(context.Context, appcli.PatchConfig) (ireport.PatchReport, error)
}

type Engine struct {
	Compiler    compileRunner
	PatchRunner patchRunner
}

func (e Engine) Run(ctx context.Context, cfg appcli.SourcePatchConfig) (ireport.PatchReport, error) {
	startedAt := time.Now()

	compiler := e.Compiler
	if compiler == nil {
		compiler = Compiler{}
	}
	compileResult, err := compiler.Compile(ctx, cfg)
	compileReport := &ireport.PatchCompileReport{
		SourceRoot:    cfg.SourcesDir,
		JavacPath:     cfg.JavacPath,
		TargetClasses: append([]string(nil), cfg.TargetClasses...),
		Classpath:     append([]string(nil), compileResult.Classpath...),
		Diagnostics:   compileResult.Diagnostics,
		Status:        ireport.StatusSucceeded,
	}
	if err != nil {
		compileReport.Status = ireport.StatusFailed
		rep := ireport.PatchReport{
			InputJar:      cfg.InputJarPath,
			OutputJar:     cfg.OutputJarPath,
			ElapsedMillis: time.Since(startedAt).Milliseconds(),
			Compile:       compileReport,
		}
		if writeErr := writePatchReports(rep); writeErr != nil {
			return rep, writeErr
		}
		return rep, err
	}
	if compileResult.ClassesDir != "" {
		defer os.RemoveAll(compileResult.ClassesDir)
	}

	runner := e.PatchRunner
	if runner == nil {
		runner = patch.Engine{}
	}
	rep, patchErr := runner.Run(ctx, appcli.PatchConfig{
		InputJarPath:  cfg.InputJarPath,
		ClassesDir:    compileResult.ClassesDir,
		OutputJarPath: cfg.OutputJarPath,
		TargetClasses: append([]string(nil), cfg.TargetClasses...),
	})
	if rep.InputJar == "" {
		rep.InputJar = cfg.InputJarPath
	}
	if rep.OutputJar == "" {
		rep.OutputJar = cfg.OutputJarPath
	}
	rep.Compile = compileReport
	rep.ElapsedMillis = time.Since(startedAt).Milliseconds()
	if writeErr := writePatchReports(rep); writeErr != nil {
		return rep, writeErr
	}
	if patchErr != nil {
		return rep, patchErr
	}
	return rep, nil
}

func writePatchReports(rep ireport.PatchReport) error {
	jsonPath, textPath := ireport.PatchReportPaths(rep.OutputJar)
	if err := ireport.WritePatchJSON(jsonPath, rep); err != nil {
		return fmt.Errorf("write patch json report: %w", err)
	}
	if err := ireport.WritePatchText(textPath, rep); err != nil {
		return fmt.Errorf("write patch text report: %w", err)
	}
	return nil
}
