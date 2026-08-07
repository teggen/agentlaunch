package agent

import (
	"errors"
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

func TestBuildIndexPanicsOnAliasCollidingWithName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for alias colliding with a canonical name")
		}
	}()
	buildIndex([]*Spec{
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
		{Name: "b", Aliases: []string{"a"}, Launcher: fakeLauncher{name: "b"}},
	})
}

func TestBuildIndexPanicsOnEmptyName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for an empty name")
		}
	}()
	buildIndex([]*Spec{{Name: "", Launcher: fakeLauncher{name: ""}}})
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
