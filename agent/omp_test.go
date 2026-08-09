package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestOMPCommandPathArgsEnv(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	cmd, err := o.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"-p", "hi"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// The openrouter/ prefix IS the provider selection in omp's dialect. A
	// bare slug is a valid-looking wrong value (pi's dialect, not omp's).
	want := []string{"--model", "openrouter/anthropic/claude-opus-4.6", "-p", "hi"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOMPCommandRequiresAPIKey(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	if _, err := o.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestOMPCommandRejectsConflictingExtras(t *testing.T) {
	o := &OMP{LookPath: stubLookPath("/usr/local/bin/omp")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"--provider", "openai"}, {"--provider=openai"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	// --api-key is the user's explicit override of omp's stored-credential
	// precedence — allowed, their call (documented in the spec's key policy).
	if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"--api-key", "sk-or-theirs"}}); err != nil {
		t.Errorf("--api-key rejected: %v", err)
	}
}

func TestOMPFindPathFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	o := &OMP{LookPath: notOnPath}

	if o.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
	for _, rel := range []string{filepath.Join(".local", "bin"), filepath.Join(".bun", "bin")} {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "omp")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if !o.CheckInstalled() {
			t.Errorf("CheckInstalled = false with binary at %s", bin)
		}
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOMPInstallHint(t *testing.T) {
	o := &OMP{}
	if hint := o.InstallHint(); !strings.Contains(hint, "https://omp.sh/install") {
		t.Errorf("InstallHint = %q", hint)
	}
}
