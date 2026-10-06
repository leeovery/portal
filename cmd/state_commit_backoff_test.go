package cmd

import (
	"errors"
	"log/slog"
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

const (
	tickBackedOff        = "tick backed off: tmux stopped answering"
	finalFlushBackedOff  = "final flush backed off: tmux stopped answering"
	commitCycleBackedOff = "commit cycle backed off: tmux stopped answering"
)

// standDown is one way a committing cycle stands down because tmux stopped
// answering, with the tmux words its cause carries. heal makes every read
// answer again; readsNoListing marks a refusal landing before the session
// listing.
type standDown struct {
	name           string
	fc             func() *daemonFakeCommander
	stderr         string
	heal           func(fc *daemonFakeCommander)
	readsNoListing bool
}

var standDowns = []standDown{
	{
		name:           "a refused skeleton-marker read",
		fc:             refusedMarkerReadCommander,
		stderr:         "no server running",
		heal:           func(fc *daemonFakeCommander) { fc.markersErr = nil },
		readsNoListing: true,
	},
	{
		name:   "a failed session listing",
		fc:     failingListSessionsCommander,
		stderr: "server exited unexpectedly",
		heal: func(fc *daemonFakeCommander) {
			fc.sessionsErr = nil
			fc.sessionsOut, fc.panesOut = oneSession()
		},
	},
	{
		name:   "a refused pane listing",
		fc:     refusedPaneListingCommander,
		stderr: "server exited unexpectedly",
		heal:   func(fc *daemonFakeCommander) { fc.panesErr = nil },
	},
	{
		name:   "every environment read and the confirmation refused",
		fc:     refusedEnvironmentReadsCommander,
		stderr: "server exited unexpectedly",
		heal: func(fc *daemonFakeCommander) {
			fc.envErrBySession = nil
			fc.confirmErr = nil
		},
	},
	{
		name:   "a refused confirmation",
		fc:     func() *daemonFakeCommander { return workOnlyCommander(refusedConfirmation()) },
		stderr: "no server running",
		heal:   func(fc *daemonFakeCommander) { fc.confirmErr = nil },
	},
}

// refusedMarkerReadCommander answers every read but the skeleton-marker read,
// which an exiting tmux refuses.
func refusedMarkerReadCommander() *daemonFakeCommander {
	fc := workOnlyCommander(nil)
	fc.markersErr = &tmux.CommandError{Args: []string{"show-options"}, Stderr: "no server running", Err: errors.New("exit status 1")}
	return fc
}

// refusedPaneListingCommander lists "work" as live, then refuses the pane
// listing the way an exiting tmux does.
func refusedPaneListingCommander() *daemonFakeCommander {
	fc := workOnlyCommander(nil)
	fc.panesErr = &tmux.CommandError{Args: []string{"list-panes"}, Stderr: "server exited unexpectedly", Err: errors.New("exit status 1")}
	return fc
}

// refusedEnvironmentReadsCommander lists "work" as live with its panes, then
// refuses its environment read and the confirmation after it, the way a tmux
// that began exiting after the pane listing does.
func refusedEnvironmentReadsCommander() *daemonFakeCommander {
	fc := workOnlyCommander(refusedConfirmation())
	fc.envErrBySession = map[string]error{
		"work": &tmux.CommandError{Args: []string{"show-environment"}, Stderr: "server exited unexpectedly", Err: errors.New("exit status 1")},
	}
	return fc
}

// assertListingUnread checks a refusal landing before the session listing ended
// the cycle without reading it.
func assertListingUnread(t *testing.T, fc *daemonFakeCommander, sd standDown) {
	t.Helper()
	if !sd.readsNoListing {
		return
	}
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions read after %s: %v", sd.name, got)
	}
}

// assertBackOffLine finds the one line msg names at INFO or above, and checks
// it carries the stand-down's cause in error and nothing beside it.
func assertBackOffLine(t *testing.T, sink *logtest.Sink, msg string, sd standDown) {
	t.Helper()
	line := sink.Records().Matching("daemon", msg).AtOrAboveLevel(slog.LevelInfo).Only(t, "back-off line")
	if want := []string{"component", "error"}; !slices.Equal(line.Keys, want) {
		t.Errorf("back-off line keys = %v, want %v", line.Keys, want)
	}
	cause := line.ErrorAttr(t, "error")
	cmdErr, ok := errors.AsType[*tmux.CommandError](cause)
	if !ok || cmdErr.Stderr != sd.stderr {
		t.Errorf("back-off line error = %v, want tmux's %q", cause, sd.stderr)
	}
}

func assertSaveRequested(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested stat err = %v, want it present", err)
	}
}

func assertNoFailedLine(t *testing.T, sink *logtest.Sink, msg string) {
	t.Helper()
	if got := sink.Records().WithMessage(msg); len(got) != 0 {
		t.Errorf("%q lines = %d beside the back-off line, want none", msg, len(got))
	}
}

func TestDaemonTickReportsAStandDownAsABackOff(t *testing.T) {
	for _, sd := range standDowns {
		t.Run(sd.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := sd.fc()
			deps := makeDeps(t, saved.dir, fc)
			deps.PrevIndex = &saved.index
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger

			tick(t.Context(), deps)

			assertBackOffLine(t, sink, tickBackedOff, sd)
			assertNoFailedLine(t, sink, "tick failed")
			assertListingUnread(t, fc, sd)
			assertSaveRequested(t, saved.dir)
			saved.assertUnchanged(t)
		})
	}
}

func TestCommitNowReportsAStandDownAsABackOff(t *testing.T) {
	for _, sd := range standDowns {
		t.Run(sd.name, func(t *testing.T) {
			saved := seedSavedState(t)
			sink := logtest.Install(t)
			withOwnTmuxServer(t, fakeOwnServerPID)
			fc := sd.fc()
			client := tmux.NewClient(fc)
			withCommitNowDeps(t, CommitNowDeps{
				NewClient:   func() state.CaptureCycleClient { return client },
				IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
			})

			_, _, err := runRootCmd(t, "state", "commit-now")

			if !errors.Is(err, errCommitNowFailed) {
				t.Errorf("commit-now error = %v, want a non-zero exit wrapping errCommitNowFailed", err)
			}
			assertBackOffLine(t, sink, commitCycleBackedOff, sd)
			assertNoFailedLine(t, sink, "commit cycle failed")
			assertListingUnread(t, fc, sd)
			assertSaveRequested(t, saved.dir)
			saved.assertUnchanged(t)
		})
	}
}

func TestShutdownFlushReportsAStandDownAsABackOff(t *testing.T) {
	for _, sd := range standDowns {
		t.Run(sd.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := sd.fc()
			deps := makeDeps(t, saved.dir, fc)
			deps.PrevIndex = &saved.index
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger

			if err := defaultShutdownFlush(deps); err != nil {
				t.Fatalf("defaultShutdownFlush: %v", err)
			}

			assertBackOffLine(t, sink, finalFlushBackedOff, sd)
			assertNoFailedLine(t, sink, "final flush failed")
			assertListingUnread(t, fc, sd)
			shutdown := sink.Records().Matching("daemon", "shutdown").AtExactLevel(slog.LevelInfo).Only(t, "shutdown line")
			if got := shutdown.AttrString(t, "flush_completed"); got != "false" {
				t.Errorf("shutdown flush_completed = %q, want false", got)
			}
			saved.assertUnchanged(t)
		})
	}
}

func TestDaemonTickCommitsOnTheTickAfterAStandDown(t *testing.T) {
	for _, sd := range standDowns {
		t.Run(sd.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := sd.fc()
			deps := makeDeps(t, saved.dir, fc)
			deps.PrevIndex = &saved.index

			tick(t.Context(), deps)

			saved.assertUnchanged(t)
			assertSaveRequested(t, saved.dir)

			fc.mu.Lock()
			sd.heal(fc)
			fc.mu.Unlock()
			deps.LastSaveAt = time.Now()

			tick(t.Context(), deps)

			assertWorkAloneCommitted(t, saved.dir)
		})
	}
}

func TestDaemonTickCommitsAKillWhoseCommitNowStoodDown(t *testing.T) {
	saved := seedSavedState(t)
	withOwnTmuxServer(t, fakeOwnServerPID)
	hookClient := tmux.NewClient(failingListSessionsCommander())
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return hookClient },
		IsRestoring: func() (bool, error) { return state.IsRestoringSet(hookClient) },
	})

	if _, _, err := runRootCmd(t, "state", "commit-now"); !errors.Is(err, errCommitNowFailed) {
		t.Fatalf("commit-now error = %v, want it to stand down", err)
	}
	saved.assertUnchanged(t)

	deps := makeDeps(t, saved.dir, workOnlyCommander(nil))
	deps.PrevIndex = &saved.index
	deps.LastSaveAt = time.Now()

	tick(t.Context(), deps)

	assertWorkAloneCommitted(t, saved.dir)
}

// assertWorkAloneCommitted checks "notes" has left sessions.json with its
// transcript file, while "work" keeps its own.
func assertWorkAloneCommitted(t *testing.T, dir string) {
	t.Helper()
	committed, skip, err := state.ReadIndex(dir)
	if err != nil || skip {
		t.Fatalf("ReadIndex = (skip %v, err %v), want the committed index", skip, err)
	}
	if len(committed.Sessions) != 1 || committed.Sessions[0].Name != "work" {
		t.Errorf("committed sessions = %+v, want work alone", committed.Sessions)
	}
	want := []string{state.SanitizePaneKey("work", 0, 0) + ".bin"}
	if got := slices.Sorted(maps.Keys(readScrollbackDir(t, dir))); !slices.Equal(got, want) {
		t.Errorf("scrollback files = %v, want %v", got, want)
	}
}

// markerCommitter runs one committer against a restore marker read through fc,
// returning the daemon-component messages it logged.
type markerCommitter struct {
	name string
	run  func(t *testing.T, dir string, fc *daemonFakeCommander) logtest.Records
}

func daemonRecords(sink *logtest.Sink) logtest.Records {
	var out logtest.Records
	for _, r := range sink.Records() {
		if r.AttrOrEmpty("component") == "daemon" {
			out = append(out, r)
		}
	}
	return out
}

var markerCommitters = []markerCommitter{
	{"the daemon's tick", func(t *testing.T, dir string, fc *daemonFakeCommander) logtest.Records {
		deps := makeDeps(t, dir, fc)
		logger, sink := newCaptureLoggerForComponent(t, "daemon")
		deps.Logger = logger
		tick(t.Context(), deps)
		return daemonRecords(sink)
	}},
	{"the daemon's shutdown flush", func(t *testing.T, dir string, fc *daemonFakeCommander) logtest.Records {
		deps := makeDeps(t, dir, fc)
		logger, sink := newCaptureLoggerForComponent(t, "daemon")
		deps.Logger = logger
		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}
		return daemonRecords(sink)
	}},
	{"commit-now", func(t *testing.T, _ string, fc *daemonFakeCommander) logtest.Records {
		sink := logtest.Install(t)
		withOwnTmuxServer(t, fakeOwnServerPID)
		client := tmux.NewClient(fc)
		withCommitNowDeps(t, CommitNowDeps{
			NewClient:          func() state.CaptureCycleClient { return client },
			IsRestoring:        func() (bool, error) { return state.IsRestoringSet(client) },
			TouchSaveRequested: func(string) error { return nil },
		})
		if _, _, err := runRootCmd(t, "state", "commit-now"); err != nil {
			t.Fatalf("commit-now: %v", err)
		}
		return daemonRecords(sink)
	}},
}

func TestCommittersKeepTheirRestoreMarkerReadFailureLines(t *testing.T) {
	want := map[string]string{
		"the daemon's tick":           "read @portal-restoring failed",
		"the daemon's shutdown flush": "read @portal-restoring at shutdown failed; skipping final flush",
		"commit-now":                  "isRestoring query failed; presuming @portal-restoring set to protect in-flight restore",
	}
	for _, c := range markerCommitters {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(nil)
			fc.optionErr = transportErrCommandError()

			records := c.run(t, saved.dir, fc)

			line := records.WithMessage(want[c.name]).AtExactLevel(slog.LevelWarn).Only(t, "marker read failure line")
			if want := []string{"component", "error"}; !slices.Equal(line.Keys, want) {
				t.Errorf("marker read failure keys = %v, want %v", line.Keys, want)
			}
			if cmdErr, ok := errors.AsType[*tmux.CommandError](line.ErrorAttr(t, "error")); !ok || cmdErr.Stderr != "lost server" {
				t.Errorf("marker read failure error = %v, want tmux's %q", cmdErr, "lost server")
			}
			assertNoBackOffLine(t, records)
			saved.assertUnchanged(t)
		})
	}
}

func TestCommittersLogNothingNewWhileARestoreIsInProgress(t *testing.T) {
	want := map[string][]string{
		"the daemon's tick":           nil,
		"the daemon's shutdown flush": {"skipping final flush: @portal-restoring set", "shutdown"},
		"commit-now":                  {"commit-now skipped: @portal-restoring set"},
	}
	for _, c := range markerCommitters {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(nil)
			fc.optionByName = map[string]string{state.RestoringMarkerName: "1"}

			records := c.run(t, saved.dir, fc)

			got := make([]string, 0, len(records))
			for _, r := range records {
				got = append(got, r.Msg)
			}
			if !slices.Equal(got, want[c.name]) {
				t.Errorf("logged %q, want %q", got, want[c.name])
			}
			saved.assertUnchanged(t)
		})
	}
}

func assertNoBackOffLine(t *testing.T, records logtest.Records) {
	t.Helper()
	for _, msg := range []string{tickBackedOff, finalFlushBackedOff, commitCycleBackedOff} {
		if got := records.WithMessage(msg); len(got) != 0 {
			t.Errorf("%q logged %d times, want none", msg, len(got))
		}
	}
}

func TestDaemonTickReportsAFailureThatIsNoRefusedReadAsFailed(t *testing.T) {
	tests := []struct {
		name  string
		stage func(t *testing.T) (dir string, fc *daemonFakeCommander, prev *state.Index)
	}{
		{"a pane listing that fails to parse", func(t *testing.T) (string, *daemonFakeCommander, *state.Index) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(nil)
			fc.panesOut = "work|||not-a-pane-row"
			return saved.dir, fc, &saved.index
		}},
		{"a carry onto a captured session's name", func(t *testing.T) (string, *daemonFakeCommander, *state.Index) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			prev := seedCarryState(t, dir)
			return dir, &daemonFakeCommander{
				sessionsOut:     "baz|1|0|\nfoo|1|0|",
				panesOut:        collidedRows(),
				envErrBySession: map[string]error{"baz": &tmux.CommandError{Stderr: "no such session: baz", Err: errors.New("exit status 1")}},
			}, &prev
		}},
		{"every session's environment read failing anomalously", func(t *testing.T) (string, *daemonFakeCommander, *state.Index) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(nil)
			fc.envErrBySession = map[string]error{"work": errors.New("environment read blew up")}
			return saved.dir, fc, &saved.index
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, fc, prev := tt.stage(t)
			deps := makeDeps(t, dir, fc)
			deps.PrevIndex = prev
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger

			tick(t.Context(), deps)

			sink.Records().Matching("daemon", "tick failed").AtExactLevel(slog.LevelWarn).Only(t, "tick failed WARN")
			assertNoBackOffLine(t, sink.Records())
		})
	}
}
