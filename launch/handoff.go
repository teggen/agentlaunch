package launch

import (
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
