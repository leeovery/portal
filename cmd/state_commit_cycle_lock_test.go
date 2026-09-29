package cmd

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

const heldCommitLockBound = 40 * time.Millisecond

// holdCommitLock takes the commit lock through an open file description of its
// own, as another committer's process would, and returns its release.
func holdCommitLock(t *testing.T, dir string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(state.CommitLock(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		t.Fatalf("flock commit lock: %v", err)
	}
	release = func() { _ = f.Close() }
	t.Cleanup(release)
	return release
}

// committedState is what a committer can change on disk: sessions.json's bytes
// and the scrollback directory's file set.
type committedState struct {
	sessionsJSON []byte
	scrollback   []string
}

func readCommittedState(t *testing.T, dir string) committedState {
	t.Helper()
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	entries, err := os.ReadDir(state.ScrollbackDir(dir))
	if err != nil {
		t.Fatalf("read scrollback dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return committedState{sessionsJSON: data, scrollback: names}
}

func assertCommittedStateUnchanged(t *testing.T, dir string, before committedState) {
	t.Helper()
	after := readCommittedState(t, dir)
	if !bytes.Equal(before.sessionsJSON, after.sessionsJSON) {
		t.Errorf("sessions.json changed:\nbefore %s\nafter  %s", before.sessionsJSON, after.sessionsJSON)
	}
	if !slices.Equal(before.scrollback, after.scrollback) {
		t.Errorf("scrollback files = %v, want %v", after.scrollback, before.scrollback)
	}
}

// seedLockedCycleState commits an index naming a session that is no longer
// live, beside its transcript and an unreferenced file, so any capture, re-file,
// commit or housekeeping pass would change what is on disk.
func seedLockedCycleState(t *testing.T, dir string) {
	t.Helper()
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	seed := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{{
			Name:        "gone",
			Environment: map[string]string{},
			Windows: []state.Window{{Index: 0, Name: "main", Panes: []state.Pane{
				{Index: 0, CWD: "/tmp", ScrollbackFile: "scrollback/gone__0.0.bin"},
			}}},
		}},
	}
	if err := state.Commit(dir, seed, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	for _, name := range []string{"gone__0.0.bin", "orphan.bin"} {
		if err := os.WriteFile(state.ScrollbackDir(dir)+"/"+name, []byte(name), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
}

func assertNoCaptureReads(t *testing.T, fc *daemonFakeCommander) {
	t.Helper()
	for _, verb := range []string{"list-sessions", "list-panes", "capture-pane"} {
		if got := fc.callsContaining(verb); len(got) != 0 {
			t.Errorf("%s invoked while another committer held the cycle: %v", verb, got)
		}
	}
	for _, call := range fc.callsContaining("show-options") {
		if slices.Equal(call, []string{"show-options", "-s"}) {
			t.Errorf("skeleton markers read while another committer held the cycle")
		}
	}
}

func TestDaemonTick_StandsDownWhileAnotherCommitterHoldsTheCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	state.SetCommitLockTimeoutForTest(t, heldCommitLockBound)
	seedLockedCycleState(t, dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	touchSaveRequested(t, dir)
	release := holdCommitLock(t, dir)
	before := readCommittedState(t, dir)

	tick(t.Context(), deps)

	warn := sink.Records().Matching("daemon", "tick failed").AtExactLevel(slog.LevelWarn).Only(t, "tick failed WARN")
	if err := warn.ErrorAttr(t, "error"); !errors.Is(err, state.ErrCommitLockHeld) {
		t.Errorf("tick failed error = %v, want ErrCommitLockHeld", err)
	}
	assertNoCaptureReads(t, fc)
	assertCommittedStateUnchanged(t, dir, before)
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must stay for the next tick to retry; stat err = %v", err)
	}

	release()
	tick(t.Context(), deps)

	got := readCommittedState(t, dir)
	if bytes.Equal(got.sessionsJSON, before.sessionsJSON) {
		t.Error("sessions.json not committed by the tick after the lock freed")
	}
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Errorf("save.requested stat err = %v, want it removed by the committing tick", err)
	}
}

func TestDefaultShutdownFlush_FailsWhileAnotherCommitterHoldsTheCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	state.SetCommitLockTimeoutForTest(t, heldCommitLockBound)
	seedLockedCycleState(t, dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGTERM)
	holdCommitLock(t, dir)
	before := readCommittedState(t, dir)

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	if n := countLines(sink, "WARN", "final flush failed"); n != 1 {
		t.Errorf("final flush failed WARNs = %d, want 1 in:\n%s", n, sink.Body())
	}
	if n := countLines(sink, "INFO", "shutdown", "flush_completed=false"); n != 1 {
		t.Errorf("shutdown flush_completed=false lines = %d, want 1 in:\n%s", n, sink.Body())
	}
	assertNoCaptureReads(t, fc)
	assertCommittedStateUnchanged(t, dir, before)
}

func TestStateCommitNow_FailsWhileAnotherCommitterHoldsTheCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("PORTAL_LOG_LEVEL", "error")
	sink := logtest.Install(t)
	state.SetCommitLockTimeoutForTest(t, heldCommitLockBound)
	seedLockedCycleState(t, dir)
	client := &fakeCaptureClient{sessions: []string{"work"}}
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return client },
		IsRestoring: func() (bool, error) { return false, nil },
	})
	holdCommitLock(t, dir)
	before := readCommittedState(t, dir)

	_, errBuf, err := runRootCmd(t, "state", "commit-now")

	if !errors.Is(err, errCommitNowFailed) {
		t.Fatalf("commit-now error = %v, want errCommitNowFailed", err)
	}
	if !strings.Contains(err.Error(), state.ErrCommitLockHeld.Error()) {
		t.Errorf("commit-now error = %v, want it to carry the lock timeout", err)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want silent", errBuf.String())
	}
	errRecords := sink.Records().AtExactLevel(slog.LevelError)
	if len(errRecords) != 1 {
		t.Fatalf("ERROR records = %d, want 1 in:\n%s", len(errRecords), sink.Body())
	}
	if client.sessionCalls != 0 {
		t.Errorf("list-sessions reads = %d, want 0", client.sessionCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested must be touched for the daemon to retry; stat err = %v", err)
	}
	assertCommittedStateUnchanged(t, dir, before)
}
