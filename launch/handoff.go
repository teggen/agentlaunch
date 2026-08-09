package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
)

// Launch records the selection and then hands off to the agent.
//
// The order is load-bearing: on Unix the handoff is syscall.Exec, which
// replaces the process, so nothing after it runs. Recording afterwards would
// mean never recording at all. Save and handoff live in one function so that
// no call site can get the order wrong.
//
// warn is called synchronously for any non-fatal problem encountered before
// the handoff. It must not block, and may be nil. It cannot be a return
// value for the same reason the ordering matters: on Unix, Launch does not
// return on success, so a returned warning would never be seen.
//
// A config that cannot be read or written costs the user their remembered
// last selection. That is a convenience, not a precondition, so it warns
// rather than refusing to start the agent.
func (s *Service) Launch(p Plan, warn func(Warning)) error {
	if err := recordSelection(p); err != nil && warn != nil {
		warn(Warning{
			Kind:    WarnSelectionNotSaved,
			Message: "could not save last selection: " + err.Error(),
		})
	}
	if err := stageFiles(p.Staged); err != nil {
		return fmt.Errorf("stage launcher-owned config: %w", err)
	}
	if cw, ok := p.Spec.Launcher.(agent.ConfigWriter); ok {
		return s.launchConfigWriter(p, cw)
	}
	return s.run(p.Command)
}

// recordSelection persists the agent and model for the next run. The config
// is re-read rather than threaded through from Plan: in a TUI a profile may
// have been added between planning and launching, and that edit must not be
// clobbered.
func recordSelection(p Plan) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.LastAgent = p.Spec.Name
	cfg.LastModel = p.Model.ID
	return config.Save(cfg)
}

// stageFiles writes the plan's launcher-owned files. It refuses any path
// outside this tool's own config dir: staged files are write site #3 of the
// amended Landmine 6, and the boundary is enforced here, not trusted.
func stageFiles(files []agent.StagedFile) error {
	if len(files) == 0 {
		return nil
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	for _, f := range files {
		rel, err := filepath.Rel(dir, f.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("staged file %q is outside the launcher config dir %q", f.Path, dir)
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.Path, f.Contents, f.Mode); err != nil {
			return err
		}
	}
	return nil
}

// launchConfigWriter is the fork-and-wait path: Apply writes the agent's
// config (the one sanctioned agent-owned write, Landmine 6 as amended), the
// agent runs as a waited-on child, and restore undoes the write afterwards —
// including after a failed session, and even if something panics between
// Apply and the run (restore is deferred for exactly that reason). The run
// error is preserved through errors.Join so main's exit-code extraction
// still sees the *exec.ExitError.
func (s *Service) launchConfigWriter(p Plan, cw agent.ConfigWriter) (err error) {
	restore, applyErr := cw.Apply(p.AgentRequest)
	if applyErr != nil {
		return fmt.Errorf("configure %s: %w", p.Spec.Name, applyErr)
	}
	defer func() {
		if rerr := restore(); rerr != nil {
			err = errors.Join(err, fmt.Errorf("restore %s config: %w", p.Spec.Name, rerr))
		}
	}()
	return s.runWait(p.Command)
}
