package launch

import (
	"errors"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
)

// The Error() strings below are the CLI's current output verbatim. They are
// asserted exactly, not by substring, because cobra prints them to the user
// and this refactor promises byte-identical output.

func TestUnsupportedAgentErrorMessage(t *testing.T) {
	err := &UnsupportedAgentError{Agent: "copilot", Reason: "talks to GitHub's own backend"}
	want := "copilot cannot be pointed at OpenRouter: talks to GitHub's own backend"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
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

// openRouterRegistry is the registry this tool actually ships. The guard
// below is a claim about every entry in it, so a synthetic binding would not
// do: the point is that no SHIPPED unsupported agent slips past CheckSupported.
func openRouterRegistry(t *testing.T) *agent.Registry {
	t.Helper()
	reg, err := agent.NewRegistry(
		agent.Binding{Provider: agent.OpenRouter, Host: agent.OpenRouterHost}, agent.Builtins())
	if err != nil {
		t.Fatalf("agent.NewRegistry: %v", err)
	}
	return reg
}

func TestCheckSupportedCoversEveryUnsupportedRegistryEntry(t *testing.T) {
	sawUnsupported := false
	for _, spec := range openRouterRegistry(t).List() {
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
