package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// hermesMinContext is the context floor hermes enforces at startup: models
// under 64K tokens are refused by hermes itself, so warn before launching.
const hermesMinContext = 65536

// Hermes launches Nous Research's Hermes Agent CLI against an OpenRouter
// model. OpenRouter is a first-class hermes provider; --provider/--model on
// the chat subcommand are vendor-documented per-run overrides with "no
// mutation to ~/.hermes/config.yaml", and CLI args outrank all config files.
// Doc-verified on v0.20.0 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/hermes.md.
type Hermes struct {
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (h *Hermes) Name() string        { return "hermes" }
func (h *Hermes) DisplayName() string { return "Hermes Agent" }

func (h *Hermes) lookPath(file string) (string, error) {
	if h.LookPath != nil {
		return h.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the hermes binary: PATH, then the installer's
// ~/.local/bin shim, then the native-Windows install location.
func (h *Hermes) findPath() (string, error) {
	if path, err := h.lookPath("hermes"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("hermes binary not found: %w", err)
	}
	candidates := []string{filepath.Join(home, ".local", "bin", "hermes")}
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates,
				filepath.Join(localAppData, "hermes", "hermes-agent", "venv", "Scripts", "hermes.exe"))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("hermes binary not found")
}

// Command builds the hermes invocation. Pure: nothing written, nothing
// spawned. Managed flags ride the chat subcommand, so a passthrough that
// starts with a different subcommand is refused rather than silently
// misconfigured.
func (h *Hermes) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("hermes: an OpenRouter API key is required")
	}
	if err := rejectModelFlag("hermes", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags("hermes", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	if len(req.ExtraArgs) > 0 && !strings.HasPrefix(req.ExtraArgs[0], "-") {
		return Command{}, fmt.Errorf("hermes: passthrough %q looks like a hermes subcommand: openrouter-launch always runs \"hermes chat\"; pass chat flags only", req.ExtraArgs[0])
	}
	path, err := h.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"chat", "--provider", "openrouter", "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{
		"OPENROUTER_API_KEY=" + req.APIKey,
		// Documented hardening pin; hermes's default already matches.
		"OPENROUTER_BASE_URL=" + openrouter.DefaultBaseURL,
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckModel warns (advisory, Landmine 7) for models under hermes's 64K
// context floor. Unknown context stays silent: missing catalog data is not
// evidence of incompatibility.
func (h *Hermes) CheckModel(m openrouter.Model) error {
	if m.ContextLength > 0 && m.ContextLength < hermesMinContext {
		return fmt.Errorf("hermes refuses models with less than a 64K context window at startup (%s has %d tokens): %w",
			m.ID, m.ContextLength, ErrIncompatibleModel)
	}
	return nil
}

// CheckInstalled reports whether the hermes binary can be found.
func (h *Hermes) CheckInstalled() bool {
	_, err := h.findPath()
	return err == nil
}

// InstallHint tells the user how to install Hermes. Printed, never run.
func (h *Hermes) InstallHint() string {
	return "Install Hermes: curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash"
}

// ShadowedCredential reports stored hermes credentials that can outrank or
// rotate past the key this launch provides: an OPENROUTER_API_KEY line in
// ~/.hermes/.env, or an OpenRouter credential pool in ~/.hermes/auth.json.
func (h *Hermes) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if hermesEnvHasOpenRouterKey(filepath.Join(home, ".hermes", ".env")) {
		return "hermes has an OPENROUTER_API_KEY saved in ~/.hermes/.env that may override the key this launch provides"
	}
	if hermesAuthHasOpenRouter(filepath.Join(home, ".hermes", "auth.json")) {
		return "hermes has stored OpenRouter credentials (~/.hermes/auth.json) that may rotate past the key this launch provides"
	}
	return ""
}

func hermesEnvHasOpenRouterKey(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if v, ok := strings.CutPrefix(line, "OPENROUTER_API_KEY="); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func hermesAuthHasOpenRouter(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return false
	}
	_, ok := store["openrouter"]
	return ok
}
