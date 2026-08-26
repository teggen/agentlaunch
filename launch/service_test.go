package launch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/teggen/openrouter-launch/internal/catalog"
	"github.com/teggen/openrouter-launch/internal/catalog/catalogtest"
	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// erroringCatalog always fails, forcing Snapshot down the stale-cache
// fallback path.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]catalog.Model, error) {
	return nil, errors.New("network down")
}

// writeCacheFileForTest writes a catalog cache file in the on-disk shape
// openrouter.Cache expects, without depending on its unexported cacheFile
// type: only the JSON shape needs to match.
func writeCacheFileForTest(t *testing.T, path string, fetchedAt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.Marshal(struct {
		Schema    int             `json:"schema"`
		FetchedAt time.Time       `json:"fetched_at"`
		Models    []catalog.Model `json:"models"`
	}{Schema: openrouter.CacheSchema, FetchedAt: fetchedAt, Models: catalogtest.Models()})
	if err != nil {
		t.Fatalf("marshal cache file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
}

// cachePathForTest isolates the catalog cache to a temp dir and returns its
// resolved path.
func cachePathForTest(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := openrouter.CachePath()
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	return path
}

func TestSnapshotServesStaleCacheWithoutFailing(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now().Add(-48*time.Hour)) // older than DefaultTTL

	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.Stale {
		t.Fatal("expected Stale when the refresh fails but a cache exists")
	}
	if len(snap.Models) == 0 {
		t.Error("a stale snapshot must still carry the cached models")
	}
}

func TestSnapshotDoesNotConsultSourceWhenFresh(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now()) // well within DefaultTTL

	// erroringCatalog fails if consulted at all, so a nil error here is the
	// evidence the fresh cache short-circuited the fetch.
	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Stale {
		t.Error("fresh cache reported as stale")
	}
}

func TestSnapshotWrapsHardFailure(t *testing.T) {
	cachePathForTest(t) // isolated, and deliberately no cache file written

	svc := &Service{Catalog: erroringCatalog{}}
	_, err := svc.Snapshot(context.Background(), false)
	if err == nil {
		t.Fatal("expected an error when the fetch fails with no cache to fall back on")
	}
	if !strings.Contains(err.Error(), "load model catalog") {
		t.Errorf("error should carry context, got: %v", err)
	}
}

func TestSnapshotForceRefreshBypassesFreshCache(t *testing.T) {
	path := cachePathForTest(t)
	writeCacheFileForTest(t, path, time.Now()) // fresh, would normally short-circuit

	// A fresh cache plus a failing source: with refresh=true the source is
	// consulted, fails, and the cache is served as stale. Without the
	// bypass, Stale would be false.
	svc := &Service{Catalog: erroringCatalog{}}
	snap, err := svc.Snapshot(context.Background(), true)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !snap.Stale {
		t.Error("refresh=true should bypass the fresh cache and consult the source")
	}
}
