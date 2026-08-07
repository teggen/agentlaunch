// Package agent registers coding agents and computes how to launch them
// against an OpenRouter model.
package agent

import (
	"errors"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// ErrIncompatibleModel reports a pairing an agent does not fully support.
// It is advisory: callers confirm rather than abort.
var ErrIncompatibleModel = errors.New("model may not be fully compatible with this agent")

// Request is everything a launcher needs to build its command.
type Request struct {
	Model     openrouter.Model
	APIKey    string
	ExtraArgs []string
}

// Command is a process to run. Env entries override any inherited
// environment variable of the same name; see ExecArgs for the merge.
type Command struct {
	Path string
	Args []string
	Env  []string
}

// Launcher computes the process that runs an agent against a model.
// Implementations MUST be pure: no file writes, no network, no spawning.
type Launcher interface {
	Name() string
	DisplayName() string
	Command(Request) (Command, error)
}

// Installable reports whether the agent's binary is present.
type Installable interface {
	CheckInstalled() bool
	InstallHint() string
}

// Installer can install the agent. Implemented by no agent in Phase 1;
// callers must confirm before invoking it.
type Installer interface {
	EnsureInstalled() error
}

// Compatible validates a model against agent-specific requirements.
// Returning an error wrapping ErrIncompatibleModel produces a confirmation
// prompt, not a hard failure.
type Compatible interface {
	CheckModel(openrouter.Model) error
}

// PlatformSupported reports whether the agent can run on this platform.
type PlatformSupported interface {
	Supported() error
}

// ConfigWriter is the escape hatch for agents that cannot be configured
// through env vars or CLI overrides. Implemented by no agent in Phase 1.
// An agent implementing it takes the fork-and-wait launch path so that the
// returned restore function can run.
type ConfigWriter interface {
	Apply(Request) (restore func() error, err error)
}
