package cmd

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxerr"
)

// unparseableListingCommander answers every capture read, but lists a live
// "work|notes" beside "work": the pipe shifts the listing's fields, so tmux
// answers with a line Portal cannot parse.
func unparseableListingCommander() *daemonFakeCommander {
	fc := workOnlyCommander(nil)
	fc.sessionsOut = "work|1|0|\nwork|notes|1|0|"
	return fc
}

// unparseableListingCommitter runs one committer against fc over the saved
// state, returning what it logged and the error commit-now exited with.
type unparseableListingCommitter struct {
	name      string
	failed    string
	backedOff string
	level     slog.Level
	run       func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander) (*logtest.Sink, error)
}

var unparseableListingCommitters = []unparseableListingCommitter{
	{"the daemon's tick", "tick failed", tickBackedOff, slog.LevelWarn,
		func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander) (*logtest.Sink, error) {
			deps := makeDeps(t, saved.dir, fc)
			deps.PrevIndex = &saved.index
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger
			tick(t.Context(), deps)
			return sink, nil
		}},
	{"the daemon's shutdown flush", "final flush failed", finalFlushBackedOff, slog.LevelWarn,
		func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander) (*logtest.Sink, error) {
			deps := makeDeps(t, saved.dir, fc)
			deps.PrevIndex = &saved.index
			logger, sink := newCaptureLoggerForComponent(t, "daemon")
			deps.Logger = logger
			if err := defaultShutdownFlush(deps); err != nil {
				t.Fatalf("defaultShutdownFlush: %v", err)
			}
			return sink, nil
		}},
	{"commit-now", "commit cycle failed", commitCycleBackedOff, slog.LevelError,
		func(t *testing.T, _ savedStateFixture, fc *daemonFakeCommander) (*logtest.Sink, error) {
			sink := logtest.Install(t)
			withOwnTmuxServer(t, fakeOwnServerPID)
			client := tmux.NewClient(fc)
			withCommitNowDeps(t, CommitNowDeps{
				NewClient:   func() state.CaptureCycleClient { return client },
				IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
			})
			_, _, err := runRootCmd(t, "state", "commit-now")
			return sink, err
		}},
}

// assertFailureRoute checks the committer took its existing failure route:
// save.requested left for the next tick, commit-now's non-zero exit, and the
// shutdown line's flush_completed=false.
func assertFailureRoute(t *testing.T, c unparseableListingCommitter, saved savedStateFixture, sink *logtest.Sink, err error) {
	t.Helper()
	switch c.name {
	case "the daemon's tick":
		assertSaveRequested(t, saved.dir)
	case "the daemon's shutdown flush":
		shutdown := sink.Records().Matching("daemon", "shutdown").AtExactLevel(slog.LevelInfo).Only(t, "shutdown line")
		if got := shutdown.AttrString(t, "flush_completed"); got != "false" {
			t.Errorf("shutdown flush_completed = %q, want false", got)
		}
	case "commit-now":
		if !errors.Is(err, errCommitNowFailed) {
			t.Errorf("commit-now error = %v, want a non-zero exit wrapping errCommitNowFailed", err)
		}
		assertSaveRequested(t, saved.dir)
	}
}

func TestCommittersReportAnUnparseableSessionListingTheirOwnServerAnswersAsFailed(t *testing.T) {
	for _, c := range unparseableListingCommitters {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := unparseableListingCommander()

			sink, err := c.run(t, saved, fc)

			line := sink.Records().Matching("daemon", c.failed).AtExactLevel(c.level).Only(t, "failed line")
			cause := line.ErrorAttr(t, "error")
			if !errors.Is(cause, tmuxerr.ErrSessionListUnparseable) {
				t.Errorf("%q error = %v, want the listing's parse error", c.failed, cause)
			}
			if errors.Is(cause, state.ErrTmuxStoppedAnswering) {
				t.Errorf("%q error = %v, want no stand-down", c.failed, cause)
			}
			if got := fc.callsContaining("display-message"); len(got) == 0 {
				t.Error("no confirmation sent after the unparseable listing")
			}
			assertNoBackOffLine(t, sink.Records())
			assertFailureRoute(t, c, saved, sink, err)
			saved.assertUnchanged(t)
		})
	}
}

func TestDaemonTickRetriesAfterAnUnparseableSessionListing(t *testing.T) {
	saved := seedSavedState(t)
	fc := unparseableListingCommander()
	deps := makeDeps(t, saved.dir, fc)
	deps.PrevIndex = &saved.index

	tick(t.Context(), deps)

	saved.assertUnchanged(t)
	assertSaveRequested(t, saved.dir)

	fc.mu.Lock()
	fc.sessionsOut, _ = oneSession()
	fc.mu.Unlock()
	deps.LastSaveAt = time.Now()

	tick(t.Context(), deps)

	assertWorkAloneCommitted(t, saved.dir)
}

func TestCommittersReportAnUnparseableSessionListingTheirOwnServerDoesNotConfirmAsABackOff(t *testing.T) {
	confirmations := []struct {
		name        string
		stage       func(fc *daemonFakeCommander)
		wantRefusal func(error) bool
	}{
		{"a refused confirmation", func(fc *daemonFakeCommander) { fc.confirmErr = refusedConfirmation() }, func(err error) bool {
			cmdErr, ok := errors.AsType[*tmux.CommandError](err)
			return ok && cmdErr.Stderr == "no server running"
		}},
		{"a confirmation naming no server", func(fc *daemonFakeCommander) { fc.silentConfirm = true }, func(err error) bool {
			return errors.Is(err, state.ErrNotOwnServer)
		}},
		{"a confirmation from another server", func(fc *daemonFakeCommander) { fc.answeringPID = fakeOwnServerPID + 1 }, func(err error) bool {
			return errors.Is(err, state.ErrNotOwnServer)
		}},
	}
	for _, conf := range confirmations {
		for _, c := range unparseableListingCommitters {
			t.Run(conf.name+"/"+c.name, func(t *testing.T) {
				saved := seedSavedState(t)
				fc := unparseableListingCommander()
				conf.stage(fc)

				sink, err := c.run(t, saved, fc)

				line := sink.Records().Matching("daemon", c.backedOff).AtOrAboveLevel(slog.LevelInfo).Only(t, "back-off line")
				cause := line.ErrorAttr(t, "error")
				if !errors.Is(cause, state.ErrTmuxStoppedAnswering) {
					t.Errorf("back-off error = %v, want one wrapping ErrTmuxStoppedAnswering", cause)
				}
				if !errors.Is(cause, tmuxerr.ErrSessionListUnparseable) {
					t.Errorf("back-off error = %v, want the listing's parse error reachable through it", cause)
				}
				if !conf.wantRefusal(cause) {
					t.Errorf("back-off error = %v, want the confirmation's cause reachable through it", cause)
				}
				assertNoFailedLine(t, sink, c.failed)
				assertFailureRoute(t, c, saved, sink, err)
				saved.assertUnchanged(t)
			})
		}
	}
}
