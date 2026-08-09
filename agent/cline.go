package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Cline launches the Cline CLI against an OpenRouter model via its native
// builtin openrouter provider (base URL baked in upstream). Nothing is
// written — ollama's integration writes providers.json + globalState.json,
// which the CLI's own -P/-m flags make unnecessary. Note cline's
// --auto-approve defaults to TRUE upstream; per the owner decision recorded
// in the Phase 4 spec, the launcher does not override agent behavior
// defaults. Doc-verified on 3.0.51 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/cline.md.
type Cline struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Cline) Name() string        { return "cline" }
func (c *Cline) DisplayName() string { return "Cline CLI" }

func (c *Cline) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// Command builds the cline invocation. Pure: nothing written, nothing
// spawned. The key travels in env only; passthrough -k stays allowed as the
// user's explicit choice, but the launcher itself never puts a key on argv.
func (c *Cline) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("cline: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("cline", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("cline", req.ExtraArgs, "-P", "--provider"); err != nil {
		return Command{}, err
	}
	path, err := c.lookPath("cline")
	if err != nil {
		return Command{}, fmt.Errorf("cline binary not found: %w", err)
	}
	args := []string{"-P", "openrouter", "-m", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the cline binary can be found. npm global
// installs land on PATH; there is no home-dir fallback.
func (c *Cline) CheckInstalled() bool {
	_, err := c.lookPath("cline")
	return err == nil
}

// InstallHint tells the user how to install the Cline CLI.
func (c *Cline) InstallHint() string {
	return "Install Cline CLI: npm install -g cline"
}

// ShadowedCredential reports cline's documented precedence trap: a saved
// OpenRouter key in ~/.cline/data/settings/providers.json outranks the
// OPENROUTER_API_KEY env var (source: resolveApiKey — saved key → OAuth →
// env), so the session would bill the stored account.
func (c *Cline) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".cline", "data", "settings", "providers.json"))
	if err != nil {
		return ""
	}
	var doc struct {
		Providers map[string]struct {
			APIKey   string `json:"apiKey"`
			Settings struct {
				APIKey string `json:"apiKey"`
			} `json:"settings"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return ""
	}
	entry, ok := doc.Providers["openrouter"]
	if !ok {
		return ""
	}
	if entry.APIKey == "" && entry.Settings.APIKey == "" {
		return ""
	}
	return "cline has a saved OpenRouter key (~/.cline/data/settings/providers.json) that outranks the key this launch provides"
}
