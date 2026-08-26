package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenCode launches opencode against an OpenRouter model. The entire config
// travels inline in OPENCODE_CONFIG_CONTENT; opencode's native openrouter
// provider reads the provider's key variable. Nothing is written to disk — in
// particular not opencode's model-state file, which ollama's integration
// edits and we deliberately do not.
type OpenCode struct {
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

func (o *OpenCode) Name() string        { return "opencode" }
func (o *OpenCode) DisplayName() string { return "OpenCode" }

func (o *OpenCode) lookPath(file string) (string, error) {
	if o.LookPath != nil {
		return o.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the opencode binary, falling back to the curl
// installer's location, which is not reliably added to PATH.
func (o *OpenCode) findPath() (string, error) {
	if p, err := o.lookPath("opencode"); err == nil {
		return p, nil
	}
	name := "opencode"
	if runtime.GOOS == "windows" {
		name = "opencode.exe"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("opencode binary not found: %w", err)
	}
	candidate := filepath.Join(home, ".opencode", "bin", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", fmt.Errorf("opencode binary not found")
}

// opencodeConfig is the inline JSON for OPENCODE_CONFIG_CONTENT. The model
// reference is "openrouter/<slug>"; opencode splits provider from model on
// the first slash, so the slug's own slash survives.
type opencodeConfig struct {
	Schema string `json:"$schema"`
	Model  string `json:"model"`
}

// Command builds the opencode invocation. It is pure: nothing is written and
// no process is started.
func (o *OpenCode) Command(req Request) (Command, error) {
	key, err := o.Provider.Credential("opencode", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := rejectModelFlag(o.Host, "opencode", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := o.findPath()
	if err != nil {
		return Command{}, err
	}

	cfg, err := json.Marshal(opencodeConfig{
		Schema: "https://opencode.ai/config.json",
		Model:  o.Provider.ModelRef(req.Model.ID),
	})
	if err != nil {
		return Command{}, fmt.Errorf("opencode: building inline config: %w", err)
	}

	env := []string{
		"OPENCODE_CONFIG_CONTENT=" + string(cfg),
		o.Provider.EnvEntry(key),
	}
	return Command{Path: path, Args: append([]string(nil), req.ExtraArgs...), Env: env}, nil
}

// CheckInstalled reports whether the opencode binary can be found.
func (o *OpenCode) CheckInstalled() bool {
	_, err := o.findPath()
	return err == nil
}

// InstallHint tells the user how to install OpenCode. Printed, never run.
func (o *OpenCode) InstallHint() string {
	return "Install OpenCode: curl -fsSL https://opencode.ai/install | bash (or: npm install -g opencode-ai)"
}
