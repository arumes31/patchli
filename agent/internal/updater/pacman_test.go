package updater

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestPacmanManagerFull(t *testing.T) {
	oldExec := execCommandContext
	defer func() { execCommandContext = oldExec }()
	oldStat := statFunc
	defer func() { statFunc = oldStat }()

	m := &PacmanManager{}
	ctx := context.Background()

	t.Run("CheckUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if _, err := m.CheckUpdates(ctx); err != nil {
			t.Errorf("CheckUpdates failed: %v", err)
		}
	})

	t.Run("ApplyUpdates", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if _, err := m.ApplyUpdates(ctx, nil); err != nil {
			t.Errorf("ApplyUpdates failed: %v", err)
		}
	})

	t.Run("RebootRequired", func(t *testing.T) {
		statFunc = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		m.RebootRequired()
	})

	t.Run("PreFlightCheck", func(t *testing.T) {
		statFunc = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		if err := m.PreFlightCheck(ctx); err != nil {
			t.Errorf("PreFlightCheck failed: %v", err)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return getMockCommand(ctx, name, arg...)
		}
		if err := m.Cleanup(ctx); err != nil {
			t.Errorf("Cleanup failed: %v", err)
		}
	})
}
