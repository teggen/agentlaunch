package launch

import (
	"github.com/teggen/openrouter-launch/internal/config"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// Flag names for the filters that have a persisted counterpart. They are
// shared so that "was --tools set?" does not depend on a string literal
// duplicated between the flag registration and the merge.
const (
	FlagTools      = "tools"
	FlagFree       = "free"
	FlagMinContext = "min-context"
	FlagMaxPrice   = "max-price"
)

// FilterFrom converts persisted filter state into a catalog filter. Provider
// and Search have no persisted counterpart and stay zero.
func FilterFrom(f config.Filters) openrouter.Filter {
	return openrouter.Filter{
		ToolsOnly:  f.ToolsOnly,
		FreeOnly:   f.FreeOnly,
		MinContext: f.MinContext,
		MaxPrice:   f.MaxPrice,
	}
}

// MergeFilters returns the persisted filters overridden by each flag the
// user explicitly set. changed reports whether a flag was provided;
// cobra's cmd.Flags().Changed satisfies it directly.
//
// The predicate is load-bearing: an explicit --tools=false and an absent
// --tools are both false by value, so without it there is no way to turn a
// persisted ToolsOnly:true back off from the command line.
//
// Provider and Search always come from flags, having no persisted form.
func MergeFilters(persisted config.Filters, flags openrouter.Filter,
	changed func(string) bool) openrouter.Filter {

	out := FilterFrom(persisted)
	if changed(FlagTools) {
		out.ToolsOnly = flags.ToolsOnly
	}
	if changed(FlagFree) {
		out.FreeOnly = flags.FreeOnly
	}
	if changed(FlagMinContext) {
		out.MinContext = flags.MinContext
	}
	if changed(FlagMaxPrice) {
		out.MaxPrice = flags.MaxPrice
	}
	out.Provider = flags.Provider
	out.Search = flags.Search
	return out
}
