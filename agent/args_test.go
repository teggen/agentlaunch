package agent

import (
	"strings"
	"testing"
)

func TestRejectModelFlag(t *testing.T) {
	for _, arg := range []string{"-m", "-mfoo", "--model", "--model=x/y"} {
		err := rejectModelFlag(testHost(), "pi", []string{"--verbose", arg})
		if err == nil {
			t.Errorf("%q accepted, want error", arg)
			continue
		}
		if !strings.Contains(err.Error(), arg) || !strings.Contains(err.Error(), "pi") {
			t.Errorf("%q: error %q does not name the argument and agent", arg, err)
		}
	}
	if err := rejectModelFlag(testHost(), "pi", []string{"--verbose", "-p", "hello", "--mode", "fast"}); err != nil {
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
		err := rejectModelFlag(testHost(), "pi", []string{arg})
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: unambiguous form should state the conflict outright, got %q", arg, err)
		}
	}
	for _, arg := range []string{"-mfoo", "-mode"} {
		err := rejectModelFlag(testHost(), "pi", []string{arg})
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
		err := rejectFlags(testHost(), "cline", []string{arg}, "--provider", "-P")
		if err == nil {
			t.Fatalf("%q accepted, want error", arg)
		}
		if strings.Contains(err.Error(), "looks like") {
			t.Errorf("%q: unambiguous form should state the conflict outright, got %q", arg, err)
		}
	}
	for _, arg := range []string{"-Pval", "-Persist"} {
		err := rejectFlags(testHost(), "cline", []string{arg}, "--provider", "-P")
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
		err := rejectFlags(testHost(), "cline", []string{arg}, "--provider", "-P")
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
		if err := rejectFlags(testHost(), "cline", []string{arg}, "--provider", "-P"); err != nil {
			t.Errorf("%q rejected, want accepted: %v", arg, err)
		}
	}
}

// TestArgGuardsNameTheHost pins that the guidance text comes from the Host
// rather than from a string literal. Both guards, and both match kinds each,
// since the two kinds build their message separately.
func TestArgGuardsNameTheHost(t *testing.T) {
	host := Host{Name: "zzz-launch", Marker: "zzz"}
	errs := []error{
		rejectModelFlag(host, "pi", []string{"-m"}),
		rejectModelFlag(host, "pi", []string{"-mfoo"}),
		rejectFlags(host, "cline", []string{"--provider"}, "--provider"),
		rejectFlags(host, "cline", []string{"-Pfoo"}, "-P"),
	}
	for _, err := range errs {
		if err == nil {
			t.Fatal("guard accepted a conflicting argument")
		}
		if !strings.Contains(err.Error(), "zzz-launch") {
			t.Errorf("error %q does not name the host", err)
		}
		if strings.Contains(err.Error(), "openrouter-launch") {
			t.Errorf("error %q still names a hardcoded host", err)
		}
	}
}

// TestEveryLauncherPassesItsOwnHostToTheArgGuards is the check that a
// per-launcher Host field is actually threaded, rather than each launcher
// reaching for a package-level default that happens to hold the same value.
//
// Every launcher runs its passthrough guards before it resolves a binary, so
// this needs no PATH and no home directory.
func TestEveryLauncherPassesItsOwnHostToTheArgGuards(t *testing.T) {
	host := Host{Name: "zzz-launch", Marker: "zzz"}
	p := testProvider()
	launchers := []Launcher{
		&Claude{Provider: p, Host: host}, &Codex{Provider: p, Host: host},
		&OpenCode{Provider: p, Host: host}, &Pi{Provider: p, Host: host},
		&Hermes{Provider: p, Host: host}, &Qwen{Provider: p, Host: host},
		&Cline{Provider: p, Host: host}, &Kimi{Provider: p, Host: host},
		&OMP{Provider: p, Host: host}, &OpenClaw{Provider: p, Host: host},
		&Droid{Provider: p, Host: host},
	}
	if want := len(List()) - 3; len(launchers) != want {
		t.Fatalf("covering %d launchers, but the registry has %d supported entries",
			len(launchers), want)
	}
	for _, l := range launchers {
		req := Request{Model: testModel(), APIKey: "sk-or-test", ExtraArgs: []string{"-m", "other/model"}}
		_, err := l.Command(req)
		if err == nil {
			t.Errorf("%s: accepted a conflicting -m", l.Name())
			continue
		}
		if !strings.Contains(err.Error(), "zzz-launch") {
			t.Errorf("%s: error %q does not name its own host", l.Name(), err)
		}
	}
}

// TestBespokeRefusalsNameTheHost covers the three refusals that do not go
// through the shared guards and so would not be caught by the table above:
// codex's conflicting -c override, hermes's subcommand-shaped passthrough,
// and openclaw's platform-administration passthrough. Each builds its own
// message, so each can regress to a literal independently.
func TestBespokeRefusalsNameTheHost(t *testing.T) {
	host := Host{Name: "zzz-launch", Marker: "zzz"}
	req := func(extras ...string) Request {
		return Request{Model: testModel(), APIKey: "sk-or-test", ExtraArgs: extras}
	}
	for _, tc := range []struct {
		what string
		l    Launcher
		req  Request
	}{
		{"codex -c override", &Codex{Provider: testProvider(), Host: host}, req("-c", `model_provider="other"`)},
		{"hermes subcommand", &Hermes{Provider: testProvider(), Host: host}, req("serve")},
		{"openclaw admin", &OpenClaw{Provider: testProvider(), Host: host}, req("gateway")},
	} {
		_, err := tc.l.Command(tc.req)
		if err == nil {
			t.Errorf("%s: accepted, want refusal", tc.what)
			continue
		}
		if !strings.Contains(err.Error(), "zzz-launch") {
			t.Errorf("%s: error %q does not name the host", tc.what, err)
		}
		if strings.Contains(err.Error(), "openrouter-launch") {
			t.Errorf("%s: error %q still names a hardcoded host", tc.what, err)
		}
	}
}
