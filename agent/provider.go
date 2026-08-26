package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// Provider is the endpoint a launcher points an agent at. Every launcher in
// this package is parameterized by one, so that the same eleven recipes serve
// OpenRouter, a locally served model, a direct vendor API, or a self-hosted
// gateway without being rewritten.
//
// It carries no catalog and no credentials. It is the answer to "where does
// this agent send tokens, and what does it call that place", nothing more.
type Provider struct {
	// ID is this provider's token as it appears on an agent's command line
	// and inside agent configuration: cline's -P, pi's and hermes's
	// --provider, codex's model_providers.<ID>.*, the "<ID>/<slug>" model
	// reference omp, opencode and openclaw take, and the key a stored
	// credential check looks for in an agent's own auth file. Lowercase.
	ID string
	// DisplayName is the human name: codex's model_providers.<ID>.name, and
	// the subject of every "an X API key is required".
	DisplayName string

	// BaseURL is the OpenAI-compatible root, and it INCLUDES whatever version
	// segment the vendor publishes ("https://openrouter.ai/api/v1",
	// "http://127.0.0.1:11434/v1"). Clients speaking that protocol append
	// only a method path — "/chat/completions", "/responses" — never a
	// version. Empty means the provider has no OpenAI-compatible surface.
	BaseURL string
	// AnthropicBaseURL is the Anthropic-Messages root, and it EXCLUDES a
	// version segment: Claude Code appends its own, so a /v1 here produces
	// /api/v1/v1/messages and breaks the launch. OpenRouter's two roots
	// therefore differ by exactly that segment — https://openrouter.ai/api
	// against https://openrouter.ai/api/v1 — which is Landmine 1.
	//
	// The two live side by side here on purpose. The invariant used to be two
	// constants in two packages that never referenced each other, so the
	// "these are the same URL, DRY them up" refactor could look reasonable
	// from either one alone. It is also a refactor that is RIGHT for other
	// providers — a local server's roots are host:11434 and host:11434/v1,
	// direct Anthropic's are identical — which is what made it tempting.
	// Adjacent fields under one comment, plus Validate's suffix check below,
	// is the strongest form this invariant has ever had.
	//
	// Empty means the provider has no Anthropic-compatible surface, which is
	// the ordinary case for a local OpenAI-compatible server, and which makes
	// Claude Code unlaunchable against it.
	AnthropicBaseURL string

	// APIKeyEnv is the environment variable the credential travels in, and
	// the name agents are told to read it from: codex's
	// model_providers.<ID>.env_key and droid's "${...}" interpolation both
	// need the NAME regardless of the value. Always set, even when
	// RequiresAPIKey is false.
	APIKeyEnv string
	// RequiresAPIKey reports whether the user must supply a key. False for a
	// local server, in which case a launch carries PlaceholderKey.
	RequiresAPIKey bool
	// PlaceholderKey is the credential sent when RequiresAPIKey is false. It
	// must be non-empty: several agents refuse an endpoint with no credential
	// at all, and Claude Code falls back to authenticating against Anthropic
	// directly when its credential slot is empty (Landmine 2).
	PlaceholderKey string
	// KeysURL is where a user obtains a key, quoted in the no-key error.
	KeysURL string

	// ModelPrefix is prepended to a model ID for agents that select a
	// provider through the model reference itself ("openrouter/" turns
	// anthropic/claude-opus-4.6 into openrouter/anthropic/claude-opus-4.6).
	// Empty means the agent is told the provider some other way.
	ModelPrefix string
	// WireAPI is codex's model_providers.<ID>.wire_api value. "responses" for
	// any endpoint serving OpenAI's newer protocol; "chat" only for one that
	// proxies /chat/completions alone. Read Landmine 18 before changing it:
	// "chat" is rejected at config-load time by codex >= 0.146.1, and the
	// value here is live-verified, not inferred from documentation.
	WireAPI string
}

// providerIDPattern is what an agent will accept as a provider token on argv
// or as a TOML/JSON key. Deliberately narrow: a provider ID reaches codex as
// a bare `model_providers.<ID>.name` path component, where a dot or a quote
// would change the shape of the config rather than the value in it.
var providerIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// versionSuffixPattern matches a trailing API version segment.
var versionSuffixPattern = regexp.MustCompile(`/v[0-9]+/?$`)

// Validate reports why a Provider cannot be used. It is called at registry
// construction so a misconfigured descriptor fails before any launch, in the
// same spirit as the nil-Launcher panic in buildIndex.
func (p Provider) Validate() error {
	switch {
	case p.ID == "":
		return fmt.Errorf("provider: ID is required")
	case !providerIDPattern.MatchString(p.ID):
		return fmt.Errorf("provider %q: ID must be lowercase letters, digits and dashes", p.ID)
	case p.DisplayName == "":
		return fmt.Errorf("provider %q: DisplayName is required", p.ID)
	case p.BaseURL == "" && p.AnthropicBaseURL == "":
		return fmt.Errorf("provider %q: needs at least one of BaseURL or AnthropicBaseURL", p.ID)
	case p.APIKeyEnv == "":
		// Needed even for a keyless provider: codex writes the variable NAME
		// into its config as env_key, and droid interpolates "${NAME}".
		return fmt.Errorf("provider %q: APIKeyEnv is required even when RequiresAPIKey is false", p.ID)
	case !p.RequiresAPIKey && p.PlaceholderKey == "":
		return fmt.Errorf("provider %q: a provider that needs no user key must set PlaceholderKey; "+
			"an empty credential makes Claude Code fall back to its own authentication", p.ID)
	}

	// Landmine 1, machine-checked. Claude Code appends its own version
	// segment to ANTHROPIC_BASE_URL, so one here is always wrong — and this
	// is the check that could not exist while the two URLs lived in packages
	// that did not import each other.
	// Landmine 1, machine-checked. Claude Code appends its own version
	// segment to ANTHROPIC_BASE_URL, so one here is always wrong — and this
	// is the check that could not exist while the two URLs lived in packages
	// that did not import each other.
	if versionSuffixPattern.MatchString(p.AnthropicBaseURL) {
		return fmt.Errorf("provider %q: AnthropicBaseURL %q ends in a version segment; "+
			"Claude Code appends its own, so this would request a doubled path (Landmine 1)",
			p.ID, p.AnthropicBaseURL)
	}
	if p.WireAPI != "" && p.WireAPI != "chat" && p.WireAPI != "responses" {
		return fmt.Errorf("provider %q: WireAPI %q is neither \"chat\" nor \"responses\"", p.ID, p.WireAPI)
	}
	return nil
}

// ModelRef returns the model reference an agent that selects a provider
// through the model name expects. Whether an agent takes this form or a bare
// slug plus a --provider flag is a fact about the AGENT, not the provider:
// pi and omp are pointed at the same OpenRouter and disagree.
func (p Provider) ModelRef(modelID string) string {
	return p.ModelPrefix + modelID
}

// Credential returns the value a launch sends. userKey is what the host
// resolved from its own configuration; it is ignored for a provider that
// needs no user key, which sends PlaceholderKey instead.
//
// The returned credential is never empty on the success path — see
// PlaceholderKey for why that matters.
func (p Provider) Credential(agentName, userKey string) (string, error) {
	if !p.RequiresAPIKey {
		return p.PlaceholderKey, nil
	}
	if userKey == "" {
		return "", fmt.Errorf("%s: an %s API key is required", agentName, p.DisplayName)
	}
	return userKey, nil
}

// EnvEntry returns the "NAME=value" entry carrying the credential.
func (p Provider) EnvEntry(key string) string {
	return p.APIKeyEnv + "=" + key
}

// EnvRef returns the shell-style interpolation of the key variable, which is
// what droid writes into its settings file so the key never touches disk.
func (p Provider) EnvRef() string {
	return "${" + p.APIKeyEnv + "}"
}

// UpperID is the provider ID in the form agents use to build per-provider
// environment variable names (hermes reads <ID>_BASE_URL). Dashes become
// underscores, since a dash cannot appear in an environment variable name.
func (p Provider) UpperID() string {
	return strings.ToUpper(strings.ReplaceAll(p.ID, "-", "_"))
}
