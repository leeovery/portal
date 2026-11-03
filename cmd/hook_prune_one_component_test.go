package cmd

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// componentOf names the component a captured record was emitted under, so a
// case asserting where a whole cycle landed reads the attribution off every
// record rather than one it selected in advance.
func componentOf(rec logtest.Record) string { return rec.AttrOrEmpty("component") }

// sweepFailureFixture stages a store whose clean fails for a reason the cycle
// does not classify: the sidecar is absent and the directory denies writes, so
// the deletion's own lock open fails. The store's own line on that path is a
// degraded-read DEBUG, so the cycle's failure is the only record at WARN.
func sweepFailureFixture(t *testing.T) *hooks.Store {
	t.Helper()
	store, _ := hookstest.StageStore(t, hookstest.Staging{
		Dir:           filepath.Join(t.TempDir(), "lock-open-denied"),
		Seed:          hookstest.StaleHookSeed,
		SidecarAbsent: true,
		WritesDenied:  true,
	})
	return store
}

// One cycle is one story, and reconstructing it must not mean correlating three
// components: whatever a cycle has to say about itself belongs to the subsystem
// it sweeps, whichever caller drove it. That the cycle emits under one component
// at all is internal/hooksweep's own to pin; what this adds is that the daemon
// adds no line of its own over it.
func TestHookSweepEmitsOneCycleUnderOneComponent(t *testing.T) {
	t.Run("it emits exactly one record for an unclassified sweep failure", func(t *testing.T) {
		sink := logtest.Install(t)
		deps := hookCleanupDeps(&daemonFakeCommander{panesOut: livePaneRowOut}, sweepFailureFixture(t), discardDaemonLogger())
		deps.lastCleanup = time.Now().Add(-2 * hookCleanupInterval)

		maybeRunHookCleanup(deps)

		rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "record at or above WARN")
		if got := componentOf(rec); got != "hooks" {
			t.Errorf("failure record component = %q, want hooks: %+v", got, rec)
		}
		if rec.Msg != sweepFailedMsg {
			t.Errorf("failure record message = %q, want %q", rec.Msg, sweepFailedMsg)
		}
		if rec.ErrorAttr(t, "error") == nil {
			t.Error("failure record carries no error")
		}
	})
}
