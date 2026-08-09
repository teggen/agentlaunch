package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/teggen/openrouter-launch/internal/config"
)

// OpenClaw launches OpenClaw against an OpenRouter model. Interactive
// sessions run `openclaw tui --local` — the embedded runtime, no gateway or
// daemon — with the model in a LAUNCHER-OWNED config file passed via
// OPENCLAW_CONFIG_PATH (openclaw's tui has no model flag or env var). That
// file is write site #3 of the amended Landmine 6; it lives under OUR
// config dir, holds only the model ref, and replaces the user's own
// openclaw config for the session (owner-approved at spec review — a
// launched session deliberately does not load their channels/plugins).
// One-shot `agent exec` passthrough needs no file: --model plus
// --auth-env-only compose config in memory. Doc-verified on 2026.7.1-2
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/openclaw.md.
type OpenClaw struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (o *OpenClaw) Name() string        { return "openclaw" }
func (o *OpenClaw) DisplayName() string { return "OpenClaw" }

func (o *OpenClaw) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the openclaw binary, falling back to the pre-rename
// clawdbot name.
func (o *OpenClaw) findPath() (string, error) {
	if path, err := o.lookPath("openclaw"); err == nil {
		return path, nil
	}
	if path, err := o.lookPath("clawdbot"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("openclaw binary not found")
}

// openclawModelRef converts an OpenRouter slug to openclaw's model ref:
// provider-prefixed and lowercased (openclaw normalizes refs to lowercase).
func openclawModelRef(slug string) string {
	return "openrouter/" + strings.ToLower(slug)
}

// openclawConfigPath is the launcher-owned staged config location.
func openclawConfigPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "openclaw.json"), nil
}

// openclawOneShot reports whether the passthrough invokes openclaw's
// embedded one-shot mode (`agent exec …`), which takes --model directly and
// needs no staged config.
func openclawOneShot(extras []string) bool {
	return len(extras) > 0 && extras[0] == "agent"
}

// Command builds the openclaw invocation. Pure: the staged file is declared
// by StagedFiles and written by the launch service, never here.
func (o *OpenClaw) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("openclaw: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("openclaw", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if len(req.ExtraArgs) > 0 && !strings.HasPrefix(req.ExtraArgs[0], "-") && req.ExtraArgs[0] != "agent" {
		return Command{}, fmt.Errorf("openclaw: passthrough %q is platform administration, not a launch: openrouter-launch runs \"openclaw tui --local\" (or \"agent exec …\" passthrough)", req.ExtraArgs[0])
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}
	ref := openclawModelRef(req.Model.ID)

	if openclawOneShot(req.ExtraArgs) {
		args := append(append([]string(nil), req.ExtraArgs...), "--model", ref, "--auth-env-only")
		return Command{
			Path: path,
			Args: args,
			Env:  []string{"OPENROUTER_API_KEY=" + req.APIKey},
		}, nil
	}

	cfgPath, err := openclawConfigPath()
	if err != nil {
		return Command{}, err
	}
	args := append([]string{"tui", "--local"}, req.ExtraArgs...)
	env := []string{
		"OPENCLAW_CONFIG_PATH=" + cfgPath,
		"OPENROUTER_API_KEY=" + req.APIKey,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// StagedFiles declares the launcher-owned model config for interactive
// launches. Pure: returns data; launch.Service.Launch writes it. No secret
// goes in — the key travels in env only — so the mode is 0644.
func (o *OpenClaw) StagedFiles(req Request) ([]StagedFile, error) {
	if openclawOneShot(req.ExtraArgs) {
		return nil, nil
	}
	ref := openclawModelRef(req.Model.ID)
	cfg := map[string]any{
		"agents": map[string]any{
			"defaults": map[string]any{
				"model":  map[string]any{"primary": ref},
				"models": map[string]any{ref: map[string]any{}},
			},
		},
	}
	contents, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("openclaw: building staged config: %w", err)
	}
	path, err := openclawConfigPath()
	if err != nil {
		return nil, err
	}
	// 0600 even though no secret is inside — the key stays in the env. This
	// file is launcher-owned and its only reader is the openclaw process we
	// spawn as this same user, so nothing needs the broader mode.
	return []StagedFile{{Path: path, Contents: contents, Mode: 0o600}}, nil
}

// CheckInstalled reports whether an openclaw (or legacy clawdbot) binary
// can be found. npm global installs land on PATH.
func (o *OpenClaw) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install OpenClaw. Printed, never run.
func (o *OpenClaw) InstallHint() string {
	return "Install OpenClaw: npm install -g openclaw@latest"
}

// ShadowedCredential reports stored OpenClaw auth profiles for OpenRouter:
// a prior onboard/OAuth stores a key that participates in auth rotation,
// and its precedence against the env key is undocumented — surface it.
func (o *OpenClaw) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(home, ".openclaw", "agents", "*", "agent", "auth-profiles.json"))
	if err != nil {
		return ""
	}
	for _, m := range matches {
		if openclawProfilesHaveOpenRouter(m) {
			return "openclaw has a stored OpenRouter auth profile (" + m + ") that may take precedence over the key this launch provides"
		}
	}
	return ""
}

func openclawProfilesHaveOpenRouter(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc struct {
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	for key := range doc.Profiles {
		if strings.Contains(key, "openrouter") {
			return true
		}
	}
	return false
}
