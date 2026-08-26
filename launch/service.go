package launch

import (
	"context"
	"errors"
	"fmt"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
)

// Service resolves launch requests and hands off to agents.
//
// Every field below is a function the CALLER supplies, and that is the whole
// design of this package: a planner shared by more than one launcher tool
// cannot know which endpoint to fetch a catalog from, where that tool keeps
// its cache, what its settings file looks like, or which directory is "ours"
// to stage files in. Each of those was once a package-level call into this
// tool's own config and OpenRouter client; each is now a seam the composition
// root fills (see cli.newService).
//
// Run and RunWait keep working defaults because the process handoff genuinely
// is the same everywhere — it is a syscall, not a policy.
type Service struct {
	// LoadCatalog returns the model catalog with its provenance, honoring a
	// refresh request. Required: there is no endpoint this package could
	// default to. openrouter.Snapshotter builds this tool's.
	LoadCatalog func(ctx context.Context, refresh bool) (catalog.Snapshot, error)

	// APIKey resolves the credential a launch carries. Required.
	//
	// Plan returns this error UNWRAPPED. The TUI tests it with errors.Is to
	// decide whether to prompt for a key in place rather than abort the
	// session, so a %w around it here would still satisfy errors.Is — but a
	// reformatting into a new error would not, and that is the regression
	// TestPlanReturnsTheKeyErrorUnwrapped exists to catch.
	APIKey func() (string, error)

	// RecordSelection persists the agent and model just launched, so the next
	// run can preselect them. nil means this tool does not remember
	// selections; it is not an error, and Launch carries on either way — a
	// remembered choice is a convenience, never a precondition for starting
	// an agent.
	RecordSelection func(agentName, modelID string) error

	// StageDir names the directory launcher-owned staged files live in, and
	// which stageFiles refuses to write outside of. Required whenever a
	// launcher stages anything. It is one field rather than two so the value
	// a launcher computes its staged paths against and the value the boundary
	// check enforces cannot drift apart.
	StageDir func() (string, error)

	// Run performs the process handoff. nil means agent.Run.
	Run func(agent.Command) error
	// RunWait performs the fork-and-wait handoff for ConfigWriter agents.
	// nil means agent.RunWait.
	RunWait func(agent.Command) error
}

func (s *Service) stageDir() (string, error) {
	if s.StageDir == nil {
		// Refusing beats guessing: filepath.Join("", "x.json") is a path in
		// the working directory, so a fallback here would put a write outside
		// the sanctioned dir — Landmine 6 — while looking like it worked.
		return "", errors.New("launch: Service.StageDir is required")
	}
	return s.StageDir()
}

func (s *Service) run(c agent.Command) error {
	if s.Run != nil {
		return s.Run(c)
	}
	return agent.Run(c)
}

func (s *Service) runWait(c agent.Command) error {
	if s.RunWait != nil {
		return s.RunWait(c)
	}
	return agent.RunWait(c)
}

// Snapshot returns the model catalog with its provenance. Staleness is
// reported on the Snapshot itself rather than written anywhere; callers turn
// it into a Warning with StaleWarning.
func (s *Service) Snapshot(ctx context.Context, refresh bool) (catalog.Snapshot, error) {
	if s.LoadCatalog == nil {
		return catalog.Snapshot{}, errors.New("launch: Service.LoadCatalog is required")
	}
	snap, err := s.LoadCatalog(ctx, refresh)
	if err != nil {
		return catalog.Snapshot{}, fmt.Errorf("load model catalog: %w", err)
	}
	return snap, nil
}

// apiKey resolves the credential. The error is returned unchanged; see the
// APIKey field.
func (s *Service) apiKey() (string, error) {
	if s.APIKey == nil {
		return "", errors.New("launch: Service.APIKey is required")
	}
	return s.APIKey()
}
