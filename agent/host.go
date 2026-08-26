package agent

import "fmt"

// Host is the identity of the tool doing the launching. Every user-facing
// string this package produces that names a TOOL — rather than an agent or a
// provider — comes from here, and so does the token stamped into the one
// agent-owned config this package writes.
type Host struct {
	// Name is the command users type. It appears in the guidance attached to
	// a rejected passthrough argument ("<Name> manages the model; pick it
	// with <Name> <agent> -m"), so it is the binary's invocable name rather
	// than a prose title.
	Name string

	// Marker identifies entries this tool owns inside an AGENT's own
	// configuration: droid's customModels displayName, and the
	// "custom:<Marker>-<n>" selection ID derived from it.
	//
	// It is a separate field from Name, and must be treated as persisted
	// data rather than a label. droid's Apply strips only entries whose
	// displayName equals it, so changing Marker orphans every entry a
	// previous version of this tool left in a user's
	// ~/.factory/settings.local.json — each one preserved forever as
	// "foreign", plus a possibly-dangling default-model reference they have
	// to clear by hand. Changing it is a data migration, not a rename.
	Marker string
}

// Validate reports why a Host cannot be used.
func (h Host) Validate() error {
	switch {
	case h.Name == "":
		return fmt.Errorf("host: Name is required")
	case h.Marker == "":
		// Defaulting Marker to Name would be worse than refusing: the two are
		// free to differ, and a tool that renames itself must keep the old
		// marker, so a silent default is exactly the migration hazard the
		// field exists to make explicit.
		return fmt.Errorf("host %q: Marker is required", h.Name)
	}
	return nil
}
