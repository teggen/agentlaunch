package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestQwenCommandPathArgsEnv(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	cmd, err := q.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--approval-mode", "plan"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// --auth-type openai is MANDATORY: without it qwen-code resolves auth
	// from persisted settings or its qwen-oauth default, both of which
	// silently ignore every OPENAI_* env var (upstream issue #891).
	want := []string{"--auth-type", "openai", "--model", "anthropic/claude-opus-4.6", "--approval-mode", "plan"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	for key, val := range map[string]string{
		"OPENAI_BASE_URL":    "https://openrouter.ai/api/v1",
		"OPENAI_API_KEY":     "sk-or-test",
		"OPENROUTER_API_KEY": "sk-or-test",
		"OPENAI_MODEL":       "anthropic/claude-opus-4.6",
	} {
		if got, ok := envValue(cmd.Env, key); !ok || got != val {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, val)
		}
	}
}

func TestQwenCommandRequiresAPIKey(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	if _, err := q.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestQwenCommandRejectsConflictingExtras(t *testing.T) {
	q := &Qwen{LookPath: stubLookPath("/usr/local/bin/qwen")}
	for _, extras := range [][]string{
		{"-m", "x"}, {"--model", "x"}, {"--model=x"},
		{"--auth-type", "qwen-oauth"}, {"--auth-type=qwen-oauth"},
		{"--openai-api-key", "sk-mine"}, {"--openai-api-key=sk-mine"},
		{"--openai-base-url", "http://mine"}, {"--openai-base-url=http://mine"},
	} {
		if _, err := q.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	if _, err := q.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"--yolo", "-p", "hi"}}); err != nil {
		t.Errorf("benign extras rejected: %v", err)
	}
}

func TestQwenFindPathFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	q := &Qwen{LookPath: notOnPath}

	if q.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	for _, rel := range []string{
		filepath.Join(".npm-global", "bin"),
		filepath.Join(".local", "bin"),
		filepath.Join(".nvm", "versions", "node", "v22.19.0", "bin"),
	} {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "qwen")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !q.CheckInstalled() {
			t.Errorf("CheckInstalled = false with binary at %s", bin)
		}
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQwenFindPathPrefersHighestNvmVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	q := &Qwen{LookPath: notOnPath}

	// Create two nvm version directories with qwen binaries
	v22Dir := filepath.Join(home, ".nvm", "versions", "node", "v22.19.0", "bin")
	v24Dir := filepath.Join(home, ".nvm", "versions", "node", "v24.1.0", "bin")

	for _, dir := range []string{v22Dir, v24Dir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "qwen")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Command should resolve to the higher version (v24.1.0)
	cmd, err := q.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	expectedPath := filepath.Join(v24Dir, "qwen")
	if cmd.Path != expectedPath {
		t.Errorf("Path = %q, want %q (highest nvm version)", cmd.Path, expectedPath)
	}
}

func TestQwenInstallHint(t *testing.T) {
	q := &Qwen{}
	if hint := q.InstallHint(); !strings.Contains(hint, "@qwen-code/qwen-code") {
		t.Errorf("InstallHint = %q", hint)
	}
}
