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

func TestExecArgsCommandEnvWinsOverInherited(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "inherited-value")

	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"ANTHROPIC_API_KEY=sk-or-test"}})

	// The command's entry must come after the inherited one: exec semantics
	// give the last occurrence priority.
	lastIdx := -1
	for i, e := range env {
		if strings.HasPrefix(e, "ANTHROPIC_API_KEY=") {
			lastIdx = i
		}
	}
	if lastIdx < 0 {
		t.Fatal("ANTHROPIC_API_KEY missing entirely")
	}
	if env[lastIdx] != "ANTHROPIC_API_KEY=sk-or-test" {
		t.Errorf("last ANTHROPIC_API_KEY = %q, want the command's value", env[lastIdx])
	}
}

func TestExecArgsEnvLengthIsSumOfBoth(t *testing.T) {
	_, env := ExecArgs(Command{Path: "/bin/true", Env: []string{"A=1", "B=2"}})
	if got, want := len(env), len(os.Environ())+2; got != want {
		t.Errorf("env length = %d, want %d", got, want)
	}
}
