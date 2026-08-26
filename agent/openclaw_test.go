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

func TestOpenClawInteractiveCommandAndStagedFile(t *testing.T) {
	stage := t.TempDir()
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	req := Request{Model: testModel(), APIKey: "sk-or-test", StageDir: stage}

	cmd, err := o.Command(req)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if want := []string{"tui", "--local"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}

	files, err := o.StagedFiles(req)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("StagedFiles returned %d files, want 1", len(files))
	}
	if want := filepath.Join(stage, "openclaw.json"); files[0].Path != want {
		t.Errorf("staged path = %q, want %q", files[0].Path, want)
	}
	// Refs are lowercased and openrouter/-prefixed; map marshaling sorts
	// keys, so the content is deterministic.
	ref := testProvider().ModelRef("anthropic/claude-opus-4.6")
	wantCfg := `{"agents":{"defaults":{"model":{"primary":"` + ref + `"},"models":{"` + ref + `":{}}}}}`
	if string(files[0].Contents) != wantCfg {
		t.Errorf("staged contents =\n%s\nwant\n%s", files[0].Contents, wantCfg)
	}
	if files[0].Mode != 0o600 {
		t.Errorf("mode = %v, want 0600 — the file is ours and only the agent we spawn, as this same user, ever reads it", files[0].Mode)
	}
	if strings.Contains(string(files[0].Contents), "sk-or-test") {
		t.Error("API key leaked into the staged file")
	}

	if got, ok := envValue(cmd.Env, "OPENCLAW_CONFIG_PATH"); !ok || got != files[0].Path {
		t.Errorf("OPENCLAW_CONFIG_PATH = %q, %v; want %q", got, ok, files[0].Path)
	}
	if got, ok := envValue(cmd.Env, testProvider().APIKeyEnv); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOpenClawLowercasesModelRef(t *testing.T) {
	stage := t.TempDir()
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	files, err := o.StagedFiles(Request{
		StageDir: stage,
		Model:    catalog.Model{ID: "MoonshotAI/Kimi-K2.5"},
		APIKey:   "k",
	})
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if !strings.Contains(string(files[0].Contents), `"`+testProvider().ModelRef("moonshotai/kimi-k2.5")+`"`) {
		t.Errorf("ref not lowercased: %s", files[0].Contents)
	}
}

func TestOpenClawOneShotSkipsStagingAndInjectsModel(t *testing.T) {
	stage := t.TempDir()
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	req := Request{
		StageDir:  stage,
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"agent", "exec", "say OK"},
	}
	cmd, err := o.Command(req)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// --auth-env-only composes config in memory and shuts off stored auth
	// profiles: nothing read from disk, nothing written, env key only.
	want := []string{"agent", "exec", "say OK", "--model", testProvider().ModelRef("anthropic/claude-opus-4.6"), "--auth-env-only"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if _, ok := envValue(cmd.Env, "OPENCLAW_CONFIG_PATH"); ok {
		t.Error("OPENCLAW_CONFIG_PATH set on the one-shot path; --auth-env-only loads no config")
	}
	files, err := o.StagedFiles(req)
	if err != nil {
		t.Fatalf("StagedFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("one-shot staged %d files, want 0", len(files))
	}
}

func TestOpenClawCommandRejectsConflictsAndOtherSubcommands(t *testing.T) {
	stage := t.TempDir()
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	for _, extras := range [][]string{
		{"--model", "x"}, {"--model=x"}, {"-m", "x"},
		{"gateway", "run"}, {"daemon", "start"}, {"onboard"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras, StageDir: stage}); err == nil {
			t.Errorf("extras %q accepted, want error", extras)
		}
	}
}

func TestOpenClawRequiresAPIKeyAndBinary(t *testing.T) {
	stage := t.TempDir()
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	if _, err := o.Command(Request{Model: testModel(), StageDir: stage}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
	missing := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) { return "", errors.New("no") }}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true with neither openclaw nor clawdbot")
	}
}

func TestOpenClawFallsBackToClawdbotBinary(t *testing.T) {
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: func(name string) (string, error) {
		if name == "clawdbot" {
			return "/usr/local/bin/clawdbot", nil
		}
		return "", errors.New("not found")
	}}
	if !o.CheckInstalled() {
		t.Error("CheckInstalled = false with legacy clawdbot binary present")
	}
}

func TestOpenClawShadowedCredential(t *testing.T) {
	home := testHome(t)
	o := &OpenClaw{Provider: testProvider(), Host: testHost()}

	if msg := o.ShadowedCredential(); msg != "" {
		t.Errorf("fresh HOME: msg = %q, want empty", msg)
	}
	dir := filepath.Join(home, ".openclaw", "agents", "main", "agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "auth-profiles.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"anthropic:default":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := o.ShadowedCredential(); msg != "" {
		t.Errorf("non-openrouter profile: msg = %q, want empty", msg)
	}
	if err := os.WriteFile(path, []byte(`{"profiles":{"`+testProvider().ID+`:default":{"key":"sk-or-old"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := o.ShadowedCredential(); !strings.Contains(msg, "auth profile") {
		t.Errorf("openrouter profile: msg = %q, want an auth-profile warning", msg)
	}
}

// TestOpenClawRefusesAnEmptyStageDir: a missing staging directory must be an
// error, never a default. filepath.Join("", "openclaw.json") is a relative
// path, so falling back would write into whatever directory the user happened
// to run from — a write outside the five sanctioned sites (Landmine 6),
// reached by omission rather than by intent.
func TestOpenClawRefusesAnEmptyStageDir(t *testing.T) {
	o := &OpenClaw{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/openclaw")}
	req := Request{Model: testModel(), APIKey: "sk-or-test"} // no StageDir
	if _, err := o.Command(req); err == nil {
		t.Error("Command accepted a request with no staging directory")
	}
	if _, err := o.StagedFiles(req); err == nil {
		t.Error("StagedFiles accepted a request with no staging directory")
	}
}
