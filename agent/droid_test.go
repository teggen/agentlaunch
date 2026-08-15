package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func droidSettingsPath(t *testing.T, home string) string {
	t.Helper()
	return filepath.Join(home, ".factory", "settings.local.json")
}

func readDroidSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return m
}

func TestDroidCommandArgsAndEnv(t *testing.T) {
	d := &Droid{LookPath: stubLookPath("/usr/local/bin/droid")}
	cmd, err := d.Command(Request{Model: testModel(), APIKey: "sk-or-test", ExtraArgs: []string{"exec", "hi"}})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	// No -m: model selection lives in the settings file Apply writes. The
	// index-derived custom: ID is only knowable at Apply time, and Command
	// is pure.
	if want := []string{"exec", "hi"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if got, ok := envValue(cmd.Env, "OPENROUTER_API_KEY"); !ok || got != "sk-or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, %v", got, ok)
	}
	for _, extras := range [][]string{{"-m", "x"}, {"--model", "x"}, {"--model=x"}} {
		if _, err := d.Command(Request{Model: testModel(), APIKey: "k", ExtraArgs: extras}); err == nil {
			t.Errorf("extras %q accepted, want conflict error", extras)
		}
	}
}

func TestDroidApplyFreshFile(t *testing.T) {
	home := testHome(t)
	d := &Droid{}

	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := droidSettingsPath(t, home)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-or-test") {
		t.Fatal("the real key was written to disk; only ${OPENROUTER_API_KEY} may appear")
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("customModels has %d entries, want 1", len(models))
	}
	entry := models[0].(map[string]any)
	for key, want := range map[string]string{
		"displayName": "openrouter-launch",
		"provider":    "generic-chat-completion-api",
		"baseUrl":     "https://openrouter.ai/api/v1",
		"model":       "anthropic/claude-opus-4.6",
		"apiKey":      "${OPENROUTER_API_KEY}",
	} {
		if entry[key] != want {
			t.Errorf("entry[%q] = %v, want %q", key, entry[key], want)
		}
	}
	if m["model"] != "custom:openrouter-launch-0" {
		t.Errorf("model = %v, want custom:openrouter-launch-0", m["model"])
	}
	// Unix-only, like the assertion in TestDroidPreservesSettingsFileMode:
	// Windows reports 0666 for any writable file.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil {
			t.Fatal(err)
		} else if info.Mode().Perm() != 0o644 {
			t.Errorf("fresh-create mode = %v, want 0644 (no prior file to preserve)", info.Mode().Perm())
		}
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("restore left behind a file we created into an empty state")
	}
}

// TestDroidCreatesFactoryDirWithoutWorldAccess pins the one directory we
// create that we do not own. ~/.factory is droid's, so this stops at
// removing world access rather than going to 0700 like our own dirs — the
// minimum that satisfies least privilege without narrowing another tool's
// directory further than it asked for. It only ever applies to a droid
// install that has never run; an existing ~/.factory keeps its own mode,
// because MkdirAll leaves an existing directory alone.
func TestDroidCreatesFactoryDirWithoutWorldAccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	home := testHome(t)

	if _, err := (&Droid{}).Apply(Request{Model: testModel(), APIKey: "sk-or-test"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, err := os.Stat(filepath.Dir(droidSettingsPath(t, home)))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o007 != 0 {
		t.Errorf("~/.factory mode = %o, want no world access (0750)", perm)
	}
}

// TestDroidPreservesSettingsFileMode pins the mode-preservation fix: a
// user's settings.local.json commonly holds foreign customModels entries
// with REAL apiKey values (ours is the only interpolated one), so Apply and
// restore must never silently broaden a 0600 file to 0644.
// Apply must not widen the permissions of a settings file the user already
// owns — ~/.factory is the agent's, not ours (write site #4).
//
// Unix-only, matching TestSaveWritesFileMode0600 in internal/config: Windows
// has no permission bits for Stat to report. os.Chmod there toggles only the
// read-only attribute, so a writable file always comes back 0666 and this
// assertion could never hold. The property itself is Unix-only too — on
// Windows the file inherits the directory's ACL, which this tool does not
// set.
func TestDroidPreservesSettingsFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := `{"customModels":[{"displayName":"Mine","provider":"generic-chat-completion-api","baseUrl":"http://mine","model":"m","apiKey":"a-real-secret-key"}]}`
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(prior), 0o600); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("after Apply: mode = %v, want 0600 preserved", info.Mode().Perm())
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("after restore: mode = %v, want 0600 preserved", info.Mode().Perm())
	}
}

// TestDroidRestoreToleratesFileAlreadyDeleted pins the ENOENT guard: if the
// user deletes settings.local.json themselves mid-session, restore's own
// cleanup os.Remove must not turn that into a restore error.
func TestDroidRestoreToleratesFileAlreadyDeleted(t *testing.T) {
	home := testHome(t)
	d := &Droid{}

	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := droidSettingsPath(t, home)
	if err := os.Remove(path); err != nil {
		t.Fatalf("simulate user deletion: %v", err)
	}

	if err := restore(); err != nil {
		t.Errorf("restore after the user already deleted the file: %v, want nil", err)
	}
}

func TestDroidApplyPreservesForeignEntriesAndPriorDefault(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := `{"model":"gpt-5.5-codex","customModels":[{"displayName":"Mine","provider":"generic-chat-completion-api","baseUrl":"http://mine","model":"m","apiKey":"k"}],"theme":"dark"}`
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}

	// Parse the original foreign entry to verify it is preserved unchanged
	var priorSettings map[string]any
	if err := json.Unmarshal([]byte(prior), &priorSettings); err != nil {
		t.Fatalf("parse prior: %v", err)
	}
	priorForeignEntry := priorSettings["customModels"].([]any)[0]

	d := &Droid{}
	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 2 {
		t.Fatalf("customModels has %d entries, want 2 (theirs + ours)", len(models))
	}
	if models[0].(map[string]any)["displayName"] != "Mine" {
		t.Error("foreign entry displaced from index 0")
	}
	// Deep-equal the foreign entry to ensure it matches the original exactly
	if !reflect.DeepEqual(models[0], priorForeignEntry) {
		t.Errorf("foreign entry after Apply = %v, want unchanged %v", models[0], priorForeignEntry)
	}
	// Ours is at index 1, so the selection ID must say 1.
	if m["model"] != "custom:openrouter-launch-1" {
		t.Errorf("model = %v, want custom:openrouter-launch-1", m["model"])
	}
	if m["theme"] != "dark" {
		t.Error("unrelated setting clobbered")
	}

	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	m = readDroidSettings(t, path)
	if m["model"] != "gpt-5.5-codex" {
		t.Errorf("restore: model = %v, want prior gpt-5.5-codex", m["model"])
	}
	models = m["customModels"].([]any)
	if len(models) != 1 {
		t.Errorf("restore: customModels has %d entries, want 1", len(models))
	}
	// Deep-equal the foreign entry again to ensure restore preserved it exactly
	if !reflect.DeepEqual(models[0], priorForeignEntry) {
		t.Errorf("foreign entry after restore = %v, want unchanged %v", models[0], priorForeignEntry)
	}
	if m["theme"] != "dark" {
		t.Errorf("restore: theme = %v, want dark", m["theme"])
	}
}

func TestDroidApplyReplacesStaleMarkerEntry(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A crashed prior run left our marker entry with an old model.
	stale := `{"customModels":[{"displayName":"openrouter-launch","provider":"generic-chat-completion-api","baseUrl":"https://openrouter.ai/api/v1","model":"old/model","apiKey":"${OPENROUTER_API_KEY}"}]}`
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	m := readDroidSettings(t, path)
	models := m["customModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("stale marker not replaced: %d entries", len(models))
	}
	if got := models[0].(map[string]any)["model"]; got != "anthropic/claude-opus-4.6" {
		t.Errorf("model = %v, want the fresh slug", got)
	}
}

func TestDroidApplyRefusesUnparseableFile(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.local.json")
	if err := os.WriteFile(path, []byte(`{definitely not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err == nil {
		t.Fatal("Apply clobbered a file it could not parse")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{definitely not json` {
		t.Error("unparseable file was modified")
	}
}

func TestDroidRestoreKeepsFileWhenUserAddedEntriesMidSession(t *testing.T) {
	home := testHome(t)
	d := &Droid{}

	// Apply on a fresh HOME (no file) — creates the file
	restore, err := d.Apply(Request{Model: testModel(), APIKey: "sk-or-test"})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	path := droidSettingsPath(t, home)

	// Simulate the user adding a foreign entry mid-session by rewriting the file
	midSessionSettings := map[string]any{
		"customModels": []any{
			// Our marker entry (what Apply wrote)
			map[string]any{
				"displayName":     "openrouter-launch",
				"provider":        "generic-chat-completion-api",
				"baseUrl":         "https://openrouter.ai/api/v1",
				"model":           "anthropic/claude-opus-4.6",
				"apiKey":          "${OPENROUTER_API_KEY}",
				"maxOutputTokens": float64(64000),
			},
			// User's foreign entry added mid-session
			map[string]any{
				"displayName": "UserAdded",
				"provider":    "generic-chat-completion-api",
				"baseUrl":     "http://custom",
				"model":       "custom/model",
				"apiKey":      "user-key",
			},
		},
		"model": "custom:openrouter-launch-0",
	}
	data, err := json.MarshalIndent(midSessionSettings, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Restore should preserve the foreign entry and delete only our marker + model key
	if err := restore(); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// File should still exist (foreign entry keeps it alive)
	m := readDroidSettings(t, path)
	models, ok := m["customModels"].([]any)
	if !ok || len(models) != 1 {
		t.Errorf("customModels = %v, want one foreign entry", models)
	}
	if got := models[0].(map[string]any)["displayName"]; got != "UserAdded" {
		t.Errorf("foreign entry displayName = %v, want UserAdded", got)
	}
	// Our marker + model key should be gone
	if _, ok := m["model"]; ok {
		t.Errorf("model key still present after restore")
	}
}

// TestDroidApplyRefusesWrongShapedCustomModels extends "never clobber what we
// cannot understand" from the whole-file parse to the one field we rewrite.
// A customModels that is valid JSON of another type used to fail the type
// assertion silently: Apply replaced the user's value and restore then took
// the len(kept)==0 branch and deleted the key outright.
func TestDroidApplyRefusesWrongShapedCustomModels(t *testing.T) {
	home := testHome(t)
	dir := filepath.Join(home, ".factory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.local.json")
	original := `{"customModels":"see the other file","model":"custom:theirs"}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	d := &Droid{}
	if _, err := d.Apply(Request{Model: testModel(), APIKey: "sk"}); err == nil {
		t.Fatal("Apply accepted a customModels it could not understand")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("file was modified:\n got %s\nwant %s", raw, original)
	}
}
