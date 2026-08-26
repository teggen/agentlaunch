package agent

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestOpenCodeCommandPathArgsEnv(t *testing.T) {
	o := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/opencode")}
	cmd, err := o.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"run", "hello"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/local/bin/opencode" {
		t.Errorf("Path = %q", cmd.Path)
	}
	if want := []string{"run", "hello"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	// The model reference is provider/model where the model id itself
	// contains a slash; opencode splits on the FIRST slash.
	wantCfg := `{"$schema":"https://opencode.ai/config.json","model":"` + testProvider().ModelRef("anthropic/claude-opus-4.6") + `"}`
	if got, ok := envValue(cmd.Env, "OPENCODE_CONFIG_CONTENT"); !ok || got != wantCfg {
		t.Errorf("OPENCODE_CONFIG_CONTENT = %q, want %q", got, wantCfg)
	}
	if got, ok := envValue(cmd.Env, testProvider().APIKeyEnv); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

func TestOpenCodeCommandRequiresAPIKey(t *testing.T) {
	o := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/opencode")}
	if _, err := o.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestOpenCodeCommandRejectsModelExtras(t *testing.T) {
	o := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/opencode")}
	for _, extras := range [][]string{
		{"-m", "x/y"},
		{"--model", "x/y"},
		{"--model=x/y"},
		{"-mx/y"},
	} {
		if _, err := o.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// Landmine 8: the fallback path is under HOME, and a real opencode install
// must not make the "absent" cases pass or fail by accident.
func TestOpenCodeFindPathFallback(t *testing.T) {
	testHome(t)
	notOnPath := func(string) (string, error) { return "", errors.New("not on PATH") }

	missing := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: notOnPath}
	if _, err := missing.Command(Request{Model: testModel(), APIKey: "k"}); err == nil {
		t.Fatal("Command found a binary in an empty HOME, want error")
	}
	if missing.CheckInstalled() {
		t.Error("CheckInstalled = true in an empty HOME")
	}
}

func TestOpenCodeInstallable(t *testing.T) {
	installed := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/opencode")}
	if !installed.CheckInstalled() {
		t.Error("CheckInstalled = false with binary present")
	}
	o := &OpenCode{Provider: testProvider(), Host: testHost()}
	if hint := o.InstallHint(); !strings.Contains(hint, "https://opencode.ai/install") {
		t.Errorf("InstallHint = %q", hint)
	}
}

// The curl installer drops the binary at ~/.opencode/bin without reliably
// editing PATH; findPath must look there. Landmine 8 discipline: build the
// fixture inside a temp HOME so the machine's real install state is invisible.
func TestOpenCodeFindPathUsesInstallerLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture writes a Unix-named binary; findPath looks for opencode.exe on Windows")
	}

	home := testHome(t)
	dir := filepath.Join(home, ".opencode", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := &OpenCode{Provider: testProvider(), Host: testHost(), LookPath: func(string) (string, error) { return "", errors.New("not on PATH") }}
	if !o.CheckInstalled() {
		t.Fatal("CheckInstalled = false with binary at ~/.opencode/bin")
	}
}
