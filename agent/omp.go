package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// OMP launches Oh My Pi against an OpenRouter model. OpenRouter is a
// built-in omp provider with the base URL baked in upstream; the model
// selector is "openrouter/<slug>" — the prefix IS the provider selection in
// omp's dialect (unlike its ancestor pi, which takes --provider plus a bare
// slug). Nothing is written; ollama's models.yml write existed only because
// Ollama is a custom provider there.
//
// Known, documented, NOT runtime-detected: omp's stored credentials
// (~/.omp/agent/agent.db, sqlite) outrank the env key — "env vars are a
// fallback, not an override". No sqlite dependency for one advisory; the
// caveat lives in the spec and README. Doc-verified on 17.2.11
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/omp.md.
type OMP struct {
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (o *OMP) Name() string        { return "omp" }
func (o *OMP) DisplayName() string { return "Oh My Pi" }

func (o *OMP) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the omp binary: PATH, then the install script's
// ~/.local/bin, then bun's global bin.
func (o *OMP) findPath() (string, error) {
	if path, err := o.lookPath("omp"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("omp binary not found: %w", err)
	}
	for _, c := range []string{
		filepath.Join(home, ".local", "bin", "omp"),
		filepath.Join(home, ".bun", "bin", "omp"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("omp binary not found")
}

// Command builds the omp invocation. Pure: nothing written, nothing
// spawned. Passthrough --api-key stays allowed: it is the user's explicit,
// documented override of omp's stored-credential precedence.
func (o *OMP) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("omp: an OpenRouter API key is required")
	}
	if err := rejectModelFlag(o.Host, "omp", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags(o.Host, "omp", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--model", "openrouter/" + req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{"OPENROUTER_API_KEY=" + req.APIKey}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the omp binary can be found.
func (o *OMP) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install Oh My Pi. Printed, never run.
func (o *OMP) InstallHint() string {
	return "Install Oh My Pi: curl -fsSL https://omp.sh/install | sh"
}
