package launch

import (
	"go/build"
	"strings"
	"testing"
)

// TestLaunchDependsOnNothingButTheAgentsAndTheCatalog pins the layering this
// package reached in A8 and the extraction made structural. The planner is
// the other half — with agent — of what a launcher tool reuses, so it must
// not know where any ONE tool keeps its settings, which endpoint it fetches
// from, or what its cache is called. Each of those was a package-level call
// until A8; each is now a function field the composition root fills.
//
// Third-party imports are refused for the same reason they are in the agent
// package: this module's zero-dependency surface is what lets a tool adopt it
// without inheriting a supply chain, and go.mod staying empty is a
// consequence of that rather than a check on it.
//
// Test imports are checked too, and deliberately: a test that reached for
// internal/config to seed a real settings file would compile and pass while
// making the package unmovable, which is exactly how the edge got there in
// the first place.
//
// build.ImportDir resolves for the CURRENT GOOS. Nothing in this package is
// build-tagged, so one run sees all of it.
func TestLaunchDependsOnNothingButTheAgentsAndTheCatalog(t *testing.T) {
	const self = "github.com/teggen/agentlaunch/"
	allowed := map[string]bool{
		self + "agent":               true,
		self + "catalog":             true,
		self + "catalog/catalogtest": true,
	}

	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	for _, list := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range list {
			switch {
			case isStdlib(imp) || allowed[imp]:
			case strings.HasPrefix(imp, self):
				t.Errorf("launch imports %s; the planner must take what it needs "+
					"as functions supplied by its caller, not reach for a sibling package", imp)
			default:
				t.Errorf("launch imports %s, which is outside the standard library; "+
					"this module has no third-party dependencies and adding one costs "+
					"every consuming tool the same dependency", imp)
			}
		}
	}
}

// isStdlib reports whether an import path names a standard library package.
// The first segment of every module path is a domain and so contains a dot;
// no standard library path does.
func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
