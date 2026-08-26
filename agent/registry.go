package agent

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownAgent is returned when a name matches no agent or alias.
var ErrUnknownAgent = errors.New("unknown agent")

// Status records whether an agent can be pointed at the bound provider.
// Unsupported agents stay registered so their absence is explained rather
// than silent.
type Status struct {
	Supported bool
	Reason    string
}

// Spec is a registry entry: one Definition resolved against one Binding.
type Spec struct {
	Name    string
	Aliases []string
	// Launcher is required: NewRegistryFromSpecs rejects a nil one, since
	// every caller (newLaunchCmds, the agents listing, Installed)
	// dereferences it unconditionally. An agent the Binding's provider
	// cannot serve still gets one — a placeholder that names the agent and
	// errors if it is ever asked for a Command.
	Launcher    Launcher
	Description string
	Status      Status
}

// Registry is a set of agents bound to one provider. It is a value rather
// than package state so that a second tool — same eleven recipes, different
// provider — builds its own instead of mutating a global.
type Registry struct {
	specs []*Spec
	index map[string]*Spec
}

// NewRegistry resolves definitions against a binding.
//
// A definition whose New reports ErrUnsupportedProvider is registered
// unsupported, carrying its reason: that is the whole mechanism by which an
// agent that cannot reach a given provider is explained rather than missing.
// Any other error from New fails the registry, because it means the
// definition is wrong, not the pairing.
func NewRegistry(b Binding, defs []Definition) (*Registry, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	specs := make([]*Spec, 0, len(defs))
	for i, def := range defs {
		spec, err := def.resolve(b, i)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return NewRegistryFromSpecs(specs)
}

// resolve turns one Definition into a Spec. i identifies the entry in
// diagnostics for the case where the name itself is what is missing.
func (d Definition) resolve(b Binding, i int) (*Spec, error) {
	switch {
	case d.Name == "":
		return nil, fmt.Errorf("agent: definition at index %d is missing a name", i)
	case d.DisplayName == "":
		return nil, fmt.Errorf("agent: definition %q is missing a display name", d.Name)
	case d.New == nil:
		return nil, fmt.Errorf("agent: definition %q has a nil New", d.Name)
	}

	spec := &Spec{
		Name:        d.Name,
		Aliases:     d.Aliases,
		Description: d.Description,
	}

	launcher, err := d.New(b)
	if err != nil {
		var unsupported *UnsupportedProviderError
		if !errors.As(err, &unsupported) {
			return nil, fmt.Errorf("agent %q: %w", d.Name, err)
		}
		// The placeholder is what keeps Spec.Launcher non-nil for an agent
		// no launcher was built for. It reports the definition's names,
		// which is why Definition carries DisplayName at all.
		spec.Launcher = &stub{name: d.Name, display: d.DisplayName, provider: b.Provider.DisplayName}
		spec.Status = Status{Supported: false, Reason: unsupported.Reason}
		return spec, nil
	}
	if launcher == nil {
		return nil, fmt.Errorf("agent %q: New returned a nil Launcher and no error", d.Name)
	}

	// Name and DisplayName now have two sources — the definition and the
	// launcher — and callers read both: Lookup and the cobra subcommand use
	// spec.Name, every rendered label uses Launcher.DisplayName(). Divergence
	// would be silent and confusing, so it is refused here.
	if got := launcher.Name(); got != d.Name {
		return nil, fmt.Errorf("agent %q: launcher reports Name() = %q", d.Name, got)
	}
	if got := launcher.DisplayName(); got != d.DisplayName {
		return nil, fmt.Errorf("agent %q: launcher reports DisplayName() = %q, definition says %q",
			d.Name, got, d.DisplayName)
	}

	spec.Launcher = launcher
	spec.Status = Status{Supported: true}
	return spec, nil
}

// MustRegistry is NewRegistry for a composition root, where a bad registry is
// a programmer error in a literal and failing before main() does any work is
// the correct outcome. A library must not panic on a caller's slice, which is
// why NewRegistry returns an error and this wrapper is separate; the
// precedent is cli.NewRootCmdWith's nil-Service panic.
func MustRegistry(b Binding, defs []Definition) *Registry {
	reg, err := NewRegistry(b, defs)
	if err != nil {
		panic("agent: " + err.Error())
	}
	return reg
}

// NewRegistryFromSpecs indexes specs that are already built, rejecting the
// programmer errors a registry literal can contain: a missing name, a nil
// Launcher, and any collision between names and aliases.
//
// It is exported because building a Spec directly is what a consumer's tests
// need — an adversarial description, a launcher that reports itself
// uninstalled — without a Provider, a Host, or the machine's PATH being
// involved. Production code should use NewRegistry.
func NewRegistryFromSpecs(specs []*Spec) (*Registry, error) {
	idx := make(map[string]*Spec, len(specs)*2)

	canonical := make(map[string]bool, len(specs))
	for i, s := range specs {
		key := strings.ToLower(s.Name)
		if key == "" {
			// The name itself is what's missing, so identify the entry by
			// its position in the registry literal instead.
			return nil, fmt.Errorf("agent: spec at index %d is missing a name", i)
		}
		if s.Launcher == nil {
			return nil, fmt.Errorf("agent: spec %q has a nil Launcher", s.Name)
		}
		if canonical[key] {
			return nil, fmt.Errorf("agent: duplicate agent name %q", key)
		}
		canonical[key] = true
		idx[key] = s
	}

	for _, s := range specs {
		for _, alias := range s.Aliases {
			key := strings.ToLower(alias)
			if key == "" {
				return nil, fmt.Errorf("agent: %q has an empty alias", s.Name)
			}
			if canonical[key] {
				return nil, fmt.Errorf("agent: %q's alias %q collides with an agent name", s.Name, key)
			}
			if prev, ok := idx[key]; ok && prev != s {
				return nil, fmt.Errorf("agent: alias %q claimed by both %q and %q", key, prev.Name, s.Name)
			}
			idx[key] = s
		}
	}

	out := make([]*Spec, len(specs))
	copy(out, specs)
	return &Registry{specs: out, index: idx}, nil
}

// Lookup resolves a canonical name or alias.
func (r *Registry) Lookup(name string) (*Spec, error) {
	spec, ok := r.index[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAgent, name)
	}
	return spec, nil
}

// List returns every registered agent in display order.
func (r *Registry) List() []*Spec {
	out := make([]*Spec, len(r.specs))
	copy(out, r.specs)
	return out
}

// Installed reports whether the agent's binary is present. Agents that do not
// implement Installable are assumed present.
//
// It is a method for the same reason List and Lookup are: a consumer injects
// one *Registry and gets all three answers from it, rather than three
// separately overridable function fields that can disagree about which
// registry they are talking about.
func (r *Registry) Installed(s *Spec) bool {
	if i, ok := s.Launcher.(Installable); ok {
		return i.CheckInstalled()
	}
	return true
}
