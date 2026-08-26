package agent

import "testing"

func TestHostValidateRejectsEachBrokenRule(t *testing.T) {
	for _, tc := range []struct {
		rule  string
		spoil func(*Host)
	}{
		{"missing Name", func(h *Host) { h.Name = "" }},
		{"missing Marker", func(h *Host) { h.Marker = "" }},
	} {
		h := testHost()
		tc.spoil(&h)
		if err := h.Validate(); err == nil {
			t.Errorf("Validate() accepted a host with %s", tc.rule)
		}
	}
}

// TestOpenRouterHostMarkerIsFrozen guards persisted user data, not a label.
//
// droid's Apply writes a customModels entry whose displayName is this marker,
// points droid's default model at "custom:<marker>-<n>", and on restore
// removes exactly the entries that carry it. Any other value makes every
// entry a previous version of this tool wrote unrecognisable — preserved as
// somebody else's forever, alongside a default-model reference to an entry
// that no longer exists. Renaming the binary does not license renaming this.
func TestOpenRouterHostMarkerIsFrozen(t *testing.T) {
	if OpenRouterHost.Marker != "openrouter-launch" {
		t.Errorf("OpenRouterHost.Marker = %q; it is written into users' "+
			"~/.factory/settings.local.json and must not change",
			OpenRouterHost.Marker)
	}
}
