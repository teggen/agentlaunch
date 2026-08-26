package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Cline launches the Cline CLI against an OpenRouter model via its native
// builtin openrouter provider (base URL baked in upstream). Note cline's
// --auto-approve defaults to TRUE upstream; per the owner decision recorded
// in the Phase 4 spec, the launcher does not override agent behavior
// defaults. Doc-verified on 3.0.51, then LIVE-verified on 3.0.52
// (2026-08-09); see .superpowers/sdd/2026-08-09-tier-2-research/cline.md.
//
// The key goes on argv via -k, and cline is a ConfigWriter. Both follow from
// two measurements that overturned this launcher's original env-only design
// (research open questions 1 and 3, resolved by running it):
//
//  1. The interactive TUI's provider gate reads PERSISTED settings and never
//     the environment, so an env-only launch renders "Connect a model
//     provider to get started" no matter what the environment holds. This is
//     the reported bug, and it applies cold or warm.
//  2. The CLI client we exec does not resolve credentials or call the model.
//     A long-lived hub daemon does (one per data dir, `--cline-hub-daemon` on
//     a local WebSocket), and its credential chain — explicit key, then OAuth
//     resolver, then OPENROUTER_API_KEY from ITS OWN process environment —
//     reads the environment of whatever first spawned it. So env delivery
//     works only while our launch is what starts the daemon; once one is
//     running, ours is ignored and its startup key is used for every later
//     session. Measured: a launch carrying a dummy key returned a real
//     completion off a daemon holding the user's own key.
//
// Together those are why the original design looked verified and still
// failed in use: the Phase 4a live gate ran one-shot prompts against a
// virgin ~/.cline, which is exactly the pair of conditions — no TUI, cold
// daemon — under which env-only does work.
//
// -k is honored in both modes, and it outranks the daemon's environment and
// a saved providers.json key alike (all three measured). It costs argv
// exposure via /proc/<pid>/cmdline — accepted by owner decision, there being
// no working alternative — and it makes cline persist the key into its own
// provider store, which is what Apply exists to undo.
type Cline struct {
	// Provider is the endpoint this agent is pointed at. Required, with no
	// fallback — see the note on Claude.Provider.
	Provider Provider
	// Host identifies this tool in the guidance attached to a rejected
	// passthrough argument, and — for droid — owns the marker stamped into
	// the agent's own settings. Required.
	Host Host
	// LookPath is injectable for tests; nil means exec.LookPath.
	LookPath func(string) (string, error)
}

func (c *Cline) Name() string        { return "cline" }
func (c *Cline) DisplayName() string { return "Cline CLI" }

func (c *Cline) lookPath(file string) (string, error) {
	if c.LookPath != nil {
		return c.LookPath(file)
	}
	return exec.LookPath(file)
}

// Command builds the cline invocation. Pure: nothing written, nothing
// spawned. The key goes on argv (-k) because nothing else reaches the
// session; see the type comment for the measurements. The env var is set as
// well, for the cold-start case where our client is what spawns the hub
// daemon and the daemon inherits our environment instead of a stray export.
func (c *Cline) Command(req Request) (Command, error) {
	key, err := c.Provider.Credential("cline", req.APIKey)
	if err != nil {
		return Command{}, err
	}
	if err := rejectModelFlag(c.Host, "cline", req.ExtraArgs); err != nil {
		return Command{}, err
	}
	if err := rejectFlags(c.Host, "cline", req.ExtraArgs, "-P", "--provider", "-k", "--key"); err != nil {
		return Command{}, err
	}
	path, err := c.lookPath("cline")
	if err != nil {
		return Command{}, fmt.Errorf("cline binary not found: %w", err)
	}
	args := []string{"-P", c.Provider.ID, "-m", req.Model.ID, "-k", key}
	args = append(args, req.ExtraArgs...)
	env := []string{c.Provider.EnvEntry(key)}
	return Command{Path: path, Args: args, Env: env}, nil
}

// clineProvidersFile is the CLI's provider store: what -k gets persisted
// into, and what ShadowedCredential reads.
func clineProvidersFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cline", "data", "settings", "providers.json"), nil
}

// Apply snapshots cline's provider store and returns the restore that puts it
// back. It is the mirror image of droid's ConfigWriter: droid's Apply writes
// the agent's config itself, whereas here the AGENT does the writing — -k
// makes cline save our key as settings.apiKey — and our only job is to
// guarantee the write does not outlive the session. Implementing the
// capability is also what puts cline on the fork-and-wait launch path, which
// is the only way any restore of ours can run.
//
// Nothing is parsed: the snapshot is raw bytes, so a provider store in a
// shape we do not recognise still round-trips exactly.
//
// The Request is deliberately unused — the signal that this Apply configures
// nothing. Model and key both reach cline on argv (Command), so there is
// nothing here to derive from the request; taking it is the interface's
// shape, not a need of ours.
//
// NOT SAFE against a concurrent launcher session of cline. The snapshot is
// taken from whatever is on disk, so two overlapping sessions interleave as:
// A snapshots the user's clean file, cline persists our key, B snapshots THAT
// (key included), A restores clean, B restores the copy holding the key —
// which then outlives every session, the one outcome this function exists to
// prevent. Serialising it needs a lock held across Apply, the run, and the
// restore, i.e. one owned by the launch service rather than by this method,
// and a lock file is a sixth write site under Landmine 6. Documented in
// README "Known caveats" instead, by owner decision (2026-08-16); the tool
// assumes one session at a time throughout.
func (c *Cline) Apply(_ Request) (func() error, error) {
	path, err := clineProvidersFile()
	if err != nil {
		return nil, err
	}
	prior, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		// Deliberately not best-effort, unlike a CredentialShadowCheck: a file
		// we cannot snapshot is a file we cannot put back, and launching anyway
		// would leave the user's key persisted in cline's store for every later
		// session. Refuse instead.
		return nil, fmt.Errorf("cline: cannot read %s (%w); refusing to launch, because -k would persist your key there with no way to restore the file", path, readErr)
	}
	if readErr != nil {
		// No provider store to preserve: cline is free to create one, and
		// restore's job is to make sure our key does not stay behind as the
		// machine's new saved credential.
		return func() error {
			// The user may have removed it themselves; that is not a failure.
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}, nil
	}

	// Snapshot the mode too, and from the file we just read: this store holds
	// an API key, so restore must reproduce the user's permissions rather than
	// pick a default of its own.
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mode := info.Mode().Perm()
	return func() error {
		return writeClineProvidersFile(path, prior, mode)
	}, nil
}

// writeClineProvidersFile restores the snapshot atomically: temp file in the
// same dir, then rename (the Landmine 9 shape). The mode is the one the file
// had — this store holds an API key, so restore must never widen it.
func writeClineProvidersFile(path string, contents []byte, mode os.FileMode) error {
	// 0700: cline creates this directory 0700 itself, and it holds a
	// credential. Only reachable if the tree was removed mid-session —
	// MkdirAll leaves an existing directory's mode alone.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".providers-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// The Close/Remove calls below are best-effort cleanup on paths already
	// returning a real error; reporting their failures would mask the error
	// that explains why the restore failed. The explicit `_ =` marks the
	// choice so errcheck does not have to guess.
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(contents); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// CheckInstalled reports whether the cline binary can be found. npm global
// installs land on PATH; there is no home-dir fallback.
func (c *Cline) CheckInstalled() bool {
	_, err := c.lookPath("cline")
	return err == nil
}

// InstallHint tells the user how to install the Cline CLI.
func (c *Cline) InstallHint() string {
	return "Install Cline CLI: npm install -g cline"
}

// Cline deliberately does NOT implement CredentialShadowCheck. It used to,
// warning that a saved key in providers.json outranks the launch's key — true
// of the env-only launcher, false now. Cline's own resolution order is
// explicit key, then OAuth resolver, then env (read from the binary's
// resolveApiKey), so the -k this launcher always passes wins over both a
// saved provider key and the hub daemon's environment; both were measured on
// 3.0.52. A warning whose premise no longer holds is worse than none, and
// Apply already guarantees our key does not become the next launch's shadow.
