// Package catalogtest provides the shared catalog fixture used by tests
// across internal/cli, internal/launch, and internal/tui.
//
// It exists because the same three-model slice was duplicated in each package
// and kept in sync by hand. Several tests in more than one package depend on
// openai/o1-mini being the only entry without tool support, so a drifting
// copy would silently change what those tests assert.
//
// It sits beside the types it serves rather than beside the OpenRouter
// client, because the packages that depend on it are the provider-neutral
// ones: a second launcher tool's planner tests need this fixture and must not
// have to import an OpenRouter-specific package to get it.
package catalogtest

import (
	"context"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// Models returns the shared fixture catalog.
//
// Invariants tests rely on, which must hold for any future edit:
//   - openai/o1-mini is the ONLY entry without tool support.
//   - qwen/qwen3-coder:free is the ONLY free entry.
//   - anthropic/claude-opus-4.6 is the ONLY entry over $5 per M completion.
//   - Exactly one entry per provider: anthropic, qwen, openai.
func Models() []catalog.Model {
	return []catalog.Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			Provider: "openai"},
	}
}

// Catalog serves a fixed model list without touching the network. The field
// is List rather than Models because it cannot share a name with the method
// that satisfies catalog.Catalog.
type Catalog struct{ List []catalog.Model }

// Models implements catalog.Catalog.
func (c *Catalog) Models(context.Context) ([]catalog.Model, error) {
	return c.List, nil
}

// NewCatalog returns a Catalog serving the shared fixture.
func NewCatalog() *Catalog { return &Catalog{List: Models()} }
