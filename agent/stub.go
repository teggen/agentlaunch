package agent

import "fmt"

// stub satisfies Launcher for an agent no launcher was built for — one whose
// Definition reported ErrUnsupportedProvider for the bound provider.
//
// Spec.Launcher must never be nil: NewRegistryFromSpecs rejects one, because
// every caller (newLaunchCmds, the agents listing, Installed) dereferences it
// unconditionally. The planner's CheckSupported guard fires on Status before
// any Command call, so this error is unreachable through the launch path; the
// launch package pins that with
// TestCheckSupportedCoversEveryUnsupportedRegistryEntry.
type stub struct {
	name     string
	display  string
	provider string
}

func (s *stub) Name() string        { return s.name }
func (s *stub) DisplayName() string { return s.display }

func (s *stub) Command(Request) (Command, error) {
	return Command{}, fmt.Errorf("%s cannot be pointed at %s", s.display, s.provider)
}
