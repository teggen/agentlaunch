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
)

// testPlan is a minimal already-resolved Plan.
func testPlan() Plan {
	return Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Model:   catalog.Model{ID: "anthropic/claude-opus-4.6"},
		Command: agent.Command{Path: "/bin/fake", Args: []string{"--model", "x"}, Env: []string{"K=V"}},
	}
}

// recorder is an in-memory stand-in for whatever settings store the caller
// wires Service.RecordSelection to. These tests are about Launch's ORDER and
// its failure handling, not about any particular store, and this package no
// longer knows that this tool's store is a JSON file under XDG_CONFIG_HOME.
type recorder struct {
	agent, model string
	calls        int
	err          error
}

func (r *recorder) record(agentName, modelID string) error {
	r.calls++
	if r.err != nil {
		return r.err
	}
	r.agent, r.model = agentName, modelID
	return nil
}

// stageRoot returns a staging directory that does NOT yet exist, so the mode
// stageFiles creates it with stays observable. It is the shape config.Dir
// returns — a named subdirectory of a parent that already exists — which is
// what these tests used before Service took the directory as a field.
func stageRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "openrouter-launch")
}

// staticDir is a Service.StageDir that always answers dir.
func staticDir(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// The ordering here is the whole reason save and handoff live in one
// function. On Unix the handoff is syscall.Exec, which replaces the process:
// a save placed after it would never run, and the stub used here returns
// normally, so the bug would be invisible to any end-state assertion. This
// inspects the config from inside the handoff itself.
func TestLaunchSavesSelectionBeforeHandoff(t *testing.T) {
	rec := &recorder{}
	var savedBeforeHandoff bool
	svc := &Service{
		RecordSelection: rec.record,
		Run: func(agent.Command) error {
			savedBeforeHandoff = rec.agent == "fake" &&
				rec.model == "anthropic/claude-opus-4.6"
			return nil
		},
	}

	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !savedBeforeHandoff {
		t.Error("the selection must be persisted before control reaches the handoff")
	}
}

func TestLaunchRecordsAgentAndModel(t *testing.T) {
	rec := &recorder{}
	svc := &Service{RecordSelection: rec.record, Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if rec.agent != "fake" {
		t.Errorf("recorded agent = %q", rec.agent)
	}
	if rec.model != "anthropic/claude-opus-4.6" {
		t.Errorf("recorded model = %q", rec.model)
	}
}

// TestLaunchWithNoRecorderStillHandsOff pins the nil case as a supported
// configuration rather than an oversight: a tool that does not remember the
// last selection leaves RecordSelection nil, and that is silent, not a
// warning — distinct from trying to record and failing, below.
func TestLaunchWithNoRecorderStillHandsOff(t *testing.T) {
	var handedOff bool
	var got []Warning
	svc := &Service{Run: func(agent.Command) error { handedOff = true; return nil }}

	if err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) }); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !handedOff {
		t.Error("a Service with no RecordSelection must still hand off")
	}
	if len(got) != 0 {
		t.Errorf("warnings = %+v, want none: not recording is not a failure to record", got)
	}
}

func TestLaunchWarnsButProceedsWhenSelectionCannotBeSaved(t *testing.T) {
	rec := &recorder{err: errors.New("settings are read-only")}

	var handedOff bool
	var got []Warning
	svc := &Service{
		RecordSelection: rec.record,
		Run:             func(agent.Command) error { handedOff = true; return nil },
	}

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
	rec := &recorder{err: errors.New("settings are read-only")}

	svc := &Service{RecordSelection: rec.record, Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), nil); err != nil {
		t.Fatalf("Launch: %v", err)
	}
}

func TestLaunchDoesNotWarnOnSuccess(t *testing.T) {
	rec := &recorder{}

	var got []Warning
	svc := &Service{RecordSelection: rec.record, Run: func(agent.Command) error { return nil }}
	if err := svc.Launch(testPlan(), func(w Warning) { got = append(got, w) }); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("warnings = %+v, want none on a successful save", got)
	}
}

func TestLaunchHandsOffTheBuiltCommandUnchanged(t *testing.T) {
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
	want := errors.New("exec failed")
	svc := &Service{Run: func(agent.Command) error { return want }}

	if err := svc.Launch(testPlan(), nil); !errors.Is(err, want) {
		t.Fatalf("Launch returned %v, want %v", err, want)
	}
}

func TestLaunchStagesFilesBeforeRun(t *testing.T) {
	dir := stageRoot(t)
	staged := agent.StagedFile{
		Path:     filepath.Join(dir, "openclaw.json"),
		Contents: []byte(`{"agents":{}}`),
		Mode:     0o644,
	}
	var contentAtRun []byte
	svc := &Service{
		StageDir: staticDir(dir),
		Run: func(agent.Command) error {
			contentAtRun, _ = os.ReadFile(staged.Path)
			return nil
		},
	}
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
	dir := stageRoot(t)
	svc := &Service{StageDir: staticDir(dir), Run: func(agent.Command) error { return nil }}
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
			dir := stageRoot(t)
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

			svc := &Service{StageDir: staticDir(dir), Run: func(agent.Command) error { return nil }}
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
	dir := stageRoot(t)
	svc := &Service{StageDir: staticDir(dir), Run: func(agent.Command) error { return nil }}
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
	// in production this is also the tool's own config dir, so config.json
	// and anything else it keeps sits here too, and pinning the full listing
	// would make this test fail for those unrelated reasons. Staged temp
	// files are dot-prefixed, the same convention writeClineProvidersFile
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
	dir := stageRoot(t)
	outside := filepath.Join(t.TempDir(), "evil.json")
	ran := false
	svc := &Service{StageDir: staticDir(dir), Run: func(agent.Command) error { ran = true; return nil }}
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
	dir = stageRoot(t) // fresh temp dir for this case
	// Create a sibling dir that shares the string prefix but is not a subdirectory
	// e.g., dir=/tmp/xyz/cfg → sibling=/tmp/xyz/cfg-evil
	siblingDir := dir + "-evil"
	if err := os.MkdirAll(siblingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	siblingPath := filepath.Join(siblingDir, "staged.json")

	ran = false
	svc = &Service{StageDir: staticDir(dir), Run: func(agent.Command) error { ran = true; return nil }}
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

// TestLaunchRefusesToStageWithNoStageDir pins the refusal rather than a
// fallback. An empty directory is not a harmless default: filepath.Join("",
// "openclaw.json") is a path in the WORKING directory, so a Service that
// guessed would put write site #3 wherever the user happened to be — outside
// the sanctioned dir, which is Landmine 6 — while every path assertion in
// this file still passed.
func TestLaunchRefusesToStageWithNoStageDir(t *testing.T) {
	target := filepath.Join(t.TempDir(), "openclaw.json")
	ran := false
	svc := &Service{Run: func(agent.Command) error { ran = true; return nil }}
	p := Plan{
		Spec:    spec("fake", &fakeLauncher{}),
		Command: agent.Command{Path: "/bin/true"},
		Staged:  []agent.StagedFile{{Path: target, Contents: []byte("x"), Mode: 0o600}},
	}

	err := svc.Launch(p, nil)
	if err == nil {
		t.Fatal("Launch staged a file with no StageDir configured")
	}
	// The whole sentence, not just the field name: t.TempDir() puts the test's
	// own name in the path, so a substring check for "StageDir" alone is
	// satisfied by the staged path quoted in ANY staging error — including the
	// "outside the launcher config dir" one a fallback to "" would produce.
	if !strings.Contains(err.Error(), "Service.StageDir is required") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
	if ran {
		t.Error("the handoff happened despite the staging failure")
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("the staged file was written anyway")
	}
}
