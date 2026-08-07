package agent

import (
	"fmt"
	"os/exec"
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

// findPath resolves the claude binary. Lookup goes exclusively through
// c.lookPath (LookPath when injected, exec.LookPath otherwise) so that
// binary resolution is fully controllable in tests — no side channel to the
// real filesystem exists that could make a test's outcome depend on what
// happens to be installed on the machine running it.
func (c *Claude) findPath() (string, error) {
	p, err := c.lookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude binary not found: %w", err)
	}
	return p, nil
}

// Command builds the Claude Code invocation. It is pure: nothing is written
// and no process is started.
func (c *Claude) Command(req Request) (Command, error) {
	if req.APIKey == "" {
		return Command{}, fmt.Errorf("claude: an OpenRouter API key is required")
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
