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
	FailureReason string   `json:"failureReason,omitempty"`
}

type Report struct {
	Jar                  string        `json:"jar"`
	TotalTopLevelClasses int           `json:"totalTopLevelClasses"`
	JadxSucceeded        int           `json:"jadxSucceeded"`
	CfrRecovered         int           `json:"cfrRecovered"`
	FinalFailed          int           `json:"finalFailed"`
	Classes              []ClassResult `json:"classes"`
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

func RenderText(rep Report) string {
	lines := []string{
		fmt.Sprintf("JAR: %s", rep.Jar),
		fmt.Sprintf("Total top-level classes: %d", rep.TotalTopLevelClasses),
		fmt.Sprintf("JADX succeeded: %d", rep.JadxSucceeded),
		fmt.Sprintf("CFR recovered: %d", rep.CfrRecovered),
		fmt.Sprintf("Final failed: %d", rep.FinalFailed),
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
		if class.FailureReason != "" {
			line += fmt.Sprintf(", failure=%s", class.FailureReason)
		}
		line += "]"
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n") + "\n"
}
