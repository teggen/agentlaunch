package agent

import (
	"os"
	"strings"
)

// ExecArgs assembles the argv and environment for a Command. argv[0] is the
// binary path, as exec expects.
//
// Every inherited entry whose key also appears in c.Env is dropped before
// c.Env is appended, so each key occurs exactly once, carrying the
// command's value. This dedup is required because execve(2) does not
// deduplicate envp: POSIX getenv returns the FIRST match, so naively
// appending c.Env after the inherited environment would let the inherited
// value win on any duplicate key - the opposite of what callers need (e.g.
// overriding ANTHROPIC_BASE_URL for a user who already has it exported).
// The relative order of the surviving inherited entries is preserved.
func ExecArgs(c Command) (argv []string, env []string) {
	argv = make([]string, 0, len(c.Args)+1)
	argv = append(argv, c.Path)
	argv = append(argv, c.Args...)

	override := make(map[string]bool, len(c.Env))
	for _, e := range c.Env {
		if key, ok := envKey(e); ok {
			override[key] = true
		}
	}

	inherited := os.Environ()
	env = make([]string, 0, len(inherited)+len(c.Env))
	for _, e := range inherited {
		if key, ok := envKey(e); ok && override[key] {
			continue
		}
		env = append(env, e)
	}
	env = append(env, c.Env...)
	return argv, env
}

// envKey extracts the key from a "KEY=VALUE" environment entry by cutting at
// the first '='. An entry with no '=' has no key: ok is false and the entry
// should pass through unchanged rather than being treated as a match.
func envKey(entry string) (key string, ok bool) {
	i := strings.IndexByte(entry, '=')
	if i < 0 {
		return "", false
	}
	return entry[:i], true
}
