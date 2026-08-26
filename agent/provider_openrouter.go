package agent

// OpenRouter is the provider this tool was built for.
//
// This value is the whole OpenRouter-specific surface of the package: every
// launcher reads its endpoints, its key variable and its ID rather than a
// constant of its own. It lives here for now and moves out to the consuming
// tool once the registry takes a provider rather than defaulting to one.
var OpenRouter = Provider{
	ID:          "openrouter",
	DisplayName: "OpenRouter",

	BaseURL: "https://openrouter.ai/api/v1",
	// No /v1 — Claude Code appends its own version segment. See the field
	// comment on Provider.AnthropicBaseURL, and Landmine 1.
	AnthropicBaseURL: "https://openrouter.ai/api",

	APIKeyEnv:      "OPENROUTER_API_KEY",
	RequiresAPIKey: true,
	KeysURL:        "https://openrouter.ai/keys",

	ModelPrefix: "openrouter/",
	// Live-verified against codex 0.146.1: "chat" is rejected at config-load
	// time. Landmine 18.
	WireAPI: "responses",
}

// OpenRouterHost is this tool's identity.
//
// Marker must stay "openrouter-launch" for as long as this tool exists,
// whatever the binary is called: it is already written into users'
// ~/.factory/settings.local.json files, and droid's restore recognises our
// entries by exact string match. See Host.Marker.
var OpenRouterHost = Host{
	Name:   "openrouter-launch",
	Marker: "openrouter-launch",
}
