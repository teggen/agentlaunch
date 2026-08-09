// Package agent registers coding agents and computes how to launch them
// against an OpenRouter model.
package agent

import (
	"errors"
	"os"

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
// through env vars or CLI overrides — implemented by droid since Phase 4.
// An agent implementing it takes the fork-and-wait launch path so that the
// returned restore function can run.
type ConfigWriter interface {
	Apply(Request) (restore func() error, err error)
}

// CredentialShadowCheck reports stored agent-side state that would make a
// launch ignore the environment this tool provides — a saved credential
// that outranks env vars (pi, cline, hermes document exactly that), or a
// binary generation that does not read them (legacy kimi-cli). Read-only
// and best-effort: implementations must never write, and must return ""
// (no warning) when the state is absent, unreadable, or unparseable —
// a detector failure must never block a launch.
type CredentialShadowCheck interface {
	ShadowedCredential() string
}

// StagedFile is a launcher-owned file a launch needs on disk — openclaw's
// model config is the canonical case. Declared as data so Command stays
// pure; launch.Service.Launch materializes it. Staged files live under this
// tool's own config dir and must never contain secrets.
type StagedFile struct {
	Path     string
	Contents []byte
	Mode     os.FileMode
}

// Staged is implemented by launchers that need launcher-owned files at
// launch time. StagedFiles MUST be pure, like Command. Distinct from
// ConfigWriter on purpose: Staged writes OUR files (idempotent overwrite,
// no undo, syscall.Exec handoff unaffected); ConfigWriter writes an
// AGENT'S file (backup and restore required, forces fork-and-wait). Do not
// merge them — the distinction is the amended Landmine 6 in type form.
type Staged interface {
	StagedFiles(Request) ([]StagedFile, error)
}
