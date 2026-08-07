package agent

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func stubLookPath(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func testModel() openrouter.Model {
	return openrouter.Model{
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
	c := &Claude{LookPath: stubLookPath("/usr/local/bin/claude")}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/claude" {
		t.Errorf("Path = %q", cmd.Path)
	}
}

func TestClaudeCommandArgs(t *testing.T) {
	c := &Claude{LookPath: stubLookPath("/usr/local/bin/claude")}
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
	c := &Claude{LookPath: stubLookPath("/usr/local/bin/claude")}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	want := map[string]string{
		"ANTHROPIC_BASE_URL":             "https://openrouter.ai/api",
		"ANTHROPIC_API_KEY":              "sk-or-test",
		"ANTHROPIC_AUTH_TOKEN":           "",
		"ANTHROPIC_DEFAULT_FABLE_MODEL":  "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "anthropic/claude-opus-4.6",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "anthropic/claude-opus-4.6",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "anthropic/claude-opus-4.6",
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

func TestClaudeBaseURLHasNoVersionSuffix(t *testing.T) {
	if strings.HasSuffix(AnthropicBaseURL, "/v1") {
		t.Errorf("AnthropicBaseURL = %q must not end in /v1", AnthropicBaseURL)
	}
}

func TestClaudeCommandRequiresAPIKey(t *testing.T) {
	c := &Claude{LookPath: stubLookPath("/usr/local/bin/claude")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Error("expected an error when the API key is empty")
	}
}

func TestClaudeCommandBinaryMissing(t *testing.T) {
	// findPath falls back to checking well-known installer locations under
	// $HOME when LookPath fails. Point HOME at an empty temp dir so those
	// fallback checks genuinely miss, regardless of what happens to be
	// installed on the machine running this test.
	t.Setenv("HOME", t.TempDir())
	c := &Claude{LookPath: func(string) (string, error) {
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

	home := t.TempDir()
	t.Setenv("HOME", home)

	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	binPath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := &Claude{LookPath: func(string) (string, error) {
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
	c := &Claude{}
	if err := c.CheckModel(testModel()); err != nil {
		t.Errorf("anthropic model rejected: %v", err)
	}
}

func TestClaudeCheckModelWarnsOnOtherProviders(t *testing.T) {
	c := &Claude{}
	m := openrouter.Model{ID: "qwen/qwen3-coder", Provider: "qwen"}
	err := c.CheckModel(m)
	if !errors.Is(err, ErrIncompatibleModel) {
		t.Errorf("got %v, want an error wrapping ErrIncompatibleModel", err)
	}
}

func TestClaudeIdentity(t *testing.T) {
	c := &Claude{}
	if c.Name() != "claude" {
		t.Errorf("Name = %q", c.Name())
	}
	if c.DisplayName() != "Claude Code" {
		t.Errorf("DisplayName = %q", c.DisplayName())
	}
}

func TestClaudeSatisfiesInterfaces(t *testing.T) {
	var _ Launcher = &Claude{}
	var _ Installable = &Claude{}
	var _ Compatible = &Claude{}
}
