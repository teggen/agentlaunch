//go:build !windows

package agent

import (
	"fmt"
	"syscall"
)

// Run replaces the current process with the agent. On success it does not
// return: signals, job control, and TTY behavior are then identical to
// invoking the agent directly.
func Run(c Command) error {
	argv, env := ExecArgs(c)
	if err := syscall.Exec(c.Path, argv, env); err != nil {
		return fmt.Errorf("exec %s: %w", c.Path, err)
	}
	return nil
}
