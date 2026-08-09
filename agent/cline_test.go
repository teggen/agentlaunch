package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClineCommandPathArgsEnv(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--auto-approve", "false"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"-P", "openrouter", "-m", "anthropic/claude-opus-4.6", "--auto-approve", "false"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	// Owner decision: key travels in env only, never on argv (-k would put
	// it in /proc/<pid>/cmdline).
	for _, a := range cmd.Args {
		if strings.Contains(a, "sk-or-test") {
			t.Errorf("key leaked onto argv: %q", a)
		}
	}
}

func TestClineCommandRequiresAPIKey(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestClineCommandRejectsConflictingExtras(t *testing.T) {
	c := &Cline{LookPath: stubLookPath("/usr/local/bin/cline")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"-P", "cline"}, {"-Pcline"}, {"--provider", "cline"}, {"--provider=cline"},
	} {
		if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
	// -k is the user's explicit per-run key override — allowed, their call.
	if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: []string{"-k", "sk-or-theirs"}}); err != nil {
		t.Errorf("-k rejected: %v", err)
	}
}

func TestClineShadowedCredential(t *testing.T) {
	home := testHome(t)
	c := &Cline{}

	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}

	dir := filepath.Join(home, ".cline", "data", "settings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "providers.json")

	// openrouter entry without a key: silent.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"model":"x"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("keyless entry: msg = %q, want empty", msg)
	}

	// apiKey at the entry level: warns.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"apiKey":"sk-or-old"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); !strings.Contains(msg, "providers.json") {
		t.Errorf("entry-level key: msg = %q, want it to name providers.json", msg)
	}

	// apiKey nested under settings: warns.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-old"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg == "" {
		t.Error("settings-level key: msg empty, want warning")
	}

	// Malformed file: silent.
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := c.ShadowedCredential(); msg != "" {
		t.Errorf("malformed file: msg = %q, want empty", msg)
	}
}

func TestClineInstallHint(t *testing.T) {
	c := &Cline{}
	if hint := c.InstallHint(); !strings.Contains(hint, "npm install -g cline") {
		t.Errorf("InstallHint = %q", hint)
	}
}
