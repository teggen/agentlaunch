package agent

import (
	"strings"
	"testing"
)

// testProvider is a synthetic provider for launcher tests. It is deliberately
// NOT OpenRouter-shaped: a launcher that ignored its provider and kept a
// hardcoded openrouter.ai constant would still satisfy a fixture that merely
// looked like OpenRouter, so the fixture that proves parameterization has to
// differ from the real value in every field a launcher touches.
func testProvider() Provider {
	return Provider{
		ID:               "acme",
		DisplayName:      "Acme",
		BaseURL:          "https://api.acme.test/v9",
		AnthropicBaseURL: "https://api.acme.test",
		APIKeyEnv:        "ACME_API_KEY",
		RequiresAPIKey:   true,
		KeysURL:          "https://acme.test/keys",
		ModelPrefix:      "acme/",
		WireAPI:          "responses",
	}
}

// testHost is the synthetic host counterpart to testProvider.
func testHost() Host {
	return Host{Name: "acme-launch", Marker: "acme-launch"}
}

func TestOpenRouterProviderAndHostValidate(t *testing.T) {
	if err := OpenRouter.Validate(); err != nil {
		t.Fatalf("OpenRouter.Validate() = %v, want nil", err)
	}
	if err := OpenRouterHost.Validate(); err != nil {
		t.Fatalf("OpenRouterHost.Validate() = %v, want nil", err)
	}
	if err := testProvider().Validate(); err != nil {
		t.Fatalf("testProvider().Validate() = %v, want nil", err)
	}
}

// TestOpenRouterBaseURLsDifferByTheVersionSegment is Landmine 1 stated as the
// relationship it actually is, rather than as two facts about two constants.
// The OpenAI-compatible root carries a version segment because clients on
// that protocol append only a method path; the Anthropic root must not,
// because Claude Code appends its own. Unifying them — which is correct for
// providers whose two roots really are the same string — yields
// /api/v1/v1/messages here.
func TestOpenRouterBaseURLsDifferByTheVersionSegment(t *testing.T) {
	if !strings.HasSuffix(OpenRouter.BaseURL, "/v1") {
		t.Errorf("BaseURL = %q, want a /v1 suffix", OpenRouter.BaseURL)
	}
	if strings.HasSuffix(OpenRouter.AnthropicBaseURL, "/v1") {
		t.Errorf("AnthropicBaseURL = %q must not end in /v1", OpenRouter.AnthropicBaseURL)
	}
	if OpenRouter.AnthropicBaseURL+"/v1" != OpenRouter.BaseURL {
		t.Errorf("the two roots must differ by exactly the version segment: %q + /v1 != %q",
			OpenRouter.AnthropicBaseURL, OpenRouter.BaseURL)
	}
}

// TestProviderValidateRejectsEachBrokenRule gives one entry per rule, each
// breaking exactly that rule and nothing else, so deleting any single check
// in Validate fails a named row rather than being absorbed by another.
func TestProviderValidateRejectsEachBrokenRule(t *testing.T) {
	for _, tc := range []struct {
		rule  string
		spoil func(*Provider)
	}{
		{"missing ID", func(p *Provider) { p.ID = "" }},
		{"uppercase ID", func(p *Provider) { p.ID = "Acme" }},
		{"dotted ID", func(p *Provider) { p.ID = "ac.me" }},
		{"missing DisplayName", func(p *Provider) { p.DisplayName = "" }},
		{"no endpoint at all", func(p *Provider) { p.BaseURL, p.AnthropicBaseURL = "", "" }},
		{"missing APIKeyEnv", func(p *Provider) { p.APIKeyEnv = "" }},
		{"keyless without a placeholder", func(p *Provider) { p.RequiresAPIKey = false }},
		{"anthropic root with /v1", func(p *Provider) { p.AnthropicBaseURL = "https://api.acme.test/v1" }},
		{"anthropic root with /v2/", func(p *Provider) { p.AnthropicBaseURL = "https://api.acme.test/v2/" }},
		{"unknown wire api", func(p *Provider) { p.WireAPI = "completions" }},
	} {
		p := testProvider()
		tc.spoil(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("Validate() accepted a provider with %s", tc.rule)
		}
	}
}

// TestProviderValidateAcceptsAKeylessProviderWithAPlaceholder pins the shape
// a locally served model needs: no user key, but never an empty credential.
func TestProviderValidateAcceptsAKeylessProviderWithAPlaceholder(t *testing.T) {
	p := testProvider()
	p.RequiresAPIKey = false
	p.PlaceholderKey = "acme"
	p.AnthropicBaseURL = ""
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for a keyless provider", err)
	}
}

func TestProviderCredential(t *testing.T) {
	p := testProvider()
	if _, err := p.Credential("claude", ""); err == nil {
		t.Error("Credential() accepted an empty key for a provider that requires one")
	}
	got, err := p.Credential("claude", "sk-real")
	if err != nil || got != "sk-real" {
		t.Errorf("Credential() = %q, %v; want \"sk-real\", nil", got, err)
	}

	// A keyless provider ignores whatever the host resolved and always sends
	// its placeholder, which must never be empty: Claude Code treats an empty
	// credential slot as "authenticate against Anthropic yourself".
	p.RequiresAPIKey = false
	p.PlaceholderKey = "acme"
	got, err = p.Credential("claude", "")
	if err != nil || got != "acme" {
		t.Errorf("keyless Credential() = %q, %v; want \"acme\", nil", got, err)
	}
}

func TestProviderEnvAndModelHelpers(t *testing.T) {
	p := testProvider()
	if got := p.EnvEntry("k"); got != "ACME_API_KEY=k" {
		t.Errorf("EnvEntry() = %q", got)
	}
	if got := p.EnvRef(); got != "${ACME_API_KEY}" {
		t.Errorf("EnvRef() = %q", got)
	}
	if got := p.ModelRef("vendor/m"); got != "acme/vendor/m" {
		t.Errorf("ModelRef() = %q", got)
	}
	if got := p.UpperID(); got != "ACME" {
		t.Errorf("UpperID() = %q", got)
	}
	// A dash is legal in an ID and illegal in an environment variable name.
	p.ID = "my-gateway"
	if got := p.UpperID(); got != "MY_GATEWAY" {
		t.Errorf("UpperID() with a dash = %q, want MY_GATEWAY", got)
	}
	// An empty prefix leaves the model ID alone, which is what agents that
	// take a bare slug plus a --provider flag need.
	p.ModelPrefix = ""
	if got := p.ModelRef("vendor/m"); got != "vendor/m" {
		t.Errorf("ModelRef() with no prefix = %q", got)
	}
}
