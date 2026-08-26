# agentlaunch

Configure a coding agent for an OpenAI-compatible provider and hand off to it.

This is the reusable half of
[openrouter-launch](https://github.com/teggen/openrouter-launch): eleven agent
recipes, the process handoff, and a terminal-free planner, parameterized by a
`Provider` descriptor so one set of recipes serves many tools. It has **no
dependencies outside the standard library**, and a test refuses to let it grow
one.

```go
reg := agent.MustRegistry(agent.Binding{
    Provider: agent.Provider{
        ID:               "acme",
        DisplayName:      "Acme",
        BaseURL:          "https://api.acme.example/v1",
        AnthropicBaseURL: "https://api.acme.example", // no /v1 — see below
        APIKeyEnv:        "ACME_API_KEY",
        RequiresAPIKey:   true,
        ModelPrefix:      "acme/",
        WireAPI:          "responses",
    },
    Host: agent.Host{Name: "acme-launch", Marker: "acme-launch"},
}, agent.Builtins())

svc := &launch.Service{
    Registry:    reg,
    LoadCatalog: yourCatalog,
    APIKey:      yourKeyLookup,
    StageDir:    yourConfigDir,
}
```

## Packages

| Package | What it holds |
|---|---|
| `agent` | `Provider`, `Host`, `Binding`, `Registry`, the eleven launchers, and the exec handoff |
| `launch` | `Service`, `Plan`, the guard chain and its typed condition errors |
| `catalog` | the neutral `Model`, the `Catalog` interface, and `Snapshot` |

`Launcher.Command(Request) (Command, error)` is the only required interface
and **must be pure** — no writes, no network, no spawning. Purity is what lets
every agent be tested by comparing a struct. Everything else is an opt-in
capability interface detected by type assertion: `Installable`, `Installer`,
`Compatible`, `PlatformSupported`, `ConfigWriter`, `CredentialShadowCheck` and
`Staged`.

The registry is a **value**, not package state. `Builtins()` returns
provider-independent `Definition`s; `NewRegistry(Binding, []Definition)`
resolves them against one provider. A definition whose `New` returns
`ErrUnsupportedProvider` stays registered with a placeholder launcher and a
reason to show the user; any other construction error fails the whole
registry, because a bug rendered as a user-facing reason would read as a
considered explanation.

## Things that cost real debugging

Each of these is enforced by a test, not left to a comment:

- **Two base URLs, and they are not the same string.** Claude Code appends its
  own version segment to `ANTHROPIC_BASE_URL`, so that root must not carry
  one, while an OpenAI-compatible client appends only a method path and its
  root must. `Provider.Validate` refuses an `AnthropicBaseURL` ending in a
  version segment.
- **One of Claude Code's two credential slots must always be non-empty**, or
  it silently authenticates against Anthropic directly and the launch runs on
  the user's own account. Which slot inverts by provider: a provider issuing
  real keys fills `ANTHROPIC_API_KEY`; a keyless one fills
  `ANTHROPIC_AUTH_TOKEN` with a placeholder. `Provider.Validate` refuses a
  keyless provider with no placeholder.
- **codex requires `wire_api = "responses"`.** `"chat"` is rejected at
  config-load time.
- **`execve` does not dedupe `envp`.** A user's stray export beats ours unless
  `ExecArgs` strips it first.
- **cline's key cannot travel in the environment.** Its hub daemon resolves
  credentials from whatever shell first started it, so the key goes on argv.
- **A `ConfigWriter` agent must never take the `syscall.Exec` handoff.** It
  forks and waits instead, so its restore can run afterwards.

## Writing an agent's own config files

Don't. Agents are configured through environment variables, inline-config env
content, CLI overrides, or a key on argv. The exception is narrow and
capability-gated: a launcher may implement `ConfigWriter`, whose `Apply`
snapshots what it touches and restores it on exit.

This module has exactly three files that call a raw write primitive —
`launch/handoff.go`, `agent/droid.go` and `agent/cline.go` — and
`writesites_test.go` fails if a fourth appears.

## Development

```bash
make help          # every target
make ci            # everything CI runs
make test-isolated # suite must stay green with real installs invisible
make lint-cross    # lint the build-tagged files the default GOOS never sees
```

`make lint-cross` is not optional. Lint is GOOS-sensitive, and
`agent/exec_windows.go` is invisible to the linter on Linux.

## License

MIT — see [LICENSE](LICENSE).
