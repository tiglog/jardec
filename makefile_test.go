package jardec_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeInstallInvokesGoInstallForCLI(t *testing.T) {
	tempDir := t.TempDir()
	argsPath := filepath.Join(tempDir, "go-args")
	goPath := filepath.Join(tempDir, "go")

	if err := os.WriteFile(goPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGS_FILE\"\n"), 0o755); err != nil {
		t.Fatalf("write fake go command: %v", err)
	}

	command := exec.Command("make", "install")
	command.Env = append(os.Environ(),
		"ARGS_FILE="+argsPath,
		"GOBIN="+filepath.Join(tempDir, "bin"),
		"PATH="+tempDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("make install: %v\n%s", err, output)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read captured go arguments: %v", err)
	}
	if got, want := strings.TrimSpace(string(args)), "install\n./cmd/jardec"; got != want {
		t.Fatalf("go arguments = %q, want %q", got, want)
	}
}
