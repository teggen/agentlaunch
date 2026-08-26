package launch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/catalog/catalogtest"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// fakeLauncher implements only the required Launcher interface. The types
// below embed it to add one optional capability each, so every guard can be
// exercised in isolation - the real registry has one agent, which is always
// supported and never implements PlatformSupported, leaving those branches
// otherwise unreachable.
type fakeLauncher struct{}

func (*fakeLauncher) Name() string        { return "fake" }
func (*fakeLauncher) DisplayName() string { return "Fake Agent" }

func (*fakeLauncher) Command(req agent.Request) (agent.Command, error) {
	return agent.Command{
		Path: "/bin/fake",
		Args: append([]string{"--model", req.Model.ID}, req.ExtraArgs...),
		Env:  []string{"FAKE_API_KEY=" + req.APIKey},
	}, nil
}

// notInstalledLauncher reports its binary as absent.
type notInstalledLauncher struct{ fakeLauncher }

func (*notInstalledLauncher) CheckInstalled() bool { return false }
func (*notInstalledLauncher) InstallHint() string  { return "brew install fake" }

// blockedLauncher is both platform-unsupported and not installed, so a test
// can assert which of the two guards wins.
type blockedLauncher struct{ fakeLauncher }

func (*blockedLauncher) Supported() error     { return errors.New("windows is not supported yet") }
func (*blockedLauncher) CheckInstalled() bool { return false }
func (*blockedLauncher) InstallHint() string  { return "brew install fake" }

// incompatibleLauncher returns an advisory ErrIncompatibleModel.
type incompatibleLauncher struct{ fakeLauncher }

func (*incompatibleLauncher) CheckModel(m catalog.Model) error {
	return fmt.Errorf("%w: fake is optimized for anthropic/* models and may fail with %s",
		agent.ErrIncompatibleModel, m.ID)
}

// brokenCheckLauncher returns a genuine failure rather than an advisory one.
type brokenCheckLauncher struct{ fakeLauncher }

func (*brokenCheckLauncher) CheckModel(catalog.Model) error {
	return errors.New("catalog service unreachable")
}

// buildErrorLauncher fails at command construction.
type buildErrorLauncher struct{ fakeLauncher }

func (*buildErrorLauncher) Command(agent.Request) (agent.Command, error) {
	return agent.Command{}, errors.New("binary vanished")
}

// spec wraps a launcher in a supported registry entry.
func spec(name string, l agent.Launcher) *agent.Spec {
	return &agent.Spec{Name: name, Launcher: l, Status: agent.Status{Supported: true}}
}

// newTestService isolates config and cache to a temp dir, serves fixed
// models, provides an API key, and stubs the handoff.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	return &Service{
		Catalog: catalogtest.NewCatalog(),
		Run:     func(agent.Command) error { return nil },
	}
}

// The guard order decides which of several simultaneous problems the user is
// told about first. Each ordering test below violates its own guard AND at
// least one later guard, so moving a guard down the sequence fails the test
// rather than quietly changing the message.

func TestPlanUnsupportedAgentBeatsEveryLaterGuard(t *testing.T) {
	svc := newTestService(t)
	s := &agent.Spec{
		Name:     "copilot",
		Launcher: &blockedLauncher{}, // also platform-unsupported and not installed
		Status:   agent.Status{Supported: false, Reason: "talks to GitHub's own backend"},
	}

	// ModelID empty, so ErrNoModel is live too.
	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	var uae *UnsupportedAgentError
	if !errors.As(err, &uae) {
		t.Fatalf("Plan returned %T (%v), want *UnsupportedAgentError", err, err)
	}
}

func TestPlanPlatformBeatsNoModelAndNotInstalled(t *testing.T) {
	svc := newTestService(t)
	s := spec("droid", &blockedLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	var upe *UnsupportedPlatformError
	if !errors.As(err, &upe) {
		t.Fatalf("Plan returned %T (%v), want *UnsupportedPlatformError", err, err)
	}
	if upe.Agent != "droid" {
		t.Errorf("Agent = %q, want droid", upe.Agent)
	}
}

// The handoff document listed the sequence as support -> platform ->
// install -> catalog, omitting this check entirely. The code has always run
// the empty-model check BEFORE the install check, and Phase 2 turns this
// exact branch into "open the picker" - so a user with no agent installed
// must still reach the picker rather than a dead end.
func TestPlanNoModelBeatsNotInstalled(t *testing.T) {
	svc := newTestService(t)
	s := spec("fake", &notInstalledLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: ""})

	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("Plan returned %v, want ErrNoModel", err)
	}
}

func TestPlanNotInstalledBeatsUnknownModel(t *testing.T) {
	svc := newTestService(t)
	s := spec("fake", &notInstalledLauncher{})

	_, err := svc.Plan(context.Background(), Request{Spec: s, ModelID: "no/such-model"})

	var nie *NotInstalledError
	if !errors.As(err, &nie) {
		t.Fatalf("Plan returned %T (%v), want *NotInstalledError", err, err)
	}
	if nie.Hint != "brew install fake" {
		t.Errorf("Hint = %q", nie.Hint)
	}
	if nie.DisplayName != "Fake Agent" {
		t.Errorf("DisplayName = %q", nie.DisplayName)
	}
}

// Pins guard 4 (agent installed) ahead of guard 5 (catalog load). With a
// working catalog both orderings produce NotInstalledError, so this uses a
// catalog that hard-fails with no cache to fall back on: correct ordering
// still reports the actionable "not installed", while a swap would surface
// the generic "load model catalog" failure instead. Fresh machine, no agent
// installed, offline is exactly when that difference matters.
func TestPlanNotInstalledBeatsCatalogFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir) // deliberately no cache file written
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	svc := &Service{Catalog: erroringCatalog{}, Run: func(agent.Command) error { return nil }}
	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &notInstalledLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})

	var nie *NotInstalledError
	if !errors.As(err, &nie) {
		t.Fatalf("Plan returned %T (%v), want *NotInstalledError", err, err)
	}
}

func TestPlanUnknownModelCarriesSuggestions(t *testing.T) {
	svc := newTestService(t)

	// "anthropic/claude-opus" is not an exact slug, but it is a substring of
	// anthropic/claude-opus-4.6, which is what Suggest's matching needs.
	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "anthropic/claude-opus",
	})

	var ume *UnknownModelError
	if !errors.As(err, &ume) {
		t.Fatalf("Plan returned %T (%v), want *UnknownModelError", err, err)
	}
	if len(ume.Suggestions) == 0 {
		t.Fatal("expected suggestions for a near-miss slug")
	}
	if ume.Suggestions[0] != "anthropic/claude-opus-4.6" {
		t.Errorf("Suggestions = %v", ume.Suggestions)
	}
}

func TestPlanMissingAPIKeyFails(t *testing.T) {
	svc := newTestService(t)
	t.Setenv("OPENROUTER_API_KEY", "")

	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})

	if !errors.Is(err, config.ErrNoAPIKey) {
		t.Fatalf("Plan returned %v, want config.ErrNoAPIKey", err)
	}
}

func TestPlanIncompatibleModelYieldsConfirmableWarning(t *testing.T) {
	svc := newTestService(t)

	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &incompatibleLauncher{}), ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("an advisory incompatibility must not fail the plan: %v", err)
	}

	if len(p.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want exactly one", p.Warnings)
	}
	w := p.Warnings[0]
	if w.Kind != WarnIncompatibleModel {
		t.Errorf("Kind = %v, want WarnIncompatibleModel", w.Kind)
	}
	// Without a Question the CLI would launch an incompatible pairing
	// silently, which is the behavior this warning exists to prevent.
	if w.Question == "" {
		t.Error("an incompatibility warning must carry a confirmation prompt")
	}
	if !strings.Contains(w.Message, "qwen/qwen3-coder:free") {
		t.Errorf("Message should name the model, got %q", w.Message)
	}
	// The plan is still runnable: confirming is the caller's job, not a
	// reason to withhold the command.
	if p.Command.Path == "" {
		t.Error("the plan should still carry a built command")
	}
}

// A genuine (non-ErrIncompatibleModel) CheckModel error is a hard failure,
// not something to soften into a warning and continue past. The returned
// error IS the primary assertion: if the error were downgraded to a
// warning, Plan would succeed and err would be nil.
//
// The Warnings check is meaningful since Plan began returning warnings
// accumulated before a fatal guard: with a fresh catalog nothing should
// have accumulated, so a warning appearing here would mean the genuine
// error had been softened into one rather than replaced by one.
func TestPlanGenuineCheckModelErrorIsFatal(t *testing.T) {
	svc := newTestService(t)

	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &brokenCheckLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})
	if err == nil {
		t.Fatal("a non-advisory CheckModel error must fail the plan")
	}
	if !strings.Contains(err.Error(), "catalog service unreachable") {
		t.Errorf("error should propagate unchanged, got %v", err)
	}
	if len(p.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none - a fresh catalog accumulates nothing", p.Warnings)
	}
}

func TestPlanStaleCatalogWarningPrecedesCompatibilityWarning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour))

	svc := &Service{Catalog: erroringCatalog{}, Run: func(agent.Command) error { return nil }}
	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &incompatibleLauncher{}), ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(p.Warnings) != 2 {
		t.Fatalf("Warnings = %+v, want two", p.Warnings)
	}
	// Slice order is the contract: the CLI prints these in sequence, and a
	// stale-catalog notice emitted after "Launch anyway? [y/N] " would
	// arrive once the user had already answered.
	if p.Warnings[0].Kind != WarnStaleCatalog {
		t.Errorf("Warnings[0].Kind = %v, want WarnStaleCatalog", p.Warnings[0].Kind)
	}
	if p.Warnings[1].Kind != WarnIncompatibleModel {
		t.Errorf("Warnings[1].Kind = %v, want WarnIncompatibleModel", p.Warnings[1].Kind)
	}
}

// A fatal guard must not swallow warnings the planner already collected.
// Offline with a stale cache, a model added since the cache was written is
// genuinely absent - so the staleness notice is the explanation for the
// unknown-model error, not noise beside it.
func TestPlanKeepsStaleWarningWhenALaterGuardFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour))

	svc := &Service{Catalog: erroringCatalog{}, Run: func(agent.Command) error { return nil }}
	p, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "no/such-model",
	})

	var ume *UnknownModelError
	if !errors.As(err, &ume) {
		t.Fatalf("Plan returned %T (%v), want *UnknownModelError", err, err)
	}
	if len(p.Warnings) != 1 {
		t.Fatalf("Warnings = %+v, want the stale-catalog warning to survive the error", p.Warnings)
	}
	if p.Warnings[0].Kind != WarnStaleCatalog {
		t.Errorf("Warnings[0].Kind = %v, want WarnStaleCatalog", p.Warnings[0].Kind)
	}
}

func TestPlanHappyPathBuildsCommandWithoutWarnings(t *testing.T) {
	svc := newTestService(t)

	p, err := svc.Plan(context.Background(), Request{
		Spec:      spec("fake", &fakeLauncher{}),
		ModelID:   "anthropic/claude-opus-4.6",
		ExtraArgs: []string{"--resume"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(p.Warnings) != 0 {
		t.Errorf("Warnings = %+v, want none", p.Warnings)
	}
	if p.Command.Path != "/bin/fake" {
		t.Errorf("Command.Path = %q", p.Command.Path)
	}
	// ExtraArgs and the resolved API key must reach the launcher.
	if len(p.Command.Args) != 3 || p.Command.Args[2] != "--resume" {
		t.Errorf("Command.Args = %v, want the trailing --resume", p.Command.Args)
	}
	if len(p.Command.Env) != 1 || p.Command.Env[0] != "FAKE_API_KEY=sk-or-test" {
		t.Errorf("Command.Env = %v, want the resolved API key", p.Command.Env)
	}
	if p.Model.ID != "anthropic/claude-opus-4.6" {
		t.Errorf("Model.ID = %q", p.Model.ID)
	}
	if p.Spec.Name != "fake" {
		t.Errorf("Spec.Name = %q", p.Spec.Name)
	}
}

func TestPlanPropagatesCommandBuildError(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &buildErrorLauncher{}), ModelID: "anthropic/claude-opus-4.6",
	})

	if err == nil || !strings.Contains(err.Error(), "binary vanished") {
		t.Fatalf("Plan returned %v, want the launcher's build error", err)
	}
}

type shadowingLauncher struct {
	fakeLauncher
	msg string
}

func (s *shadowingLauncher) ShadowedCredential() string { return s.msg }

func TestPlanShadowedCredentialYieldsConfirmableWarning(t *testing.T) {
	svc := newTestService(t)
	// Copy the Request literal from
	// TestPlanIncompatibleModelYieldsConfirmableWarning so the fake catalog
	// resolves the model; only the Spec differs.
	sh := &shadowingLauncher{msg: "fake has a stored OpenRouter credential that outranks the launched key"}
	p, err := svc.Plan(context.Background(), Request{
		Spec:    spec("fake", sh),
		ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var found *Warning
	for i := range p.Warnings {
		if p.Warnings[i].Kind == WarnShadowedCredential {
			found = &p.Warnings[i]
		}
	}
	if found == nil {
		t.Fatalf("no WarnShadowedCredential in %+v", p.Warnings)
	}
	if found.Message != sh.msg {
		t.Errorf("Message = %q, want %q", found.Message, sh.msg)
	}
	if found.Question == "" {
		t.Error("Question empty: warning is not confirmable, launch would proceed unasked")
	}
}

func TestPlanNoShadowWarningWhenDetectorSilent(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.Plan(context.Background(), Request{
		Spec:    spec("fake", &shadowingLauncher{msg: ""}),
		ModelID: "qwen/qwen3-coder:free",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, w := range p.Warnings {
		if w.Kind == WarnShadowedCredential {
			t.Fatalf("empty detector produced warning %+v", w)
		}
	}
}

type stagedLauncher struct {
	fakeLauncher
	files []agent.StagedFile
}

func (s *stagedLauncher) StagedFiles(agent.Request) ([]agent.StagedFile, error) {
	return s.files, nil
}

func TestPlanCarriesStagedFilesAndAgentRequest(t *testing.T) {
	svc := newTestService(t)
	want := []agent.StagedFile{{Path: "/tmp/x/openclaw.json", Contents: []byte("{}"), Mode: 0o644}}
	p, err := svc.Plan(context.Background(), Request{
		Spec:      spec("fake", &stagedLauncher{files: want}),
		ModelID:   "qwen/qwen3-coder:free",
		ExtraArgs: []string{"--flag"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(p.Staged) != 1 || p.Staged[0].Path != want[0].Path {
		t.Errorf("Staged = %+v, want %+v", p.Staged, want)
	}
	if p.AgentRequest.Model.ID != "qwen/qwen3-coder:free" {
		t.Errorf("AgentRequest.Model.ID = %q", p.AgentRequest.Model.ID)
	}
	if !slices.Equal(p.AgentRequest.ExtraArgs, []string{"--flag"}) {
		t.Errorf("AgentRequest.ExtraArgs = %q", p.AgentRequest.ExtraArgs)
	}
}

// TestPlanSuppliesTheStageDirToTheLauncher pins the seam that replaced
// internal/agent's direct call to config.Dir. The launcher no longer knows
// where this tool keeps its files; the planner tells it, and tells
// stageFiles the same thing, so the path a launcher builds and the boundary
// the write is checked against come from one source.
func TestPlanSuppliesTheStageDirToTheLauncher(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	want := t.TempDir()
	svc := &Service{
		Catalog:  catalogtest.NewCatalog(),
		StageDir: func() (string, error) { return want, nil },
	}
	// A fake launcher rather than a real spec: Landmine 8 — the isolated run
	// has no claude on PATH, so a real one would be refused by the install
	// guard long before the stage dir is set.
	plan, err := svc.Plan(context.Background(), Request{
		Spec:    spec("fake", &fakeLauncher{}),
		ModelID: catalogtest.Models()[0].ID,
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.AgentRequest.StageDir != want {
		t.Errorf("AgentRequest.StageDir = %q, want %q", plan.AgentRequest.StageDir, want)
	}
}
