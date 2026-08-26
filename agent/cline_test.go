package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestClineCommandPathArgsEnv(t *testing.T) {
	c := &Cline{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/cline")}
	cmd, err := c.Command(Request{
		Model:     testModel(),
		APIKey:    "sk-or-test",
		ExtraArgs: []string{"--auto-approve", "false"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"-P", testProvider().ID, "-m", "anthropic/claude-opus-4.6", "-k", "sk-or-test", "--auto-approve", "false"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	// The env var stays, even though the CLI client never reads it: on a cold
	// start our client is what spawns the hub daemon, and the daemon resolves
	// credentials from ITS OWN environment. Setting it there is what stops a
	// stray OPENROUTER_API_KEY export from becoming the daemon's key for the
	// rest of its life (the Landmine 3 class, one process removed).
	if got, ok := envValue(cmd.Env, testProvider().APIKeyEnv); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
}

// TestClineCommandPutsKeyOnArgv is the inversion of an earlier invariant, so
// it is spelled out rather than left implicit in the args comparison above.
// Measured on 3.0.52: the interactive TUI's provider gate reads persisted
// settings and never the environment, so an env-only launch lands on "Connect
// a model provider to get started"; and the model call happens in a
// long-lived hub daemon rather than the process we exec, so once a daemon is
// running our env is ignored and its startup key bills the session. -k is
// honored in both modes and outranks the daemon's environment and a saved
// providers.json key alike. The cost (key visible in /proc/<pid>/cmdline) is
// accepted; Apply is what contains the other cost.
func TestClineCommandPutsKeyOnArgv(t *testing.T) {
	c := &Cline{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/cline")}
	cmd, err := c.Command(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	i := slices.Index(cmd.Args, "-k")
	if i < 0 {
		t.Fatalf("no -k in Args = %q", cmd.Args)
	}
	if i+1 >= len(cmd.Args) || cmd.Args[i+1] != "sk-or-test" {
		t.Errorf("-k value = %q, want the request's key", cmd.Args[i+1:])
	}
}

func TestClineCommandRequiresAPIKey(t *testing.T) {
	c := &Cline{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/cline")}
	if _, err := c.Command(Request{Model: testModel()}); err == nil {
		t.Fatal("Command with empty APIKey succeeded, want error")
	}
}

func TestClineCommandRejectsConflictingExtras(t *testing.T) {
	c := &Cline{Provider: testProvider(), Host: testHost(), LookPath: stubLookPath("/usr/local/bin/cline")}
	for _, extras := range [][]string{
		{"-m", "x/y"}, {"-mx/y"}, {"--model", "x/y"}, {"--model=x/y"},
		{"-P", "cline"}, {"-Pcline"}, {"--provider", "cline"}, {"--provider=cline"},
		// -k used to be allowed as the user's explicit per-run override. The
		// launcher now owns it: a second -k would decide which key the session
		// bills, and Apply's snapshot/restore is scoped to ours.
		{"-k", "sk-or-theirs"}, {"-ksk-or-theirs"}, {"--key", "sk-or-theirs"}, {"--key=sk-or-theirs"},
	} {
		if _, err := c.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

// TestClineDoesNotClaimCredentialShadowing pins the removal, so that
// re-adding the check is a deliberate act with a test to answer rather than a
// plausible-looking restoration. -k outranks a saved providers.json key and
// the hub daemon's environment alike (measured on 3.0.52, and the ordering is
// explicit in the CLI's own resolveApiKey), so there is nothing left to warn
// about — and a saved key present on the machine must NOT produce a warning.
func TestClineDoesNotClaimCredentialShadowing(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".cline", "data", "settings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "providers.json"),
		[]byte(`{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-theirs"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := any(&Cline{Provider: testProvider(), Host: testHost()}).(CredentialShadowCheck); ok {
		t.Error("*Cline implements CredentialShadowCheck again; its premise (env loses to a saved key) does not hold now that the launcher passes -k")
	}
}

// TestClineImplementsConfigWriter pins the capability itself: it is the type
// assertion in launch.Service.Launch that moves cline off the syscall.Exec
// handoff onto fork-and-wait, and without that path the restore below never
// runs at all.
func TestClineImplementsConfigWriter(t *testing.T) {
	var _ ConfigWriter = (*Cline)(nil)
	if _, ok := any(&Cline{Provider: testProvider(), Host: testHost()}).(ConfigWriter); !ok {
		t.Fatal("*Cline does not satisfy ConfigWriter")
	}
}

// clineProviders writes a providers.json for the tests and returns its path.
func clineProviders(t *testing.T, home, contents string, mode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(home, ".cline", "data", "settings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "providers.json")
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestClineApplyRestoresPriorProvidersFile covers the whole reason cline is a
// ConfigWriter: -k makes the CLI persist our key into its own provider store
// (measured on 3.0.52 — the key lands in providers.json as
// settings.apiKey), which would outlive the session and shadow every later
// launch. Apply writes nothing itself; it snapshots, and restore is what puts
// the user's file back byte for byte.
func TestClineApplyRestoresPriorProvidersFile(t *testing.T) {
	home := testHome(t)
	prior := `{"version":1,"providers":{"openrouter":{"settings":{"model":"theirs/model"}}}}`
	path := clineProviders(t, home, prior, 0o600)

	c := &Cline{Provider: testProvider(), Host: testHost()}
	restore, err := c.Apply(Request{Model: testModel(), APIKey: "sk-or-ours"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Stand in for what cline does to the file during the session.
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-ours"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if string(got) != prior {
		t.Errorf("after restore the file is\n%s\nwant\n%s", got, prior)
	}
	if strings.Contains(string(got), "sk-or-ours") {
		t.Error("our key survived restore in cline's own provider store")
	}
}

// TestClineRestorePreservesProvidersFileMode is split out of the test above
// and skipped on Windows, matching TestDroidPreservesSettingsFileMode: os.Chmod
// there toggles only the read-only attribute, so a writable file always reads
// back 0666 and the assertion could never hold. The property is Unix-only too.
//
// The mid-session file is deliberately given a WIDER mode than the snapshot:
// restoring 0600 then proves the mode came from what we recorded at Apply
// time, not from whatever happened to be on disk afterwards. This file holds
// an API key, so a restore that widened it would be a real defect.
func TestClineRestorePreservesProvidersFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	home := testHome(t)
	path := clineProviders(t, home, `{"version":1,"providers":{}}`, 0o600)

	c := &Cline{Provider: testProvider(), Host: testHost()}
	restore, err := c.Apply(Request{Model: testModel(), APIKey: "sk-or-ours"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-ours"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode after restore = %v, want the snapshotted 0600 — the file holds a key", perm)
	}
}

// TestClineApplyRemovesAFileItDidNotFind is the other half: on a machine
// where cline has never saved a provider, restore must leave no file behind
// rather than leaving ours as the new "saved" credential.
func TestClineApplyRemovesAFileItDidNotFind(t *testing.T) {
	home := testHome(t)
	c := &Cline{Provider: testProvider(), Host: testHost()}
	restore, err := c.Apply(Request{Model: testModel(), APIKey: "sk-or-ours"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := clineProviders(t, home, `{"providers":{"openrouter":{"settings":{"apiKey":"sk-or-ours"}}}}`, 0o600)

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("providers.json still present after restore (err = %v), want it gone", err)
	}
}

// TestClineApplyRestoreToleratesADeletedFile: the user may clear their cline
// state mid-session. Restoring our snapshot over the gap is correct, and it
// must not report failure for a directory cline itself removed.
func TestClineApplyRestoreToleratesADeletedFile(t *testing.T) {
	home := testHome(t)
	prior := `{"version":1,"providers":{}}`
	path := clineProviders(t, home, prior, 0o600)

	c := &Cline{Provider: testProvider(), Host: testHost()}
	restore, err := c.Apply(Request{Model: testModel(), APIKey: "sk-or-ours"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := restore(); err != nil {
		t.Fatalf("restore after the file was deleted: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if string(got) != prior {
		t.Errorf("restored contents = %s, want %s", got, prior)
	}
}

// TestClineApplyRefusesAnUnreadableProvidersFile: unlike ShadowedCredential,
// Apply must NOT be best-effort. If we cannot snapshot the file we cannot put
// it back, and launching anyway would persist the user's key into a store we
// have no way to restore — so the launch fails instead.
func TestClineApplyRefusesAnUnreadableProvidersFile(t *testing.T) {
	home := testHome(t)
	// A directory where the file belongs makes the read fail for a reason
	// every platform agrees on.
	if err := os.MkdirAll(filepath.Join(home, ".cline", "data", "settings", "providers.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := &Cline{Provider: testProvider(), Host: testHost()}
	if _, err := c.Apply(Request{Model: testModel(), APIKey: "sk-or-ours"}); err == nil {
		t.Fatal("Apply succeeded on an unreadable providers.json, want an error")
	}
}

func TestClineInstallHint(t *testing.T) {
	c := &Cline{Provider: testProvider(), Host: testHost()}
	if hint := c.InstallHint(); !strings.Contains(hint, "npm install -g cline") {
		t.Errorf("InstallHint = %q", hint)
	}
}
