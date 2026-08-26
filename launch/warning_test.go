package launch

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

func TestStaleWarningFreshSnapshotProducesNothing(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	snap := catalog.Snapshot{FetchedAt: now.Add(-time.Hour)}

	if _, ok := StaleWarning(snap, now); ok {
		t.Error("a fresh snapshot should produce no warning")
	}
}

func TestStaleWarningReportsAgeAndCause(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	snap := catalog.Snapshot{
		FetchedAt: now.Add(-90 * time.Minute),
		Stale:     true,
		StaleErr:  errors.New("network down"),
	}

	w, ok := StaleWarning(snap, now)
	if !ok {
		t.Fatal("a stale snapshot should produce a warning")
	}
	if w.Kind != WarnStaleCatalog {
		t.Errorf("Kind = %v, want WarnStaleCatalog", w.Kind)
	}
	// A stale catalog is informational. If it carried a Question, every
	// offline run would stop and wait for an answer.
	if w.Question != "" {
		t.Errorf("Question = %q, want empty for an informational warning", w.Question)
	}
	if !strings.Contains(w.Message, "network down") {
		t.Errorf("Message should name the refresh failure, got %q", w.Message)
	}
	if !strings.Contains(w.Message, "1h30m0s") {
		t.Errorf("Message should report the data's age, got %q", w.Message)
	}
}
