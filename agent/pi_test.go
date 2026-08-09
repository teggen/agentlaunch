package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPiCommandPathArgsEnv(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	cmd, err := p.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--thinking", "high"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/pi" {
		t.Errorf("Path = %q", cmd.Path)
	}
	// Bare OpenRouter slug: pi selects the provider with --provider, never
	// an "openrouter/" prefix on the model (that is omp's dialect, not pi's).
	want := []string{"--provider", "openrouter", "--model", "anthropic/claude-opus-4.6", "--thinking", "high"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	for _, e := range cmd.Env {
		if strings.Contains(e, "sk-or-test") && !strings.HasPrefix(e, "OPENROUTER_API_KEY=") {
			t.Errorf("key leaked into unexpected env entry %q", e)
		}
	}
}

func TestPiCommandRequiresAPIKey(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	if _, err := p.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestPiCommandRejectsConflictingExtras(t *testing.T) {
	p := &Pi{LookPath: stubLookPath("/usr/local/bin/pi")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "anthropic"}, {"--provider=anthropic"},
	} {
		if _, err := p.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8: pi is really installed at ~/.local/bin/pi on this machine.
func TestPiFindPathFallback(t *testing.T) {
	home := testHome(t)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	missing := &Pi{LookPath: notOnPath}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}

	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !missing.CheckInstalled() {
		t.Error("CheckInstalled = false with binary at ~/.local/bin/pi")
	}
}

func TestPiInstallHint(t *testing.T) {
	p := &Pi{}
	if hint := p.InstallHint(); !strings.Contains(hint, "@earendil-works/pi-coding-agent") {
		t.Errorf("InstallHint = %q; the legacy @mariozechner package is deprecated", hint)
	}
}

func TestPiShadowedCredential(t *testing.T) {
	home := testHome(t)
	p := &Pi{}

	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("no auth.json: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(dir, "auth.json")

	if err := os.WriteFile(authPath, []byte(`{"anthropic":{"type":"oauth"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("auth.json without openrouter: msg = %q, want empty", msg)
	}

	if err := os.WriteFile(authPath, []byte(`{"openrouter":{"type":"api_key"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); !strings.Contains(msg, "auth.json") {
		t.Errorf("stored openrouter credential: msg = %q, want it to name auth.json", msg)
	}

	if err := os.WriteFile(authPath, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := p.ShadowedCredential(); msg != "" {
		t.Errorf("malformed auth.json: msg = %q, want empty (detector failure must not warn)", msg)
	}
}
