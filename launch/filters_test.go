package launch

import (
	"testing"

	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// changedSet builds the "was this flag explicitly set?" predicate that
// cmd.Flags().Changed provides in production.
func changedSet(names ...string) func(string) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(name string) bool { return set[name] }
}

func TestFilterFromCopiesPersistedFields(t *testing.T) {
	got := FilterFrom(config.Filters{
		ToolsOnly: true, FreeOnly: true, MinContext: 1000, MaxPrice: 3,
	})

	want := openrouter.Filter{ToolsOnly: true, FreeOnly: true, MinContext: 1000, MaxPrice: 3}
	if got != want {
		t.Errorf("FilterFrom = %+v, want %+v", got, want)
	}
}

func TestFilterFromLeavesProviderAndSearchZero(t *testing.T) {
	// config.Filters has no Provider or Search field, so FilterFrom must not
	// invent one.
	got := FilterFrom(config.Filters{ToolsOnly: true})
	if got.Provider != "" {
		t.Errorf("Provider = %q, want empty", got.Provider)
	}
	if got.Search != "" {
		t.Errorf("Search = %q, want empty", got.Search)
	}
}

func TestMergeFiltersUsesPersistedWhenNoFlagSet(t *testing.T) {
	persisted := config.Filters{ToolsOnly: true, FreeOnly: true, MinContext: 100000, MaxPrice: 9}
	got := MergeFilters(persisted, openrouter.Filter{}, changedSet())

	want := openrouter.Filter{ToolsOnly: true, FreeOnly: true, MinContext: 100000, MaxPrice: 9}
	if got != want {
		t.Errorf("MergeFilters = %+v, want %+v", got, want)
	}
}

// This is the case the `changed` predicate exists for. An explicit
// --tools=false and an absent --tools are both `false` by value, so without
// the predicate this is unrepresentable and the persisted true silently
// wins. Deleting the changed(FlagTools) branch must fail this test.
func TestMergeFiltersExplicitFalseBeatsPersistedTrue(t *testing.T) {
	persisted := config.Filters{ToolsOnly: true}
	got := MergeFilters(persisted, openrouter.Filter{ToolsOnly: false}, changedSet(FlagTools))

	if got.ToolsOnly {
		t.Error("an explicit --tools=false must override a persisted ToolsOnly:true")
	}
}

func TestMergeFiltersFlagsOverridePersistedValues(t *testing.T) {
	persisted := config.Filters{FreeOnly: false, MinContext: 1000, MaxPrice: 100}
	flags := openrouter.Filter{FreeOnly: true, MinContext: 200000, MaxPrice: 5}
	got := MergeFilters(persisted, flags, changedSet(FlagFree, FlagMinContext, FlagMaxPrice))

	if !got.FreeOnly {
		t.Error("--free should override the persisted false")
	}
	if got.MinContext != 200000 {
		t.Errorf("MinContext = %d, want the flag value 200000", got.MinContext)
	}
	if got.MaxPrice != 5 {
		t.Errorf("MaxPrice = %v, want the flag value 5", got.MaxPrice)
	}
}

func TestMergeFiltersLeavesUnchangedFlagsAlone(t *testing.T) {
	// Only --free was set; MinContext must keep its persisted value rather
	// than being clobbered by the flag's zero.
	persisted := config.Filters{MinContext: 100000, MaxPrice: 7}
	flags := openrouter.Filter{FreeOnly: true}
	got := MergeFilters(persisted, flags, changedSet(FlagFree))

	if got.MinContext != 100000 {
		t.Errorf("MinContext = %d, want the persisted 100000", got.MinContext)
	}
	if got.MaxPrice != 7 {
		t.Errorf("MaxPrice = %v, want the persisted 7", got.MaxPrice)
	}
}

func TestMergeFiltersProviderAndSearchAlwaysComeFromFlags(t *testing.T) {
	// Neither has a persisted counterpart, so both pass through even though
	// `changed` reports nothing was set.
	flags := openrouter.Filter{Provider: "anthropic", Search: "opus"}
	got := MergeFilters(config.Filters{}, flags, changedSet())

	if got.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", got.Provider)
	}
	if got.Search != "opus" {
		t.Errorf("Search = %q, want opus", got.Search)
	}
}
