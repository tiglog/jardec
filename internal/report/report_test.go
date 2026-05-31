package report

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONSerializesSummaryAndClasses(t *testing.T) {
	t.Parallel()

	rep := Report{
		Jar:                  "sample.jar",
		TotalTopLevelClasses: 2,
		JadxSucceeded:        1,
		CfrRecovered:         1,
		FinalFailed:          0,
		Classes: []ClassResult{
			{
				BinaryName:   "com.example.Foo",
				Status:       StatusSucceeded,
				Origin:       OriginCFR,
				RetryReasons: []string{"jadx_warn"},
			},
		},
	}

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	text := string(data)
	for _, want := range []string{
		`"jar":"sample.jar"`,
		`"jadxSucceeded":1`,
		`"cfrRecovered":1`,
		`"binaryName":"com.example.Foo"`,
		`"origin":"cfr"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("json = %s, want substring %s", text, want)
		}
	}
}

func TestRenderTextSummarizesCoverage(t *testing.T) {
	t.Parallel()

	rep := Report{
		Jar:                  "sample.jar",
		TotalTopLevelClasses: 3,
		JadxSucceeded:        1,
		CfrRecovered:         1,
		FinalFailed:          1,
		Classes: []ClassResult{
			{
				BinaryName:    "com.example.Bad",
				Status:        StatusFailed,
				RetryReasons:  []string{"jadx_warn"},
				FailureReason: "missing_retry_output",
			},
		},
	}

	text := RenderText(rep)
	for _, want := range []string{
		"JAR: sample.jar",
		"Total top-level classes: 3",
		"JADX succeeded: 1",
		"CFR recovered: 1",
		"Final failed: 1",
		"com.example.Bad",
		"missing_retry_output",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderText() = %q, want substring %q", text, want)
		}
	}
}
