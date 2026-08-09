package launch

import (
	"context"
	"errors"
	"time"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Request is a launch request.
type Request struct {
	// Spec must be non-nil; Plan dereferences it unconditionally starting
	// with its first guard, CheckSupported.
	Spec      *agent.Spec
	ModelID   string
	ExtraArgs []string
	// Refresh bypasses the cached catalog.
	Refresh bool
}

// Plan is a resolved launch: a runnable command plus the conditions the
// caller must render and, where Warning.Question is set, get approved.
type Plan struct {
	Spec    *agent.Spec
	Model   openrouter.Model
	Command agent.Command
	// AgentRequest is the request Command was built from. The fork-and-wait
	// path re-uses it for ConfigWriter.Apply, which cannot run at plan time
	// (it writes).
	AgentRequest agent.Request
	// Staged are launcher-owned files Launch materializes before the
	// handoff. Computed here (purely) so Launch stays a straight line.
	Staged   []agent.StagedFile
	Warnings []Warning
}

// Plan resolves req into a runnable command. It performs IO - catalog fetch,
// config read - but never touches the terminal: every condition a user must
// see comes back as a Warning or a typed error.
//
// Warnings accumulated before a fatal guard are returned alongside the
// error, not discarded. Callers must render Plan.Warnings before inspecting
// err - "p, err := svc.Plan(...); if err != nil { return err }" silently
// drops them, and a stale catalog is frequently the reason a later guard
// failed.
//
// The guard order is load-bearing. It decides which of several simultaneous
// problems the user is told about first, and the empty-model check sits
// deliberately ahead of the install check so that a user with no agent
// installed still reaches the model picker in Phase 2.
//
// Confirmation is NOT performed here. The caller renders the warnings,
// obtains approval, and only then calls Launch.
func (s *Service) Plan(ctx context.Context, req Request) (Plan, error) {
	spec := req.Spec

	// Warnings accumulated before a fatal guard are returned alongside the
	// error rather than discarded. A stale catalog is frequently the reason
	// the guard below it failed - a model added since the cache was written
	// will not be found - so dropping it would hide the explanation. The CLI
	// used to print this from inside the catalog load, where no later guard
	// could suppress it; returning it here preserves that.
	var warnings []Warning

	if err := CheckSupported(spec); err != nil {
		return Plan{Warnings: warnings}, err
	}

	if platform, ok := spec.Launcher.(agent.PlatformSupported); ok {
		if err := platform.Supported(); err != nil {
			return Plan{Warnings: warnings}, &UnsupportedPlatformError{Agent: spec.Name, Err: err}
		}
	}

	if req.ModelID == "" {
		return Plan{Warnings: warnings}, ErrNoModel
	}

	if installable, ok := spec.Launcher.(agent.Installable); ok && !installable.CheckInstalled() {
		return Plan{Warnings: warnings}, &NotInstalledError{
			Agent:       spec.Name,
			DisplayName: spec.Launcher.DisplayName(),
			Hint:        installable.InstallHint(),
		}
	}

	snap, err := s.Snapshot(ctx, req.Refresh)
	if err != nil {
		return Plan{Warnings: warnings}, err
	}

	if w, ok := StaleWarning(snap, time.Now()); ok {
		warnings = append(warnings, w)
	}

	model, ok := openrouter.FindByID(snap.Models, req.ModelID)
	if !ok {
		return Plan{Warnings: warnings}, &UnknownModelError{
			ModelID:     req.ModelID,
			Suggestions: openrouter.Suggest(snap.Models, req.ModelID, 5),
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return Plan{Warnings: warnings}, err
	}
	apiKey, err := config.ResolveAPIKey(cfg)
	if err != nil {
		return Plan{Warnings: warnings}, err
	}

	if compatible, ok := spec.Launcher.(agent.Compatible); ok {
		if err := compatible.CheckModel(model); err != nil {
			// Incompatibility is advisory: Claude Code works with many
			// non-Anthropic models, so this warns rather than aborts.
			// Anything else is a genuine failure.
			if !errors.Is(err, agent.ErrIncompatibleModel) {
				return Plan{Warnings: warnings}, err
			}
			warnings = append(warnings, Warning{
				Kind:     WarnIncompatibleModel,
				Message:  err.Error(),
				Question: "Launch anyway?",
			})
		}
	}

	if shadow, ok := spec.Launcher.(agent.CredentialShadowCheck); ok {
		if msg := shadow.ShadowedCredential(); msg != "" {
			warnings = append(warnings, Warning{
				Kind:     WarnShadowedCredential,
				Message:  msg,
				Question: "Launch anyway?",
			})
		}
	}

	areq := agent.Request{
		Model:     model,
		APIKey:    apiKey,
		ExtraArgs: req.ExtraArgs,
	}
	command, err := spec.Launcher.Command(areq)
	if err != nil {
		return Plan{Warnings: warnings}, err
	}

	var staged []agent.StagedFile
	if st, ok := spec.Launcher.(agent.Staged); ok {
		staged, err = st.StagedFiles(areq)
		if err != nil {
			return Plan{Warnings: warnings}, err
		}
	}

	return Plan{
		Spec:         spec,
		Model:        model,
		Command:      command,
		AgentRequest: areq,
		Staged:       staged,
		Warnings:     warnings,
	}, nil
}
