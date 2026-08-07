//go:build windows

package agent

import (
	"fmt"
	"os"
	"os/exec"
)

// Run starts the agent and waits for it, since Windows has no exec(2). On a
// nonzero exit, the returned error wraps *exec.ExitError; main extracts its
// ExitCode() to set the process's own exit status (see exitCode in
// main.go), and internal/cli's resolveAndRun recognizes the same shape to
// suppress cobra's redundant error line for it.
func Run(c Command) error {
	_, env := ExecArgs(c)

	cmd := exec.Command(c.Path, c.Args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", c.Path, err)
	}
	return nil
}
