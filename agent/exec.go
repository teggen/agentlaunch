package agent

import "os"

// ExecArgs assembles the argv and environment for a Command. argv[0] is the
// binary path, as exec expects. Command entries are appended after the
// inherited environment so they win on duplicate keys.
func ExecArgs(c Command) (argv []string, env []string) {
	argv = make([]string, 0, len(c.Args)+1)
	argv = append(argv, c.Path)
	argv = append(argv, c.Args...)

	inherited := os.Environ()
	env = make([]string, 0, len(inherited)+len(c.Env))
	env = append(env, inherited...)
	env = append(env, c.Env...)
	return argv, env
}
