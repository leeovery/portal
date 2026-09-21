package cmd

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// daemonPaneRow renders one captureFormat row for session "work". The twelfth
// column is the resume-pending marker, which an unmarked pane reads as empty;
// the eleventh (the pane token) is left empty, which is what an un-stamped pane
// reads as.
func daemonPaneRow(window, pane int, pending bool) string {
	pendingCol := ""
	if pending {
		pendingCol = "1"
	}
	return fmt.Sprintf("work|||%d|||main|||layout|||0|||%s|||%d|||/tmp|||%s|||zsh||||||%s",
		window, col01(window == 0), pane, col01(pane == 0), pendingCol)
}

func col01(set bool) string {
	if set {
		return "1"
	}
	return "0"
}

func resumePendingFixture(t *testing.T, fc *daemonFakeCommander) (*daemonDeps, *logtest.Sink) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)
	return makeCaptureDeps(t, dir, fc), sink
}

func captureTargets(fc *daemonFakeCommander) []string {
	var targets []string
	for _, call := range fc.callsContaining("capture-pane") {
		targets = append(targets, call[len(call)-1])
	}
	return targets
}

func scrollbackFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(state.ScrollbackDir(dir))
	if err != nil {
		t.Fatalf("read scrollback dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func paneKeyBreadcrumbs(sink *logtest.Sink) []string {
	var keys []string
	for _, rec := range sink.Records().WithMessage("pane captured") {
		keys = append(keys, rec.AttrOrEmpty("pane_key"))
	}
	slices.Sort(keys)
	return keys
}

func tickPaneCount(t *testing.T, sink *logtest.Sink) int64 {
	t.Helper()
	return sink.Records().Matching("capture", "tick complete").Only(t, "capture tick complete record").IntAttr(t, "panes")
}

// logShape reduces a capture sink to the set of messages and the set of attr
// keys it emitted, which is the taxonomy a pending pane must not widen.
func logShape(sink *logtest.Sink) (msgs, keys []string) {
	msgSet := map[string]struct{}{}
	keySet := map[string]struct{}{}
	for _, rec := range sink.Records() {
		msgSet[rec.Msg] = struct{}{}
		for _, k := range rec.Keys {
			keySet[k] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(msgSet)), slices.Sorted(maps.Keys(keySet))
}

func TestCaptureAndCommit_SkipsResumePendingPanes(t *testing.T) {
	t.Run("it captures a pane carrying neither marker exactly as today", func(t *testing.T) {
		fc := &daemonFakeCommander{
			sessionsOut:     "work|1|0|",
			panesOut:        daemonPaneRow(0, 0, false),
			captureByTarget: map[string]string{"work:0.0": "body-0"},
		}
		deps, sink := resumePendingFixture(t, fc)

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit: %v", err)
		}

		if got, want := captureTargets(fc), []string{"=work:0.0"}; !slices.Equal(got, want) {
			t.Errorf("capture-pane targets = %v, want %v", got, want)
		}
		if got, want := scrollbackFiles(t, deps.Dir), []string{"work__0.0.bin"}; !slices.Equal(got, want) {
			t.Errorf("scrollback files = %v, want %v", got, want)
		}
		if got, want := paneKeyBreadcrumbs(sink), []string{"work__0.0"}; !slices.Equal(got, want) {
			t.Errorf("pane captured breadcrumbs = %v, want %v", got, want)
		}
		if got := tickPaneCount(t, sink); got != 1 {
			t.Errorf("panes = %d, want 1", got)
		}
	})

	t.Run("it skips a pane carrying the pending marker and captures its live sibling", func(t *testing.T) {
		fc := &daemonFakeCommander{
			sessionsOut: "work|1|0|",
			panesOut:    daemonPaneRow(0, 0, false) + "\n" + daemonPaneRow(0, 1, true),
			captureByTarget: map[string]string{
				"work:0.0": "body-0",
				"work:0.1": "must-not-be-captured",
			},
		}
		deps, _ := resumePendingFixture(t, fc)

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit: %v", err)
		}

		if got, want := captureTargets(fc), []string{"=work:0.0"}; !slices.Equal(got, want) {
			t.Errorf("capture-pane targets = %v, want %v", got, want)
		}
		if got, want := scrollbackFiles(t, deps.Dir), []string{"work__0.0.bin"}; !slices.Equal(got, want) {
			t.Errorf("scrollback files = %v, want %v", got, want)
		}
	})

	t.Run("it skips a pane carrying both markers once", func(t *testing.T) {
		skipKey := state.SanitizePaneKey("work", 0, 1)
		fc := &daemonFakeCommander{
			markersOut:  fmt.Sprintf(`%s%s "1"`, state.SkeletonMarkerPrefix, skipKey),
			sessionsOut: "work|1|0|",
			panesOut:    daemonPaneRow(0, 0, false) + "\n" + daemonPaneRow(0, 1, true),
			captureByTarget: map[string]string{
				"work:0.0": "body-0",
				"work:0.1": "must-not-be-captured",
			},
		}
		deps, sink := resumePendingFixture(t, fc)

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit: %v", err)
		}

		if got, want := captureTargets(fc), []string{"=work:0.0"}; !slices.Equal(got, want) {
			t.Errorf("capture-pane targets = %v, want %v", got, want)
		}
		if got, want := paneKeyBreadcrumbs(sink), []string{"work__0.0"}; !slices.Equal(got, want) {
			t.Errorf("pane captured breadcrumbs = %v, want %v", got, want)
		}
		if got := tickPaneCount(t, sink); got != 1 {
			t.Errorf("panes = %d, want 1", got)
		}
	})

	t.Run("it leaves a skipped pane out of the tick summary pane count and emits no capture breadcrumb", func(t *testing.T) {
		fc := &daemonFakeCommander{
			sessionsOut: "work|1|0|",
			panesOut:    daemonPaneRow(0, 0, false) + "\n" + daemonPaneRow(0, 1, true),
			captureByTarget: map[string]string{
				"work:0.0": "body-0",
				"work:0.1": "must-not-be-captured",
			},
		}
		deps, sink := resumePendingFixture(t, fc)

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit: %v", err)
		}

		if got := tickPaneCount(t, sink); got != 1 {
			t.Errorf("panes = %d, want 1", got)
		}
		if got, want := paneKeyBreadcrumbs(sink), []string{"work__0.0"}; !slices.Equal(got, want) {
			t.Errorf("pane captured breadcrumbs = %v, want %v", got, want)
		}
	})

	t.Run("it returns a pane to ordinary capture and re-files it at its current address once the marker clears", func(t *testing.T) {
		fc := &daemonFakeCommander{
			sessionsOut:     "work|1|0|",
			panesOut:        daemonPaneRow(0, 1, true),
			captureByTarget: map[string]string{"work:0.1": "must-not-be-captured"},
		}
		deps, _ := resumePendingFixture(t, fc)

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit while pending: %v", err)
		}
		if got := scrollbackFiles(t, deps.Dir); len(got) != 0 {
			t.Fatalf("scrollback files while pending = %v, want none", got)
		}

		fc.panesOut = daemonPaneRow(1, 0, false)
		fc.captureByTarget = map[string]string{"work:1.0": "body-after-clear"}

		if err := captureAndCommit(context.Background(), deps); err != nil {
			t.Fatalf("captureAndCommit after clear: %v", err)
		}

		if got, want := captureTargets(fc), []string{"=work:1.0"}; !slices.Equal(got, want) {
			t.Errorf("capture-pane targets = %v, want %v", got, want)
		}
		if got, want := scrollbackFiles(t, deps.Dir), []string{"work__1.0.bin"}; !slices.Equal(got, want) {
			t.Errorf("scrollback files = %v, want %v", got, want)
		}
	})

	t.Run("it applies the same skip to the shutdown flush", func(t *testing.T) {
		fc := &daemonFakeCommander{
			sessionsOut: "work|1|0|",
			panesOut:    daemonPaneRow(0, 0, false) + "\n" + daemonPaneRow(0, 1, true),
			captureByTarget: map[string]string{
				"work:0.0": "body-0",
				"work:0.1": "must-not-be-captured",
			},
		}
		deps, sink := resumePendingFixture(t, fc)

		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}

		if got, want := captureTargets(fc), []string{"=work:0.0"}; !slices.Equal(got, want) {
			t.Errorf("capture-pane targets = %v, want %v", got, want)
		}
		if got, want := scrollbackFiles(t, deps.Dir), []string{"work__0.0.bin"}; !slices.Equal(got, want) {
			t.Errorf("scrollback files = %v, want %v", got, want)
		}
		rec := sink.Records().Matching("daemon", "shutdown").Only(t, "daemon shutdown record")
		if got := rec.AttrOrEmpty("flush_completed"); got != "true" {
			t.Errorf("flush_completed = %q, want \"true\"", got)
		}
	})

	t.Run("it introduces no new log event or attr key", func(t *testing.T) {
		run := func(t *testing.T, pending bool) *logtest.Sink {
			t.Helper()
			fc := &daemonFakeCommander{
				sessionsOut: "work|1|0|",
				panesOut:    daemonPaneRow(0, 0, false) + "\n" + daemonPaneRow(0, 1, pending),
				captureByTarget: map[string]string{
					"work:0.0": "body-0",
					"work:0.1": "body-1",
				},
			}
			deps, sink := resumePendingFixture(t, fc)
			if err := captureAndCommit(context.Background(), deps); err != nil {
				t.Fatalf("captureAndCommit: %v", err)
			}
			return sink
		}

		var withPending, withoutPending *logtest.Sink
		t.Run("pending", func(t *testing.T) { withPending = run(t, true) })
		t.Run("unmarked", func(t *testing.T) { withoutPending = run(t, false) })

		pendingMsgs, pendingKeys := logShape(withPending)
		plainMsgs, plainKeys := logShape(withoutPending)
		if !slices.Equal(pendingMsgs, plainMsgs) {
			t.Errorf("messages with a pending pane = %v, want %v", pendingMsgs, plainMsgs)
		}
		if !slices.Equal(pendingKeys, plainKeys) {
			t.Errorf("attr keys with a pending pane = %v, want %v", pendingKeys, plainKeys)
		}
		if got, want := paneKeyBreadcrumbs(withPending), []string{"work__0.0"}; !slices.Equal(got, want) {
			t.Errorf("pending-tick breadcrumbs = %v, want %v", got, want)
		}
		if got, want := paneKeyBreadcrumbs(withoutPending), []string{"work__0.0", "work__0.1"}; !slices.Equal(got, want) {
			t.Errorf("unmarked-tick breadcrumbs = %v, want %v", got, want)
		}
	})
}
