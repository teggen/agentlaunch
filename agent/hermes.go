package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// hermesMinContext is the context floor hermes enforces at startup: models
// under this many tokens are refused by hermes itself, so warn before
// launching. Live-verified on v0.20.0 (2026-08-09) as 64,000 (decimal),
// not the binary-K 65,536 the doc-verified value assumed: hermes's own
// startup error reads "below the minimum 64,000", and a 65,535-context
// model (microsoft/wizardlm-2-8x22b) passed hermes's context gate and only
// failed later for an unrelated reason (64,000 decimal, not 65,536 —
// live-verified 2026-08-09; see the Phase 4 spec's live-verification
// results).
const hermesMinContext = 64000

// Hermes launches Nous Research's Hermes Agent CLI against an OpenRouter
// model. OpenRouter is a first-class hermes provider; --provider/--model on
// the chat subcommand are vendor-documented per-run overrides with "no
// mutation to ~/.hermes/config.yaml", and CLI args outrank all config files.
// Doc-verified on v0.20.0 (2026-08-09); see
// .superpowers/sdd/2026-08-09-tier-2-research/hermes.md.
type Hermes struct {
	// Provider is the endpoint this agent is pointed at. Required, with no
	// fallback — see the note on Claude.Provider.
	Provider Provider
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
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
	key, err := h.Provider.Credential("hermes", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := rejectModelFlag(h.Host, "hermes", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags(h.Host, "hermes", req.ExtraArgs, "--provider"); err != nil {
		return Command{}, err
	}
	if len(req.ExtraArgs) > 0 && !strings.HasPrefix(req.ExtraArgs[0], "-") {
		return Command{}, fmt.Errorf("hermes: passthrough %q looks like a hermes subcommand: %s always runs \"hermes chat\"; pass chat flags only", req.ExtraArgs[0], h.Host.Name)
	}
	path, err := h.findPath()
	if err != nil {
		return Command{}, err
	}
	args := []string{"chat", "--provider", h.Provider.ID, "--model", req.Model.ID}
	args = append(args, req.ExtraArgs...)
	env := []string{
		h.Provider.EnvEntry(key),
	}
	// Documented hardening pin; hermes derives both variable names from the
	// provider it was told to use, so the pin only exists when we have a
	// base URL to pin it to.
	if h.Provider.BaseURL != "" {
		env = append(env, h.Provider.UpperID()+"_BASE_URL="+h.Provider.BaseURL)
	}
	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckModel warns (advisory, Landmine 7) for models under hermes's context
// floor. Unknown context stays silent: missing catalog data is not evidence
// of incompatibility.
func (h *Hermes) CheckModel(m catalog.Model) error {
	if m.ContextLength > 0 && m.ContextLength < hermesMinContext {
		return fmt.Errorf("hermes refuses models with less than a 64,000-token context window at startup (%s has %d tokens): %w",
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
// rotate past the key this launch provides: a key line for the provider's
// variable in ~/.hermes/.env, or a credential pool for the provider in
// ~/.hermes/auth.json. Both are keyed on the provider, since hermes stores
// one set per provider it knows about.
func (h *Hermes) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if hermesEnvHasProviderKey(filepath.Join(home, ".hermes", ".env"), h.Provider.APIKeyEnv) {
		return "hermes has a " + h.Provider.APIKeyEnv + " saved in ~/.hermes/.env that may override the key this launch provides"
	}
	if hermesAuthHasProvider(filepath.Join(home, ".hermes", "auth.json"), h.Provider.ID) {
		return "hermes has stored " + h.Provider.DisplayName + " credentials (~/.hermes/auth.json) that may rotate past the key this launch provides"
	}
	return ""
}

func hermesEnvHasProviderKey(path, envVar string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if v, ok := strings.CutPrefix(line, envVar+"="); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func hermesAuthHasProvider(path, providerID string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var store map[string]json.RawMessage
	if err := json.Unmarshal(data, &store); err != nil {
		return false
	}
	_, ok := store[providerID]
	return ok
}
