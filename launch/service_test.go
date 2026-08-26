package launch

import (
	"context"
	"strings"
	"testing"

	"github.com/teggen/openrouter-launch/internal/catalog"
)

// Service.Snapshot is now three lines around an injected loader, so what
// these tests can honestly claim has narrowed — and that is the point of A8.
// Whether a fresh cache short-circuits a fetch, whether a failed refresh
// falls back to stale data, and whether --refresh bypasses a fresh file are
// facts about openrouter.Cache; they are tested there directly
// (TestCacheServesFreshWithoutFetching and its neighbours) and end to end
// through the real wiring in internal/cli (TestModelsCommandRendersStaleCatalogWarning).
// What is left here is what this package still decides.

func TestSnapshotWrapsHardFailure(t *testing.T) {
	svc := &Service{LoadCatalog: failingCatalog()}

	_, err := svc.Snapshot(context.Background(), false)
	if err == nil {
		t.Fatal("expected an error when the loader fails")
	}
	if !strings.Contains(err.Error(), "load model catalog") {
		t.Errorf("error should carry context, got: %v", err)
	}
}

// TestSnapshotRequiresALoader pins the nil case as a misconfiguration. There
// is no endpoint this package could default to — that is the whole reason
// LoadCatalog is a field — so guessing one is not available and a nil
// dereference is not an acceptable way to report it.
func TestSnapshotRequiresALoader(t *testing.T) {
	_, err := (&Service{}).Snapshot(context.Background(), false)
	if err == nil {
		t.Fatal("Snapshot with no LoadCatalog returned no error")
	}
	if !strings.Contains(err.Error(), "Service.LoadCatalog is required") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
}

// TestSnapshotPassesRefreshThrough is what remains of
// TestSnapshotForceRefreshBypassesFreshCache: the bypass itself belongs to
// the cache, but the flag reaching it from Request.Refresh is this package's
// wiring, and a dropped argument would silently pin every user to cached
// data until the TTL expired.
func TestSnapshotPassesRefreshThrough(t *testing.T) {
	for _, want := range []bool{false, true} {
		var got bool
		svc := &Service{LoadCatalog: func(_ context.Context, refresh bool) (catalog.Snapshot, error) {
			got = refresh
			return freshSnapshot(), nil
		}}
		if _, err := svc.Snapshot(context.Background(), want); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if got != want {
			t.Errorf("loader saw refresh = %v, want %v", got, want)
		}
	}
}

// TestPlanPassesRefreshThrough carries the same claim one level up: Plan is
// what actually reads Request.Refresh, and --refresh is inert if it stops
// there.
func TestPlanPassesRefreshThrough(t *testing.T) {
	var got bool
	svc := newTestService(t)
	svc.LoadCatalog = func(_ context.Context, refresh bool) (catalog.Snapshot, error) {
		got = refresh
		return freshSnapshot(), nil
	}
	if _, err := svc.Plan(context.Background(), Request{
		Spec: spec("fake", &fakeLauncher{}), ModelID: "anthropic/claude-opus-4.6", Refresh: true,
	}); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !got {
		t.Error("Request.Refresh did not reach the catalog loader")
	}
}
