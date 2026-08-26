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
