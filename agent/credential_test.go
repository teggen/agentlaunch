package agent

import (
	"strings"
	"testing"
)

// These tests come from a Windows bug report: launching opencode died with
//
//	run C:\...\opencode.cmd: exec: environment variable contains NUL
//
// The NUL was in the user's stored API key. os/exec refuses an environment
// entry containing one (dedupEnv, for #56284) on every platform but Plan 9,
// and the failure surfaces at exec time naming neither the variable nor the
// key — after the planner has already recorded the selection and staged files.
//
// The key is the ONLY user-controlled value that reaches the environment
// unescaped. Everything else a launcher sets is either a literal or passes
// through json.Marshal, which renders a NUL as the six characters \u0000.
// A config file makes the round trip silent: encoding/json escapes the NUL on
// save and decodes it back to a raw byte on load, so nothing between the
// prompt and the exec ever sees anything wrong.

// TestCredentialRejectsControlCharacters pins the chokepoint. All thirteen
// launchers call Credential before doing anything else, so refusing here is
// what keeps a bad key out of every environment this module builds.
func TestCredentialRejectsControlCharacters(t *testing.T) {
	p := testProvider()
	for _, tc := range []struct {
		what string
		key  string
	}{
		{"NUL", "sk-acme-abc\x00def"},
		{"trailing NUL", "sk-acme-abc\x00"},
		{"carriage return", "sk-acme-abc\rdef"},
		{"newline", "sk-acme-abc\ndef"},
		{"tab", "sk-acme-abc\tdef"},
		{"DEL", "sk-acme-abc\x7fdef"},
	} {
		_, err := p.Credential("opencode", tc.key)
		if err == nil {
			t.Errorf("%s: Credential accepted a key containing one", tc.what)
			continue
		}
		// The message has to be actionable: the user cannot see a NUL in
		// their own config file, so it must say where the key came from.
		if !strings.Contains(err.Error(), "API key") {
			t.Errorf("%s: error %q does not identify the key as the problem", tc.what, err)
		}
	}
}

// A key with no control characters still works, including the punctuation
// real keys carry. Without this the test above is satisfied by a Credential
// that rejects everything.
func TestCredentialAcceptsAnOrdinaryKey(t *testing.T) {
	p := testProvider()
	const key = "sk-or-v1-0a1b2c3d_4e5f-6789.abcdef"
	got, err := p.Credential("opencode", key)
	if err != nil {
		t.Fatalf("Credential(%q) = %v, want nil", key, err)
	}
	if got != key {
		t.Errorf("Credential returned %q, want the key unchanged", got)
	}
}

// TestNoLauncherPutsAControlCharacterInTheEnvironment is the property that
// actually matters, stated over every launcher rather than over Credential:
// a launcher that stopped calling Credential, or that grew a second
// user-controlled value, would pass the test above and still hand os/exec an
// environment it refuses.
//
// It needs no PATH: every launcher resolves the credential before it looks
// for its binary, which is what makes this a plan-time refusal rather than a
// failure after the planner has already had side effects.
func TestNoLauncherPutsAControlCharacterInTheEnvironment(t *testing.T) {
	p, host := testProvider(), testHost()
	launchers := []Launcher{
		&Claude{Provider: p, Host: host}, &Codex{Provider: p, Host: host},
		&OpenCode{Provider: p, Host: host}, &Pi{Provider: p, Host: host},
		&Hermes{Provider: p, Host: host}, &Qwen{Provider: p, Host: host},
		&Cline{Provider: p, Host: host}, &Kimi{Provider: p, Host: host},
		&OMP{Provider: p, Host: host}, &OpenClaw{Provider: p, Host: host},
		&Droid{Provider: p, Host: host}, &Pool{Provider: p, Host: host},
		&HermesDesktop{Hermes: Hermes{Provider: p, Host: host}},
	}
	if want := len(builtinRegistry(t).List()) - 2; len(launchers) != want {
		t.Fatalf("covering %d launchers, but the registry has %d supported entries",
			len(launchers), want)
	}

	req := Request{Model: testModel(), APIKey: "sk-acme-abc\x00def", StageDir: t.TempDir()}
	for _, l := range launchers {
		cmd, err := l.Command(req)
		if err != nil {
			continue // refused, which is the point
		}
		for _, e := range cmd.Env {
			if i := strings.IndexFunc(e, isEnvControlChar); i >= 0 {
				t.Errorf("%s: env entry %q carries a control character at %d; "+
					"os/exec will refuse the whole environment at launch",
					l.Name(), e, i)
			}
		}
		t.Errorf("%s: Command accepted a key containing a NUL", l.Name())
	}
}
