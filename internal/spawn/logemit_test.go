package spawn

// White-box tests for the shared `spawn`-component log-emission helpers
// (logemit.go). These are the SINGLE SOURCE of the closed spawn log vocabulary:
// cmd/spawn.go (the CLI test seam) and internal/tui/burst_observability.go (the
// dominant picker path) both delegate their emission to these helpers, so the
// golden bodies pinned here are — by construction — byte-identical across both
// callers. The `wantPermissionBody` literal below is the same one asserted at both
// call sites (cmd TestLogSpawnPermission_ParityBody / tui TestEmitPermission_ParityWithCLI),
// making this the cross-caller parity anchor for the permission event.

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
)

// ghosttyID is the recognised Ghostty identity the golden bodies name.
func ghosttyID() Identity {
	return Identity{BundleID: "com.mitchellh.ghostty", Name: "Ghostty"}
}

// appleTerminalID is the recognised-but-undriven Apple Terminal identity the
// unsupported golden names.
func appleTerminalID() Identity {
	return Identity{BundleID: "com.apple.Terminal", Name: "Apple Terminal"}
}

// closedSpawnAttrKeys is the spec's closed attr-key set every emitted spawn record
// must stay within (spec §Observability). The baseline pid/version/process_role are
// injected by the production handler, never the call site, so they never appear here.
var closedSpawnAttrKeys = map[string]bool{
	"batch":      true,
	"terminal":   true,
	"bundle_id":  true,
	"resolution": true,
	"session":    true,
	"ack":        true,
	"opened":     true,
	"total":      true,
	"detail":     true,
}

func assertClosedKeys(t *testing.T, sink *logtest.Sink) {
	t.Helper()
	for _, r := range sink.Records() {
		for _, k := range r.Keys {
			if !closedSpawnAttrKeys[k] {
				t.Errorf("record %q (%s) carries non-closed attr key %q", r.Msg, r.Level, k)
			}
		}
	}
}

func debugRecords(recs []logtest.Record) []logtest.Record {
	var out []logtest.Record
	for _, r := range recs {
		if r.Level == slog.LevelDebug {
			out = append(out, r)
		}
	}
	return out
}

func infoRecords(recs []logtest.Record) []logtest.Record {
	var out []logtest.Record
	for _, r := range recs {
		if r.Level == slog.LevelInfo {
			out = append(out, r)
		}
	}
	return out
}

// warnRecords (the *logtest.Sink-based filter) is declared in
// terminalsconfig_test.go and reused here — the package's single WARN-record
// helper, mirroring debugRecords/infoRecords above.

// TestLogBatchSummary_FullSuccessBody pins the full-success rendered body: one DEBUG
// per external window (session/ack/detail) then one INFO `opened N/N` with the closed
// resolution/terminal/bundle_id/opened/total/batch attrs, in that exact order.
func TestLogBatchSummary_FullSuccessBody(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	results := []WindowResult{
		{Session: "alpha", Ack: AckConfirmed, Result: Success("d1")},
		{Session: "bravo", Ack: AckConfirmed, Result: Success("d2")},
	}

	LogBatchSummary(logger, ghosttyID(), ResolutionNative, results, 3, true, "batch-xyz")

	const want = "DEBUG external window session=alpha ack=confirmed detail=d1\n" +
		"DEBUG external window session=bravo ack=confirmed detail=d2\n" +
		"INFO opened 3/3 resolution=native terminal=Ghostty bundle_id=com.mitchellh.ghostty opened=3 total=3 batch=batch-xyz"
	if got := sink.Body(); got != want {
		t.Errorf("LogBatchSummary body =\n  %q\nwant\n  %q", got, want)
	}
	assertClosedKeys(t, sink)
}

// TestLogBatchSummary_OpenedDerivedFromPartitionResults asserts the summary's opened
// count is derived from spawn.PartitionResults (confirmed windows only) plus the
// trigger bonus — NOT a naive len(results). A mixed confirmed/timeout/failed slice
// must count exactly the confirmed window(s).
func TestLogBatchSummary_OpenedDerivedFromPartitionResults(t *testing.T) {
	results := []WindowResult{
		{Session: "alpha", Ack: AckConfirmed, Result: Success("d")},
		{Session: "bravo", Ack: AckTimeout, Result: Success("")},
		{Session: "charlie", Ack: AckFailed, Result: SpawnFailed("boom")},
	}
	confirmed, _ := PartitionResults(results)
	if len(confirmed) != 1 {
		t.Fatalf("precondition: want 1 confirmed window from PartitionResults, got %d", len(confirmed))
	}

	// triggerAttached=false → opened == len(confirmed) == 1 (NOT len(results)==3).
	failLogger, failSink := logtest.NewCaptureLogger(t)
	LogBatchSummary(failLogger, ghosttyID(), ResolutionNative, results, 4, false, "b")
	failInfo := infoRecords(failSink.Records())
	if len(failInfo) != 1 {
		t.Fatalf("want exactly 1 INFO summary, got %d", len(failInfo))
	}
	if got := failInfo[0].IntAttr(t, "opened"); got != int64(len(confirmed)) {
		t.Errorf("opened = %d, want %d (PartitionResults confirmed count, not len(results)=%d)", got, len(confirmed), len(results))
	}
	if got := failInfo[0].IntAttr(t, "total"); got != 4 {
		t.Errorf("total = %d, want 4 (N passed through)", got)
	}
	if failInfo[0].Msg != "opened 1/4" {
		t.Errorf("summary msg = %q, want %q", failInfo[0].Msg, "opened 1/4")
	}

	// triggerAttached=true → opened == len(confirmed)+1 == 2.
	okLogger, okSink := logtest.NewCaptureLogger(t)
	LogBatchSummary(okLogger, ghosttyID(), ResolutionNative, results, 4, true, "b")
	okInfo := infoRecords(okSink.Records())
	if got := okInfo[0].IntAttr(t, "opened"); got != int64(len(confirmed)+1) {
		t.Errorf("opened = %d, want %d (confirmed + trigger self-attach)", got, len(confirmed)+1)
	}
	if okInfo[0].Msg != "opened 2/4" {
		t.Errorf("summary msg = %q, want %q", okInfo[0].Msg, "opened 2/4")
	}

	// Only confirmed windows emit DEBUG now; the two non-permission failed
	// windows (bravo AckTimeout, charlie AckFailed) emit WARN.
	if n := len(debugRecords(failSink.Records())); n != 1 {
		t.Errorf("DEBUG records = %d, want 1 (only the confirmed alpha window)", n)
	}
	if n := len(warnRecords(failSink)); n != 2 {
		t.Errorf("WARN records = %d, want 2 (the two failed windows bravo, charlie)", n)
	}
}

// TestLogWindowResults_SplitsByOutcome asserts the standalone per-window loop (the
// CLI permission path's detail emission) renders a confirmed window at DEBUG
// "external window" and a non-permission failed window at WARN "external window
// failed", each carrying session/ack/detail and with NO summary INFO.
func TestLogWindowResults_SplitsByOutcome(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	results := []WindowResult{
		{Session: "alpha", Ack: AckConfirmed, Result: Success("opened alpha")},
		{Session: "bravo", Ack: AckTimeout, Result: SpawnFailed("boom bravo")},
	}

	LogWindowResults(logger, results)

	const want = "DEBUG external window session=alpha ack=confirmed detail=opened alpha\n" +
		"WARN external window failed session=bravo ack=timeout detail=boom bravo"
	if got := sink.Body(); got != want {
		t.Errorf("LogWindowResults body =\n  %q\nwant\n  %q", got, want)
	}
	if n := len(infoRecords(sink.Records())); n != 0 {
		t.Errorf("LogWindowResults must emit NO INFO record, got %d", n)
	}
	assertClosedKeys(t, sink)
}

// TestLogWindowResults_FailedWindowsWarn pins the per-outcome split: any
// non-permission failed window (AckFailed open-failure OR AckTimeout after a
// successful open) emits WARN "external window failed" carrying session/ack/detail;
// a confirmed window and the permission-required window (excluded to avoid a
// double-report with LogPermission) stay at DEBUG "external window".
func TestLogWindowResults_FailedWindowsWarn(t *testing.T) {
	t.Run("ack_failed_open_failure_warns", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		// AckFailed = the adapter reported no window opened (OutcomeSpawnFailed);
		// detail is the osascript error text — the observed primary defect.
		results := []WindowResult{{Session: "x", Ack: AckFailed, Result: SpawnFailed("osascript boom")}}

		LogWindowResults(logger, results)

		r := sink.OnlyRecord(t)
		if r.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", r.Level)
		}
		if r.Msg != "external window failed" {
			t.Errorf("msg = %q, want %q", r.Msg, "external window failed")
		}
		if got := r.AttrString(t, "session"); got != "x" {
			t.Errorf("session = %q, want %q", got, "x")
		}
		if got := r.AttrString(t, "ack"); got != "failed" {
			t.Errorf("ack = %q, want %q", got, "failed")
		}
		if got := r.AttrString(t, "detail"); got != "osascript boom" {
			t.Errorf("detail = %q, want %q", got, "osascript boom")
		}
		assertClosedKeys(t, sink)
	})

	t.Run("ack_timeout_after_success_warns", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		// The adapter opened the window (OutcomeSuccess) but its token never
		// arrived within budget — a genuine window failure even though detail is a
		// benign success string; ack=timeout distinguishes the mode.
		results := []WindowResult{{Session: "y", Ack: AckTimeout, Result: Success("opened y")}}

		LogWindowResults(logger, results)

		r := sink.OnlyRecord(t)
		if r.Level != slog.LevelWarn {
			t.Errorf("level = %v, want WARN", r.Level)
		}
		if r.Msg != "external window failed" {
			t.Errorf("msg = %q, want %q", r.Msg, "external window failed")
		}
		if got := r.AttrString(t, "ack"); got != "timeout" {
			t.Errorf("ack = %q, want %q", got, "timeout")
		}
		if got := r.AttrString(t, "detail"); got != "opened y" {
			t.Errorf("detail = %q, want %q (benign success string still surfaced)", got, "opened y")
		}
		assertClosedKeys(t, sink)
	})

	t.Run("confirmed_window_stays_debug", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		results := []WindowResult{{Session: "z", Ack: AckConfirmed, Result: Success("opened z")}}

		LogWindowResults(logger, results)

		r := sink.OnlyRecord(t)
		if r.Level != slog.LevelDebug {
			t.Errorf("level = %v, want DEBUG", r.Level)
		}
		if r.Msg != "external window" {
			t.Errorf("msg = %q, want %q", r.Msg, "external window")
		}
		if n := len(warnRecords(sink)); n != 0 {
			t.Errorf("confirmed window must emit NO WARN, got %d", n)
		}
		assertClosedKeys(t, sink)
	})

	t.Run("permission_required_excluded_from_warn", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		// The permission window is AckFailed (!Confirmed()) but its detail is
		// already carried by the dedicated LogPermission INFO event, so it must NOT
		// also WARN here — the Outcome != OutcomePermissionRequired guard excludes it.
		results := []WindowResult{{Session: "p", Ack: AckFailed, Result: PermissionRequired("evt -1743", "grant automation")}}

		LogWindowResults(logger, results)

		r := sink.OnlyRecord(t)
		if r.Level != slog.LevelDebug {
			t.Errorf("level = %v, want DEBUG (permission window excluded from WARN)", r.Level)
		}
		if r.Msg != "external window" {
			t.Errorf("msg = %q, want %q", r.Msg, "external window")
		}
		if n := len(warnRecords(sink)); n != 0 {
			t.Errorf("permission-required window must emit NO WARN (no double-report), got %d", n)
		}
		assertClosedKeys(t, sink)
	})
}

// wantPermissionBody is the exact rendered body BOTH the CLI (logSpawnPermission) and
// the picker (emitPermission) must produce — pinned identically at all three sites so
// a drift in the shared helper fails every golden. Kept byte-for-byte in lockstep with
// cmd/spawn_test.go and internal/tui/burst_observability_test.go.
const wantPermissionBody = "INFO permission required — nothing self-attached resolution=native terminal=Ghostty bundle_id=com.mitchellh.ghostty detail=evt -1743"

// TestLogPermission_Body pins the permission event's rendered body.
func TestLogPermission_Body(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	LogPermission(logger, ghosttyID(), ResolutionNative, "evt -1743")

	if got := sink.Body(); got != wantPermissionBody {
		t.Errorf("LogPermission body =\n  %q\nwant\n  %q", got, wantPermissionBody)
	}
	assertClosedKeys(t, sink)
}

// TestLogUnsupported_Body pins the unsupported no-op event's rendered body: the
// resolution attr is always the ResolutionUnsupported literal, with the detected
// terminal/bundle_id and NO opened/total/per-window attrs.
func TestLogUnsupported_Body(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	LogUnsupported(logger, appleTerminalID())

	const want = "INFO unsupported terminal — nothing opened resolution=unsupported terminal=Apple Terminal bundle_id=com.apple.Terminal"
	if got := sink.Body(); got != want {
		t.Errorf("LogUnsupported body =\n  %q\nwant\n  %q", got, want)
	}
	if r := sink.OnlyRecord(t); r.HasAttr("opened") || r.HasAttr("total") || r.HasAttr("batch") {
		t.Errorf("unsupported event must carry no opened/total/batch attrs: keys=%v", r.Keys)
	}
	assertClosedKeys(t, sink)
}

// TestLogGone_Body pins the pre-flight gone event's rendered body for one and several
// gone sessions (message composed via the shared GoneMessage renderer, no attrs).
func TestLogGone_Body(t *testing.T) {
	t.Run("single", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		LogGone(logger, []string{"bravo"})
		if got := sink.Body(); got != "INFO 'bravo' is gone — nothing opened" {
			t.Errorf("LogGone body = %q, want %q", got, "INFO 'bravo' is gone — nothing opened")
		}
		if r := sink.OnlyRecord(t); len(r.Keys) != 0 {
			t.Errorf("gone event must carry no attrs, got keys=%v", r.Keys)
		}
	})
	t.Run("plural", func(t *testing.T) {
		logger, sink := logtest.NewCaptureLogger(t)
		LogGone(logger, []string{"s2", "s4"})
		if got := sink.Body(); got != "INFO 's2', 's4' are gone — nothing opened" {
			t.Errorf("LogGone body = %q, want %q", got, "INFO 's2', 's4' are gone — nothing opened")
		}
	})
}

// TestLogEmit_NilLoggerDoesNotPanic asserts every helper is nil-logger tolerant (the
// offline capture harness / unit tests that never assert logging pass a nil logger).
func TestLogEmit_NilLoggerDoesNotPanic(t *testing.T) {
	results := []WindowResult{{Session: "alpha", Ack: AckConfirmed, Result: Success("d")}}
	// A panic here fails the test.
	LogWindowResults(nil, results)
	LogBatchSummary(nil, ghosttyID(), ResolutionNative, results, 2, true, "b")
	LogPermission(nil, ghosttyID(), ResolutionNative, "evt")
	LogUnsupported(nil, appleTerminalID())
	LogGone(nil, []string{"s2"})
}
