package runner_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const runnerPkg = "github.com/leeovery/portal/internal/runner"

// runnerOnly is what only the attach runner may bring into a build: the
// runner, and the pseudo-terminal it owns.
var runnerOnly = []string{runnerPkg, "github.com/creack/pty"}

// detectingOrDrawing detect a terminal or draw a picker, and attach nothing,
// so none of them may link the runner: spawn detection reads a leader's token
// through internal/runnertoken instead.
var detectingOrDrawing = []string{
	"github.com/leeovery/portal/internal/spawn",
	"github.com/leeovery/portal/internal/tui",
	"github.com/leeovery/portal/cmd/capturetool",
}

func TestOnlyWhatAttachesLinksTheRunner(t *testing.T) {
	root := sourceguardtest.InDir(sourceguardtest.ProjectRoot(t))
	for _, lane := range sourceguardtest.Lanes() {
		// The runner's own set holds each forbidden path, so a renamed or
		// mistyped one can't pass for ever.
		own := sourceguardtest.PackageDeps(t, runnerPkg, root, lane)
		for _, forbidden := range runnerOnly {
			if !slices.Contains(own, forbidden) {
				t.Fatalf("%s does not depend on %s, so this guard proves nothing about it", runnerPkg, forbidden)
			}
		}
		for _, pkg := range detectingOrDrawing {
			deps := sourceguardtest.PackageDeps(t, pkg, root, lane)
			for _, forbidden := range runnerOnly {
				if slices.Contains(deps, forbidden) {
					t.Errorf("%s depends on %s; it attaches nothing, so it must not link the runner", pkg, forbidden)
				}
			}
		}
	}
}
