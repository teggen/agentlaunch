package launch

import (
	"fmt"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// WarningKind identifies an advisory condition so a caller can render it in
// its own idiom instead of parsing Message.
type WarningKind int

const (
	// WarnStaleCatalog reports that a catalog refresh failed and cached data
	// was served instead.
	WarnStaleCatalog WarningKind = iota
	// WarnIncompatibleModel reports a pairing the agent may not fully
	// support. Advisory by design: Claude Code works with many non-Anthropic
	// models, so hard-blocking would refuse valid setups.
	WarnIncompatibleModel
	// WarnSelectionNotSaved reports that the last selection could not be
	// persisted. The launch proceeds regardless.
	WarnSelectionNotSaved
	// WarnShadowedCredential reports agent-side stored credentials or state
	// that outrank the environment this launch provides. Advisory: the
	// wrong-account risk is made visible, the user decides.
	WarnShadowedCredential
)

// Warning is an advisory condition the caller renders.
type Warning struct {
	Kind WarningKind
	// Message is the diagnostic text, rendered after the caller's own
	// "warning: " prefix.
	Message string
	// Question is non-empty when the caller must get the user's approval
	// before launching, and is the prompt to put to them. Carrying the
	// wording here rather than a bare Confirm bool means a caller cannot
	// ask "Launch anyway?" about a warning that is not about launching.
	Question string
}

// StaleWarning returns the warning for a snapshot served from a failed
// refresh, and false when the snapshot is fresh. now is a parameter so this
// stays pure and testable.
func StaleWarning(snap catalog.Snapshot, now time.Time) (Warning, bool) {
	if !snap.Stale {
		return Warning{}, false
	}
	return Warning{
		Kind: WarnStaleCatalog,
		Message: fmt.Sprintf(
			"could not refresh the model catalog (%v); using cached data from %s ago",
			snap.StaleErr, snap.Age(now).Round(time.Minute)),
	}, true
}
