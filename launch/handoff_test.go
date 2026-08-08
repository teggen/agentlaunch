package launch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// testPlan is a minimal already-resolved Plan.
func testPlan() Plan {
	return Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Model:   openrouter.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/bin/fake", Args: []string{"--model", "x"}, Env: []string{"K=V"}},
	}
}

// blockConfigWrites points XDG_CONFIG_HOME at a regular file, so the config
// directory cannot be created. config.Load fails first (ENOTDIR) and
// returns, so the selection is never recorded and config.Save is never
// reached in these tests.
func blockConfigWrites(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
}

// The ordering here is the whole reason save and handoff live in one
// function. On Unix the handoff is syscall.Exec, which replaces the process:
// a save placed after it would never run, and the stub used here returns
// normally, so the bug would be invisible to any end-state assertion. This
// inspects the config from inside the handoff itself.
func TestLaunchSavesSelectionBeforeHandoff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var savedBeforeHandoff bool
	svc := &Service{Run: func(agent.Command) error {
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("config.Load inside the handoff: %v", err)
		}
		savedBeforeHandoff = cfg.LastAgent == "fake" &&
			cfg.LastModel == "anthropic/claude-opus-4.6"
		return nil
	}}

	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !savedBeforeHandoff {
		t.Error("the selection must be persisted before control reaches the handoff")
	}
}

func TestLaunchRecordsAgentAndModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.LastAgent != "fake" {
		t.Errorf("LastAgent = %q", cfg.LastAgent)
	}
	if cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", cfg.LastModel)
	}
}

func TestLaunchWarnsButProceedsWhenSelectionCannotBeSaved(t *testing.T) {
	blockConfigWrites(t)

	var handedOff bool
	var got []Warning
	svc := &Service{Run: func(agent.Command) error { handedOff = true; return nil }}

	err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) })
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// Failing to remember the last selection is a convenience loss, not a
	// reason to refuse to start the agent.
	if !handedOff {
		t.Error("a failed save must not block the launch")
	}
	if len(got) != 1 {
		t.Fatalf("warnings = %+v, want exactly one", got)
	}
	if got[0].Kind != WarnSelectionNotSaved {
		t.Errorf("Kind = %v, want WarnSelectionNotSaved", got[0].Kind)
	}
	if got[0].Question != "" {
		t.Error("an unsaved selection is informational; it must not gate the launch on an answer")
	}
}

// warn is documented as optional. The crash case is a save failure with no
// callback to report it to, so that is what this exercises.
func TestLaunchNilWarnIsSafeOnSaveFailure(t *testing.T) {
	blockConfigWrites(t)

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
}

func TestLaunchDoesNotWarnOnSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got []Warning
	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) }); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warnings = %+v, want none on a successful save", got)
	}
}

func TestLaunchHandsOffTheBuiltCommandUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got agent.Command
	svc := &Service{Run: func(c agent.Command) error { got = c; return nil }}
	p := testPlan()

	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !reflect.DeepEqual(got, p.Command) {
		t.Errorf("Launch handed off %+v, want %+v unchanged", got, p.Command)
	}
}

func TestLaunchPropagatesHandoffError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := errors.New("exec failed")
	svc := &Service{Run: func(agent.Command) error { return want }}

	if err := svc.Launch(testPlan(), nil); !errors.Is(err, want) {
		t.Fatalf("Launch returned %v, want %v", err, want)
	}
}
