package agent

import (
	"go/build"
	"strings"
	"testing"
)

// TestAgentDependsOnNothingButTheCatalog pins the layering this package is
// being moved toward: the launchers and the exec handoff are the reusable
// half of this tool, so they must not reach back into anything that belongs
// to THIS tool in particular. Everything that once did — the tool's name, its
// XDG directory, its provider's endpoints — now arrives as data, on a
// Provider, a Host, or the Request.
//
// One in-repo import remains, for the Model type, and it is the next thing to
// go. Adding a second is the regression this test exists to catch: a new
// launcher reaching for internal/config to find "our" directory, say, would
// compile and pass every other test in the package.
//
// build.ImportDir resolves for the CURRENT GOOS, so the build-tagged exec
// files are only half visible on any one run. Neither imports anything in
// this repo, and `make lint-cross` covers the other platforms.
func TestAgentDependsOnNothingButTheCatalog(t *testing.T) {
	const self = "github.com/teggen/openrouter-launch/"
	allowed := map[string]bool{self + "internal/openrouter": true}

	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	for _, list := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range list {
			if !strings.HasPrefix(imp, self) || allowed[imp] {
				continue
			}
			t.Errorf("internal/agent imports %s; the reusable half of this tool "+
				"must take what it needs as data, not reach for this tool's own packages", imp)
		}
	}
}
