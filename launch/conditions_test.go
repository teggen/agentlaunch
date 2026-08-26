package launch

import (
	"errors"
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/agent"
)

// The Error() strings below are the CLI's current output verbatim. They are
// asserted exactly, not by substring, because cobra prints them to the user
// and this refactor promises byte-identical output.

// TestUnsupportedAgentErrorMessage covers both halves of the message, and the
// named-provider case uses a provider that is deliberately not the one this
// package was extracted from: the string here was a hardcoded "OpenRouter"
// until a second consumer was written against the module and its refusals
// named the wrong company. A fixture that said "OpenRouter" would be
// satisfied by that literal coming back.
func TestUnsupportedAgentErrorMessage(t *testing.T) {
	err := &UnsupportedAgentError{
		Agent:    "copilot",
		Provider: "Acme",
		Reason:   "talks to GitHub's own backend",
	}
	want := "copilot cannot be pointed at Acme: talks to GitHub's own backend"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// A Spec built with no Binding — which NewRegistryFromSpecs allows, for
// consumers' tests over adversarial entries — has no provider name. The
// message must still read as a sentence rather than leaving a blank where a
// name belongs.
func TestUnsupportedAgentErrorMessageWithoutAProviderName(t *testing.T) {
	err := &UnsupportedAgentError{Agent: "copilot", Reason: "talks to GitHub's own backend"}
	want := "copilot cannot be pointed at this provider: talks to GitHub's own backend"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestCheckSupportedCarriesTheBoundProviderName is the wiring half: Error()
// can format a provider correctly while CheckSupported never supplies one,
// and every refusal a user sees comes through CheckSupported.
func TestCheckSupportedCarriesTheBoundProviderName(t *testing.T) {
	spec, err := builtinRegistry(t).Lookup("chatgpt")
	if err != nil {
		t.Fatalf("Lookup(chatgpt): %v", err)
	}
	unsupported := CheckSupported(spec)
	if unsupported == nil {
		t.Fatal("CheckSupported accepted a desktop app")
	}
	var e *UnsupportedAgentError
	if !errors.As(unsupported, &e) {
		t.Fatalf("CheckSupported returned %T, want *UnsupportedAgentError", unsupported)
	}
	if e.Provider != testProvider().DisplayName {
		t.Errorf("Provider = %q, want the bound provider's %q",
			e.Provider, testProvider().DisplayName)
	}
	if !strings.Contains(unsupported.Error(), testProvider().DisplayName) {
		t.Errorf("message %q does not name the bound provider", unsupported.Error())
	}
}

func TestNotInstalledErrorMessage(t *testing.T) {
	err := &NotInstalledError{
		Agent:       "claude",
		DisplayName: "Claude Code",
		Hint:        "Install Claude Code: https://code.claude.com/docs/en/quickstart",
	}
	want := "Claude Code is not installed.\nInstall Claude Code: https://code.claude.com/docs/en/quickstart"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// The TUI renders the install hint as its own element, so it has to be
// reachable as data and not only as a substring of Error().
func TestNotInstalledErrorCarriesPayload(t *testing.T) {
	var err error = &NotInstalledError{
		Agent: "claude", DisplayName: "Claude Code", Hint: "brew install claude",
	}
	var nie *NotInstalledError
	if !errors.As(err, &nie) {
		t.Fatalf("errors.As did not recover *NotInstalledError from %T", err)
	}
	if nie.Hint != "brew install claude" {
		t.Errorf("Hint = %q", nie.Hint)
	}
	if nie.Agent != "claude" {
		t.Errorf("Agent = %q", nie.Agent)
	}
}

func TestUnknownModelErrorWithoutSuggestions(t *testing.T) {
	err := &UnknownModelError{ModelID: "nope/nope"}
	want := `unknown model "nope/nope"`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnknownModelErrorWithSuggestions(t *testing.T) {
	err := &UnknownModelError{
		ModelID:     "anthropic/claude",
		Suggestions: []string{"anthropic/claude-opus-4.6", "anthropic/claude-sonnet-4.5"},
	}
	want := "unknown model \"anthropic/claude\". Did you mean:\n" +
		"  anthropic/claude-opus-4.6\n  anthropic/claude-sonnet-4.5"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// UnsupportedPlatformError must not restate the launcher's message: the CLI
// prints the launcher's error verbatim today, and errors.Is has to keep
// reaching it.
func TestUnsupportedPlatformErrorWrapsAndPreservesText(t *testing.T) {
	inner := errors.New("windows is not supported yet")
	err := &UnsupportedPlatformError{Agent: "droid", Err: inner}

	if got := err.Error(); got != "windows is not supported yet" {
		t.Errorf("Error() = %q, want the launcher's own text", got)
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is should reach the wrapped launcher error")
	}
}

func TestCheckSupportedRejectsUnsupportedAgent(t *testing.T) {
	unsupported := &agent.Spec{
		Name:   "copilot",
		Status: agent.Status{Supported: false, Reason: "talks to GitHub's own backend"},
	}
	err := CheckSupported(unsupported)

	var uae *UnsupportedAgentError
	if !errors.As(err, &uae) {
		t.Fatalf("CheckSupported returned %T (%v), want *UnsupportedAgentError", err, err)
	}
	if uae.Agent != "copilot" {
		t.Errorf("Agent = %q", uae.Agent)
	}
	if uae.Reason != "talks to GitHub's own backend" {
		t.Errorf("Reason = %q", uae.Reason)
	}
}

func TestCheckSupportedAcceptsSupportedAgent(t *testing.T) {
	supported := &agent.Spec{Name: "claude", Status: agent.Status{Supported: true}}
	if err := CheckSupported(supported); err != nil {
		t.Errorf("CheckSupported(supported) = %v, want nil", err)
	}
}

// testProvider is a synthetic provider for the planner's tests. It duplicates
// the agent package's fixture rather than sharing it, because that one is
// unexported — and exporting a test fixture to spare eleven lines would put a
// synthetic provider in the module's public API, where a consuming tool could
// bind a registry to it.
func testProvider() agent.Provider {
	return agent.Provider{
		ID:               "acme",
		DisplayName:      "Acme",
		BaseURL:          "https://api.acme.test/v9",
		AnthropicBaseURL: "https://api.acme.test",
		APIKeyEnv:        "ACME_API_KEY",
		RequiresAPIKey:   true,
		KeysURL:          "https://acme.test/keys",
		ModelPrefix:      "acme/",
		WireAPI:          "responses",
	}
}

// testHost is the synthetic host counterpart to testProvider.
func testHost() agent.Host {
	return agent.Host{Name: "acme-launch", Marker: "acme-launch"}
}

// builtinRegistry is every builtin bound to a synthetic provider. The guard
// below is a claim about every entry in it: no unsupported agent may slip
// past CheckSupported. That property holds per binding, and the bindings a
// consuming tool creates are its own — so the fixture here is synthetic, and
// the agents refused for a given real provider are the consumer's to pin.
func builtinRegistry(t *testing.T) *agent.Registry {
	t.Helper()
	reg, err := agent.NewRegistry(
		agent.Binding{Provider: testProvider(), Host: testHost()}, agent.Builtins())
	if err != nil {
		t.Fatalf("agent.NewRegistry: %v", err)
	}
	return reg
}

func TestCheckSupportedCoversEveryUnsupportedRegistryEntry(t *testing.T) {
	sawUnsupported := false
	for _, spec := range builtinRegistry(t).List() {
		if spec.Status.Supported {
			if err := CheckSupported(spec); err != nil {
				t.Errorf("%q: CheckSupported = %v, want nil", spec.Name, err)
			}
			continue
		}
		sawUnsupported = true
		err := CheckSupported(spec)
		var uae *UnsupportedAgentError
		if !errors.As(err, &uae) {
			t.Errorf("%q: CheckSupported returned %T (%v), want *UnsupportedAgentError", spec.Name, err, err)
			continue
		}
		if uae.Reason != spec.Status.Reason {
			t.Errorf("%q: Reason = %q, want %q", spec.Name, uae.Reason, spec.Status.Reason)
		}
	}
	if !sawUnsupported {
		t.Fatal("registry contains no unsupported agents; this test no longer tests anything")
	}
}
