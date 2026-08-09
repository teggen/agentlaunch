package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

func kimiModel() openrouter.Model {
	return openrouter.Model{ID: "moonshotai/kimi-k2.5", ContextLength: 262144}
}

func TestKimiCommandPathArgsEnv(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	cmd, err := k.Command(Request{
		Model:     kimiModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--verbose"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// The whole configuration is the KIMI_MODEL_* env family; argv carries
	// only passthrough. No --config: that flag belongs to the deprecated
	// legacy Python kimi-cli and does not exist on Kimi Code CLI.
	if want := []string{"--verbose"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	for key, val := range map[string]string{
		"KIMI_MODEL_NAME":             "moonshotai/kimi-k2.5",
		"KIMI_MODEL_API_KEY":          "sk-or-test",
		"KIMI_MODEL_PROVIDER_TYPE":    "openai",
		"KIMI_MODEL_BASE_URL":         "https://openrouter.ai/api/v1",
		"KIMI_MODEL_MAX_CONTEXT_SIZE": "262144",
	} {
		if got, ok := envValue(cmd.Env, key); !ok || got != val {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, val)
		}
	}
}

func TestKimiCommandOmitsContextSizeWhenUnknown(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	cmd, err := k.Command(Request{
		Model:  openrouter.Model{ID: "moonshotai/kimi-k2.5"}, // ContextLength 0
		APIKey: "sk-or-test",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// Better kimi's documented default (262144) than our fabricated zero:
	// KIMI_MODEL_MAX_CONTEXT_SIZE=0 would break the session.
	if _, ok := envValue(cmd.Env, "KIMI_MODEL_MAX_CONTEXT_SIZE"); ok {
		t.Error("KIMI_MODEL_MAX_CONTEXT_SIZE set for unknown context, want omitted")
	}
}

func TestKimiCommandRequiresAPIKey(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	if _, err := k.Command(Request{Model: kimiModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestKimiCommandRejectsConflictingExtras(t *testing.T) {
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}
	for _, extras := range [][]string{
		{"-m", "other"}, {"-mother"}, {"--model", "other"}, {"--model=other"},
		{"--config", "{}"}, {"--config={}"},
		{"--config-file", "x.toml"}, {"--config-file=x.toml"},
	} {
		if _, err := k.Command(Request{Model: kimiModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8 discipline for every location probe below.
func TestKimiFindPathPrefersKimiCodeOverLegacy(t *testing.T) {
	home := testHome(t)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	k := &Kimi{LookPath: notOnPath}

	if k.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}

	legacy := filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !k.CheckInstalled() {
		t.Fatal("CheckInstalled = false with legacy binary present")
	}

	kimiCode := filepath.Join(home, ".kimi-code", "bin")
	if err := os.MkdirAll(kimiCode, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kimiCode, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := k.Command(Request{Model: kimiModel(), APIKey: "sk"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != filepath.Join(kimiCode, "kimi") {
		t.Errorf("Path = %q: Kimi Code's own install dir must beat the legacy uv path", cmd.Path)
	}
}

func TestKimiShadowedCredentialFlagsLegacyOnlyInstall(t *testing.T) {
	home := testHome(t)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }
	k := &Kimi{LookPath: notOnPath}

	if msg := k.ShadowedCredential(); msg != "" {
		t.Errorf("no binary anywhere: msg = %q, want empty", msg)
	}

	legacy := filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if msg := k.ShadowedCredential(); !strings.Contains(msg, "legacy") {
		t.Errorf("legacy-only install: msg = %q, want a legacy kimi-cli warning", msg)
	}

	kimiCode := filepath.Join(home, ".kimi-code", "bin")
	if err := os.MkdirAll(kimiCode, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kimiCode, "kimi"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if msg := k.ShadowedCredential(); msg != "" {
		t.Errorf("Kimi Code present: msg = %q, want empty", msg)
	}
}

// A PATH hit is trusted per ShadowedCredential's doc comment: the Kimi Code
// installer renames legacy shims to kimi-legacy, so anything still resolved
// via LookPath is assumed current, with no path-heuristic warning.
func TestKimiShadowedCredentialTrustsPATHHit(t *testing.T) {
	testHome(t)
	k := &Kimi{LookPath: stubLookPath("/usr/local/bin/kimi")}

	if msg := k.ShadowedCredential(); msg != "" {
		t.Errorf("PATH hit: msg = %q, want empty", msg)
	}
}
