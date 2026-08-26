package agent

import "testing"

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

// otherProvider is a SECOND synthetic provider, differing from testProvider in
// every field a launcher reads.
//
// It exists for the tests that compare two registries. Bound to the same
// provider twice, such a test passes no matter how badly a binding leaks
// between them — it is the DIFFERENCE between the two that carries the claim,
// so one fixture used twice would be no fixture at all.
func otherProvider() Provider {
	return Provider{
		ID:               "zenith",
		DisplayName:      "Zenith",
		BaseURL:          "https://gw.zenith.test/v2",
		AnthropicBaseURL: "https://gw.zenith.test",
		APIKeyEnv:        "ZENITH_TOKEN",
		RequiresAPIKey:   true,
		KeysURL:          "https://zenith.test/tokens",
		ModelPrefix:      "zenith/",
		WireAPI:          "chat",
	}
}

// otherHost is the synthetic host counterpart to otherProvider.
func otherHost() Host {
	return Host{Name: "zenith-launch", Marker: "zenith-launch"}
}

// TestSyntheticFixturesValidate checks the fixtures themselves, because every
// launcher test in this package runs against them: an invalid testProvider
// would make the whole synthetic half of the pair meaningless without ever
// failing on its own terms. The OpenRouter values these replaced are pinned
// by the consuming tool, which is the only place that knows them.
func TestSyntheticFixturesValidate(t *testing.T) {
	if err := testProvider().Validate(); err != nil {
		t.Fatalf("testProvider().Validate() = %v, want nil", err)
	}
	if err := testHost().Validate(); err != nil {
		t.Fatalf("testHost().Validate() = %v, want nil", err)
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
