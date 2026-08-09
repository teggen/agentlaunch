package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Qwen launches Qwen Code against an OpenRouter model through its generic
// OpenAI-protocol auth: OPENAI_BASE_URL/OPENAI_API_KEY/OPENAI_MODEL env vars
// plus the MANDATORY --auth-type openai flag. Without the flag, qwen-code
// resolves auth from the user's persisted settings or its qwen-oauth
// default, both of which silently ignore every OPENAI_* env var (upstream
// issue #891) — the launch would look configured and run against the wrong
// backend. Doc-verified on 0.21.8 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/qwen.md.
type Qwen struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (q *Qwen) Name() string        { return "qwen" }
func (q *Qwen) DisplayName() string { return "Qwen Code" }

func (q *Qwen) lookPath(file string) (string, error) {
	if q.LookPath != nil {
		return q.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the qwen binary: PATH, then npm-global and installer
// locations, then nvm's per-version bins (highest version wins).
func (q *Qwen) findPath() (string, error) {
	if path, err := q.lookPath("qwen"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("qwen binary not found: %w", err)
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				candidates = append(candidates,
					filepath.Join(base, "npm", "qwen.cmd"),
					filepath.Join(base, "npm", "qwen.exe"))
			}
		}
	} else {
		candidates = append(candidates,
			filepath.Join(home, ".npm-global", "bin", "qwen"),
			filepath.Join(home, ".local", "bin", "qwen"))
		if nvm, err := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "qwen")); err == nil {
			sort.Strings(nvm) // ascending: the last entry is the highest version
			for i := len(nvm) - 1; i >= 0; i-- {
				candidates = append(candidates, nvm[i])
			}
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("qwen binary not found")
}

// Command builds the qwen invocation. Pure: nothing written, nothing
// spawned. Both OPENAI_API_KEY and OPENROUTER_API_KEY carry the key: the
// generic openai auth path reads the former, qwen-code's dedicated
// OpenRouter recipe documents the latter.
func (q *Qwen) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("qwen: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("qwen", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("qwen", req.ExtraArgs, "--auth-type", "--openai-api-key", "--openai-base-url"); err != nil {
		return Command{}, err
	}
	path, err := q.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"--auth-type", "openai", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{
		"OPENAI_BASE_URL=" + openrouter.DefaultBaseURL,
		"OPENAI_API_KEY=" + req.APIKey,
		"OPENROUTER_API_KEY=" + req.APIKey,
		"OPENAI_MODEL=" + req.Model.ID,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the qwen binary can be found.
func (q *Qwen) CheckInstalled() bool {
	_, err := q.findPath()
	return err == nil
}

// InstallHint tells the user how to install Qwen Code. Printed, never run.
func (q *Qwen) InstallHint() string {
	return "Install Qwen Code: npm install -g @qwen-code/qwen-code@latest"
}
