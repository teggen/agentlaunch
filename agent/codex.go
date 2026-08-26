package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// Codex launches the OpenAI Codex CLI against an OpenRouter model. All
// configuration travels as -c overrides on the command line; nothing is
// written into ~/.codex.
type Codex struct {
	// Provider is the endpoint codex is pointed at. Required, with no
	// fallback — see the note on Claude.Provider.
	Provider Provider
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Codex) Name() string        { return "codex" }
func (c *Codex) DisplayName() string { return "Codex CLI" }

func (c *Codex) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// Command builds the codex invocation. It is pure: nothing is written and no
// process is started. Managed overrides come before passthrough args so they
// apply even when the passthrough starts with a subcommand; conflicting
// passthrough is rejected because a later -c with the same key would win and
// silently point codex somewhere else.
func (c *Codex) Command(req Request) (Command, error) {
	if c.Provider.BaseURL == "" {
		return Command{}, fmt.Errorf("codex: %s exposes no OpenAI-compatible endpoint",
			c.Provider.DisplayName)
	}
	key, err := c.Provider.Credential("codex", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := codexValidateExtraArgs(c.Host, req.ExtraArgs); err != nil {
		return Command{}, err
	}
	path, err := c.lookPath("codex")
	if err != nil {
		return Command{}, fmt.Errorf("codex binary not found: %w", err)
	}

	// The provider ID is an arbitrary key here rather than a name codex has
	// to already know: model_providers.<key> declares a provider inline, so
	// any OpenAI-compatible endpoint can be reached this way. Upstream
	// ollama's launcher uses its own tool name as the key for the same
	// reason.
	//
	// wire_api comes from the provider because it is a fact about the
	// endpoint, not about codex. Read Landmine 18 before changing the value
	// OpenRouter carries: "chat" is rejected at config-load time by codex
	// >= 0.146.1, and "responses" is live-verified rather than inferred.
	prefix := "model_providers." + c.Provider.ID + "."
	args := []string{
		"-c", `model_provider="` + c.Provider.ID + `"`,
		"-c", prefix + `name="` + c.Provider.DisplayName + `"`,
		"-c", prefix + `base_url="` + c.Provider.BaseURL + `"`,
		"-c", prefix + `env_key="` + c.Provider.APIKeyEnv + `"`,
		"-c", prefix + `wire_api="` + c.Provider.WireAPI + `"`,
		"-m", req.Model.ID,
	}
	args = append(args, req.ExtraArgs...)

	// env_key makes codex read the key from this variable; setting it here
	// (rather than relying on the user's shell) means ExecArgs' dedupe
	// guarantees our value wins over any stray export.
	env := []string{c.Provider.EnvEntry(key)}

	return Command{Path: path, Args: args, Env: env}, nil
}

// codexValidateExtraArgs rejects passthrough that would defeat the managed
// provider config. Later -c overrides win in codex, so silently accepting
// these would let a user flag beat ours while the tool reports success.
func codexValidateExtraArgs(host Host, args []string) error {
	if err := rejectModelFlag(host, "codex", args); err != nil {
		return err
	}
	for i, arg := range args {
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) && codexOverrideConflicts(args[i+1]) {
				return fmt.Errorf("codex: conflicting override %s: %s manages the model provider", args[i+1], host.Name)
			}
		case strings.HasPrefix(arg, "-c") && len(arg) > len("-c"):
			if codexOverrideConflicts(strings.TrimPrefix(arg, "-c")) {
				return fmt.Errorf("codex: conflicting override %s: %s manages the model provider", arg, host.Name)
			}
		case strings.HasPrefix(arg, "--config="):
			if codexOverrideConflicts(strings.TrimPrefix(arg, "--config=")) {
				return fmt.Errorf("codex: conflicting override %s: %s manages the model provider", arg, host.Name)
			}
		}
	}
	return nil
}

func codexOverrideConflicts(override string) bool {
	key, _, ok := strings.Cut(strings.TrimSpace(override), "=")
	if !ok {
		return false
	}
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	return key == "model" || key == "model_provider" ||
		strings.HasPrefix(key, "model_providers.")
}

// CheckInstalled reports whether the codex binary can be found. npm global
// installs land on PATH, so there is no home-dir fallback.
func (c *Codex) CheckInstalled() bool {
	_, err := c.lookPath("codex")
	return err == nil
}

// InstallHint tells the user how to install Codex.
func (c *Codex) InstallHint() string {
	return "Install Codex: npm install -g @openai/codex"
}
