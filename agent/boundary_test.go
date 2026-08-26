package agent

import (
	"go/build"
	"strings"
	"testing"
)

// TestAgentDependsOnNothingButTheCatalog pins the layering that made this
// package movable. The launchers and the exec handoff are the reusable half
// of a launcher tool, so they must not reach back into anything belonging to
// one tool in particular. Everything that once did — the tool's name, its XDG
// directory, its provider's endpoints — now arrives as data, on a Provider, a
// Host, or the Request.
//
// One in-module import remains, catalog, for the Model type, and it is the
// right one: catalog is provider-neutral and ships in this same module.
//
// Since the extraction the test also refuses THIRD-PARTY imports, which the
// in-repo version could not express. Zero dependencies outside the standard
// library is what lets a tool adopt this module without inheriting a supply
// chain, and it is a property nothing else states: `go.mod` staying empty is
// a consequence, not a check, and it would grow a require line silently on
// the first stray import.
//
// Test imports are checked too, and deliberately: a test reaching for a
// consuming tool's config package to seed a real settings file would compile
// and pass while making this package unmovable — which is how the edge this
// test exists to prevent got there the first time.
//
// build.ImportDir resolves for the CURRENT GOOS, so the build-tagged exec
// files are only half visible on any one run. Neither imports anything
// outside the standard library, and `make lint-cross` covers the other
// platforms.
func TestAgentDependsOnNothingButTheCatalog(t *testing.T) {
	const self = "github.com/teggen/agentlaunch/"
	allowed := map[string]bool{self + "catalog": true}

	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	for _, list := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range list {
			switch {
			case isStdlib(imp) || allowed[imp]:
			case strings.HasPrefix(imp, self):
				t.Errorf("agent imports %s; the reusable half of a launcher tool "+
					"must take what it needs as data, not reach for a sibling package", imp)
			default:
				t.Errorf("agent imports %s, which is outside the standard library; "+
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
