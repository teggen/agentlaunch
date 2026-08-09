package agent

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestRunWaitPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available")
	}
	err := RunWait(Command{Path: "/bin/sh", Args: []string{"-c", "exit 3"}})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want *exec.ExitError", err)
	}
	if code := exitErr.ExitCode(); code != 3 {
		t.Errorf("ExitCode = %d, want 3", code)
	}
	if err := RunWait(Command{Path: "/bin/sh", Args: []string{"-c", "exit 0"}}); err != nil {
		t.Errorf("clean exit returned %v", err)
	}
}
