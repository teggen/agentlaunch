package agent

import (
	"fmt"
	"strings"
)

// rejectModelFlag returns an error naming the first passthrough argument
// that would override the managed model selection: -m / --model in
// separate, attached (-mfoo), or equals (--model=foo) form. Launchers
// reject these because the agent-side flag outranks the managed
// configuration, so accepting one would silently launch a different model
// while the tool reports success — the Landmine 3 failure class, on argv.
func rejectModelFlag(agentName string, args []string) error {
	for _, arg := range args {
		if arg == "-m" || arg == "--model" ||
			strings.HasPrefix(arg, "--model=") ||
			(strings.HasPrefix(arg, "-m") && len(arg) > len("-m")) {
			return fmt.Errorf("%s: conflicting argument %q: openrouter-launch manages the model; pick it with openrouter-launch %s -m", agentName, arg, agentName)
		}
	}
	return nil
}

// rejectFlags returns an error naming the first passthrough argument that
// matches one of flags, in separate ("--flag value"), equals
// ("--flag=value"), or — for single-dash short flags — attached ("-Pval")
// form. Launchers list the flags whose values the managed launch owns.
func rejectFlags(agentName string, args []string, flags ...string) error {
	for _, arg := range args {
		for _, f := range flags {
			short := len(f) == 2 && f[0] == '-' && f[1] != '-'
			if arg == f || strings.HasPrefix(arg, f+"=") ||
				(short && strings.HasPrefix(arg, f) && len(arg) > len(f)) {
				return fmt.Errorf("%s: conflicting argument %q: openrouter-launch manages this setting for the launch", agentName, arg)
			}
		}
	}
	return nil
}
