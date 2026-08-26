package agent

import (
	"errors"
	"fmt"
)

// Binding is everything a launcher needs that is the same for every launcher
// in one tool: where the tokens go, who is doing the launching, and how a
// binary is found on PATH.
//
// It exists because the eleven launchers each carried an identical
// Provider/Host/LookPath triple. Constructing them against a Binding is what
// turns the registry from a package-level literal — one provider, decided at
// compile time — into a value a second tool can build for its own provider.
type Binding struct {
	// Provider is the endpoint every launcher in this registry points at.
	Provider Provider
	// Host is the identity of the tool doing the launching.
	Host Host
	// LookPath resolves a binary on PATH. nil means exec.LookPath, which is
	// what every launcher falls back to on its own; it is here so a caller
	// can make an entire registry answer install checks from a fixture
	// rather than from the machine running the test.
	LookPath func(string) (string, error)
}

// Validate reports why a Binding cannot be used. NewRegistry calls it before
// constructing anything, so a malformed descriptor fails at composition
// rather than at the first launch that happens to read the bad field.
func (b Binding) Validate() error {
	if err := b.Provider.Validate(); err != nil {
		return err
	}
	return b.Host.Validate()
}

// Definition is a registry INPUT: what an agent is, independent of any
// provider. It is the provider-neutral half of a Spec — everything a Spec
// carries except the constructed Launcher and the Status that construction
// determines.
type Definition struct {
	// Name is the canonical name users type. It must equal the constructed
	// launcher's Name(); NewRegistry checks that rather than trusting it,
	// because the two are now separate sources for one string.
	Name string
	// DisplayName is the human name. It must equal the constructed
	// launcher's DisplayName(), and it is also what a rejected agent's
	// placeholder launcher reports — the reason this field exists at all,
	// since a launcher that was never constructed cannot be asked.
	DisplayName string
	// Description is the one-line summary the agents listing renders.
	Description string
	// Aliases are alternate names resolving to this agent.
	Aliases []string
	// New builds the launcher for a Binding. An error wrapping
	// ErrUnsupportedProvider means "this agent cannot be pointed at that
	// provider": the registry records the reason on Spec.Status and carries
	// on, so no new guard is needed anywhere — the existing Status machinery
	// already refuses the launch, lists the agent under --all with its
	// reason, and skips it on the TUI's root screen. Any other error is a
	// construction failure and fails the whole registry.
	New func(Binding) (Launcher, error)
}

// ErrUnsupportedProvider is the sentinel that distinguishes "this agent does
// not work against this provider", which is a routine fact a registry
// records, from a genuine construction failure, which is a bug.
var ErrUnsupportedProvider = errors.New("unsupported provider")

// UnsupportedProviderError carries the human reason an agent cannot be
// pointed at the bound provider.
//
// It is a typed error rather than a wrapped string because the registry
// stores the reason on Spec.Status verbatim, and that value is rendered to
// users by `agents --all` and by UnsupportedAgentError. Recovering it from
// err.Error() would prepend the sentinel's own text to every one of those
// messages.
type UnsupportedProviderError struct {
	// Reason is user-facing prose explaining what cannot be done. The
	// registry copies it to Spec.Status.Reason unchanged.
	Reason string
}

func (e *UnsupportedProviderError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnsupportedProvider, e.Reason)
}

func (e *UnsupportedProviderError) Unwrap() error { return ErrUnsupportedProvider }

// Unsupported is what a Definition's New returns when the agent cannot be
// pointed at the bound provider. The reason is user-facing prose and reaches
// the user unaltered.
func Unsupported(reason string) error {
	return &UnsupportedProviderError{Reason: reason}
}
