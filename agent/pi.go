package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Pi launches the pi coding agent (earendil-works/pi) against an OpenRouter
// model. OpenRouter is a built-in pi provider with the base URL baked in
// upstream, so the launch is two flags plus one env var; nothing is written.
// Doc-verified on pi 0.84.1 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/pi.md.
type Pi struct {
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (p *Pi) Name() string        { return "pi" }
func (p *Pi) DisplayName() string { return "Pi" }

func (p *Pi) lookPath(file string) (string, error) {
	if p.LookPath != nil {
		return p.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the pi binary, falling back to the install script's
// ~/.local/bin location, which is not reliably on PATH.
func (p *Pi) findPath() (string, error) {
	if path, err := p.lookPath("pi"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pi binary not found: %w", err)
	}
	candidate := filepath.Join(home, ".local", "bin", "pi")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("pi binary not found")
}

// Command builds the pi invocation. Pure: nothing written, nothing spawned.
// The slug passes through verbatim — pi's catalog keys models by bare
// OpenRouter slugs; the provider is selected by --provider, never by an
// "openrouter/" prefix on the model.
func (p *Pi) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("pi: an OpenRouter API key is required")
	}
	if err := rejectModelFlag(p.Host, "pi", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags(p.Host, "pi", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	path, err := p.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--provider", "openrouter", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the pi binary can be found.
func (p *Pi) CheckInstalled() bool {
	_, err := p.findPath()
	return err == nil
}

// InstallHint tells the user how to install pi. The legacy
// @mariozechner/pi-coding-agent npm package is deprecated; install only the
// earendil-works one.
func (p *Pi) InstallHint() string {
	return "Install pi: npm install -g --ignore-scripts @earendil-works/pi-coding-agent"
}

// ShadowedCredential reports pi's documented precedence trap: a credential
// in ~/.pi/agent/auth.json (e.g. from "/login openrouter") outranks the
// OPENROUTER_API_KEY env var, so the session would bill that stored account
// instead of the key this launch provides.
func (p *Pi) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "auth.json"))
	if err != nil {
		return ""
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return ""
	}
	if _, ok := store["openrouter"]; !ok {
		return ""
	}
	return "pi has a stored OpenRouter credential (~/.pi/agent/auth.json) that outranks the key this launch provides"
}
