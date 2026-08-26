package catalog

import "testing"

// fixture is the three-model set the filter and lookup tests share. It is a
// local literal rather than the catalogtest fixture on purpose: catalogtest
// imports this package, so depending on it here would be a cycle, and these
// tests are about the lookups themselves rather than about the shared
// fixture's invariants.
func fixture() []Model {
	return []Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			SupportsTools: false, Provider: "openai"},
	}
}

func TestFindByID(t *testing.T) {
	m, ok := FindByID(fixture(), "openai/o1-mini")
	if !ok {
		t.Fatal("model not found")
	}
	if m.Provider != "openai" {
		t.Errorf("provider = %q, want openai", m.Provider)
	}
	if _, ok := FindByID(fixture(), "nope/nope"); ok {
		t.Error("unexpectedly found a missing model")
	}
}

func TestSuggest(t *testing.T) {
	got := Suggest(fixture(), "claude", 5)
	if len(got) != 1 || got[0] != "anthropic/claude-opus-4.6" {
		t.Errorf("got %v, want [anthropic/claude-opus-4.6]", got)
	}
}

func TestSuggestRespectsLimit(t *testing.T) {
	if got := Suggest(fixture(), "", 2); len(got) != 2 {
		t.Errorf("got %d suggestions, want 2", len(got))
	}
}

// TestSuggestIsCaseInsensitive pins the half of Matches that Suggest's own
// callers exercise: an unknown slug is typed by a user, so the "did you mean"
// list has to match the way the picker's search box does. Suggest used to
// lowercase the query itself before calling an unexported helper; the shared
// Model.Matches now owns that, and this is what proves the lowering did not
// get lost in the move.
func TestSuggestIsCaseInsensitive(t *testing.T) {
	if got := Suggest(fixture(), "CLAUDE", 5); len(got) != 1 {
		t.Errorf("Suggest(CLAUDE) = %v, want the one claude model", got)
	}
}

func TestSnapshotAge(t *testing.T) {
	now := timeAt(t, 3)
	snap := Snapshot{FetchedAt: timeAt(t, 1)}
	if got := snap.Age(now); got.Hours() != 2 {
		t.Errorf("Age = %v, want 2h", got)
	}
}
