package agent

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

func stubLookPath(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func testModel() catalog.Model {
	return catalog.Model{
		ID:            "anthropic/claude-opus-4.6",
		Name:          "Anthropic: Claude Opus 4.6",
		Provider:      "anthropic",
		SupportsTools: true,
	}
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix), true
		}
	}
	return "", false
}

func TestClaudeCommandPath(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q", cmd.Path)
	}
}

func TestClaudeCommandArgs(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--resume"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"--model", "anthropic/claude-opus-4.6", "--resume"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}

func TestClaudeCommandEnv(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	want := map[string]string{
		"ANTHROPIC_BASE_URL":             testProvider().AnthropicBaseURL,
		"ANTHROPIC_API_KEY":              "sk-or-test",
		"ANTHROPIC_AUTH_TOKEN":           "",
		"ANTHROPIC_DEFAULT_FABLE_MODEL":  "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "anthropic/claude-opus-4.6",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "anthropic/claude-opus-4.6",
	}
	// This env block IS the product: assert the exact set, not just that the
	// expected keys are present, so a stray variable fails the test.
	if len(cmd.Env) != len(want) {
		t.Errorf("len(cmd.Env) = %d, want %d (exact set): %v", len(cmd.Env), len(want), cmd.Env)
	}
	for key, wantVal := range want {
		got, ok := envValue(cmd.Env, key)
		if !ok {
			t.Errorf("%s not set", key)
			continue
		}
		if got != wantVal {
			t.Errorf("%s = %q, want %q", key, got, wantVal)
		}
	}
}

func TestClaudeCommandRequiresAPIKey(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Error("expected an error when the API key is empty")
	}
}

func TestClaudeCommandBinaryMissing(t *testing.T) {
	// findPath falls back to checking well-known installer locations under
	// $HOME when LookPath fails. Point HOME at an empty temp dir so those
	// fallback checks genuinely miss, regardless of what happens to be
	// installed on the machine running this test.
	testHome(t)
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) {
		return "", errors.New("not found")
	}}
	if _, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"}); err == nil {
		t.Error("expected an error when the binary is missing")
	}
}

func TestClaudeCommandFallsBackToInstallerPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fallback candidate filename differs on windows (claude.exe); covered by findPath's runtime.GOOS branch, not by this fixture")
	}

	home := testHome(t)

	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	binPath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) {
		return "", errors.New("not on PATH")
	}}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != binPath {
		t.Errorf("Path = %q, want %q", cmd.Path, binPath)
	}
}

func TestClaudeCheckModelAcceptsAnthropic(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost()}
	if err := c.CheckModel(testModel()); err != nil {
		t.Errorf("anthropic model rejected: %v", err)
	}
}

func TestClaudeCheckModelWarnsOnOtherProviders(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost()}
	m := catalog.Model{ID: "qwen/qwen3-coder", Provider: "qwen"}
	err := c.CheckModel(m)
	if !errors.Is(err, ErrIncompatibleModel) {
		t.Errorf("got %v, want an error wrapping ErrIncompatibleModel", err)
	}
}

func TestClaudeIdentity(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost()}
	if c.Name() != "claude" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.DisplayName() != "Claude Code" {
		t.Errorf("DisplayName = %q", c.DisplayName())
	}
}

func TestClaudeSatisfiesInterfaces(t *testing.T) {
	var _ Launcher = &Claude{Provider: testProvider(), Host: testHost()}
	var _ Installable = &Claude{Provider: testProvider(), Host: testHost()}
	var _ Compatible = &Claude{Provider: testProvider(), Host: testHost()}
}

// TestClaudeCommandRejectsConflictingExtras pins claude into the rule the
// other ten launchers already follow. Claude Code's own --model outranks the
// managed one on argv, and the ANTHROPIC_DEFAULT_*_MODEL env vars keep
// pointing at ours, so accepting both would run the session and its subagents
// on different models while every report says the managed one.
func TestClaudeCommandRejectsConflictingExtras(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
	} {
		if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}

	// The rule is about the conflict, not a ban on passthrough: everything
	// that does not touch the managed model must still reach argv, in order.
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "k",
		ExtraArgs: []string{"--resume", "--verbose"}})
	if err != nil {
		t.Fatalf("benign extras rejected: %v", err)
	}
	want := []string{"--model", "anthropic/claude-opus-4.6", "--resume", "--verbose"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}

// TestClaudeCredentialSlotsInvertForAKeylessProvider pins the rule that
// produces both known-good configurations. One slot always carries the
// credential and the other is present but empty; a provider issuing real keys
// uses x-api-key, one that needs none uses the bearer token. Both being empty
// is Landmine 2 — Claude Code then authenticates against Anthropic directly.
func TestClaudeCredentialSlotsInvertForAKeylessProvider(t *testing.T) {
	local := testProvider()
	local.RequiresAPIKey = false
	local.PlaceholderKey = "local"
	local.AnthropicBaseURL = "http://127.0.0.1:11434"

	c := &Claude{Provider: local, Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	// No key is resolved for a provider that needs none.
	cmd, err := c.Command(Request{Model: testModel(), APIKey: ""})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got, _ := envValue(cmd.Env, "ANTHROPIC_API_KEY"); got != "" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want empty for a keyless provider", got)
	}
	if got, _ := envValue(cmd.Env, "ANTHROPIC_AUTH_TOKEN"); got != "local" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want the placeholder", got)
	}
	if got, _ := envValue(cmd.Env, "ANTHROPIC_BASE_URL"); got != "http://127.0.0.1:11434" {
		t.Errorf("ANTHROPIC_BASE_URL = %q", got)
	}

	// Whichever way round it is, never both empty.
	api, _ := envValue(cmd.Env, "ANTHROPIC_API_KEY")
	tok, _ := envValue(cmd.Env, "ANTHROPIC_AUTH_TOKEN")
	if api == "" && tok == "" {
		t.Error("both credential slots empty: Claude Code would fall back to its own authentication")
	}
}

// TestClaudeRefusesAProviderWithNoAnthropicSurface covers the case a bare
// OpenAI-compatible server presents: there is nothing to point ANTHROPIC_BASE_URL
// at, and guessing the OpenAI root would produce a launch that fails inside
// Claude Code rather than here.
func TestClaudeRefusesAProviderWithNoAnthropicSurface(t *testing.T) {
	p := testProvider()
	p.AnthropicBaseURL = ""
	c := &Claude{Provider: p, Host: testHost(), LookPath: stubLookPath("/usr/local/bin/claude")}
	_, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err == nil {
		t.Fatal("Command accepted a provider with no Anthropic endpoint")
	}
	if !strings.Contains(err.Error(), p.DisplayName) {
		t.Errorf("error %q does not name the provider", err)
	}
}

// TestClaudeCheckModelIgnoresAnAbsentVendorNamespace: a catalog that does not
// express one — a local server's "qwen3-coder:30b" — must not make the
// advisory fire on every model it offers.
func TestClaudeCheckModelIgnoresAnAbsentVendorNamespace(t *testing.T) {
	c := &Claude{Provider: testProvider(), Host: testHost()}
	if err := c.CheckModel(catalog.Model{ID: "qwen3-coder:30b"}); err != nil {
		t.Errorf("CheckModel on a model with no vendor namespace = %v, want nil", err)
	}
	// A KNOWN non-Anthropic vendor still warns; the exemption is for absence,
	// not for everything.
	if err := c.CheckModel(catalog.Model{ID: "qwen/q3", Provider: "qwen"}); err == nil {
		t.Error("CheckModel on a known non-anthropic vendor = nil, want an advisory")
	}
}
