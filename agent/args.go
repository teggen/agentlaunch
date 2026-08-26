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
//
// The attached form is matched by PREFIX, which is a guess rather than a
// reading: "-mfoo" is the model flag, but so is any hypothetical agent flag
// spelled "-mode" or "-max-tokens". Rejecting both is deliberate and stays —
// the two are indistinguishable on argv without a per-agent flag table, and
// the asymmetry is stark, since a false negative launches a different model
// than the one the tool reports while a false positive costs one renamed
// flag. What changes with the match kind is the WORDING: only the exact and
// equals forms have earned the right to tell the user what they meant.
func rejectModelFlag(host Host, agentName string, args []string) error {
	for _, arg := range args {
		switch {
		case arg == "-m" || arg == "--model" || strings.HasPrefix(arg, "--model="):
			return fmt.Errorf("%s: conflicting argument %q: %s manages the model; pick it with %[3]s %[1]s -m", agentName, arg, host.Name)
		case strings.HasPrefix(arg, "-m") && len(arg) > len("-m"):
			return fmt.Errorf("%s: argument %q looks like an attached model flag (-m<model>), and %s manages the model; pick it with %[3]s %[1]s -m. If %[2]q is a different %[1]s flag, it cannot be passed through in this form", agentName, arg, host.Name)
		}
	}
	return nil
}

// rejectFlags returns an error naming the first passthrough argument that
// matches one of flags, in separate ("--flag value"), equals
// ("--flag=value"), or — for single-dash short flags — attached ("-Pval")
// form. Launchers list the flags whose values the managed launch owns.
//
// As in rejectModelFlag, the attached short form is a prefix guess ("-Pval"
// and a hypothetical "-Persist" match alike), so it is reported as a
// resemblance rather than as a statement about what the user wanted. The
// long forms are exact and say so.
func rejectFlags(host Host, agentName string, args []string, flags ...string) error {
	for _, arg := range args {
		for _, f := range flags {
			short := len(f) == 2 && f[0] == '-' && f[1] != '-'
			switch {
			case arg == f || strings.HasPrefix(arg, f+"="):
				return fmt.Errorf("%s: conflicting argument %q: %s manages this setting for the launch", agentName, arg, host.Name)
			case short && strings.HasPrefix(arg, f) && len(arg) > len(f):
				return fmt.Errorf("%s: argument %q looks like an attached %s flag, and %s manages this setting for the launch. If it is a different flag, it cannot be passed through in this form", agentName, arg, f, host.Name)
			}
		}
	}
	return nil
}
