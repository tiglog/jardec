package pipeline

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"jardec/internal/decompiler"
	jarpkg "jardec/internal/jar"
)

type RetryReason string

const (
	RetryReasonMissingOutput RetryReason = "missing_output"
	RetryReasonEmptyOutput   RetryReason = "empty_output"
	RetryReasonJADXWarn      RetryReason = "jadx_warn"
	RetryReasonPlaceholder   RetryReason = "placeholder_output"
	RetryReasonLoggedFailure RetryReason = "logged_failure"
)

type Classification struct {
	NeedsRetry bool
	Reasons    []RetryReason
}

func ClassifyJadxResult(class jarpkg.Class, sourcesDir string, result decompiler.RunResult) (Classification, error) {
	reasons := make([]RetryReason, 0)

	sourcePath := filepath.Join(sourcesDir, filepath.FromSlash(class.SourcePath))
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Classification{
				NeedsRetry: true,
				Reasons:    []RetryReason{RetryReasonMissingOutput},
			}, nil
		}
		return Classification{}, err
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		reasons = appendReason(reasons, RetryReasonEmptyOutput)
	}
	if strings.Contains(string(content), "JADX WARN") {
		reasons = appendReason(reasons, RetryReasonJADXWarn)
	}
	if hasPlaceholderFailure(trimmed) {
		reasons = appendReason(reasons, RetryReasonPlaceholder)
	}
	if hasLoggedFailure(class, result) {
		reasons = appendReason(reasons, RetryReasonLoggedFailure)
	}

	return Classification{
		NeedsRetry: len(reasons) > 0,
		Reasons:    reasons,
	}, nil
}

func hasPlaceholderFailure(content string) bool {
	placeholders := []string{
		"JADX ERROR",
		"Method not decompiled",
		"Code decompiled incorrectly",
	}
	for _, marker := range placeholders {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func hasLoggedFailure(class jarpkg.Class, result decompiler.RunResult) bool {
	logText := result.Stdout + "\n" + result.Stderr
	if logText == "" {
		return false
	}

	identifiers := []string{class.BinaryName, class.EntryPath}
	hasIdentifier := false
	for _, identifier := range identifiers {
		if identifier != "" && strings.Contains(logText, identifier) {
			hasIdentifier = true
			break
		}
	}
	if !hasIdentifier {
		return false
	}

	failureMarkers := []string{"ERROR", "error", "failed", "Failed"}
	for _, marker := range failureMarkers {
		if strings.Contains(logText, marker) {
			return true
		}
	}

	return false
}

func appendReason(reasons []RetryReason, reason RetryReason) []RetryReason {
	if slices.Contains(reasons, reason) {
		return reasons
	}
	return append(reasons, reason)
}
