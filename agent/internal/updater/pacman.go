package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
)

// PacmanManager implements the PackageManager interface for pacman (Arch Linux).
type PacmanManager struct{}

func (m *PacmanManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	cmd := execCommandContext(ctx, "pacman", "-Sy")
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}

	cmd = execCommandContext(ctx, "pacman", "-Qu")
	out.Reset()
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()
	if err != nil {
		return UpdateResult{Success: false, Output: out.String(), Error: err}, err
	}
	return UpdateResult{Success: true, Output: out.String(), Error: nil}, nil
}

func (m *PacmanManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	args := []string{"-Su", "--noconfirm"}
	if len(packages) > 0 {
		args = append([]string{"-S", "--noconfirm"}, packages...)
	}

	cmd := execCommandContext(ctx, "pacman", args...)
	
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return UpdateResult{Success: err == nil, Output: out.String(), Error: err}, err
}

func (m *PacmanManager) RebootRequired() bool {
	unameOut, err := execCommand("uname", "-r").Output()
	if err == nil {
		runningKernel := strings.TrimSpace(string(unameOut))
		if _, err := statFunc(fmt.Sprintf("/usr/lib/modules/%s", runningKernel)); os.IsNotExist(err) {
			return true
		}
		
		pacmanOut, err := execCommand("pacman", "-Q", "linux").Output()
		if err == nil {
			// output is generally "linux 6.4.12.arch1-1"
			installedStr := strings.TrimSpace(string(pacmanOut))
			parts := strings.Split(installedStr, " ")
			if len(parts) >= 2 {
				installedKernel := parts[1]
				// Basic check, replace - with . if needed or just do a substring match.
				if !strings.Contains(installedKernel, strings.ReplaceAll(runningKernel, "-", "")) && 
				   !strings.Contains(strings.ReplaceAll(runningKernel, "-", ""), strings.ReplaceAll(installedKernel, "-", "")) {
					return true
				}
			}
		}
	}
	
	out, err := execCommand("needrestart", "-b", "-r", "l").Output()
	if err == nil && strings.Contains(string(out), "NEEDRESTART-KSTA: 3") {
		return true
	}
	
	return false
}

func (m *PacmanManager) PreFlightCheck(ctx context.Context) error {
	if err := CheckDiskSpace("/var/lib/patchli", 1024*1024*1024); err != nil {
		return err
	}

	if _, err := statFunc("/var/lib/pacman/db.lck"); err == nil {
		return fmt.Errorf("pacman is currently locked")
	}
	return nil
}

func (m *PacmanManager) Cleanup(ctx context.Context) error {
	return execCommandContext(ctx, "pacman", "-Sc", "--noconfirm").Run()
}

