package launch

import (
	"go/build"
	"strings"
	"testing"
)

// TestLaunchDependsOnNothingButTheAgentsAndTheCatalog pins the layering this
// package reached in A8. The planner is the other half — with internal/agent
// — of what a second launcher tool reuses, so it must not know where THIS
// tool keeps its settings, which endpoint it fetches from, or what its cache
// is called. Each of those was a package-level call until this step; each is
// now a function field the composition root fills.
//
// Test imports are checked too, and deliberately: a test that reached for
// internal/config to seed a real settings file would compile and pass while
// making the package unmovable, which is exactly how the edge got there in
// the first place.
//
// build.ImportDir resolves for the CURRENT GOOS. Nothing in this package is
// build-tagged, so one run sees all of it.
func TestLaunchDependsOnNothingButTheAgentsAndTheCatalog(t *testing.T) {
	const self = "github.com/teggen/openrouter-launch/"
	allowed := map[string]bool{
		self + "internal/agent":               true,
		self + "internal/catalog":             true,
		self + "internal/catalog/catalogtest": true,
	}

	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	for _, list := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range list {
			if !strings.HasPrefix(imp, self) || allowed[imp] {
				continue
			}
			t.Errorf("internal/launch imports %s; the planner must take what it "+
				"needs as functions supplied by its caller, not reach for this tool's own packages", imp)
		}
	}
}
