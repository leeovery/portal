package logtest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	modulePrefix   = "github.com/leeovery/portal/"
	logTestPkg     = modulePrefix + "internal/logtest"
	harnessTestPkg = modulePrefix + "internal/harnesstest"
	logPkg         = modulePrefix + "internal/log"
)

// Every test package in the tree may reach for the capture handler, so what
// logtest imports arrives everywhere tests run — and a helper here that
// resolved a value from another package, rather than taking it as a parameter,
// would put that package there too. It reaches the logging machinery it
// captures, and the stand-in its failing helpers report through. A third-party
// import is judged on the same footing, for the same reason: it would land in
// every test package in the tree as surely as an internal one.
var logTestMayImport = []string{harnessTestPkg, logPkg}

func TestLogTestPackage(t *testing.T) {
	t.Run("it holds logtest's transitive dependencies inside the declared allowlist", func(t *testing.T) {
		for _, lane := range sourceguardtest.Lanes() {
			sourceguardtest.AssertDepsWithin(t, logTestPkg, logTestMayImport, lane, sourceguardtest.ForbiddingThirdParty())
		}
	})

	// Narrowing the allowlist to one of the two entries makes the other a
	// dependency outside it, so the assertion above is shown to bite without a
	// package having to import something it must not.
	t.Run("it reports a dependency outside the allowlist", func(t *testing.T) {
		lanes := sourceguardtest.Lanes()
		for i, lane := range lanes {
			rec := &harnesstest.Recorder{}

			rec.Run(func() {
				sourceguardtest.AssertDepsWithin(rec, logTestPkg, []string{harnessTestPkg}, lane, sourceguardtest.ForbiddingThirdParty())
			})

			if !reported(rec, logPkg) {
				t.Errorf("lane %d of the %d the guard runs over judged nothing: %s went unreported, so a dependency outside the allowlist would pass under that configuration; reported %v", i, len(lanes), logPkg, rec.Errors)
			}
		}
	})
}

// reported says whether any complaint the guard recorded names pkg as the
// offending dependency. The match is anchored on the dependency slot of the
// message, because the subject package is named first and its own path extends
// log's: an unanchored match would find pkg in that first mention and so accept
// a complaint about some other dependency entirely.
func reported(rec *harnesstest.Recorder, pkg string) bool {
	return slices.ContainsFunc(rec.Errors, func(msg string) bool {
		return strings.Contains(msg, "depends on "+pkg+" ")
	})
}
