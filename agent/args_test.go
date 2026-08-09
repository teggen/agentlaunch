package agent

import (
	"strings"
	"testing"
)

func TestRejectModelFlag(t *testing.T) {
	for _, arg := range []string{"-m", "-mfoo", "--model", "--model=x/y"} {
		err := rejectModelFlag("pi", []string{"--verbose", arg})
		if err == nil {
			t.Errorf("%q accepted, want error", arg)
			continue
		}
		if !strings.Contains(err.Error(), arg) || !strings.Contains(err.Error(), "pi") {
			t.Errorf("%q: error %q does not name the argument and agent", arg, err)
		}
	}
	if err := rejectModelFlag("pi", []string{"--verbose", "-p", "hello", "--mode", "fast"}); err != nil {
		t.Errorf("benign args rejected: %v", err)
	}
}

func TestRejectFlags(t *testing.T) {
	// Long flag: separate and equals forms. Short flag: separate, attached.
	for _, arg := range []string{"--provider", "--provider=x", "-P", "-Px"} {
		err := rejectFlags("cline", []string{arg}, "--provider", "-P")
		if err == nil {
			t.Errorf("%q accepted, want error", arg)
			continue
		}
		if !strings.Contains(err.Error(), arg) {
			t.Errorf("%q: error %q does not name the argument", arg, err)
		}
	}
	// --providerfoo is a DIFFERENT flag, not an attached form of --provider.
	for _, arg := range []string{"--providerfoo", "-Q", "--prov"} {
		if err := rejectFlags("cline", []string{arg}, "--provider", "-P"); err != nil {
			t.Errorf("%q rejected, want accepted: %v", arg, err)
		}
	}
}
