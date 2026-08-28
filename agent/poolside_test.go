package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/agentlaunch/catalog"
)

func poolFixture(t *testing.T) *Pool {
	t.Helper()
	return &Pool{
		Provider: testProvider(),
		Host:     testHost(),
		LookPath: func(string) (string, error) { return "/usr/bin/pool", nil },
	}
}

func TestPoolCommandCarriesEveryManagedValueInTheEnvironment(t *testing.T) {
	p := poolFixture(t)
	cmd, err := p.Command(Request{
		Model:  catalog.Model{ID: "acme/model-1", ContextLength: 128000},
		APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("Command() = %v, want nil", err)
	}
	if cmd.Path != "/usr/bin/pool" {
		t.Errorf("Path = %q, want /usr/bin/pool", cmd.Path)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("Args = %q, want empty: every managed value travels by env", cmd.Args)
	}
	want := []string{
		"POOLSIDE_STANDALONE_BASE_URL=https://api.acme.test/v9",
		"POOLSIDE_API_KEY=sk-test",
		"POOLSIDE_STANDALONE_MODEL=acme/model-1",
		"POOLSIDE_STANDALONE_CONTEXT_LENGTH=128000",
	}
	if !slices.Equal(cmd.Env, want) {
		t.Errorf("Env =\n  %q\nwant\n  %q", cmd.Env, want)
	}
}

// The base URL is what selects pool's standalone mode, so a provider with no
// OpenAI-compatible surface must be refused rather than launched into the
// user's saved Poolside account.
func TestPoolRefusesAProviderWithNoOpenAICompatibleRoot(t *testing.T) {
	p := poolFixture(t)
	p.Provider.BaseURL = ""
	_, err := p.Command(Request{Model: catalog.Model{ID: "acme/model-1"}, APIKey: "sk-test"})
	if err == nil {
		t.Fatal("Command() = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "OpenAI-compatible") {
		t.Errorf("error = %q, want it to name the missing endpoint", err)
	}
}

// Unknown context must be ABSENT, never zero: pool documents the value as a
// positive integer, and a 0 would be a claim we cannot support.
func TestPoolOmitsContextLengthWhenUnknown(t *testing.T) {
	p := poolFixture(t)
	cmd, err := p.Command(Request{
		Model:  catalog.Model{ID: "acme/model-1", ContextLength: 0},
		APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("Command() = %v, want nil", err)
	}
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "POOLSIDE_STANDALONE_CONTEXT_LENGTH=") {
			t.Errorf("Env contains %q, want the variable omitted entirely", e)
		}
	}
}

func TestPoolRejectsManagedPassthroughFlags(t *testing.T) {
	for _, arg := range []string{"-m", "--model"} {
		p := poolFixture(t)
		_, err := p.Command(Request{
			Model:     catalog.Model{ID: "acme/model-1"},
			APIKey:    "sk-test",
			ExtraArgs: []string{arg, "other/model"},
		})
		if err == nil {
			t.Errorf("Command() with %s = nil error, want a rejection", arg)
		}
	}
}

func TestPoolPassesThroughUnmanagedArgs(t *testing.T) {
	p := poolFixture(t)
	cmd, err := p.Command(Request{
		Model:     catalog.Model{ID: "acme/model-1"},
		APIKey:    "sk-test",
		ExtraArgs: []string{"--help"},
	})
	if err != nil {
		t.Fatalf("Command() = %v, want nil", err)
	}
	if !slices.Equal(cmd.Args, []string{"--help"}) {
		t.Errorf("Args = %q, want [--help]", cmd.Args)
	}
}

// findPath must reach the installer's ~/.local/bin, which is not always on
// PATH. testHome is mandatory: pool is genuinely installed on the dev machine.
func TestPoolFindsTheInstallerShimOutsidePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the ~/.local/bin fallback is a Unix path")
	}
	home := testHome(t)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "pool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := poolFixture(t)
	p.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if !p.CheckInstalled() {
		t.Error("CheckInstalled() = false, want true via the ~/.local/bin fallback")
	}
}

func TestPoolReportsAbsentWhenNothingIsInstalled(t *testing.T) {
	testHome(t)
	p := poolFixture(t)
	p.LookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if p.CheckInstalled() {
		t.Error("CheckInstalled() = true, want false")
	}
}
