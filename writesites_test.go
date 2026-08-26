// Package agentlaunch has no source of its own. It exists so the invariants
// that are properties of the MODULE rather than of any one package — the
// Landmine 6 write-site enumeration, the Makefile's target contract — have a
// place to live at the root, where a walk from "." sees everything.
package agentlaunch

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writeSitePattern matches every raw filesystem-write primitive Landmine 6
// cares about. os.Create( is anchored on the opening paren (rather than a
// bare \bos\.Create\b) so it cannot also match os.CreateTemp. That anchor is
// not a nicety: an earlier revision of this check used the looser pattern and
// silently missed every CreateTemp hit — which is the atomic-write shape
// writeDroidSettingsFile and writeClineProvidersFile both use, so the pattern
// that felt more permissive in fact saw fewer of the write sites it exists to
// find. Kept verbatim from openrouter-launch, where it was established.
var writeSitePattern = regexp.MustCompile(
	`\bos\.WriteFile\b|\bos\.Create\(|\bos\.OpenFile\b|\bos\.MkdirAll\b|\bos\.Rename\b|\bos\.CreateTemp\b`,
)

// writeSiteAllowlist is this module's half of the Landmine 6 enumeration.
//
// Landmine 6 is a five-site claim, and the extraction split it across two
// repositories: three sites came here, two stayed in openrouter-launch
// (internal/openrouter/cache.go, internal/config/config.go). Neither half
// can see the other, which is why openrouter-launch also runs a check over
// this module's source as a dependency — otherwise a sixth site introduced
// here would pass both repos' own tests while making the app's README claim
// of "exactly five files" false.
//
// A write primitive anywhere else in this module is a Critical defect: the
// zero-touch principle is "never write an agent's files EXCEPT through the
// capability-gated, restoring ConfigWriter", and an unsanctioned write site
// slipping in unnoticed is exactly what this invariant exists to prevent.
var writeSiteAllowlist = map[string]bool{
	"launch/handoff.go": true, // site 3: the Staged materializer
	"agent/droid.go":    true, // site 4: droid's ConfigWriter
	// Site 5: cline's ConfigWriter. Note the inversion relative to site 4 —
	// this one only ever RESTORES. Cline itself does the writing (-k is
	// persisted into its provider store), and the write primitive here exists
	// to put the user's file back, which is the narrower of the two powers.
	"agent/cline.go": true,
}

// TestWriteSitesAreExhaustivelyEnumerated pins Landmine 6 as a regression
// tripwire instead of leaving it as a grep a human has to remember to run:
// it walks every non-test .go file in the module and asserts that a raw
// write primitive appears only in the sanctioned files above, and that each
// of those still has one (an entry the allowlist keeps around after its
// write moved elsewhere would silently understate the real enumeration).
func TestWriteSitesAreExhaustivelyEnumerated(t *testing.T) {
	seen := make(map[string]bool, len(writeSiteAllowlist))
	var stray []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !writeSitePattern.Match(data) {
			return nil
		}
		rel := filepath.ToSlash(path)
		if writeSiteAllowlist[rel] {
			seen[rel] = true
		} else {
			stray = append(stray, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(stray) > 0 {
		t.Errorf("write primitive found outside the sanctioned Landmine 6 sites: %v", stray)
	}
	for f := range writeSiteAllowlist {
		if !seen[f] {
			t.Errorf("sanctioned write site %q has no write primitive anymore — narrow writeSiteAllowlist", f)
		}
	}
}
