// Package launch resolves a launch request into a runnable command without
// touching the terminal, so both the CLI and a TUI can drive it.
//
// Every condition a user must see comes back as a value: an advisory
// Warning, or one of the typed errors below. Nothing here writes to stdout,
// stderr, or reads from stdin.
package launch

import (
	"errors"
	"fmt"
	"strings"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// ErrNoModel reports that no model was selected. It is deliberately bare:
// the CLI's message names a CLI flag and the binary, which this package has
// no business knowing. Phase 2 turns this branch into "open the picker".
var ErrNoModel = errors.New("no model selected")

// UnsupportedAgentError reports an agent that cannot be pointed at
// OpenRouter at all.
type UnsupportedAgentError struct {
	Agent  string
	Reason string
}

func (e *UnsupportedAgentError) Error() string {
	return fmt.Sprintf("%s cannot be pointed at OpenRouter: %s", e.Agent, e.Reason)
}

// UnsupportedPlatformError reports an agent that cannot run on this
// platform. Error() is the launcher's own message unchanged, so CLI output
// is what it always was; Agent is carried for callers that want to name the
// agent themselves.
type UnsupportedPlatformError struct {
	Agent string
	Err   error
}

func (e *UnsupportedPlatformError) Error() string { return e.Err.Error() }
func (e *UnsupportedPlatformError) Unwrap() error { return e.Err }

// NotInstalledError reports a missing agent binary. The hint is a separate
// field rather than baked into the message so a caller can render it as
// something other than a line of error text.
type NotInstalledError struct {
	Agent       string
	DisplayName string
	Hint        string
}

func (e *NotInstalledError) Error() string {
	return fmt.Sprintf("%s is not installed.\n%s", e.DisplayName, e.Hint)
}

// UnknownModelError reports a slug that matched nothing, carrying the
// suggestions as data so a caller can offer them as choices rather than as a
// formatted list.
type UnknownModelError struct {
	ModelID     string
	Suggestions []string
}

func (e *UnknownModelError) Error() string {
	if len(e.Suggestions) == 0 {
		return fmt.Sprintf("unknown model %q", e.ModelID)
	}
	return fmt.Sprintf("unknown model %q. Did you mean:\n  %s",
		e.ModelID, strings.Join(e.Suggestions, "\n  "))
}

// CheckSupported reports why an agent cannot be pointed at OpenRouter. It is
// Plan's first guard, and `profile add` calls it directly to refuse saving a
// profile for an unsupported agent without planning a launch.
func CheckSupported(spec *agent.Spec) error {
	if !spec.Status.Supported {
		return &UnsupportedAgentError{Agent: spec.Name, Reason: spec.Status.Reason}
	}
	return nil
}
