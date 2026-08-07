//go:build windows

package agent

import (
	"fmt"
	"os"
	"os/exec"
)

// Run starts the agent and waits for it, since Windows has no exec(2). The
// child's exit code is propagated by the caller via exec.ExitError.
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
