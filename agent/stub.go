package agent

import "fmt"

// stub satisfies Launcher for agents that cannot be pointed at OpenRouter.
// Spec.Launcher must never be nil — buildIndex panics on construction, which
// would take down the binary and every CLI test (see registry.go). The
// planner's CheckSupported guard fires on Status before any Command call, so
// this error is unreachable through the launch path; the launch package pins
// that with TestCheckSupportedCoversEveryUnsupportedRegistryEntry.
type stub struct {
	name    string
	display string
}

func (s *stub) Name() string        { return s.name }
func (s *stub) DisplayName() string { return s.display }

func (s *stub) Command(Request) (Command, error) {
	return Command{}, fmt.Errorf("%s cannot be pointed at OpenRouter", s.display)
}
