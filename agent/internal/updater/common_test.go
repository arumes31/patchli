package updater

import (
	"context"
	"os/exec"
	"runtime"
)

func getMockCommand(ctx context.Context, name string, arg ...string) *exec.Cmd {
	if name == "pgrep" || name == "ls" || name == "stat" {
		// Specific pgrep mock for AptManager lock detection
		if name == "pgrep" && len(arg) > 0 && (arg[len(arg)-1] == "apt-get" || arg[len(arg)-1] == "dpkg") {
			// If we want it to "find" the process, return 0.
			// But common_test.go's default seems to be returning 1 for these.
			// The AptManager test expects pgrep to return 0 to trigger the error.
			// I'll handle this in the test file itself or make getMockCommand smarter.
		}

		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd.exe", "/c", "exit 1")
		}
		return exec.CommandContext(ctx, "false")
	}
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd.exe", "/c", "exit 0")
	}
	return exec.CommandContext(ctx, "true")
}

func getMockCommandNoCtx(_ string, _ ...string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/c", "exit 0")
	}
	return exec.Command("true")
}
