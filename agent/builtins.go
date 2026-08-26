package agent

// Builtins returns the agent definitions this package ships, in display
// order. A tool composes its registry from them —
// MustRegistry(binding, Builtins()) for all of them, or a filtered slice —
// which is what lets a second tool reuse eleven launch recipes rather than
// rewrite them against its own provider.
//
// The three desktop apps are definitions like any other; they simply report
// that no provider can be injected into them. Their New never inspects the
// binding, which is the honest statement: the refusal is a fact about the
// app, not about where it would have been pointed.
func Builtins() []Definition {
	return []Definition{
		{
			Name:        "claude",
			DisplayName: "Claude Code",
			Aliases:     []string{"claude-code", "cc"},
			Description: "Anthropic's coding tool with subagents",
			New: func(b Binding) (Launcher, error) {
				return &Claude{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "codex",
			DisplayName: "Codex CLI",
			Description: "OpenAI's Codex CLI",
			New: func(b Binding) (Launcher, error) {
				return &Codex{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "opencode",
			DisplayName: "OpenCode",
			Description: "Open-source terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &OpenCode{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "pi",
			DisplayName: "Pi",
			Description: "Minimal extensible terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &Pi{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "hermes",
			DisplayName: "Hermes Agent",
			Description: "Nous Research's terminal agent",
			New: func(b Binding) (Launcher, error) {
				return &Hermes{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "qwen",
			DisplayName: "Qwen Code",
			Description: "Qwen's terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &Qwen{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "cline",
			DisplayName: "Cline CLI",
			Description: "Cline's terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &Cline{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "kimi",
			DisplayName: "Kimi Code CLI",
			Description: "Moonshot AI's Kimi Code CLI",
			New: func(b Binding) (Launcher, error) {
				return &Kimi{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "omp",
			DisplayName: "Oh My Pi",
			Description: "Oh My Pi terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &OMP{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "openclaw",
			DisplayName: "OpenClaw",
			Description: "Personal AI assistant with a terminal session",
			New: func(b Binding) (Launcher, error) {
				return &OpenClaw{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		// droid's description is kept terse on purpose. It used to carry
		// "(session-scoped managed config; Factory account required)", which
		// made it the widest DESCRIPTION cell by a wide margin — and `agents`
		// renders through tabwriter, so one long cell stretches the table.
		// Both caveats are surfaced by the launcher itself when droid is
		// actually run.
		{
			Name:        "droid",
			DisplayName: "Factory Droid",
			Description: "Factory's terminal coding agent",
			New: func(b Binding) (Launcher, error) {
				return &Droid{Provider: b.Provider, Host: b.Host, LookPath: b.LookPath}, nil
			},
		},
		{
			Name:        "chatgpt",
			DisplayName: "ChatGPT / Codex app",
			Description: "OpenAI's desktop app",
			New:         func(Binding) (Launcher, error) { return nil, errDesktopApp },
		},
		{
			Name:        "claude-desktop",
			DisplayName: "Claude Desktop",
			Description: "Anthropic's desktop app",
			New:         func(Binding) (Launcher, error) { return nil, errDesktopApp },
		},
		{
			Name:        "hermes-desktop",
			DisplayName: "Hermes Desktop",
			Description: "Nous Research's desktop app",
			New:         func(Binding) (Launcher, error) { return nil, errDesktopApp },
		},
	}
}

// errDesktopApp is the reason all three desktop apps report. It is one value
// rather than three identical literals so the three cannot drift apart; the
// string is user-facing and is rendered verbatim by `agents --all`.
var errDesktopApp = Unsupported(
	"desktop app authenticates through its own account; a launcher cannot inject a provider")
