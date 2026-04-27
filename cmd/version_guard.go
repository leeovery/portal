package cmd

import (
	"sync"

	"github.com/leeovery/portal/internal/tmux"
)

// versionChecker is the package-level injection seam for the tmux runtime
// version check. Tests replace this to stub the call without invoking real
// tmux. Defaults to the production implementation in the tmux package.
var versionChecker func(tmux.Commander) error = tmux.CheckTmuxVersion

// versionCheckOnce ensures the runtime version check runs at most once per
// Portal process. It guards versionCheckErr so concurrent PersistentPreRunE
// invocations cannot race the check.
var versionCheckOnce sync.Once

// versionCheckErr stores the result of the (single) version check so it can
// be returned to every caller of runVersionCheck after the first invocation.
var versionCheckErr error

// runVersionCheck executes the tmux runtime version check exactly once per
// Portal process and returns the cached result on every subsequent call.
//
// It builds a fresh tmux.RealCommander for the production path; tests inject
// a stubbed versionChecker that ignores the Commander argument.
func runVersionCheck() error {
	versionCheckOnce.Do(func() {
		versionCheckErr = versionChecker(&tmux.RealCommander{})
	})
	return versionCheckErr
}

// resetVersionCheckForTest re-initialises the sync.Once gate and clears the
// cached error so successive tests can exercise the check independently.
// Tests register this with t.Cleanup. It is package-private and never
// referenced from production code paths.
func resetVersionCheckForTest() {
	versionCheckOnce = sync.Once{}
	versionCheckErr = nil
}
