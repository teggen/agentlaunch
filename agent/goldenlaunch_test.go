package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// This file is the other half of the provider parameterization. Every other
// launcher test runs against a synthetic provider, which is what proves a
// launcher reads its Provider field instead of a constant — but a launcher
// could read the field correctly and still be wired to the WRONG OpenRouter
// value, and no synthetic-provider test can see that. So this one pins the
// real thing: the exact argv and environment each agent is launched with
// against the registry's own OpenRouter provider.
//
// The values below were captured from the launchers as they stood before the
// provider descriptor existed and verified byte-for-byte against them, so a
// failure here means an observable change to what a user's agent receives —
// which is a major version bump by this project's own semver contract, not a
// refactor. Neither half of the pair is sufficient alone.

func openclawPath(t *testing.T) string {
	t.Helper()
	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir: %v", err)
	}
	return filepath.Join(dir, "openclaw.json")
}

func TestOpenRouterLaunchSurfaceIsUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake binaries this plants are not executable on Windows")
	}
	// Landmine 8: real installs exist on this machine, and findPath has
	// home-directory fallbacks, so both HOME and PATH have to be ours.
	home := testHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	bin := t.TempDir()
	for _, n := range []string{"claude", "codex", "opencode", "pi", "hermes",
		"qwen", "cline", "kimi", "omp", "openclaw", "droid"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	req := Request{
		Model: openrouter.Model{
			ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, SupportsTools: true, Provider: "anthropic",
		},
		APIKey: "sk-or-test",
	}

	golden := []struct {
		name   string
		args   []string
		env    map[string]string
		staged string
	}{
		{
			name: "claude",
			args: []string{"--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"ANTHROPIC_API_KEY":              "sk-or-test",
				"ANTHROPIC_AUTH_TOKEN":           "",
				"ANTHROPIC_BASE_URL":             "https://openrouter.ai/api",
				"ANTHROPIC_DEFAULT_FABLE_MODEL":  "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_OPUS_MODEL":   "anthropic/claude-opus-4.6",
				"ANTHROPIC_DEFAULT_SONNET_MODEL": "anthropic/claude-opus-4.6",
				"CLAUDE_CODE_SUBAGENT_MODEL":     "anthropic/claude-opus-4.6",
			},
		},
		{
			name: "codex",
			args: []string{"-c", "model_provider=\"openrouter\"", "-c", "model_providers.openrouter.name=\"OpenRouter\"", "-c", "model_providers.openrouter.base_url=\"https://openrouter.ai/api/v1\"", "-c", "model_providers.openrouter.env_key=\"OPENROUTER_API_KEY\"", "-c", "model_providers.openrouter.wire_api=\"responses\"", "-m", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "opencode",
			args: []string{},
			env: map[string]string{
				"OPENCODE_CONFIG_CONTENT": "{\"$schema\":\"https://opencode.ai/config.json\",\"model\":\"openrouter/anthropic/claude-opus-4.6\"}",
				"OPENROUTER_API_KEY":      "sk-or-test",
			},
		},
		{
			name: "pi",
			args: []string{"--provider", "openrouter", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "hermes",
			args: []string{"chat", "--provider", "openrouter", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY":  "sk-or-test",
				"OPENROUTER_BASE_URL": "https://openrouter.ai/api/v1",
			},
		},
		{
			name: "qwen",
			args: []string{"--auth-type", "openai", "--model", "anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENAI_API_KEY":     "sk-or-test",
				"OPENAI_BASE_URL":    "https://openrouter.ai/api/v1",
				"OPENAI_MODEL":       "anthropic/claude-opus-4.6",
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "cline",
			args: []string{"-P", "openrouter", "-m", "anthropic/claude-opus-4.6", "-k", "sk-or-test"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "kimi",
			args: []string{},
			env: map[string]string{
				"KIMI_MODEL_API_KEY":          "sk-or-test",
				"KIMI_MODEL_BASE_URL":         "https://openrouter.ai/api/v1",
				"KIMI_MODEL_MAX_CONTEXT_SIZE": "200000",
				"KIMI_MODEL_NAME":             "anthropic/claude-opus-4.6",
				"KIMI_MODEL_PROVIDER_TYPE":    "openai",
			},
		},
		{
			name: "omp",
			args: []string{"--model", "openrouter/anthropic/claude-opus-4.6"},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		},
		{
			name: "openclaw",
			args: []string{"tui", "--local"},
			env: map[string]string{
				"OPENCLAW_CONFIG_PATH": openclawPath(t),
				"OPENROUTER_API_KEY":   "sk-or-test",
			},
			staged: "{\"agents\":{\"defaults\":{\"model\":{\"primary\":\"openrouter/anthropic/claude-opus-4.6\"},\"models\":{\"openrouter/anthropic/claude-opus-4.6\":{}}}}}",
		},
		{
			name: "droid",
			args: []string{},
			env: map[string]string{
				"OPENROUTER_API_KEY": "sk-or-test",
			},
		}}

	if want := len(golden); want != len(List())-3 {
		t.Fatalf("golden covers %d launchers, registry has %d supported entries",
			want, len(List())-3)
	}

	for _, g := range golden {
		spec, err := Lookup(g.name)
		if err != nil {
			t.Errorf("%s: %v", g.name, err)
			continue
		}
		cmd, err := spec.Launcher.Command(req)
		if err != nil {
			t.Errorf("%s: Command: %v", g.name, err)
			continue
		}
		if !slices.Equal(cmd.Args, g.args) {
			t.Errorf("%s: Args =\n  %q\nwant\n  %q", g.name, cmd.Args, g.args)
		}
		// The env block IS the product: assert the exact set, so a stray or
		// dropped variable fails rather than passing unnoticed.
		if len(cmd.Env) != len(g.env) {
			t.Errorf("%s: len(Env) = %d, want %d (exact set): %q",
				g.name, len(cmd.Env), len(g.env), cmd.Env)
		}
		for k, want := range g.env {
			got, ok := envValue(cmd.Env, k)
			if !ok {
				t.Errorf("%s: %s not set", g.name, k)
				continue
			}
			if got != want {
				t.Errorf("%s: %s = %q, want %q", g.name, k, got, want)
			}
		}
		if g.staged != "" {
			st, ok := spec.Launcher.(Staged)
			if !ok {
				t.Errorf("%s: expected a Staged launcher", g.name)
				continue
			}
			files, err := st.StagedFiles(req)
			if err != nil || len(files) != 1 {
				t.Errorf("%s: StagedFiles = %d files, %v", g.name, len(files), err)
				continue
			}
			if string(files[0].Contents) != g.staged {
				t.Errorf("%s: staged contents =\n  %s\nwant\n  %s",
					g.name, files[0].Contents, g.staged)
			}
		}
	}
}
