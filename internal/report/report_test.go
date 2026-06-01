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
		RetryCandidates:      2,
		TotalElapsedMillis:   15,
		RetryElapsedMillis:   4,
		Classes: []ClassResult{
			{
				BinaryName:   "com.example.Bad",
				Status:       StatusFailed,
				RetryReasons: []string{"jadx_warn"},
				RetryOutcome: "missing_retry_output",
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
		"Retry candidates: 2",
		"Total elapsed:",
		"Retry elapsed:",
		"com.example.Bad",
		"missing_retry_output",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderText() = %q, want substring %q", text, want)
		}
	}
}

func TestRenderPatchTextSummarizesPatchedGroupsAndCleanup(t *testing.T) {
	t.Parallel()

	rep := PatchReport{
		InputJar:              "input.jar",
		OutputJar:             "patched.jar",
		DryRun:                true,
		ReplacedGroups:        1,
		RemovedStaleEntries:   2,
		RemovedSignatureFiles: 1,
		ElapsedMillis:         9,
		Groups: []PatchGroupResult{
			{
				BinaryName:          "com.example.Foo",
				ReplacedEntries:     []string{"com/example/Foo.class", "com/example/Foo$Inner.class"},
				RemovedStaleEntries: []string{"com/example/Foo$Old.class"},
			},
		},
		SignatureFiles: []string{"META-INF/SIGNER.SF"},
	}

	text := RenderPatchText(rep)
	for _, want := range []string{
		"Input JAR: input.jar",
		"Output JAR: patched.jar",
		"Dry run: true",
		"Replaced groups: 1",
		"Removed stale entries: 2",
		"Removed signature files: 1",
		"Elapsed:",
		"com.example.Foo",
		"com/example/Foo$Old.class",
		"META-INF/SIGNER.SF",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderPatchText() = %q, want substring %q", text, want)
		}
	}
}
