package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/agentlaunch/catalog"
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

// rejectBareSubcommand refuses a passthrough whose first element does not
// start with "-": that shape is a hermes subcommand name, and this launcher
// already supplies one (chat, or desktop for HermesDesktop) — accepting it
// would silently misconfigure the run rather than refuse it.
//
// agentName and subcommand are parameters, not something the callee works
// out for itself, because Go embedding gives no virtual dispatch: were this
// guard to call back out to a method on h, HermesDesktop embedding Hermes
// would still reach Hermes's own method, never an override — embedding is
// composition, not inheritance-with-dispatch. So HermesDesktop.Command calls
// this directly, before delegating to Hermes.Command, telling it which
// launcher and subcommand are actually in play rather than letting the
// embedded Hermes guess wrong.
func rejectBareSubcommand(host Host, agentName, subcommand string, extraArgs []string) error {
	if len(extraArgs) > 0 && !strings.HasPrefix(extraArgs[0], "-") {
		return fmt.Errorf("%s: passthrough %q looks like a hermes subcommand: %s always runs \"hermes %s\"; pass %s flags only",
			agentName, extraArgs[0], host.Name, subcommand, subcommand)
	}
	return nil
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
	if err := rejectBareSubcommand(h.Host, "hermes", "chat", req.ExtraArgs); err != nil {
		return Command{}, err
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

// HermesDesktop launches hermes's Electron desktop app against the same
// provider and model the terminal launcher uses.
//
// It is NOT a separate application with its own account — it is the hermes
// binary plus a subcommand, which is why it can be pointed at a provider at
// all where the other desktop apps could not. Everything except the argv
// shape is inherited: the context floor, the credential-shadow check, the
// binary search and the install hint are hermes's, and duplicating them here
// would let them drift.
//
// The provider flags are TOP-LEVEL and must precede the subcommand.
// Live-verified on v0.20.5 (2026-08-28): `hermes --provider X --model Y
// desktop --help` exits 0, and `hermes desktop` itself declares no provider
// or model option, so the flags after the subcommand would be a parse error.
type HermesDesktop struct {
	Hermes
}

func (h *HermesDesktop) Name() string        { return "hermes-desktop" }
func (h *HermesDesktop) DisplayName() string { return "Hermes Desktop" }

// Command builds the desktop invocation by rewriting the terminal one: the
// embedded Hermes owns every guard and the environment, and this replaces
// only the subcommand its args are built around.
//
// The bare-subcommand guard runs here FIRST, named for this launcher, before
// delegating to Hermes.Command — which runs its own copy of the same guard,
// worded for "hermes chat". Checking here first is what makes the
// desktop-worded error the one the user actually sees.
func (h *HermesDesktop) Command(req Request) (Command, error) {
	if err := rejectBareSubcommand(h.Host, "hermes-desktop", "desktop", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	cmd, err := h.Hermes.Command(req)
	if err != nil {
		return Command{}, err
	}
	args := []string{"--provider", h.Provider.ID, "--model", req.Model.ID, "desktop"}
	cmd.Args = append(args, req.ExtraArgs...)
	return cmd, nil
}
