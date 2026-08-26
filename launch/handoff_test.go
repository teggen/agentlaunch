package launch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/agent"
	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/config"
)

// testPlan is a minimal already-resolved Plan.
func testPlan() Plan {
	return Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Model:   catalog.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/bin/fake", Args: []string{"--model", "x"}, Env: []string{"K=V"}},
	}
}

// blockConfigWrites points XDG_CONFIG_HOME at a regular file, so the config
// directory cannot be created. config.Load fails first (ENOTDIR) and
// returns, so the selection is never recorded and config.Save is never
// reached in these tests.
func blockConfigWrites(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
}

// The ordering here is the whole reason save and handoff live in one
// function. On Unix the handoff is syscall.Exec, which replaces the process:
// a save placed after it would never run, and the stub used here returns
// normally, so the bug would be invisible to any end-state assertion. This
// inspects the config from inside the handoff itself.
func TestLaunchSavesSelectionBeforeHandoff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var savedBeforeHandoff bool
	svc := &Service{Run: func(agent.Command) error {
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("config.Load inside the handoff: %v", err)
		}
		savedBeforeHandoff = cfg.LastAgent == "fake" &&
			cfg.LastModel == "anthropic/claude-opus-4.6"
		return nil
	}}

	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !savedBeforeHandoff {
		t.Error("the selection must be persisted before control reaches the handoff")
	}
}

func TestLaunchRecordsAgentAndModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.LastAgent != "fake" {
		t.Errorf("LastAgent = %q", cfg.LastAgent)
	}
	if cfg.LastModel != "anthropic/claude-opus-4.6" {
		t.Errorf("LastModel = %q", cfg.LastModel)
	}
}

func TestLaunchWarnsButProceedsWhenSelectionCannotBeSaved(t *testing.T) {
	blockConfigWrites(t)

	var handedOff bool
	var got []Warning
	svc := &Service{Run: func(agent.Command) error { handedOff = true; return nil }}

	err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) })
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	// Failing to remember the last selection is a convenience loss, not a
	// reason to refuse to start the agent.
	if !handedOff {
		t.Error("a failed save must not block the launch")
	}
	if len(got) != 1 {
		t.Fatalf("warnings = %+v, want exactly one", got)
	}
	if got[0].Kind != WarnSelectionNotSaved {
		t.Errorf("Kind = %v, want WarnSelectionNotSaved", got[0].Kind)
	}
	if got[0].Question != "" {
		t.Error("an unsaved selection is informational; it must not gate the launch on an answer")
	}
}

// warn is documented as optional. The crash case is a save failure with no
// callback to report it to, so that is what this exercises.
func TestLaunchNilWarnIsSafeOnSaveFailure(t *testing.T) {
	blockConfigWrites(t)

	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
}

func TestLaunchDoesNotWarnOnSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got []Warning
	svc := &Service{Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) }); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warnings = %+v, want none on a successful save", got)
	}
}

func TestLaunchHandsOffTheBuiltCommandUnchanged(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var got agent.Command
	svc := &Service{Run: func(c agent.Command) error { got = c; return nil }}
	p := testPlan()

	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !reflect.DeepEqual(got, p.Command) {
		t.Errorf("Launch handed off %+v, want %+v unchanged", got, p.Command)
	}
}

func TestLaunchPropagatesHandoffError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := errors.New("exec failed")
	svc := &Service{Run: func(agent.Command) error { return want }}

	if err := svc.Launch(testPlan(), nil); !errors.Is(err, want) {
		t.Fatalf("Launch returned %v, want %v", err, want)
	}
}

func TestLaunchStagesFilesBeforeRun(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	staged := agent.StagedFile{
		Path:     filepath.Join(dir, "openclaw.json"),
		Contents: []byte(`{"agents":{}}`),
		Mode:     0o644,
	}
	var contentAtRun []byte
	svc := &Service{Run: func(agent.Command) error {
		contentAtRun, _ = os.ReadFile(staged.Path)
		return nil
	}}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{staged},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if string(contentAtRun) != string(staged.Contents) {
		t.Errorf("at run time file held %q, want %q — staging must precede the handoff", contentAtRun, staged.Contents)
	}
	if _, err := os.Stat(staged.Path); err != nil {
		t.Fatal(err)
	}
	// The mode is only assertable on Unix (see internal/config's
	// TestSaveWritesFileMode0600): Windows reports 0666 for any writable
	// file. Guarding just this assertion rather than skipping the test
	// keeps the staging-precedes-handoff ordering — the thing this test is
	// actually named for — covered on all three platforms.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(staged.Path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v, want 0644", info.Mode().Perm())
		}
	}
}

// TestLaunchStagesIntoAPrivateDir pins least privilege on the directory
// stageFiles creates for write site #3. It is our own config dir — the same
// one holding the 0600 API key file — so nothing outside this process needs
// to reach it. The assertion is on the group/other bits rather than an exact
// mode so a stricter umask does not fail it spuriously; under the usual 0022
// it is exactly 0700.
func TestLaunchStagesIntoAPrivateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Run: func(agent.Command) error { return nil }}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged: []agent.StagedFile{{
			Path:     filepath.Join(dir, "openclaw.json"),
			Contents: []byte(`{}`),
			Mode:     0o600,
		}},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("staged-file dir mode = %o, want no group or other access (0700)", perm)
	}
}

// TestLaunchStagedFileModeIsAppliedOverAnExistingFile pins the reason write
// site #3 uses the temp+rename shape (the Landmine 9 form) rather than a
// plain os.WriteFile, which is what it used to do.
//
// os.WriteFile passes its mode to open(2) as the CREATE mode, and open
// applies that only when it creates the file. Against an existing path the
// mode argument is silently ignored, so a staged file that once landed with
// a wide mode keeps it forever no matter what StagedFiles declares — the
// declared 0600 becomes a value the code states and does not enforce.
// Rename-over-a-temp-file has no such gap: the mode belongs to the new inode
// and replaces whatever was there.
//
// Both directions are exercised deliberately. The narrowing case (0644 file,
// 0600 declared) is the one that matters in production, but it CANNOT on its
// own prove the mode was applied: os.CreateTemp already creates its file
// 0600, so that case passes with the chmod deleted. Only the broadening case
// distinguishes "we set the declared mode" from "the temp file's default
// happened to be the answer".
func TestLaunchStagedFileModeIsAppliedOverAnExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	tests := []struct {
		name     string
		existing os.FileMode
		declared os.FileMode
	}{
		{"narrows a stale broad file", 0o644, 0o600},
		{"broadens past the temp file's 0600 default", 0o600, 0o644},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			dir, err := config.Dir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "openclaw.json")
			// A stale file from an earlier run, at the mode this case starts from.
			if err := os.WriteFile(path, []byte(`{"stale":true}`), tt.existing); err != nil {
				t.Fatal(err)
			}
			// Chmod separately: os.WriteFile's mode is subject to the umask,
			// which would otherwise narrow the starting point out from under us.
			if err := os.Chmod(path, tt.existing); err != nil {
				t.Fatal(err)
			}

			svc := &Service{Run: func(agent.Command) error { return nil }}
			p := Plan{
				Spec:    spec("fake", &fakeLauncher{}),
				Command: agent.Command{Path: "/bin/true"},
				Staged: []agent.StagedFile{{
					Path:     path,
					Contents: []byte(`{"agents":{}}`),
					Mode:     tt.declared,
				}},
			}
			if err := svc.Launch(p, nil); err != nil {
				t.Fatalf("Launch: %v", err)
			}

			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != tt.declared {
				t.Errorf("mode = %o, want %o: the declared mode must replace the existing file's", got, tt.declared)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != `{"agents":{}}` {
				t.Errorf("contents = %q, want the staged contents", got)
			}
		})
	}
}

// TestLaunchStagedFileLeavesNoTempBehind guards the other half of the
// temp+rename shape: the temp file must be renamed into place, not left
// beside it. Our config dir is also the openclaw config dir for the session,
// so a stray .openclaw-*.json is a file the agent may try to read.
func TestLaunchStagedFileLeavesNoTempBehind(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Run: func(agent.Command) error { return nil }}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged: []agent.StagedFile{{
			Path:     filepath.Join(dir, "openclaw.json"),
			Contents: []byte(`{}`),
			Mode:     0o600,
		}},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Asserting on the dot prefix rather than on an exact directory listing:
	// Launch also writes config.json here via recordSelection, and pinning the
	// full listing would make this test fail for that unrelated reason. Staged
	// temp files are dot-prefixed, the same convention writeClineProvidersFile
	// uses.
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("leftover temp file %q in the config dir; it must be renamed into place, not abandoned", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "openclaw.json")); err != nil {
		t.Errorf("staged file missing after Launch: %v", err)
	}
}

func TestLaunchRefusesStagedFileOutsideConfigDir(t *testing.T) {
	// Case 1: unrelated directory (no shared prefix)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	outside := filepath.Join(t.TempDir(), "evil.json")
	ran := false
	svc := &Service{Run: func(agent.Command) error { ran = true; return nil }}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{{Path: outside, Contents: []byte("x"), Mode: 0o644}},
	}
	err := svc.Launch(p, nil)
	if err == nil {
		t.Fatal("Launch staged a file outside the launcher config dir (unrelated dir case)")
	}
	if ran {
		t.Error("run happened despite staging failure (unrelated dir case)")
	}
	if _, statErr := os.Stat(outside); statErr == nil {
		t.Error("the outside file was written (unrelated dir case)")
	}

	// Case 2: sibling directory sharing the string prefix (distinguishes filepath.Rel from naive strings.HasPrefix)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // fresh temp dir for this case
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	// Create a sibling dir that shares the string prefix but is not a subdirectory
	// e.g., dir=/tmp/xyz/cfg → sibling=/tmp/xyz/cfg-evil
	siblingDir := dir + "-evil"
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	siblingPath := filepath.Join(siblingDir, "staged.json")

	ran = false
	svc = &Service{Run: func(agent.Command) error { ran = true; return nil }}
	p = Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{{Path: siblingPath, Contents: []byte("x"), Mode: 0o644}},
	}
	err = svc.Launch(p, nil)
	if err == nil {
		t.Fatal("Launch staged a file in sibling dir with shared prefix")
	}
	if ran {
		t.Error("run happened despite staging failure (sibling case)")
	}
	if _, statErr := os.Stat(siblingPath); statErr == nil {
		t.Error("the sibling file was written")
	}
}

type recordingConfigWriter struct {
	fakeLauncher
	log        *[]string
	applyErr   error
	restoreErr error
}

func (r *recordingConfigWriter) Apply(agent.Request) (func() error, error) {
	if r.applyErr != nil {
		return nil, r.applyErr
	}
	*r.log = append(*r.log, "apply")
	return func() error {
		*r.log = append(*r.log, "restore")
		return r.restoreErr
	}, nil
}

func TestLaunchConfigWriterOrderApplyRunRestore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	svc := &Service{
		RunWait: func(agent.Command) error { log = append(log, "run"); return nil },
		Run: func(agent.Command) error {
			t.Error("exec-style Run used for a ConfigWriter agent")
			return nil
		},
	}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log}),
		Command: agent.Command{Path: "/bin/true"},
	}
	if err := svc.Launch(p, nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if want := []string{"apply", "run", "restore"}; !slices.Equal(log, want) {
		t.Errorf("order = %v, want %v", log, want)
	}
}

func TestLaunchConfigWriterRestoreRunsOnRunFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	runErr := errors.New("agent exited 1")
	svc := &Service{RunWait: func(agent.Command) error { return runErr }}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log}),
		Command: agent.Command{Path: "/bin/true"},
	}
	err := svc.Launch(p, nil)
	if !errors.Is(err, runErr) {
		t.Errorf("err = %v, want the run error preserved (main extracts the exit code from it)", err)
	}
	if !slices.Contains(log, "restore") {
		t.Error("restore did not run after a failed session")
	}
}

func TestLaunchConfigWriterApplyFailureSkipsRun(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	ran := false
	svc := &Service{RunWait: func(agent.Command) error { ran = true; return nil }}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log, applyErr: errors.New("settings file unparseable")}),
		Command: agent.Command{Path: "/bin/true"},
	}
	if err := svc.Launch(p, nil); err == nil {
		t.Fatal("Launch succeeded despite Apply failure")
	}
	if ran {
		t.Error("agent ran despite Apply failure")
	}
}

func TestLaunchConfigWriterRestoreErrorPreservesRunError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var log []string
	runErr := errors.New("agent exited 42")
	restoreErr := errors.New("backup file corrupted")
	svc := &Service{RunWait: func(agent.Command) error { return runErr }}
	p := Plan{
		Spec:    spec("fake", &recordingConfigWriter{log: &log, restoreErr: restoreErr}),
		Command: agent.Command{Path: "/bin/true"},
	}
	err := svc.Launch(p, nil)
	if !errors.Is(err, runErr) {
		t.Errorf("err = %v, want runErr %v preserved (main extracts the exit code from it)", err, runErr)
	}
	if !slices.Contains(log, "restore") {
		t.Error("restore did not run despite its error")
	}
	if !strings.Contains(err.Error(), "restore") {
		t.Errorf("error text = %q, want to contain 'restore' (the restore failure must not be swallowed)", err.Error())
	}
}
