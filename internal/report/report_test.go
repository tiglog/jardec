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
				BinaryName:         "com.example.Bad",
				Status:             StatusFailed,
				RetryReasons:       []string{"jadx_warn"},
				RetryOutcome:       "missing_retry_output",
				DependencyWarnings: []string{"Could not load the following classes"},
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
		"Could not load the following classes",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderText() = %q, want substring %q", text, want)
		}
	}
}

func TestWriteJSONSerializesDependencyWarnings(t *testing.T) {
	t.Parallel()

	rep := Report{
		Jar: "sample.jar",
		Classes: []ClassResult{
			{
				BinaryName:         "com.example.Warned",
				Status:             StatusSucceeded,
				Origin:             OriginJADX,
				DependencyWarnings: []string{"Could not load the following classes"},
			},
		},
	}

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"dependencyWarnings":["Could not load the following classes"]`) {
		t.Fatalf("json = %s, want dependency warnings", string(data))
	}
}

func TestRenderPatchTextSummarizesPatchedGroupsAndCleanup(t *testing.T) {
	t.Parallel()

	rep := PatchReport{
		InputJar:              "input.jar",
		OutputJar:             "patched.jar",
		DryRun:                true,
		Noop:                  false,
		ReplacedGroups:        1,
		UnchangedGroups:       1,
		RemovedStaleEntries:   2,
		RemovedSignatureFiles: 1,
		ElapsedMillis:         9,
		Groups: []PatchGroupResult{
			{
				BinaryName:          "com.example.Foo",
				Status:              "changed",
				ReplacedEntries:     []string{"com/example/Foo.class", "com/example/Foo$Inner.class"},
				RemovedStaleEntries: []string{"com/example/Foo$Old.class"},
			},
			{
				BinaryName: "com.example.Bar",
				Status:     "unchanged",
			},
		},
		SignatureFiles: []string{"META-INF/SIGNER.SF"},
	}

	text := RenderPatchText(rep)
	for _, want := range []string{
		"Input JAR: input.jar",
		"Output JAR: patched.jar",
		"Dry run: true",
		"No-op: false",
		"Replaced groups: 1",
		"Unchanged groups: 1",
		"Removed stale entries: 2",
		"Removed signature files: 1",
		"Elapsed:",
		"com.example.Foo [changed]",
		"com.example.Bar [unchanged]",
		"com/example/Foo$Old.class",
		"META-INF/SIGNER.SF",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderPatchText() = %q, want substring %q", text, want)
		}
	}
}

func TestRenderPatchTextSummarizesNoOpAndPreservedSignatures(t *testing.T) {
	t.Parallel()

	rep := PatchReport{
		InputJar:            "input.jar",
		OutputJar:           "patched.jar",
		Noop:                true,
		ReplacedGroups:      0,
		UnchangedGroups:     1,
		ElapsedMillis:       3,
		PreservedSignatures: []string{"META-INF/SIGNER.SF"},
		Groups: []PatchGroupResult{
			{
				BinaryName: "com.example.Foo",
				Status:     "unchanged",
			},
		},
	}

	text := RenderPatchText(rep)
	for _, want := range []string{
		"No-op: true",
		"Unchanged groups: 1",
		"com.example.Foo [unchanged]",
		"Preserved signature files:",
		"META-INF/SIGNER.SF",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderPatchText() = %q, want substring %q", text, want)
		}
	}
}

func TestRenderPatchTextIncludesCompileDetails(t *testing.T) {
	t.Parallel()

	rep := PatchReport{
		InputJar:      "input.jar",
		OutputJar:     "patched.jar",
		ElapsedMillis: 5,
		Compile: &PatchCompileReport{
			SourceRoot:    "src",
			JavacPath:     "/tools/javac",
			TargetClasses: []string{"com.example.Foo"},
			Classpath:     []string{"input.jar", "deps/a.jar"},
			Status:        StatusFailed,
			Diagnostics:   "Foo.java:1: error",
		},
	}

	text := RenderPatchText(rep)
	for _, want := range []string{
		"Compile source root: src",
		"Compile javac: /tools/javac",
		"Compile status: failed",
		"Compile targets: com.example.Foo",
		"Compile classpath: input.jar",
		"Foo.java:1: error",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("RenderPatchText() = %q, want substring %q", text, want)
		}
	}
}
