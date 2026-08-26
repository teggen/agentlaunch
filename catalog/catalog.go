package catalog

import (
	"context"
	"time"
)

// Catalog supplies the model list. Swapping this implementation is the
// single-file change needed to adopt an official SDK later.
type Catalog interface {
	Models(ctx context.Context) ([]Model, error)
}

// Snapshot is a catalog read plus its provenance.
type Snapshot struct {
	Models    []Model
	FetchedAt time.Time
	// Stale is true when a refresh failed and cached data was served instead.
	Stale bool
	// StaleErr is the refresh failure that caused Stale.
	StaleErr error
}

// Age reports how old the snapshot's data is relative to now.
func (s Snapshot) Age(now time.Time) time.Duration {
	return now.Sub(s.FetchedAt)
}

// FindByID returns the model with exactly this ID.
func FindByID(models []Model, id string) (Model, bool) {
	for _, m := range models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// Suggest returns up to limit model IDs loosely matching query, for use in
// "did you mean" messages after an unknown slug.
func Suggest(models []Model, query string, limit int) []string {
	out := make([]string, 0, limit)
	for _, m := range models {
		if len(out) >= limit {
			break
		}
		if query == "" || m.Matches(query) {
			out = append(out, m.ID)
		}
	}
	return out
}
