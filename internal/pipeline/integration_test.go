package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	ireport "jardec/internal/report"
)

func TestRealToolDecompileIntegration(t *testing.T) {
	jadxPath := os.Getenv("JARDEC_INTEGRATION_JADX_PATH")
	procyonPath := os.Getenv("JARDEC_INTEGRATION_PROCYON_PATH")
	jarPath := os.Getenv("JARDEC_INTEGRATION_JAR")
	if jadxPath == "" || procyonPath == "" || jarPath == "" {
		t.Skip("set JARDEC_INTEGRATION_JADX_PATH, JARDEC_INTEGRATION_PROCYON_PATH, and JARDEC_INTEGRATION_JAR to enable")
	}

	root := t.TempDir()
	outDir := filepath.Join(root, "out")
	concurrency := min(runtime.NumCPU(), 8)
	rep, err := (Engine{}).Run(context.Background(), Config{InputPath: jarPath, OutputDir: outDir, JadxPath: jadxPath, ProcyonPath: procyonPath, TempDir: filepath.Join(root, "work"), RetryConcurrency: concurrency})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.TotalTopLevelClasses == 0 || rep.JadxSucceeded+rep.ProcyonRecovered+rep.FinalFailed != rep.TotalTopLevelClasses {
		t.Fatalf("report totals = %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sources")); err != nil {
		t.Fatalf("sources directory: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("ReadFile(report.json): %v", err)
	}
	var written ireport.Report
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("report JSON: %v", err)
	}
}
