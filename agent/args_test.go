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

// TestRejectModelFlagHedgesOnAttachedPrefixMatches pins the difference
// between what the two match kinds actually KNOW.
//
// "-m", "--model" and "--model=x" are the model flag; nothing else parses
// that way, so stating the user's intent is accurate. "-mfoo" is a guess:
// the check is a prefix test, so a hypothetical agent flag like "-mode" or
// "-max-tokens" matches it too. Rejecting either is right — a false negative
// silently launches a different model than the one reported, which is the
// worse failure by a wide margin — but the message must not tell someone who
// typed "-mode" that they were trying to set the model. It says what was
// observed (this looks like an attached model flag) rather than what was
// intended.
func TestRejectModelFlagHedgesOnAttachedPrefixMatches(t *testing.T) {
	for _, arg := range []string{"-m", "--model", "--model=x/y"} {
		err := rejectModelFlag("pi", []string{arg})
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: unambiguous form should state the conflict outright, got %q", arg, err)
		}
	}
	for _, arg := range []string{"-mfoo", "-mode"} {
		err := rejectModelFlag("pi", []string{arg})
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if !strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: prefix match should hedge rather than assert intent, got %q", arg, err)
		}
		if !strings.Contains(err.Error(), arg) || !strings.Contains(err.Error(), "pi") {
			t.Errorf("%q: error %q does not name the argument and agent", arg, err)
		}
	}
}

// TestRejectFlagsHedgesOnAttachedPrefixMatches is the same distinction for
// the short-flag attached form, which today reaches "-P" and "-k" on cline.
func TestRejectFlagsHedgesOnAttachedPrefixMatches(t *testing.T) {
	for _, arg := range []string{"-P", "--provider", "--provider=x"} {
		err := rejectFlags("cline", []string{arg}, "--provider", "-P")
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: unambiguous form should state the conflict outright, got %q", arg, err)
		}
	}
	for _, arg := range []string{"-Pval", "-Persist"} {
		err := rejectFlags("cline", []string{arg}, "--provider", "-P")
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if !strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: prefix match should hedge rather than assert intent, got %q", arg, err)
		}
		if !strings.Contains(err.Error(), arg) {
			t.Errorf("%q: error %q does not name the argument", arg, err)
		}
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
