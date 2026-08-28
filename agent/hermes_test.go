package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/catalog"
)

func TestHermesCommandPathArgsEnv(t *testing.T) {
	h := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/hermes")}
	cmd, err := h.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--source", "tool"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"chat", "--provider", testProvider().ID, "--model", "anthropic/claude-opus-4.6", "--source", "tool"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, testProvider().APIKeyEnv); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	// The base-URL pin is deliberate hardening; losing it silently is the
	// class of drift Landmine 18 exists for.
	if got, ok := envValue(cmd.Env, testProvider().UpperID()+"_BASE_URL"); !ok || got != testProvider().BaseURL {
		t.Errorf("OPENROUTER_BASE_URL = %q, %v", got, ok)
	}
}

func TestHermesCommandRequiresAPIKey(t *testing.T) {
	h := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/hermes")}
	if _, err := h.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestHermesCommandRejectsConflictingExtras(t *testing.T) {
	h := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/hermes")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "nous"}, {"--provider=nous"},
		{"gateway"},         // our flags are chat-scoped; another subcommand misapplies them
		{"chat", "--voice"}, // duplicate subcommand
	} {
		if _, err := h.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	if _, err := h.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"-q", "hello"}}); err != nil {
		t.Errorf("chat flags rejected: %v", err)
	}
}

func TestHermesCheckModelContextFloor(t *testing.T) {
	h := &Hermes{Provider: testProvider(), Host: testHost()}
	small := catalog.Model{ID: "small/model", ContextLength: 32768}
	err := h.CheckModel(small)
	if !errors.Is(err, ErrIncompatibleModel) {
		t.Fatalf("32K context: err = %v, want ErrIncompatibleModel (advisory)", err)
	}
	if !strings.Contains(err.Error(), "small/model") {
		t.Errorf("error %q does not name the model", err)
	}
	if err := h.CheckModel(catalog.Model{ID: "big/model", ContextLength: 65536}); err != nil {
		t.Errorf("64K context rejected: %v", err)
	}
	// Unknown context (0) stays silent: missing catalog data is not evidence
	// of incompatibility.
	if err := h.CheckModel(catalog.Model{ID: "unknown/model"}); err != nil {
		t.Errorf("unknown context rejected: %v", err)
	}
	// Live-verified 2026-08-09: hermes's own startup message reads "below
	// the minimum 64,000" (decimal), not the binary-K 65,536 this test used
	// to assert as the floor. Confirmed by launching
	// microsoft/wizardlm-2-8x22b (context 65,535) through raw hermes: it
	// passed hermes's own context gate and failed later for an unrelated
	// reason (no tool-use endpoints) — proving hermes's real floor sits
	// below 65,536. These three cases pin the true boundary at 64,000.
	if err := h.CheckModel(catalog.Model{ID: "boundary/model", ContextLength: 65000}); err != nil {
		t.Errorf("65,000 context (above hermes's real 64,000 floor) rejected: %v", err)
	}
	if err := h.CheckModel(catalog.Model{ID: "floor/model", ContextLength: 64000}); err != nil {
		t.Errorf("exactly 64,000 context rejected: %v", err)
	}
	if err := h.CheckModel(catalog.Model{ID: "under-floor/model", ContextLength: 63999}); !errors.Is(err, ErrIncompatibleModel) {
		t.Errorf("63,999 context: err = %v, want ErrIncompatibleModel", err)
	}
}

// Landmine 8: hermes is really installed at ~/.local/bin/hermes here.
func TestHermesFindPathFallback(t *testing.T) {
	home := testHome(t)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	h := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: notOnPath}
	if h.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hermes"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !h.CheckInstalled() {
		t.Error("CheckInstalled = false with binary at ~/.local/bin/hermes")
	}
}

func TestHermesShadowedCredential(t *testing.T) {
	home := testHome(t)
	h := &Hermes{Provider: testProvider(), Host: testHost()}

	if msg := h.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// .env with an unrelated key: silent.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("BRAVE_API_KEY=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); msg != "" {
		t.Errorf(".env without our key: msg = %q, want empty", msg)
	}

	// .env with a stored OpenRouter key (export form included): warns.
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("export "+testProvider().APIKeyEnv+"=sk-or-old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); !strings.Contains(msg, ".env") {
		t.Errorf("stored .env key: msg = %q, want it to name ~/.hermes/.env", msg)
	}

	// Remove .env; auth.json pool entry alone also warns.
	if err := os.Remove(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"`+testProvider().ID+`":[{"key":"sk-or-old"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := h.ShadowedCredential(); !strings.Contains(msg, "auth.json") {
		t.Errorf("auth pool: msg = %q, want it to name auth.json", msg)
	}
}

func TestHermesDesktopPutsProviderFlagsBeforeTheSubcommand(t *testing.T) {
	h := &HermesDesktop{Hermes: Hermes{
		Provider: testProvider(),
		Host:     testHost(),
		LookPath: func(string) (string, error) { return "/usr/bin/hermes", nil },
	}}
	cmd, err := h.Command(Request{
		Model:  catalog.Model{ID: "acme/model-1", ContextLength: 128000},
		APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("Command() = %v, want nil", err)
	}
	// Verified on hermes v0.20.5: --provider/--model are top-level options,
	// and `hermes desktop` declares neither. Putting them after the
	// subcommand is an argparse error, not a stylistic choice.
	want := []string{"--provider", "acme", "--model", "acme/model-1", "desktop"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args =\n  %q\nwant\n  %q", cmd.Args, want)
	}
}

func TestHermesDesktopAppendsPassthroughAfterTheSubcommand(t *testing.T) {
	h := &HermesDesktop{Hermes: Hermes{
		Provider: testProvider(),
		Host:     testHost(),
		LookPath: func(string) (string, error) { return "/usr/bin/hermes", nil },
	}}
	cmd, err := h.Command(Request{
		Model:     catalog.Model{ID: "acme/model-1"},
		APIKey:    "sk-test",
		ExtraArgs: []string{"--skip-build"},
	})
	if err != nil {
		t.Fatalf("Command() = %v, want nil", err)
	}
	want := []string{"--provider", "acme", "--model", "acme/model-1", "desktop", "--skip-build"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args =\n  %q\nwant\n  %q", cmd.Args, want)
	}
}

func TestHermesDesktopIdentifiesItselfSeparately(t *testing.T) {
	h := &HermesDesktop{}
	if h.Name() != "hermes-desktop" {
		t.Errorf("Name() = %q, want hermes-desktop", h.Name())
	}
	if h.DisplayName() != "Hermes Desktop" {
		t.Errorf("DisplayName() = %q, want Hermes Desktop", h.DisplayName())
	}
}

// The context floor is hermes's, not the CLI's, so the desktop app inherits
// it. This pins the inheritance rather than the floor.
func TestHermesDesktopInheritsTheContextFloor(t *testing.T) {
	h := &HermesDesktop{Hermes: Hermes{Provider: testProvider()}}
	err := h.CheckModel(catalog.Model{ID: "acme/tiny", ContextLength: 8000})
	if !errors.Is(err, ErrIncompatibleModel) {
		t.Errorf("CheckModel() = %v, want it to wrap ErrIncompatibleModel", err)
	}
}

// The bare-subcommand guard is inherited from Hermes.Command via delegation,
// but its wording must name the launcher that actually runs — a user hitting
// this through hermes-desktop and being told "always runs hermes chat" would
// be misdirected. This pins the desktop-worded message.
func TestHermesDesktopBareSubcommandErrorNamesDesktopNotChat(t *testing.T) {
	h := &HermesDesktop{Hermes: Hermes{
		Provider: testProvider(),
		Host:     testHost(),
		LookPath: func(string) (string, error) { return "/usr/bin/hermes", nil },
	}}
	_, err := h.Command(Request{
		Model:     catalog.Model{ID: "acme/model-1"},
		APIKey:    "sk-test",
		ExtraArgs: []string{"foo"},
	})
	if err == nil {
		t.Fatal("Command() = nil, want an error for a bare-subcommand passthrough")
	}
	want := `hermes-desktop: passthrough "foo" looks like a hermes subcommand: acme-launch always runs "hermes desktop"; pass desktop flags only`
	if err.Error() != want {
		t.Errorf("err =\n  %q\nwant\n  %q", err.Error(), want)
	}
}

// Regression bar for the guard's extraction into a parameterized helper:
// Hermes's own message must come out byte-identical to what it was before
// the refactor.
func TestHermesBareSubcommandErrorWordingUnchanged(t *testing.T) {
	h := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/hermes")}
	_, err := h.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"gateway"}})
	if err == nil {
		t.Fatal("Command() = nil, want an error for a bare-subcommand passthrough")
	}
	want := `hermes: passthrough "gateway" looks like a hermes subcommand: acme-launch always runs "hermes chat"; pass chat flags only`
	if err.Error() != want {
		t.Errorf("err =\n  %q\nwant\n  %q", err.Error(), want)
	}
}

// A bare-subcommand-shaped passthrough loses to an earlier guard exactly the
// way it does in Hermes: threading the launcher's naming through as data
// (rather than checking the bare-subcommand guard early, before delegating)
// keeps HermesDesktop and Hermes agreeing on precedence — the whole point
// of this restructure.
func TestHermesDesktopGuardPrecedenceMatchesHermes(t *testing.T) {
	// "gateway" looks like a bare subcommand (first element, no leading
	// "-"), but "--model" elsewhere in the same passthrough must trip the
	// earlier rejectModelFlag guard first: it scans the whole slice, while
	// the bare-subcommand guard only looks at args[0].
	extras := []string{"gateway", "--model", "x/y"}

	desktop := &HermesDesktop{Hermes: Hermes{
		Provider: testProvider(),
		Host:     testHost(),
		LookPath: func(string) (string, error) { return "/usr/bin/hermes", nil },
	}}
	_, desktopErr := desktop.Command(Request{
		Model:     catalog.Model{ID: "acme/model-1"},
		APIKey:    "sk-test",
		ExtraArgs: extras,
	})
	if desktopErr == nil {
		t.Fatal("HermesDesktop.Command() = nil, want the earlier --model guard's error")
	}
	if !strings.Contains(desktopErr.Error(), "conflicting argument") {
		t.Errorf("HermesDesktop error = %q, want the --model guard's error", desktopErr.Error())
	}
	if strings.Contains(desktopErr.Error(), "looks like a hermes subcommand") {
		t.Errorf("HermesDesktop error = %q, want the earlier guard to win, not the bare-subcommand guard", desktopErr.Error())
	}
	// The precedence agreement alone isn't enough: rejectModelFlag's message
	// is also agent-named, and it must say which launcher failed rather than
	// merely that a --model guard fired somewhere. "contains conflicting
	// argument" alone would pass even if this still said "hermes: ...".
	if !strings.HasPrefix(desktopErr.Error(), "hermes-desktop: ") {
		t.Errorf("HermesDesktop error = %q, want it prefixed %q", desktopErr.Error(), "hermes-desktop: ")
	}

	plain := &Hermes{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/hermes")}
	_, plainErr := plain.Command(Request{Model: testModel(), APIKey: "sk-test", ExtraArgs: extras})
	if plainErr == nil {
		t.Fatal("Hermes.Command() = nil, want the earlier --model guard's error")
	}
	if !strings.Contains(plainErr.Error(), "conflicting argument") {
		t.Errorf("Hermes error = %q, want the --model guard's error", plainErr.Error())
	}
	if strings.Contains(plainErr.Error(), "looks like a hermes subcommand") {
		t.Errorf("Hermes error = %q, want the earlier guard to win, not the bare-subcommand guard", plainErr.Error())
	}
	if !strings.HasPrefix(plainErr.Error(), "hermes: ") {
		t.Errorf("Hermes error = %q, want it prefixed %q", plainErr.Error(), "hermes: ")
	}
}
