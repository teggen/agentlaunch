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

	"github.com/teggen/openrouter-launch/internal/openrouter"
)

// erroringCatalog always fails, forcing Snapshot down the stale-cache
// fallback path.
type erroringCatalog struct{}

func (erroringCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return nil, errors.New("network down")
}

// fakeCatalog serves fixed models without touching the network.
type fakeCatalog struct{ models []openrouter.Model }

func (f *fakeCatalog) Models(context.Context) ([]openrouter.Model, error) {
	return f.models, nil
}

// fakeModels mirrors the CLI's fixture set. openai/o1-mini is the only entry
// without tool support, which is what several filter tests key on.
func fakeModels() []openrouter.Model {
	return []openrouter.Model{
		{ID: "anthropic/claude-opus-4.6", Name: "Anthropic: Claude Opus 4.6",
			ContextLength: 200000, PromptPricePerM: 15, CompletionPricePerM: 75,
			SupportsTools: true, Provider: "anthropic"},
		{ID: "qwen/qwen3-coder:free", Name: "Qwen: Qwen3 Coder (free)",
			ContextLength: 262144, SupportsTools: true, Provider: "qwen"},
		{ID: "openai/o1-mini", Name: "OpenAI: o1-mini",
			ContextLength: 128000, PromptPricePerM: 1.1, CompletionPricePerM: 4.4,
			Provider: "openai"},
	}
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
		FetchedAt time.Time          `json:"fetched_at"`
		Models    []openrouter.Model `json:"models"`
	}{FetchedAt: fetchedAt, Models: fakeModels()})
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
