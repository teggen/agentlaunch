package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// Claude launches Claude Code against a model on the bound provider.
type Claude struct {
	// Provider is the endpoint Claude Code is pointed at. Required, with no
	// fallback: a launcher whose provider was never wired would otherwise
	// reach a default while its tests agreed it was configured correctly.
	Provider Provider
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Claude) Name() string        { return "claude" }
func (c *Claude) DisplayName() string { return "Claude Code" }

func (c *Claude) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// findPath resolves the claude binary, falling back to the locations the
// official installer uses when it is not on PATH.
func (c *Claude) findPath() (string, error) {
	if p, err := c.lookPath("claude"); err == nil {
		return p, nil
	}

	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("claude binary not found: %w", err)
	}
	for _, candidate := range []string{
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(home, ".claude", "local", name),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("claude binary not found")
}

// Command builds the Claude Code invocation. It is pure: nothing is written
// and no process is started.
func (c *Claude) Command(req Request) (Command, error) {
	// Claude Code speaks the Anthropic Messages protocol and nothing else, so
	// a provider exposing only an OpenAI-compatible surface cannot host it at
	// all — reaching one needs a translating proxy in front, which is then
	// the provider as far as this launcher is concerned.
	if c.Provider.AnthropicBaseURL == "" {
		return Command{}, fmt.Errorf("claude: %s exposes no Anthropic-compatible endpoint",
			c.Provider.DisplayName)
	}
	key, err := c.Provider.Credential("claude", req.APIKey)
	if err != nil {
		return Command{}, err
	}

	// Claude Code's own --model wins on argv, and it would win only there:
	// the ANTHROPIC_DEFAULT_*_MODEL and CLAUDE_CODE_SUBAGENT_MODEL vars below
	// still carry the managed model, so a passthrough --model splits the
	// session between two models while the tool reports one. Landmine 3's
	// failure class, on argv — same reason the other ten launchers reject it.
	if err := rejectModelFlag(c.Host, "claude", req.ExtraArgs); err != nil {
		return Command{}, err
	}

	path, err := c.findPath()
	if err != nil {
		return Command{}, err
	}

	model := req.Model.ID
	args := []string{"--model", model}
	args = append(args, req.ExtraArgs...)

	// Exactly one of the two credential slots carries the value; the other is
	// present but empty. Which one holds it depends on the provider, and BOTH
	// being empty is the failure Landmine 2 describes: with neither set,
	// Claude Code falls back to authenticating against Anthropic directly, so
	// the launch quietly runs on the user's own Anthropic account while
	// reporting success.
	//
	// A provider issuing real keys takes the x-api-key slot. A provider that
	// needs no user key takes the bearer slot instead, carrying its
	// placeholder — which is how the upstream ollama integration points
	// Claude Code at a local server, and why Provider.Validate refuses a
	// keyless provider with an empty placeholder.
	apiKey, authToken := key, ""
	if !c.Provider.RequiresAPIKey {
		apiKey, authToken = "", key
	}
	env := []string{
		"ANTHROPIC_BASE_URL=" + c.Provider.AnthropicBaseURL,
		"ANTHROPIC_API_KEY=" + apiKey,
		"ANTHROPIC_AUTH_TOKEN=" + authToken,
		"ANTHROPIC_DEFAULT_FABLE_MODEL=" + model,
		"ANTHROPIC_DEFAULT_OPUS_MODEL=" + model,
		"ANTHROPIC_DEFAULT_SONNET_MODEL=" + model,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=" + model,
		"CLAUDE_CODE_SUBAGENT_MODEL=" + model,
	}

	return Command{Path: path, Args: args, Env: env}, nil
}

// CheckInstalled reports whether the claude binary can be found.
func (c *Claude) CheckInstalled() bool {
	_, err := c.findPath()
	return err == nil
}

// InstallHint tells the user how to install Claude Code.
func (c *Claude) InstallHint() string {
	return "Install Claude Code: https://code.claude.com/docs/en/quickstart"
}

// CheckModel warns when pairing Claude Code with a non-Anthropic model.
// OpenRouter documents that Claude Code may fail on context-management
// features with other providers, but it does work for many, so this is
// advisory rather than fatal.
//
// An empty Provider means the catalog does not express a vendor namespace at
// all — a locally served model is just "qwen3-coder:30b" — and an unknown
// vendor is not evidence of incompatibility. Warning there would fire on
// every model the catalog offers, which is the same "advisory that is always
// on" that hermes's context floor avoids by ignoring an unknown length.
func (c *Claude) CheckModel(m catalog.Model) error {
	if m.Provider == "" || strings.EqualFold(m.Provider, "anthropic") {
		return nil
	}
	return fmt.Errorf("%w: Claude Code is optimized for anthropic/* models and may fail on context-management features with %s", ErrIncompatibleModel, m.ID)
}
