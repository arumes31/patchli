package updater

import (
	"context"
	"os/exec"
	"runtime"
)

func getMockCommand(ctx context.Context, name string, _ ...string) *exec.Cmd {
	if name == "pgrep" {
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

func init() {
	checkDiskSpaceFunc = func(path string, minBytes uint64) error {
		return nil
	}
}
