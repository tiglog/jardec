package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Status string

const (
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Origin string

const (
	OriginJADX Origin = "jadx"
	OriginCFR  Origin = "cfr"
)

type ClassResult struct {
	BinaryName    string   `json:"binaryName"`
	Status        Status   `json:"status"`
	Origin        Origin   `json:"origin,omitempty"`
	RetryReasons  []string `json:"retryReasons,omitempty"`
	RetryOutcome  string   `json:"retryOutcome,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
}

type Report struct {
	Jar                  string        `json:"jar"`
	TotalTopLevelClasses int           `json:"totalTopLevelClasses"`
	JadxSucceeded        int           `json:"jadxSucceeded"`
	CfrRecovered         int           `json:"cfrRecovered"`
	FinalFailed          int           `json:"finalFailed"`
	RetryCandidates      int           `json:"retryCandidates"`
	TotalElapsedMillis   int64         `json:"totalElapsedMillis"`
	RetryElapsedMillis   int64         `json:"retryElapsedMillis"`
	Classes              []ClassResult `json:"classes"`
}

type PatchGroupResult struct {
	BinaryName          string   `json:"binaryName"`
	ReplacedEntries     []string `json:"replacedEntries"`
	RemovedStaleEntries []string `json:"removedStaleEntries,omitempty"`
}

type PatchReport struct {
	InputJar              string             `json:"inputJar"`
	OutputJar             string             `json:"outputJar"`
	DryRun                bool               `json:"dryRun,omitempty"`
	ReplacedGroups        int                `json:"replacedGroups"`
	RemovedStaleEntries   int                `json:"removedStaleEntries"`
	RemovedSignatureFiles int                `json:"removedSignatureFiles"`
	ElapsedMillis         int64              `json:"elapsedMillis"`
	Groups                []PatchGroupResult `json:"groups"`
	SignatureFiles        []string           `json:"signatureFiles,omitempty"`
}

func WriteJSON(path string, rep Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WriteText(path string, rep Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(RenderText(rep)), 0o644)
}

func WritePatchJSON(path string, rep PatchReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func WritePatchText(path string, rep PatchReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(RenderPatchText(rep)), 0o644)
}

func PatchReportPaths(outputJarPath string) (jsonPath, textPath string) {
	return outputJarPath + ".report.json", outputJarPath + ".report.txt"
}

func RenderText(rep Report) string {
	lines := []string{
		fmt.Sprintf("JAR: %s", rep.Jar),
		fmt.Sprintf("Total top-level classes: %d", rep.TotalTopLevelClasses),
		fmt.Sprintf("JADX succeeded: %d", rep.JadxSucceeded),
		fmt.Sprintf("CFR recovered: %d", rep.CfrRecovered),
		fmt.Sprintf("Final failed: %d", rep.FinalFailed),
		fmt.Sprintf("Retry candidates: %d", rep.RetryCandidates),
		fmt.Sprintf("Total elapsed: %s", formatElapsedMillis(rep.TotalElapsedMillis)),
		fmt.Sprintf("Retry elapsed: %s", formatElapsedMillis(rep.RetryElapsedMillis)),
	}

	if len(rep.Classes) == 0 {
		return strings.Join(lines, "\n") + "\n"
	}

	lines = append(lines, "", "Class results:")
	for _, class := range rep.Classes {
		line := fmt.Sprintf("- %s [%s", class.BinaryName, class.Status)
		if class.Origin != "" {
			line += fmt.Sprintf(", origin=%s", class.Origin)
		}
		if len(class.RetryReasons) > 0 {
			line += fmt.Sprintf(", retry=%s", strings.Join(class.RetryReasons, ","))
		}
		switch {
		case class.RetryOutcome != "":
			line += fmt.Sprintf(", retryOutcome=%s", class.RetryOutcome)
		case class.FailureReason != "":
			line += fmt.Sprintf(", failure=%s", class.FailureReason)
		}
		line += "]"
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n") + "\n"
}

func RenderPatchText(rep PatchReport) string {
	lines := []string{
		fmt.Sprintf("Input JAR: %s", rep.InputJar),
		fmt.Sprintf("Output JAR: %s", rep.OutputJar),
		fmt.Sprintf("Dry run: %t", rep.DryRun),
		fmt.Sprintf("Replaced groups: %d", rep.ReplacedGroups),
		fmt.Sprintf("Removed stale entries: %d", rep.RemovedStaleEntries),
		fmt.Sprintf("Removed signature files: %d", rep.RemovedSignatureFiles),
		fmt.Sprintf("Elapsed: %s", formatElapsedMillis(rep.ElapsedMillis)),
	}

	if rep.RemovedSignatureFiles > 0 {
		lines = append(lines, "Signature cleanup: invalidated archive signatures were removed")
	}

	if len(rep.Groups) > 0 {
		lines = append(lines, "", "Patched groups:")
		for _, group := range rep.Groups {
			lines = append(lines, fmt.Sprintf("- %s", group.BinaryName))
			for _, entry := range group.ReplacedEntries {
				lines = append(lines, fmt.Sprintf("  replace: %s", entry))
			}
			for _, entry := range group.RemovedStaleEntries {
				lines = append(lines, fmt.Sprintf("  remove stale: %s", entry))
			}
		}
	}

	if len(rep.SignatureFiles) > 0 {
		lines = append(lines, "", "Removed signature files:")
		for _, entry := range rep.SignatureFiles {
			lines = append(lines, fmt.Sprintf("- %s", entry))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func formatElapsedMillis(value int64) string {
	return fmt.Sprintf("%dms", value)
}
