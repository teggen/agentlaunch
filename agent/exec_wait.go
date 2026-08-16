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
			//
			// Note this forward DUPLICATES a keyboard interrupt. The child is
			// not given a process group of its own, so it stays in the
			// terminal's foreground group with us and the tty delivers Ctrl+C
			// to both of us directly; the forward below is then a second
			// SIGINT for the child. An agent that treats interrupt
			// idempotently — every one we launch, as far as measured — cannot
			// tell the difference, but one that COUNTS interrupts (first press
			// cancels the current turn, second exits) would see one keypress
			// as two.
			//
			// Left as is on purpose. Setting SysProcAttr.Setpgid to isolate
			// the child stops the duplication, but it also takes the child out
			// of the foreground process group, so its first read from the
			// terminal raises SIGTTIN and an interactive session hangs
			// instead. Doing it properly means transferring terminal
			// ownership with tcsetpgrp and handing it back afterwards,
			// including on the panic path — a real amount of new
			// signal-handling surface for both ConfigWriter agents (droid,
			// cline) to fix a cosmetic double-interrupt neither one exhibits.
			// The forward itself is not optional: it is what lets a SIGTERM
			// from outside the terminal reach the child at all, and Ctrl+C is
			// the only case where the tty has already done our job for us.
			_ = cmd.Process.Signal(s)
		case err := <-done:
			return err
		}
	}
}
