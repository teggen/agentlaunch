package agent

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownAgent is returned when a name matches no agent or alias.
var ErrUnknownAgent = errors.New("unknown agent")

// Status records whether an agent can be pointed at OpenRouter. Unsupported
// agents stay registered so their absence is explained rather than silent.
type Status struct {
	Supported bool
	Reason    string
}

// Spec is a registry entry.
type Spec struct {
	Name    string
	Aliases []string
	// Launcher is required: buildIndex panics at startup if it is nil,
	// since every caller (newLaunchCmds, the agents listing, Installed)
	// dereferences it unconditionally.
	Launcher    Launcher
	Description string
	Status      Status
}

// specs is the declarative registry, in display order.
var specs = []*Spec{
	{
		Name:        "claude",
		Aliases:     []string{"claude-code", "cc"},
		Launcher:    &Claude{},
		Description: "Anthropic's coding tool with subagents",
		Status:      Status{Supported: true},
	},
}

var index = buildIndex(specs)

// buildIndex maps names and aliases to specs, panicking on any collision.
// Collisions are programmer errors in the registry literal, so failing at
// startup is correct.
func buildIndex(all []*Spec) map[string]*Spec {
	idx := make(map[string]*Spec, len(all)*2)

	canonical := make(map[string]bool, len(all))
	for _, s := range all {
		key := strings.ToLower(s.Name)
		if key == "" {
			panic("agent: spec is missing a name")
		}
		if s.Launcher == nil {
			panic(fmt.Sprintf("agent: spec %q has a nil Launcher", s.Name))
		}
		if canonical[key] {
			panic(fmt.Sprintf("agent: duplicate agent name %q", key))
		}
		canonical[key] = true
		idx[key] = s
	}

	for _, s := range all {
		for _, alias := range s.Aliases {
			key := strings.ToLower(alias)
			if key == "" {
				panic(fmt.Sprintf("agent: %q has an empty alias", s.Name))
			}
			if canonical[key] {
				panic(fmt.Sprintf("agent: alias %q collides with an agent name", key))
			}
			if prev, ok := idx[key]; ok && prev != s {
				panic(fmt.Sprintf("agent: alias %q claimed by both %q and %q", key, prev.Name, s.Name))
			}
			idx[key] = s
		}
	}
	return idx
}

// Lookup resolves a canonical name or alias.
func Lookup(name string) (*Spec, error) {
	spec, ok := index[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownAgent, name)
	}
	return spec, nil
}

// List returns every registered agent in display order.
func List() []*Spec {
	out := make([]*Spec, len(specs))
	copy(out, specs)
	return out
}

// Installed reports whether the agent's binary is present. Agents that do not
// implement Installable are assumed present.
func Installed(s *Spec) bool {
	if i, ok := s.Launcher.(Installable); ok {
		return i.CheckInstalled()
	}
	return true
}
