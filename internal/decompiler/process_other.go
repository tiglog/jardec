//go:build !linux

package decompiler

import "os/exec"

func configureCommandCancellation(cmd *exec.Cmd) {}
