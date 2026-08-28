package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

// Pool launches Poolside's coding agent against a provider-hosted model.
//
// Poolside documents OpenRouter as a supported endpoint by name, and its
// standalone mode is entirely environment-driven: POOLSIDE_STANDALONE_BASE_URL
// is what SELECTS that mode, and the vendor documents the environment as being
// read BEFORE settings.yaml and credentials.json. That precedence is why this
// launcher is zero-touch and why it needs no CredentialShadowCheck — a saved
// Poolside credential cannot outrank what we pass. Live-verified on v1.0.16
// (2026-08-28) against OpenRouter.
//
// The model travels as POOLSIDE_STANDALONE_MODEL rather than on argv.
// Ollama's launcher passes -m and never sets the documented variable; the
// documented one is what was verified here.
type Pool struct {
	// Provider is the endpoint this agent is pointed at. Required, with no
	// fallback — see the note on Claude.Provider.
	Provider Provider
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (p *Pool) Name() string        { return "pool" }
func (p *Pool) DisplayName() string { return "Poolside" }

func (p *Pool) lookPath(file string) (string, error) {
	if p.LookPath != nil {
		return p.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the pool binary: PATH, then the installer's default
// location, which it does not always add to PATH.
func (p *Pool) findPath() (string, error) {
	if path, err := p.lookPath("pool"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("pool binary not found: %w", err)
	}
	candidates := []string{filepath.Join(home, ".local", "bin", "pool")}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "Programs", "pool", "bin", "pool.exe"))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("pool binary not found")
}

// Command builds the pool invocation. Pure: nothing written, nothing spawned.
// Every managed value rides the environment, so the passthrough is forwarded
// untouched apart from the flags we own.
func (p *Pool) Command(req Request) (Command, error) {
	if p.Provider.BaseURL == "" {
		return Command{}, fmt.Errorf("pool: %s exposes no OpenAI-compatible endpoint",
			p.Provider.DisplayName)
	}
	key, err := p.Provider.Credential("pool", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := rejectModelFlag(p.Host, "pool", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := p.findPath()
	if err != nil {
		return Command{}, err
	}

	env := []string{
		"POOLSIDE_STANDALONE_BASE_URL=" + p.Provider.BaseURL,
		"POOLSIDE_API_KEY=" + key,
		"POOLSIDE_STANDALONE_MODEL=" + req.Model.ID,
	}
	// Pool budgets its automatic compaction against this value. Unknown
	// context stays ABSENT rather than zero: the vendor documents a positive
	// integer, and a catalog that does not report a context length is not
	// evidence that the window is nothing.
	if req.Model.ContextLength > 0 {
		env = append(env, "POOLSIDE_STANDALONE_CONTEXT_LENGTH="+strconv.Itoa(req.Model.ContextLength))
	}

	return Command{
		Path: path,
		Args: append([]string(nil), req.ExtraArgs...),
		Env:  env,
	}, nil
}

// CheckInstalled reports whether the pool binary can be found.
func (p *Pool) CheckInstalled() bool {
	_, err := p.findPath()
	return err == nil
}

// InstallHint tells the user how to install pool. Printed, never run — and
// the installer requires accepting Poolside's EULA, which is the user's to
// accept, not ours.
func (p *Pool) InstallHint() string {
	return "Install Poolside: curl -fsSL https://downloads.poolside.ai/pool/install.sh | sh"
}
