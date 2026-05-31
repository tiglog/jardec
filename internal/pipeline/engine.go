package pipeline

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"jardec/internal/decompiler"
	jarpkg "jardec/internal/jar"
	"jardec/internal/merge"
	ireport "jardec/internal/report"
)

type Config struct {
	InputPath        string
	OutputDir        string
	JadxPath         string
	CfrPath          string
	TempDir          string
	KeepTemp         bool
	RetryConcurrency int
}

type Engine struct {
	JadxRunner decompiler.Runner
	CfrRunner  decompiler.Runner
}

func (e Engine) Run(ctx context.Context, cfg Config) (ireport.Report, error) {
	if e.JadxRunner == nil {
		e.JadxRunner = decompiler.CommandRunner{}
	}
	if e.CfrRunner == nil {
		e.CfrRunner = decompiler.CommandRunner{}
	}
	if cfg.RetryConcurrency <= 0 {
		cfg.RetryConcurrency = 1
	}

	classes, err := jarpkg.EnumerateTopLevelClasses(cfg.InputPath)
	if err != nil {
		return ireport.Report{}, err
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return ireport.Report{}, err
	}

	jadxWorkspace, err := ExecuteJadx(ctx, e.JadxRunner, JadxWorkspaceConfig{
		BaseTempDir: cfg.TempDir,
		JadxPath:    cfg.JadxPath,
		InputJar:    cfg.InputPath,
	})
	if err != nil {
		return ireport.Report{}, err
	}
	if !cfg.KeepTemp && jadxWorkspace.RootDir != "" {
		defer os.RemoveAll(jadxWorkspace.RootDir)
	}

	if jadxWorkspace.OutputDir != "" {
		if err := copyTree(jadxWorkspace.OutputDir, cfg.OutputDir); err != nil {
			return ireport.Report{}, err
		}
	}

	classReports := make(map[string]ireport.ClassResult, len(classes))
	retryReasons := make(map[string][]string, len(classes))
	retryClasses := make([]jarpkg.Class, 0)

	for _, class := range classes {
		classification, err := ClassifyJadxResult(class, jadxWorkspace.SourcesDir, jadxWorkspace.Diagnostics)
		if err != nil {
			return ireport.Report{}, err
		}

		if !classification.NeedsRetry {
			classReports[class.BinaryName] = ireport.ClassResult{
				BinaryName: class.BinaryName,
				Status:     ireport.StatusSucceeded,
				Origin:     ireport.OriginJADX,
			}
			continue
		}

		reasons := make([]string, 0, len(classification.Reasons))
		for _, reason := range classification.Reasons {
			reasons = append(reasons, string(reason))
		}
		retryReasons[class.BinaryName] = reasons
		retryClasses = append(retryClasses, class)
	}

	retryResults, err := ExecuteCFRRetries(ctx, e.CfrRunner, CfrRetryConfig{
		BaseTempDir: cfg.TempDir,
		CfrPath:     cfg.CfrPath,
		InputJar:    cfg.InputPath,
		Concurrency: cfg.RetryConcurrency,
	}, retryClasses)
	if err != nil {
		return ireport.Report{}, err
	}
	for _, result := range retryResults {
		if !cfg.KeepTemp && result.RootDir != "" {
			defer os.RemoveAll(result.RootDir)
		}

		classReport := ireport.ClassResult{
			BinaryName:   result.Class.BinaryName,
			RetryReasons: retryReasons[result.Class.BinaryName],
		}

		if result.Err != nil {
			classReport.Status = ireport.StatusFailed
			classReport.FailureReason = "cfr_execution_failed"
			classReports[result.Class.BinaryName] = classReport
			continue
		}

		if err := ValidateRetryOutput(result.Class, result.OutputDir); err != nil {
			classReport.Status = ireport.StatusFailed
			classReport.FailureReason = mapRetryFailure(err)
			classReports[result.Class.BinaryName] = classReport
			continue
		}

		if err := merge.ApplyRecovery(filepath.Join(cfg.OutputDir, "sources"), result.Class, result.OutputDir); err != nil {
			classReport.Status = ireport.StatusFailed
			classReport.FailureReason = "merge_failed"
			classReports[result.Class.BinaryName] = classReport
			continue
		}

		classReport.Status = ireport.StatusSucceeded
		classReport.Origin = ireport.OriginCFR
		classReports[result.Class.BinaryName] = classReport
	}

	rep := ireport.Report{
		Jar:                  filepath.Base(cfg.InputPath),
		TotalTopLevelClasses: len(classes),
		Classes:              make([]ireport.ClassResult, 0, len(classes)),
	}
	for _, class := range classes {
		classReport := classReports[class.BinaryName]
		rep.Classes = append(rep.Classes, classReport)
		switch {
		case classReport.Status == ireport.StatusSucceeded && classReport.Origin == ireport.OriginJADX:
			rep.JadxSucceeded++
		case classReport.Status == ireport.StatusSucceeded && classReport.Origin == ireport.OriginCFR:
			rep.CfrRecovered++
		case classReport.Status == ireport.StatusFailed:
			rep.FinalFailed++
		}
	}

	if err := ireport.WriteJSON(filepath.Join(cfg.OutputDir, "report.json"), rep); err != nil {
		return ireport.Report{}, err
	}
	if err := ireport.WriteText(filepath.Join(cfg.OutputDir, "report.txt"), rep); err != nil {
		return ireport.Report{}, err
	}

	return rep, nil
}

func mapRetryFailure(err error) string {
	switch {
	case errors.Is(err, ErrAmbiguousRetryOutput):
		return "ambiguous_retry_output"
	case errors.Is(err, ErrMissingRetryOutput):
		return "missing_retry_output"
	default:
		return "retry_validation_failed"
	}
}

func copyTree(srcRoot, dstRoot string) error {
	return filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstRoot, relPath)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
