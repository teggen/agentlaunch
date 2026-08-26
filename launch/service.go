package launch

import (
	"context"
	"fmt"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Service resolves launch requests and hands off to agents. The zero value
// is usable and talks to the live OpenRouter API.
//
// Catalog and Run are fields rather than package globals so that a CLI run
// and a TUI session can hold their own, and so tests inject fakes without
// mutating process state.
type Service struct {
	// Catalog is the model source. nil means the live OpenRouter client.
	Catalog openrouter.Catalog
	// Run performs the process handoff. nil means agent.Run.
	Run func(agent.Command) error
	// RunWait performs the fork-and-wait handoff for ConfigWriter agents.
	// nil means agent.RunWait.
	RunWait func(agent.Command) error
	// StageDir names the directory launcher-owned staged files live in, and
	// which stageFiles refuses to write outside of. nil means config.Dir,
	// this tool's own XDG directory. It is one field rather than two so the
	// value a launcher computes its staged paths against and the value the
	// boundary check enforces cannot drift apart.
	StageDir func() (string, error)
}

func (s *Service) stageDir() (string, error) {
	if s.StageDir != nil {
		return s.StageDir()
	}
	return config.Dir()
}

func (s *Service) catalog() openrouter.Catalog {
	if s.Catalog != nil {
		return s.Catalog
	}
	return openrouter.NewClient()
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
func (s *Service) Snapshot(ctx context.Context, refresh bool) (openrouter.Snapshot, error) {
	path, err := openrouter.CachePath()
	if err != nil {
		return openrouter.Snapshot{}, err
	}

	cache := &openrouter.Cache{
		Path:   path,
		TTL:    openrouter.DefaultTTL,
		Source: s.catalog(),
	}
	snap, err := cache.Load(ctx, refresh)
	if err != nil {
		return openrouter.Snapshot{}, fmt.Errorf("load model catalog: %w", err)
	}
	return snap, nil
}
