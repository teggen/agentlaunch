package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/catalog"
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
