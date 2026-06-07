// Tests in this file mutate package-level state and MUST NOT use t.Parallel.
//
// Phase 5 Task 5-1: the daemon tick cycle summary. captureAndCommit emits
// exactly ONE INFO "capture: tick complete" line per tick that does capture
// work, under the capture-bound logger (component "capture", promoted out of
// daemon), plus per-pane DEBUG breadcrumbs (steady) and the existing per-pane
// WARN (anomaly). natural_churn classification (option a): a pane/session that
// the user closed mid-tick — surfaced by tmux's "can't find {session,window,
// pane}" capture-pane stderr — counts as natural_churn (DEBUG), not anomalous.
//
// Spec reference: § Cycle-level summary cadence and shape (daemon-tick row of
// the concrete cycle catalog); § Subsystem prefix taxonomy (capture component).

package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// captureSummarySink is a thin wrapper over the shared logtest.Sink that adds
// the capture-cycle's "tick complete" record filtering. The capture-cycle
// summary tests assert on the structured record (component=capture, msg, int
// attr values, took rendered as a duration) via the sink's shared accessors.
type captureSummarySink struct {
	*logtest.Sink
}

// summaries returns every record whose component=capture and msg="tick complete".
func (s *captureSummarySink) summaries() []logtest.Record {
	var out []logtest.Record
	for _, r := range s.Records() {
		comp, ok := r.Attrs["component"]
		if !ok || comp.String() != "capture" || r.Msg != "tick complete" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// onlySummary asserts exactly one capture: tick complete record was emitted and
// returns it.
func (s *captureSummarySink) onlySummary(t *testing.T) logtest.Record {
	t.Helper()
	sums := s.summaries()
	if len(sums) != 1 {
		t.Fatalf("expected exactly 1 capture: tick complete summary, got %d: %+v", len(sums), s.Records())
	}
	return sums[0]
}

func installCaptureSummarySink(t *testing.T) *captureSummarySink {
	t.Helper()
	sink := &captureSummarySink{Sink: &logtest.Sink{}}
	log.SetTestHandler(t, sink.Sink)
	return sink
}

// paneVanishedCommandErr returns a *tmux.CommandError whose stderr carries
// tmux's canonical "can't find {session,window,pane}: <x>" phrasing — the shape
// CapturePane surfaces (un-sentinel-wrapped) when a pane/session the index
// still references vanished mid-tick. This is the natural-churn signal that
// option (a) classifies as a clean close, not an anomalous capture failure.
func paneVanishedCommandErr(kind, name string) error {
	return &tmux.CommandError{
		Stderr: "can't find " + kind + ": " + name,
		Err:    errors.New("exit status 1"),
	}
}

// makeCaptureDeps assembles a tick-ready daemonDeps over the given fake
// commander. deps.Logger is io.Discard-backed (the daemon-component WARNs are
// asserted via the process-wide capture sink, not deps.Logger, in these tests).
func makeCaptureDeps(t *testing.T, dir string, fc *daemonFakeCommander) *daemonDeps {
	t.Helper()
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	return &daemonDeps{
		Dir:          dir,
		Logger:       daemonLogger,
		Client:       tmux.NewClient(fc),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
		LastSaveAt:   time.Now(),
	}
}

// breakScrollbackDir replaces the state dir's scrollback subdirectory with a
// regular file so WriteScrollbackIfChanged's AtomicWrite0600 fails at the
// MkdirAll/temp-create phase — a genuine (non-vanished) write failure that
// must classify as anomalous.
func breakScrollbackDir(t *testing.T, dir string) {
	t.Helper()
	sbDir := state.ScrollbackDir(dir)
	if err := os.RemoveAll(sbDir); err != nil {
		t.Fatalf("remove scrollback dir: %v", err)
	}
	if err := os.WriteFile(sbDir, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("seed scrollback-dir-as-file: %v", err)
	}
}

// breakCommitTarget creates a directory at the sessions.json path so
// state.Commit's atomic rename fails — a phase-boundary error that must NOT
// produce a tick-complete summary.
func breakCommitTarget(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(state.SessionsJSON(dir), 0o700); err != nil {
		t.Fatalf("seed sessions.json-as-dir: %v", err)
	}
}

func TestCaptureAndCommit_EmitsOneTickCompleteSummaryOnSuccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.onlySummary(t)
	if rec.Level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "sessions"); got != 1 {
		t.Errorf("sessions = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "panes"); got != 1 {
		t.Errorf("panes = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "natural_churn"); got != 0 {
		t.Errorf("natural_churn = %d, want 0", got)
	}
	if got := rec.IntAttr(t, "anomalous"); got != 0 {
		t.Errorf("anomalous = %d, want 0", got)
	}
	tookVal, ok := rec.Attrs["took"]
	if !ok {
		t.Fatalf("summary missing took attr: %+v", rec.Attrs)
	}
	if tookVal.Kind() != slog.KindDuration {
		t.Errorf("took kind = %v, want Duration", tookVal.Kind())
	}
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	// Cancel before entry — obs point 1 (pre-enumeration) returns nil first.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Fatalf("captureAndCommit on cancelled ctx = %v, want nil", err)
	}
	if got := sink.summaries(); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-1 cancel, got %d: %+v", len(got), got)
	}
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked after obs-point-1 cancel: %v", got)
	}
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint2(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	// dispatchHook fires cancel() after CaptureStructure's show-environment
	// subcall, so the obs-point-2 check (post-enumeration, pre-iteration)
	// observes the cancellation and returns nil before any per-pane work.
	ctx, cancel := context.WithCancel(context.Background())
	fc := &daemonFakeCommander{
		sessionsOut: sess,
		panesOut:    panes,
		dispatchHook: func(args []string) {
			if len(args) > 0 && args[0] == "show-environment" {
				cancel()
			}
		},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Fatalf("captureAndCommit = %v, want nil", err)
	}
	if got := sink.summaries(); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-2 cancel, got %d: %+v", len(got), got)
	}
	if got := fc.callsContaining("capture-pane"); len(got) != 0 {
		t.Errorf("capture-pane invoked after obs-point-2 cancel: %v", got)
	}
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint3(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	// Two panes. dispatchHook fires cancel() on the FIRST capture-pane call, so
	// the first iteration captures normally but the SECOND iteration's
	// obs-point-3 (between per-pane iterations) observes the cancellation and
	// returns nil before reaching Commit — no summary is emitted.
	ctx, cancel := context.WithCancel(context.Background())
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh",
		dispatchHook: func(args []string) {
			if len(args) > 0 && args[0] == "capture-pane" {
				cancel()
			}
		},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Fatalf("captureAndCommit = %v, want nil", err)
	}
	if got := sink.summaries(); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-3 cancel, got %d: %+v", len(got), got)
	}
}

func TestCaptureAndCommit_AnomalousCapturePaneFailureIncrementsAnomalousAndWarns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	// A genuine (non-vanished) capture failure: a plain error whose chain is
	// neither ErrNoSuchSession nor a "can't find" *tmux.CommandError.
	sentinel := errors.New("capture-pane transport boom")
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh",
		captureErrByTarget: map[string]error{"work:0.0": sentinel},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.onlySummary(t)
	if got := rec.IntAttr(t, "anomalous"); got != 1 {
		t.Errorf("anomalous = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "natural_churn"); got != 0 {
		t.Errorf("natural_churn = %d, want 0 on a genuine failure", got)
	}
	// Both panes are processed (the failing one and its healthy peer): the loop
	// continues past the failure.
	if got := rec.IntAttr(t, "panes"); got != 2 {
		t.Errorf("panes = %d, want 2 (loop continued past failure)", got)
	}

	// One per-pane WARN on component=daemon naming the failing pane + wrapped err.
	var warns []logtest.Record
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn && r.Msg == "capture pane failed" {
			warns = append(warns, r)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("expected 1 'capture pane failed' WARN, got %d: %+v", len(warns), sink.Records())
	}
	if comp := warns[0].Attrs["component"]; comp.String() != "daemon" {
		t.Errorf("WARN component = %q, want daemon (per-pane WARN stays on daemon)", comp.String())
	}
}

func TestCaptureAndCommit_AnomalousWriteScrollbackFailureIncrementsAnomalousAndWarns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{
		sessionsOut:     sess,
		panesOut:        panes,
		captureByTarget: map[string]string{"work:0.0": "some scrollback bytes"},
	}
	deps := makeCaptureDeps(t, dir, fc)
	// Force WriteScrollbackIfChanged to fail by removing the scrollback dir and
	// replacing it with a regular file so AtomicWrite0600's temp-create fails.
	breakScrollbackDir(t, dir)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.onlySummary(t)
	if got := rec.IntAttr(t, "anomalous"); got != 1 {
		t.Errorf("anomalous = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "natural_churn"); got != 0 {
		t.Errorf("natural_churn = %d, want 0", got)
	}

	var warns []logtest.Record
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn && r.Msg == "write scrollback failed" {
			warns = append(warns, r)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("expected 1 'write scrollback failed' WARN, got %d: %+v", len(warns), sink.Records())
	}
	if comp := warns[0].Attrs["component"]; comp.String() != "daemon" {
		t.Errorf("WARN component = %q, want daemon", comp.String())
	}
}

func TestCaptureAndCommit_NoSummaryOnCommitPhaseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)
	// Break the state dir so state.Commit's atomic write fails: replace the
	// sessions.json parent's writability. Commit writes via os.WriteFile into
	// dir, so making dir read-only forces the phase-boundary error.
	breakCommitTarget(t, dir)

	err := captureAndCommit(context.Background(), deps)
	if err == nil {
		t.Fatal("expected a commit phase-boundary error, got nil")
	}
	if got := sink.summaries(); len(got) != 0 {
		t.Errorf("expected no summary on commit phase error, got %d: %+v", len(got), got)
	}
}

func TestCaptureAndCommit_CountsUserClosedPaneAsNaturalChurnNotAnomalous(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	// Two panes: one vanished mid-tick (tmux "can't find pane"), one healthy.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh",
		captureErrByTarget: map[string]error{
			"work:0.0": paneVanishedCommandErr("pane", "work:0.0"),
		},
		captureByTarget: map[string]string{"work:0.1": "healthy"},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.onlySummary(t)
	if got := rec.IntAttr(t, "natural_churn"); got != 1 {
		t.Errorf("natural_churn = %d, want 1 (option a: user-closed pane is natural churn)", got)
	}
	if got := rec.IntAttr(t, "anomalous"); got != 0 {
		t.Errorf("anomalous = %d, want 0 (a vanished pane is not anomalous)", got)
	}

	// A vanished pane emits a capture-bound DEBUG "pane vanished", NOT a WARN.
	for _, r := range sink.Records() {
		if r.Level == slog.LevelWarn && r.Msg == "capture pane failed" {
			t.Errorf("vanished pane must not emit a WARN: %+v", r)
		}
	}
	var vanished []logtest.Record
	for _, r := range sink.Records() {
		if r.Level == slog.LevelDebug && r.Msg == "pane vanished" {
			vanished = append(vanished, r)
		}
	}
	if len(vanished) != 1 {
		t.Fatalf("expected 1 DEBUG 'pane vanished', got %d: %+v", len(vanished), sink.Records())
	}
	if comp := vanished[0].Attrs["component"]; comp.String() != "capture" {
		t.Errorf("'pane vanished' component = %q, want capture", comp.String())
	}
	if ec, ok := vanished[0].Attrs["error_class"]; !ok || ec.String() != "expected" {
		t.Errorf("'pane vanished' error_class = %v, want expected", vanished[0].Attrs["error_class"])
	}
}

func TestCaptureAndCommit_EmitsPerPaneDebugBreadcrumbUnderCapture(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := installCaptureSummarySink(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	var dbg []logtest.Record
	for _, r := range sink.Records() {
		if r.Level == slog.LevelDebug && r.Msg == "pane captured" {
			dbg = append(dbg, r)
		}
	}
	if len(dbg) != 1 {
		t.Fatalf("expected 1 DEBUG 'pane captured' breadcrumb, got %d: %+v", len(dbg), sink.Records())
	}
	if comp := dbg[0].Attrs["component"]; comp.String() != "capture" {
		t.Errorf("breadcrumb component = %q, want capture", comp.String())
	}
	// pane_key is the canonical persisted form (SanitizePaneKey), not the
	// tmux -t target form: "work__0.0", not "work:0.0".
	if pk, ok := dbg[0].Attrs["pane_key"]; !ok || pk.String() != "work__0.0" {
		t.Errorf("breadcrumb pane_key = %v, want work__0.0", dbg[0].Attrs["pane_key"])
	}
	if s, ok := dbg[0].Attrs["session"]; !ok || s.String() != "work" {
		t.Errorf("breadcrumb session = %v, want work", dbg[0].Attrs["session"])
	}
}
