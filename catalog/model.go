// Package catalog holds the provider-neutral model catalog: the normalized
// Model, the narrow Catalog interface a launcher tool reads it through, and
// the Snapshot that carries a read's provenance.
//
// It is deliberately free of any one vendor. The OpenRouter wire format, the
// HTTP client that speaks it, the on-disk cache and the presentation helpers
// all live in internal/openrouter, which imports this. Nothing here does.
package catalog

import "strings"

// Model is a normalized catalog entry. Prices are USD per million tokens and
// are only meaningful when PricingUnknown is false.
//
// The fields carry NO json struct tags, and adding them is a correctness bug
// rather than a tidy-up. The on-disk cache marshals this type directly, so
// today's files store Go field names ("PromptPricePerM"). Adding snake_case
// tags would not break decoding — encoding/json matches case-insensitively —
// it would silently zero every price, because "PromptPricePerM" does not
// match "prompt_price_per_m". A $75/M model would then render as free, which
// is Landmine 4's exact false claim reached by a new route.
type Model struct {
	ID                  string
	Name                string
	Description         string
	ContextLength       int
	PromptPricePerM     float64
	CompletionPricePerM float64
	// PricingUnknown reports that a price could not be parsed. Unknown
	// pricing must never be mistaken for free pricing.
	PricingUnknown bool
	SupportsTools  bool
	Provider       string
}

// IsFree reports whether both prompt and completion tokens cost nothing.
// Unknown pricing is never free.
func (m Model) IsFree() bool {
	if m.PricingUnknown {
		return false
	}
	return m.PromptPricePerM == 0 && m.CompletionPricePerM == 0
}

// Matches reports whether term appears in the model's ID or Name,
// case-insensitively. It is the single definition of "this search term hits
// this model", shared by Suggest here and by the Filter that stayed in
// internal/openrouter — a "did you mean" list that disagreed with what the
// picker's search box shows would be worse than either behavior alone.
func (m Model) Matches(term string) bool {
	term = strings.ToLower(term)
	return strings.Contains(strings.ToLower(m.ID), term) ||
		strings.Contains(strings.ToLower(m.Name), term)
}
