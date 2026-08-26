package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Kimi launches Moonshot AI's Kimi Code CLI against an OpenRouter model via
// the KIMI_MODEL_* env family, which synthesizes a provider+model in memory
// ("nothing is written back to the config file" — vendor docs) and outranks
// config.toml; only a -m flag beats it, which we never pass and reject in
// passthrough.
//
// Deliberately NOT ported from ollama: its `kimi --config '<json>'` with
// provider type "openai_legacy" targets the deprecated legacy Python
// kimi-cli. Kimi Code CLI has neither the flag nor the type — porting it
// would repeat Landmine 18 (see the Phase 4 spec and
// .superpowers/sdd/2026-08-09-tier-2-research/kimi.md). Doc-verified on
// kimi-code 0.34.0, KIMI_MODEL_* channel present since 0.6.0 (2026-08-09).
type Kimi struct {
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

func (k *Kimi) Name() string        { return "kimi" }
func (k *Kimi) DisplayName() string { return "Kimi Code CLI" }

func (k *Kimi) lookPath(file string) (string, error) {
	if k.LookPath != nil {
		return k.LookPath(file)
	}
	return exec.LookPath(file)
}

func kimiBinary() string {
	if runtime.GOOS == "windows" {
		return "kimi.exe"
	}
	return "kimi"
}

// kimiCodePath is the current CLI's own install location; legacyKimiPaths
// are where the deprecated Python kimi-cli lands. Both generations install
// a binary named "kimi", so the order here IS the disambiguation: Kimi
// Code's dir always wins over uv tool paths.
func kimiCodePath(home string) string {
	return filepath.Join(home, ".kimi-code", "bin", kimiBinary())
}

func legacyKimiPaths(home string) []string {
	return []string{
		filepath.Join(home, ".local", "share", "uv", "tools", "kimi-cli", "bin", "kimi"),
		filepath.Join(home, ".local", "bin", "kimi"),
	}
}

func (k *Kimi) findPath() (string, error) {
	if path, err := k.lookPath("kimi"); err == nil {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("kimi binary not found: %w", err)
	}
	candidates := append([]string{kimiCodePath(home)}, legacyKimiPaths(home)...)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("kimi binary not found")
}

// Command builds the kimi invocation. Pure: nothing written, nothing
// spawned. KIMI_MODEL_MAX_CONTEXT_SIZE comes from the catalog; when the
// catalog does not know the context length, the variable is omitted so
// kimi's documented default applies instead of a fabricated zero.
func (k *Kimi) Command(req Request) (Command, error) {
	if k.Provider.BaseURL == "" {
		return Command{}, fmt.Errorf("kimi: %s exposes no OpenAI-compatible endpoint",
			k.Provider.DisplayName)
	}
	key, err := k.Provider.Credential("kimi", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := rejectModelFlag(k.Host, "kimi", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags(k.Host, "kimi", req.ExtraArgs, "--config", "--config-file"); err != nil {
		return Command{}, err
	}
	path, err := k.findPath()
	if err != nil {
		return Command{}, err
	}
	env := []string{
		"KIMI_MODEL_NAME=" + req.Model.ID,
		"KIMI_MODEL_API_KEY=" + key,
		"KIMI_MODEL_PROVIDER_TYPE=openai",
		"KIMI_MODEL_BASE_URL=" + k.Provider.BaseURL,
	}
	if req.Model.ContextLength > 0 {
		env = append(env, fmt.Sprintf("KIMI_MODEL_MAX_CONTEXT_SIZE=%d", req.Model.ContextLength))
	}
	return Command{Path: path, Args: append([]string(nil), req.ExtraArgs...), Env: env}, nil
}

// CheckInstalled reports whether a kimi binary can be found.
func (k *Kimi) CheckInstalled() bool {
	_, err := k.findPath()
	return err == nil
}

// InstallHint tells the user how to install Kimi Code CLI. Printed, never
// run. Windows additionally needs Git for Windows (kimi uses Git Bash as
// its shell backend).
func (k *Kimi) InstallHint() string {
	return "Install Kimi Code CLI: curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash"
}

// ShadowedCredential flags a legacy-only install: the deprecated Python
// kimi-cli ignores KIMI_MODEL_* entirely, so a launch would silently run on
// the user's Moonshot account instead of OpenRouter. Pure path heuristic —
// executing the binary to ask its version would violate launch purity. A
// PATH hit is trusted (the Kimi Code installer renames legacy shims to
// kimi-legacy); only a uv-tools-dir resolution with no Kimi Code install
// alongside is confidently legacy.
func (k *Kimi) ShadowedCredential() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if _, err := os.Stat(kimiCodePath(home)); err == nil {
		return ""
	}
	path, err := k.findPath()
	if err != nil {
		return ""
	}
	if path == legacyKimiPaths(home)[0] {
		return "the kimi binary at " + path + " looks like the legacy Python kimi-cli, which ignores KIMI_MODEL_* configuration and would run on your Moonshot account; install Kimi Code CLI (https://code.kimi.com)"
	}
	return ""
}
