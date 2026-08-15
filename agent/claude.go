package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// AnthropicBaseURL is what Claude Code is pointed at. OpenRouter's
// Anthropic-compatible surface lives here; Claude Code appends its own
// version segment, so this must NOT end in /v1.
const AnthropicBaseURL = "https://openrouter.ai/api"

// Claude launches Claude Code against an OpenRouter model.
type Claude struct {
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
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("claude: an OpenRouter API key is required")
	}

	// Claude Code's own --model wins on argv, and it would win only there:
	// the ANTHROPIC_DEFAULT_*_MODEL and CLAUDE_CODE_SUBAGENT_MODEL vars below
	// still carry the managed model, so a passthrough --model splits the
	// session between two models while the tool reports one. Landmine 3's
	// failure class, on argv — same reason the other ten launchers reject it.
	if err := rejectModelFlag("claude", req.ExtraArgs); err != nil {
		return Command{}, err
	}

	path, err := c.findPath()
	if err != nil {
		return Command{}, err
	}

	model := req.Model.ID
	args := []string{"--model", model}
	args = append(args, req.ExtraArgs...)

	// ANTHROPIC_AUTH_TOKEN must be present but empty: when unset, Claude Code
	// falls back to its own Anthropic authentication.
	env := []string{
		"ANTHROPIC_BASE_URL=" + AnthropicBaseURL,
		"ANTHROPIC_API_KEY=" + req.APIKey,
		"ANTHROPIC_AUTH_TOKEN=",
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
func (c *Claude) CheckModel(m openrouter.Model) error {
	if strings.EqualFold(m.Provider, "anthropic") {
		return nil
	}
	return fmt.Errorf("%w: Claude Code is optimized for anthropic/* models and may fail on context-management features with %s", ErrIncompatibleModel, m.ID)
}
