package agent

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCodexCommandPathArgsEnv(t *testing.T) {
	c := &Codex{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/codex")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"resume"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/codex" {
		t.Errorf("Path = %q", cmd.Path)
	}
	// Managed overrides first, then -m, then passthrough. Order matters:
	// codex options are global, so managed flags must precede a passthrough
	// subcommand like "resume".
	want := []string{
		"-c", `model_provider="` + testProvider().ID + `"`,
		"-c", "model_providers." + testProvider().ID + `.name="` + testProvider().DisplayName + `"`,
		"-c", "model_providers." + testProvider().ID + `.base_url="` + testProvider().BaseURL + `"`,
		"-c", "model_providers." + testProvider().ID + `.env_key="` + testProvider().APIKeyEnv + `"`,
		"-c", "model_providers." + testProvider().ID + `.wire_api="` + testProvider().WireAPI + `"`,
		"-m", "anthropic/claude-opus-4.6",
		"resume",
	}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args =\n%q\nwant\n%q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, testProvider().APIKeyEnv); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestCodexCommandRequiresAPIKey(t *testing.T) {
	c := &Codex{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/codex")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestCodexCommandRejectsConflictingExtras(t *testing.T) {
	c := &Codex{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/codex")}
	for _, extras := range [][]string{
		{"-m", "gpt-5"},
		{"-mgpt-5"},
		{"--model", "gpt-5"},
		{"--model=gpt-5"},
		{"-c", "model=gpt-5"},
		{"-c", `model_provider="mine"`},
		{"-c", "model_providers.mine.base_url=http://x"},
		{`-cmodel_provider="mine"`},
		{"--config", "model_provider=mine"},
		{"--config=model_providers.mine.name=X"},
	} {
		_, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras})
		if err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
			continue
		}
		named := strings.Contains(err.Error(), extras[0]) ||
			(len(extras) > 1 && strings.Contains(err.Error(), extras[1]))
		if !named {
			t.Errorf("extras %q: error %q does not name the argument", extras, err)
		}
	}
}

func TestCodexCommandAllowsBenignExtras(t *testing.T) {
	c := &Codex{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/codex")}
	extras := []string{"exec", "--full-auto", "-c", "foo=bar", "--profile", "mine", "--config", "sandbox_mode=read-only"}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras})
	if err != nil {
		t.Fatalf("benign extras rejected: %v", err)
	}
	if !slices.Equal(cmd.Args[len(cmd.Args)-len(extras):], extras) {
		t.Errorf("extras not appended verbatim: %q", cmd.Args)
	}
}

func TestCodexCommandBinaryMissing(t *testing.T) {
	c := &Codex{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	if _, err := c.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("Command with missing binary succeeded, want error")
	}
}

func TestCodexInstallable(t *testing.T) {
	installed := &Codex{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/codex")}
	if !installed.CheckInstalled() {
		t.Error("CheckInstalled = false with binary present")
	}
	missing := &Codex{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) { return "", errors.New("no") }}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true with binary absent")
	}
	if hint := missing.InstallHint(); !strings.Contains(hint, "npm install -g @openai/codex") {
		t.Errorf("InstallHint = %q", hint)
	}
}
