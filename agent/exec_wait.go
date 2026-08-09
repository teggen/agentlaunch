package agent

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// RunWait runs the command as a child process and waits for it — the launch
// path for ConfigWriter agents, whose restore must run after the session
// ends (syscall.Exec would replace this process and nothing after it could
// run). Same env merge as Run via ExecArgs; stdio is inherited. SIGINT and
// SIGTERM are forwarded to the child so the interactive session dies on its
// own terms while our restore still runs; on Windows Signal is best-effort
// and a failed forward is ignored. The returned error is cmd.Wait()'s —
// including *exec.ExitError, which main's exit-code extraction understands.
func RunWait(c Command) error {
	argv, env := ExecArgs(c)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("run %s: %w", c.Path, err)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case s := <-sig:
			// Best-effort forward: the child may have already exited between
			// the signal arriving and this call, so a failed Signal is not
			// actionable here.
			_ = cmd.Process.Signal(s)
		case err := <-done:
			return err
		}
	}
}
