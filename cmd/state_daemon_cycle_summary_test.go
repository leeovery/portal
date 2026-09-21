package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// paneVanishedCommandErr carries tmux's canonical "can't find
// {session,window,pane}" phrasing, the un-sentinel-wrapped shape CapturePane
// surfaces when a pane vanishes mid-tick.
func paneVanishedCommandErr(kind, name string) error {
	return &tmux.CommandError{
		Stderr: "can't find " + kind + ": " + name,
		Err:    errors.New("exit status 1"),
	}
}

// makeCaptureDeps assembles a tick-ready daemonDeps. deps.Logger is discarded:
// these tests read the daemon WARNs off the process-wide capture sink.
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

// breakScrollbackDir puts a regular file where the scrollback directory belongs
// so the atomic write fails at temp-create - a genuine, non-vanished failure.
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

// breakCommitTarget puts a directory at the sessions.json path so Commit's
// rename fails, giving a phase-boundary error.
func breakCommitTarget(t *testing.T, dir string) {
	t.Helper()
	if err := os.Mkdir(state.SessionsJSON(dir), 0o700); err != nil {
		t.Fatalf("seed sessions.json-as-dir: %v", err)
	}
}

func TestCaptureAndCommit_EmitsOneTickCompleteSummaryOnSuccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.Records().Matching("capture", "tick complete").Only(t, "capture tick complete record")
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
	rec.RequireDuration(t, "took")
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Fatalf("captureAndCommit on cancelled ctx = %v, want nil", err)
	}
	if got := sink.Records().Matching("capture", "tick complete"); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-1 cancel, got %d: %+v", len(got), got)
	}
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked after obs-point-1 cancel: %v", got)
	}
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint2(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	// cancel() fires after CaptureStructure's show-environment subcall, so the
	// post-enumeration check observes it before any per-pane work.
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
	if got := sink.Records().Matching("capture", "tick complete"); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-2 cancel, got %d: %+v", len(got), got)
	}
	if got := fc.callsContaining("capture-pane"); len(got) != 0 {
		t.Errorf("capture-pane invoked after obs-point-2 cancel: %v", got)
	}
}

func TestCaptureAndCommit_NoSummaryWhenCtxCancelledAtObsPoint3(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	// cancel() fires on the FIRST capture-pane call, so the second iteration's
	// between-panes check returns before Commit.
	ctx, cancel := context.WithCancel(context.Background())
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh||||||",
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
	if got := sink.Records().Matching("capture", "tick complete"); len(got) != 0 {
		t.Errorf("expected no summary on obs-point-3 cancel, got %d: %+v", len(got), got)
	}
}

func TestCaptureAndCommit_AnomalousCapturePaneFailureIncrementsAnomalousAndWarns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	// A genuine capture failure: neither ErrNoSuchSession nor a "can't find"
	// *tmux.CommandError.
	sentinel := errors.New("capture-pane transport boom")
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh||||||",
		captureErrByTarget: map[string]error{"work:0.0": sentinel},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.Records().Matching("capture", "tick complete").Only(t, "capture tick complete record")
	if got := rec.IntAttr(t, "anomalous"); got != 1 {
		t.Errorf("anomalous = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "natural_churn"); got != 0 {
		t.Errorf("natural_churn = %d, want 0 on a genuine failure", got)
	}
	if got := rec.IntAttr(t, "panes"); got != 2 {
		t.Errorf("panes = %d, want 2 (loop continued past failure)", got)
	}

	warn := sink.Records().WithMessage("capture pane failed").AtExactLevel(slog.LevelWarn).Only(t, "'capture pane failed' WARN")
	if comp := warn.AttrString(t, "component"); comp != "daemon" {
		t.Errorf("WARN component = %q, want daemon (per-pane WARN stays on daemon)", comp)
	}
}

func TestCaptureAndCommit_AnomalousWriteScrollbackFailureIncrementsAnomalousAndWarns(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{
		sessionsOut:     sess,
		panesOut:        panes,
		captureByTarget: map[string]string{"work:0.0": "some scrollback bytes"},
	}
	deps := makeCaptureDeps(t, dir, fc)
	breakScrollbackDir(t, dir)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.Records().Matching("capture", "tick complete").Only(t, "capture tick complete record")
	if got := rec.IntAttr(t, "anomalous"); got != 1 {
		t.Errorf("anomalous = %d, want 1", got)
	}
	if got := rec.IntAttr(t, "natural_churn"); got != 0 {
		t.Errorf("natural_churn = %d, want 0", got)
	}

	warn := sink.Records().WithMessage("write scrollback failed").AtExactLevel(slog.LevelWarn).Only(t, "'write scrollback failed' WARN")
	if comp := warn.AttrString(t, "component"); comp != "daemon" {
		t.Errorf("WARN component = %q, want daemon", comp)
	}
}

func TestCaptureAndCommit_NoSummaryOnCommitPhaseError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)
	// Commit writes into dir, so a read-only dir forces the phase-boundary error.
	breakCommitTarget(t, dir)

	err := captureAndCommit(context.Background(), deps)
	if err == nil {
		t.Fatal("expected a commit phase-boundary error, got nil")
	}
	if got := sink.Records().Matching("capture", "tick complete"); len(got) != 0 {
		t.Errorf("expected no summary on commit phase error, got %d: %+v", len(got), got)
	}
}

func TestCaptureAndCommit_CountsUserClosedPaneAsNaturalChurnNotAnomalous(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||1|||zsh||||||",
		captureErrByTarget: map[string]error{
			"work:0.0": paneVanishedCommandErr("pane", "work:0.0"),
		},
		captureByTarget: map[string]string{"work:0.1": "healthy"},
	}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	rec := sink.Records().Matching("capture", "tick complete").Only(t, "capture tick complete record")
	if got := rec.IntAttr(t, "natural_churn"); got != 1 {
		t.Errorf("natural_churn = %d, want 1 (option a: user-closed pane is natural churn)", got)
	}
	if got := rec.IntAttr(t, "anomalous"); got != 0 {
		t.Errorf("anomalous = %d, want 0 (a vanished pane is not anomalous)", got)
	}

	if warns := sink.Records().WithMessage("capture pane failed").AtExactLevel(slog.LevelWarn); warns != nil {
		t.Errorf("vanished pane must not emit a WARN: %+v", warns)
	}
	vanished := sink.Records().WithMessage("pane vanished").AtExactLevel(slog.LevelDebug).Only(t, "DEBUG 'pane vanished'")
	if comp := vanished.AttrString(t, "component"); comp != "capture" {
		t.Errorf("'pane vanished' component = %q, want capture", comp)
	}
	if ec := vanished.AttrString(t, "error_class"); ec != "expected" {
		t.Errorf("'pane vanished' error_class = %q, want expected", ec)
	}
}

func TestCaptureAndCommit_EmitsPerPaneDebugBreadcrumbUnderCapture(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sink := logtest.Install(t)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeCaptureDeps(t, dir, fc)

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit: %v", err)
	}

	dbg := sink.Records().WithMessage("pane captured").AtExactLevel(slog.LevelDebug).Only(t, "DEBUG 'pane captured' breadcrumb")
	if comp := dbg.AttrString(t, "component"); comp != "capture" {
		t.Errorf("breadcrumb component = %q, want capture", comp)
	}
	// The canonical persisted form, not the tmux -t target form.
	if pk := dbg.AttrString(t, "pane_key"); pk != "work__0.0" {
		t.Errorf("breadcrumb pane_key = %q, want work__0.0", pk)
	}
	if s := dbg.AttrString(t, "session"); s != "work" {
		t.Errorf("breadcrumb session = %q, want work", s)
	}
}
