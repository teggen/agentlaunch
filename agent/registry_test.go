package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeLauncher struct{ name string }

func (f fakeLauncher) Name() string        { return f.name }
func (f fakeLauncher) DisplayName() string { return "Fake " + f.name }
func (f fakeLauncher) Command(Request) (Command, error) {
	return Command{Path: "/bin/" + f.name}, nil
}

func TestLookupByName(t *testing.T) {
	spec, err := Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if spec.Name != "claude" {
		t.Errorf("Name = %q, want claude", spec.Name)
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	if _, err := Lookup("CLAUDE"); err != nil {
		t.Errorf("Lookup(CLAUDE): %v", err)
	}
}

func TestLookupUnknown(t *testing.T) {
	_, err := Lookup("nope")
	if !errors.Is(err, ErrUnknownAgent) {
		t.Errorf("got %v, want ErrUnknownAgent", err)
	}
}

func TestListIncludesClaude(t *testing.T) {
	var found bool
	for _, s := range List() {
		if s.Name == "claude" {
			found = true
		}
	}
	if !found {
		t.Error("claude missing from List()")
	}
}

func TestBuildIndexResolvesAliases(t *testing.T) {
	specs := []*Spec{{
		Name:     "claude",
		Aliases:  []string{"cc"},
		Launcher: fakeLauncher{name: "claude"},
		Status:   Status{Supported: true},
	}}
	idx := buildIndex(specs)
	if idx["cc"] == nil || idx["cc"].Name != "claude" {
		t.Error("alias did not resolve to its spec")
	}
}

func TestBuildIndexPanicsOnDuplicateName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for duplicate names")
		}
	}()
	buildIndex([]*Spec{
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
	})
}

// TestBuildIndexPanicsOnAliasCollidingWithName also pins that the panic
// message names the offending spec ("b", whose alias "a" is the collision) -
// with many registry entries (Phase 3), that is what lets someone locate the
// bad one from the panic message alone.
func TestBuildIndexPanicsOnAliasCollidingWithName(t *testing.T) {
	var msg any
	func() {
		defer func() { msg = recover() }()
		buildIndex([]*Spec{
			{Name: "a", Launcher: fakeLauncher{name: "a"}},
			{Name: "b", Aliases: []string{"a"}, Launcher: fakeLauncher{name: "b"}},
		})
	}()
	if msg == nil {
		t.Fatal("expected a panic for alias colliding with a canonical name")
	}
	if !strings.Contains(fmt.Sprint(msg), "b") {
		t.Errorf("panic message should name the offending spec %q, got: %v", "b", msg)
	}
}

// TestBuildIndexPanicsOnEmptyName also pins that the panic message identifies
// which registry entry is missing a name (by position, since the name itself
// is what's missing) so it can be located among many entries.
func TestBuildIndexPanicsOnEmptyName(t *testing.T) {
	var msg any
	func() {
		defer func() { msg = recover() }()
		buildIndex([]*Spec{
			{Name: "a", Launcher: fakeLauncher{name: "a"}},
			{Name: "", Launcher: fakeLauncher{name: ""}},
		})
	}()
	if msg == nil {
		t.Fatal("expected a panic for an empty name")
	}
	if !strings.Contains(fmt.Sprint(msg), "1") {
		t.Errorf("panic message should identify the offending index (1), got: %v", msg)
	}
}

// TestBuildIndexPanicsOnNilLauncher pins Launcher as a required field: every
// caller that dereferences spec.Launcher (newLaunchCmds, agents.go,
// Installed) would otherwise panic obscurely, far from the registry bug that
// caused it. Catching this at buildIndex, alongside the other registry-literal
// programmer errors it already guards, makes the failure immediately
// diagnosable and, since buildIndex runs at package-variable initialization
// (var index = buildIndex(specs)), fires before main() ever runs.
func TestBuildIndexPanicsOnNilLauncher(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a nil Launcher")
		}
	}()
	buildIndex([]*Spec{{Name: "a", Launcher: nil}})
}

func TestUnsupportedSpecCarriesReason(t *testing.T) {
	specs := []*Spec{{
		Name:     "copilot",
		Launcher: fakeLauncher{name: "copilot"},
		Status:   Status{Supported: false, Reason: "talks to GitHub's own backend"},
	}}
	idx := buildIndex(specs)
	got := idx["copilot"]
	if got.Status.Supported {
		t.Error("spec should be unsupported")
	}
	if got.Status.Reason == "" {
		t.Error("unsupported spec must carry a reason")
	}
}

func TestRegistryPhase3Agents(t *testing.T) {
	for _, name := range []string{"codex", "opencode"} {
		spec, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if !spec.Status.Supported {
			t.Errorf("%q registered unsupported", name)
		}
		if len(spec.Aliases) != 0 {
			t.Errorf("%q has aliases %q, spec says none", name, spec.Aliases)
		}
	}
}

func TestRegistryUnsupportedDesktopApps(t *testing.T) {
	for _, name := range []string{"chatgpt", "claude-desktop", "hermes-desktop"} {
		spec, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if spec.Status.Supported {
			t.Errorf("%q registered as supported", name)
		}
		if spec.Status.Reason == "" {
			t.Errorf("%q has no reason", name)
		}
		if spec.Launcher == nil {
			t.Errorf("%q has nil Launcher", name) // Landmine 10
		}
		if spec.Launcher.Name() != name {
			t.Errorf("%q launcher Name() = %q", name, spec.Launcher.Name())
		}
	}
}

func TestStubCommandErrors(t *testing.T) {
	s := &stub{name: "chatgpt", display: "ChatGPT"}
	if _, err := s.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("stub.Command succeeded; it must always error")
	}
}

func TestRegistryTier2Agents(t *testing.T) {
	// Grows by one name per Phase 4a agent task.
	for _, name := range []string{"pi", "hermes", "qwen", "cline"} {
		spec, err := Lookup(name)
		if err != nil {
			t.Fatalf("Lookup(%q): %v", name, err)
		}
		if !spec.Status.Supported {
			t.Errorf("%q registered unsupported", name)
		}
		if len(spec.Aliases) != 0 {
			t.Errorf("%q has aliases %q, spec says none", name, spec.Aliases)
		}
	}
}
