package agentlaunch

import (
	"os"
	"regexp"
	"testing"
)

// The owner's /quality command (~/.claude/commands/quality.md) invokes these
// targets by name, and /preflight invokes a subset. Renaming or dropping one
// breaks those commands silently — nothing in the Go build would notice, and
// the failure would surface as a confusing "No rule to make target" during an
// unrelated session. This is the same class of structural tripwire as
// TestWriteSitesAreExhaustivelyEnumerated.
//
// The list is identical to openrouter-launch's on purpose: one command has to
// work the same way in both checkouts, so the contract cannot be a per-repo
// choice. Targets this module has no use for (build, snapshot) are simply
// absent from the Makefile; targets in this list are not.
var qualityContractTargets = []string{
	"clean",
	"fmt",
	"fmt-check",
	"vet",
	"lint",
	"security",
	"test",
	"test-unit",
	"pre-commit",
}

func TestMakefileDeclaresQualityContractTargets(t *testing.T) {
	src, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("reading Makefile: %v", err)
	}
	for _, target := range qualityContractTargets {
		// Anchored at line start and followed by a colon, so a mention
		// inside a recipe or comment cannot satisfy the assertion.
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `:`)
		if !re.Match(src) {
			t.Errorf("Makefile declares no %q target; the /quality command invokes it by name", target)
		}
	}
}
