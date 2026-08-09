package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// testHome points every home-directory lookup in this package at a fresh
// temp dir, on every platform, and returns that dir.
//
// `t.Setenv("HOME", dir)` alone is NOT enough — the gap Landmine 8 did not
// account for. os.UserHomeDir reads HOME on Unix but USERPROFILE on
// Windows, so on Windows the agent code kept resolving the real user's
// home. These tests did not merely fail there: Droid.Apply WROTE
// ~/.factory/settings.local.json into the developer's actual profile every
// time `go test ./internal/agent/` ran.
//
// APPDATA and LOCALAPPDATA are redirected for the same reason —
// Hermes.findPath and Qwen.findPath consult them on Windows, so leaving
// them pointed at the real profile lets a genuinely installed agent
// satisfy a test that needs the binary to be absent.
func testHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)        // Unix, and anything reading it directly
	t.Setenv("USERPROFILE", dir) // Windows: what os.UserHomeDir actually reads
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	return dir
}

// The property every findPath and ShadowedCredential test in this package
// silently depends on.
//
// One assertion covers both platforms: it holds through HOME on Unix and
// through USERPROFILE on Windows. That is the point — the platform that
// used to be unisolated is now pinned by the same line as the one that
// was, rather than by a Windows-only test nobody runs locally.
func TestTestHomeIsolatesTheHomeDirectoryOnEveryPlatform(t *testing.T) {
	dir := testHome(t)

	got, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if got != dir {
		t.Errorf("os.UserHomeDir() = %q, want the test's temp home %q", got, dir)
	}
}
