package agent

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestExecArgsPrependsBinaryPath(t *testing.T) {
	argv, _ := ExecArgs(Command{
		Path: "/usr/local/bin/claude",
		Args: []string{"--model", "anthropic/claude-opus-4.6"},
	})
	want := []string{"/usr/local/bin/claude", "--model", "anthropic/claude-opus-4.6"}
	if !slices.Equal(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestExecArgsAppendsToProcessEnvironment(t *testing.T) {
	t.Setenv("OPENROUTER_LAUNCH_TEST_MARKER", "present")

	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"ANTHROPIC_API_KEY=sk-or-test"}})

	var foundMarker, foundNew bool
	for _, e := range env {
		if e == "OPENROUTER_LAUNCH_TEST_MARKER=present" {
			foundMarker = true
		}
		if e == "ANTHROPIC_API_KEY=sk-or-test" {
			foundNew = true
		}
	}
	if !foundMarker {
		t.Error("inherited environment entry missing")
	}
	if !foundNew {
		t.Error("command environment entry missing")
	}
}

// TestExecArgsCommandEnvWinsOverInherited pins the fix for the environment
// precedence inversion: execve(2) does not deduplicate envp, and POSIX
// getenv returns the FIRST match, so simply appending the command's entries
// after the inherited ones (the old approach) let the inherited value win.
// ExecArgs must instead produce exactly one occurrence of a colliding key,
// carrying the command's value.
func TestExecArgsCommandEnvWinsOverInherited(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "inherited-value")

	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"ANTHROPIC_API_KEY=sk-or-test"}})

	var matches []string
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("got %d ANTHROPIC_API_KEY entries, want exactly 1: %v", len(matches), matches)
	}
	if matches[0] != "ANTHROPIC_API_KEY=sk-or-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want the command's value", matches[0])
	}
}

// TestExecArgsEnvLengthIsSumOfBoth pins the arithmetic in both directions: a
// colliding key must be deduplicated away (not counted twice), while a
// non-colliding key must be added (not dropped).
func TestExecArgsEnvLengthIsSumOfBoth(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "inherited-value") // colliding key
	inherited := os.Environ()

	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"ANTHROPIC_API_KEY=sk-or-test", "B=2"}})

	// len(inherited) + 2 command entries, minus 1 for the deduplicated collision.
	want := len(inherited) + 2 - 1
	if got := len(env); got != want {
		t.Errorf("env length = %d, want %d (len(inherited)=%d)", got, want, len(inherited))
	}
}

// TestExecArgsRealWorldANTHROPICOverride is the concrete scenario from the
// bug report: a user with ANTHROPIC_BASE_URL exported to their own value
// must not have it leak into the child process. If it did, Claude Code would
// silently run against the user's own Anthropic account instead of
// OpenRouter while the tool reported success.
func TestExecArgsRealWorldANTHROPICOverride(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")

	_, env := ExecArgs(Command{
		Path: "/bin/true",
		Env:  []string{"ANTHROPIC_BASE_URL=https://openrouter.ai/api"},
	})

	var matches []string
	for _, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_BASE_URL=") {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("got %d ANTHROPIC_BASE_URL entries, want exactly 1: %v", len(matches), matches)
	}
	if matches[0] != "ANTHROPIC_BASE_URL=https://openrouter.ai/api" {
		t.Errorf("ANTHROPIC_BASE_URL = %q, want our value", matches[0])
	}
}

// TestExecArgsMalformedCommandEnvEntryPassesThrough proves a Env entry with
// no '=' is passed through rather than crashing the key-parse.
func TestExecArgsMalformedCommandEnvEntryPassesThrough(t *testing.T) {
	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"NOEQUALSSIGN"}})

	var found bool
	for _, e := range env {
		if e == "NOEQUALSSIGN" {
			found = true
		}
	}
	if !found {
		t.Error("malformed command env entry was dropped instead of passed through")
	}
}
