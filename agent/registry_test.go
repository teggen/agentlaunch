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

// openRouterRegistry is the registry this tool's composition root builds:
// every builtin bound to OpenRouter. Tests asserting something about the
// SHIPPED registry — the golden launch surface, how many agents are
// supported, that "cc" still resolves — must build it from the real binding,
// since a synthetic one would prove nothing about what users get.
func openRouterRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(Binding{Provider: OpenRouter, Host: OpenRouterHost}, Builtins())
	if err != nil {
		t.Fatalf("NewRegistry(OpenRouter): %v", err)
	}
	return reg
}

// fakeDefinition builds a definition over fakeLauncher, for the registry
// mechanics that have nothing to do with any real agent.
func fakeDefinition(name string, aliases ...string) Definition {
	return Definition{
		Name:        name,
		DisplayName: "Fake " + name,
		Aliases:     aliases,
		New:         func(Binding) (Launcher, error) { return fakeLauncher{name: name}, nil },
	}
}

func TestLookupByName(t *testing.T) {
	spec, err := openRouterRegistry(t).Lookup("claude")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if spec.Name != "claude" {
		t.Errorf("Name = %q, want claude", spec.Name)
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	if _, err := openRouterRegistry(t).Lookup("CLAUDE"); err != nil {
		t.Errorf("Lookup(CLAUDE): %v", err)
	}
}

func TestLookupUnknown(t *testing.T) {
	_, err := openRouterRegistry(t).Lookup("nope")
	if !errors.Is(err, ErrUnknownAgent) {
		t.Errorf("got %v, want ErrUnknownAgent", err)
	}
}

func TestListIncludesClaude(t *testing.T) {
	var found bool
	for _, s := range openRouterRegistry(t).List() {
		if s.Name == "claude" {
			found = true
		}
	}
	if !found {
		t.Error("claude missing from List()")
	}
}

func TestNewRegistryFromSpecsResolvesAliases(t *testing.T) {
	reg, err := NewRegistryFromSpecs([]*Spec{{
		Name:     "claude",
		Aliases:  []string{"cc"},
		Launcher: fakeLauncher{name: "claude"},
		Status:   Status{Supported: true},
	}})
	if err != nil {
		t.Fatalf("NewRegistryFromSpecs: %v", err)
	}
	spec, err := reg.Lookup("cc")
	if err != nil {
		t.Fatalf("Lookup(cc): %v", err)
	}
	if spec.Name != "claude" {
		t.Errorf("alias resolved to %q, want claude", spec.Name)
	}
}

func TestNewRegistryFromSpecsRejectsDuplicateName(t *testing.T) {
	_, err := NewRegistryFromSpecs([]*Spec{
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
	})
	if err == nil {
		t.Error("expected an error for duplicate names")
	}
}

// TestNewRegistryFromSpecsRejectsAliasCollidingWithName also pins that the
// message names the offending spec ("b", whose alias "a" is the collision) —
// with fourteen registry entries, that is what lets someone locate the bad
// one from the message alone.
func TestNewRegistryFromSpecsRejectsAliasCollidingWithName(t *testing.T) {
	_, err := NewRegistryFromSpecs([]*Spec{
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
		{Name: "b", Aliases: []string{"a"}, Launcher: fakeLauncher{name: "b"}},
	})
	if err == nil {
		t.Fatal("expected an error for an alias colliding with a canonical name")
	}
	if !strings.Contains(err.Error(), "b") {
		t.Errorf("message should name the offending spec %q, got: %v", "b", err)
	}
}

// TestNewRegistryFromSpecsRejectsEmptyName also pins that the message
// identifies which entry is missing a name (by position, since the name
// itself is what's missing) so it can be located among many entries.
func TestNewRegistryFromSpecsRejectsEmptyName(t *testing.T) {
	_, err := NewRegistryFromSpecs([]*Spec{
		{Name: "a", Launcher: fakeLauncher{name: "a"}},
		{Name: "", Launcher: fakeLauncher{name: ""}},
	})
	if err == nil {
		t.Fatal("expected an error for an empty name")
	}
	if !strings.Contains(err.Error(), "1") {
		t.Errorf("message should identify the offending index (1), got: %v", err)
	}
}

// TestNewRegistryFromSpecsRejectsNilLauncher pins Launcher as a required
// field: every caller that dereferences spec.Launcher (newLaunchCmds,
// agents.go, Installed) would otherwise panic obscurely, far from the
// registry bug that caused it.
func TestNewRegistryFromSpecsRejectsNilLauncher(t *testing.T) {
	if _, err := NewRegistryFromSpecs([]*Spec{{Name: "a", Launcher: nil}}); err == nil {
		t.Error("expected an error for a nil Launcher")
	}
}

// TestMustRegistryPanicsOnAMalformedRegistry pins the split that exists
// because this package is on its way to being a library: NewRegistry reports
// a caller's bad slice as an error, and only the composition-root wrapper
// turns it into a startup panic.
func TestMustRegistryPanicsOnAMalformedRegistry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for duplicate names")
		}
	}()
	MustRegistry(Binding{Provider: OpenRouter, Host: OpenRouterHost},
		[]Definition{fakeDefinition("a"), fakeDefinition("a")})
}

func TestNewRegistryRejectsAnInvalidBinding(t *testing.T) {
	// A provider with no Anthropic root and no OpenAI-compatible root fails
	// Provider.Validate; the point here is that NewRegistry runs that check
	// at all, before any launcher is built.
	_, err := NewRegistry(Binding{Provider: Provider{ID: "x", DisplayName: "X", APIKeyEnv: "X_KEY", RequiresAPIKey: true}},
		[]Definition{fakeDefinition("a")})
	if err == nil {
		t.Error("NewRegistry accepted a provider its own Validate rejects")
	}
}

// TestRegistriesAreIndependent is the whole point of the registry becoming a
// value: two tools, two providers, one set of launch recipes. It fails if
// Builtins stops threading the Binding — if claude reached for a package
// default, both registries would report the same endpoint.
func TestRegistriesAreIndependent(t *testing.T) {
	live := openRouterRegistry(t)
	other, err := NewRegistry(Binding{Provider: testProvider(), Host: testHost()}, Builtins())
	if err != nil {
		t.Fatalf("NewRegistry(acme): %v", err)
	}

	liveSpec, err := live.Lookup("claude")
	if err != nil {
		t.Fatalf("live Lookup: %v", err)
	}
	otherSpec, err := other.Lookup("claude")
	if err != nil {
		t.Fatalf("acme Lookup: %v", err)
	}
	if liveSpec == otherSpec {
		t.Fatal("both registries returned the same *Spec; they share state")
	}

	got := otherSpec.Launcher.(*Claude).Provider.ID
	if got != "acme" {
		t.Errorf("acme registry's claude is bound to %q, want acme", got)
	}
	if got := liveSpec.Launcher.(*Claude).Provider.ID; got != "openrouter" {
		t.Errorf("OpenRouter registry's claude is bound to %q, want openrouter", got)
	}
}

// TestBindingLookPathReachesEveryLauncher pins the field Binding hoisted out
// of eleven identical per-launcher declarations. Without the threading, each
// launcher would fall back to exec.LookPath and consult the real machine.
func TestBindingLookPathReachesEveryLauncher(t *testing.T) {
	testHome(t) // Landmine 8: no real install may satisfy the negative case.

	var asked []string
	reg, err := NewRegistry(Binding{
		Provider: OpenRouter,
		Host:     OpenRouterHost,
		LookPath: func(file string) (string, error) {
			asked = append(asked, file)
			return "", errors.New("not found")
		},
	}, Builtins())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	for _, spec := range reg.List() {
		if !spec.Status.Supported {
			continue
		}
		if reg.Installed(spec) {
			t.Errorf("%s reported installed against a LookPath that finds nothing", spec.Name)
		}
	}
	if len(asked) == 0 {
		t.Fatal("the binding's LookPath was never called")
	}
}

func TestUnsupportedSpecCarriesReason(t *testing.T) {
	reg, err := NewRegistry(Binding{Provider: OpenRouter, Host: OpenRouterHost}, []Definition{{
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		New:         func(Binding) (Launcher, error) { return nil, Unsupported("talks to GitHub's own backend") },
	}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	got, err := reg.Lookup("copilot")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Status.Supported {
		t.Error("spec should be unsupported")
	}
	// Verbatim, not merely non-empty: this string is rendered to users by
	// `agents --all` and by UnsupportedAgentError, so recovering it from
	// err.Error() would prefix every one of those with the sentinel's text.
	if got.Status.Reason != "talks to GitHub's own backend" {
		t.Errorf("Reason = %q, want the reason unaltered", got.Status.Reason)
	}
	if got.Launcher == nil {
		t.Fatal("an unsupported spec still needs a Launcher") // Landmine 10
	}
	if got.Launcher.DisplayName() != "GitHub Copilot" {
		t.Errorf("placeholder DisplayName() = %q", got.Launcher.DisplayName())
	}
}

// TestNewRegistryFailsOnANonProviderError separates the two error kinds a New
// can report: "this agent cannot reach that provider", which is data the
// registry records, from a genuine construction bug, which must not be
// silently rendered to users as a reason.
func TestNewRegistryFailsOnANonProviderError(t *testing.T) {
	_, err := NewRegistry(Binding{Provider: OpenRouter, Host: OpenRouterHost}, []Definition{{
		Name:        "broken",
		DisplayName: "Broken",
		New:         func(Binding) (Launcher, error) { return nil, errors.New("boom") },
	}})
	if err == nil {
		t.Fatal("NewRegistry swallowed a construction failure")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the definition, got: %v", err)
	}
}

// TestNewRegistryRefusesNameDivergence covers the cost of Definition carrying
// names the Launcher also reports: Lookup and the cobra subcommand read
// spec.Name while every rendered label reads Launcher.DisplayName(), so a
// mismatch would be invisible until a user saw two different names for one
// agent.
func TestNewRegistryRefusesNameDivergence(t *testing.T) {
	for _, tc := range []struct {
		what string
		def  Definition
	}{
		{"Name", Definition{
			Name: "a", DisplayName: "Fake b",
			New: func(Binding) (Launcher, error) { return fakeLauncher{name: "b"}, nil },
		}},
		{"DisplayName", Definition{
			Name: "b", DisplayName: "Something Else",
			New: func(Binding) (Launcher, error) { return fakeLauncher{name: "b"}, nil },
		}},
	} {
		_, err := NewRegistry(Binding{Provider: OpenRouter, Host: OpenRouterHost}, []Definition{tc.def})
		if err == nil {
			t.Errorf("%s: NewRegistry accepted a launcher that disagrees with its definition", tc.what)
		}
	}
}

func TestNewRegistryRequiresADisplayNameAndANewFunc(t *testing.T) {
	b := Binding{Provider: OpenRouter, Host: OpenRouterHost}
	if _, err := NewRegistry(b, []Definition{{Name: "a", New: fakeDefinition("a").New}}); err == nil {
		t.Error("NewRegistry accepted a definition with no DisplayName")
	}
	if _, err := NewRegistry(b, []Definition{{Name: "a", DisplayName: "A"}}); err == nil {
		t.Error("NewRegistry accepted a definition with a nil New")
	}
	if _, err := NewRegistry(b, []Definition{{DisplayName: "A", New: fakeDefinition("a").New}}); err == nil {
		t.Error("NewRegistry accepted a definition with no Name")
	}
}

func TestRegistryPhase3Agents(t *testing.T) {
	reg := openRouterRegistry(t)
	for _, name := range []string{"codex", "opencode"} {
		spec, err := reg.Lookup(name)
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
	reg := openRouterRegistry(t)
	for _, name := range []string{"chatgpt", "claude-desktop", "hermes-desktop"} {
		spec, err := reg.Lookup(name)
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

// TestStubCommandErrors names a provider that is deliberately NOT OpenRouter,
// so a placeholder that went back to a hardcoded "OpenRouter" fails here
// rather than passing against a fixture that merely looks right.
func TestStubCommandErrors(t *testing.T) {
	s := &stub{name: "chatgpt", display: "ChatGPT", provider: "Acme"}
	_, err := s.Command(Request{Model: testModel(), APIKey: "k"})
	if err == nil {
		t.Fatal("stub.Command succeeded; it must always error")
	}
	if !strings.Contains(err.Error(), "Acme") {
		t.Errorf("stub error should name the bound provider, got: %v", err)
	}
}

func TestRegistryTier2Agents(t *testing.T) {
	reg := openRouterRegistry(t)
	// Grows by one name per Phase 4a agent task.
	for _, name := range []string{"pi", "hermes", "qwen", "cline", "kimi", "omp", "openclaw", "droid"} {
		spec, err := reg.Lookup(name)
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

// TestUnsupportedProviderSurvivesWrapping pins both halves of the contract a
// Definition's New relies on: the sentinel is reachable through errors.Is for
// a caller that only wants to classify the failure, and the reason is
// recoverable through errors.As even when the error has been wrapped on the
// way out — which is what lets NewRegistry record the prose verbatim.
func TestUnsupportedProviderSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", Unsupported("no such thing"))
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Error("UnsupportedProviderError must satisfy errors.Is(ErrUnsupportedProvider)")
	}
	var upe *UnsupportedProviderError
	if !errors.As(err, &upe) || upe.Reason != "no such thing" {
		t.Errorf("errors.As did not recover the reason: %v", err)
	}
}
