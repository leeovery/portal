package tui

// restore-host-terminal-windows-6-10 — spawn batch-summary observability from the
// burst completion chokepoint.
//
// White-box (package tui) tests of the picker burst's spawn-component emission:
//   - full success  → one INFO `opened N/N` (the trigger self-attach counted) +
//     one DEBUG per external window (session/ack/detail),
//   - partial/permission failure → `opened k/N` where k counts ONLY confirmed
//     external windows (the skipped trigger self-attach is NOT counted),
//   - unsupported N≥2 no-op → one INFO `resolution=unsupported` (terminal/bundle_id),
//     no per-window records,
//   - pre-flight abort → one INFO naming the gone session(s), no per-window records,
//   - every emitted record carries ONLY the closed spawn attr keys.
//
// The spawn logger is injected white-box via a logtest.Sink-backed *slog.Logger; a
// nil logger is discard-safe (log.OrDiscard) so the sibling burst tests that never
// set it keep passing. Shared seam/model helpers (newPendingBurstModel,
// injectComplete, wireUnsupportedBurstSeams, markTwo, resolveDetection,
// ghosttyIdentity, appleTerminalIdentity, pressEnter) live in the sibling burst
// test files. No t.Parallel: consistent with the rest of the tui test surface.

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// obsTwoSessions is the two-session set the unsupported-no-op observability cases
// mark and Enter through.
func obsTwoSessions() []tmux.Session {
	return []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
}

// closedSpawnAttrKeys is the §6-10 closed attr-key set every emitted spawn record
// must stay within (the baseline pid/version/process_role are injected by the
// production handler, not the call site, so they never appear via the raw sink).
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

// recordsByLevel returns the captured records at the given slog level, in order.
func recordsByLevel(recs []logtest.Record, level slog.Level) []logtest.Record {
	var out []logtest.Record
	for _, r := range recs {
		if r.Level == level {
			out = append(out, r)
		}
	}
	return out
}

// onlyInfoRecord fails unless exactly one INFO record was captured and returns it.
func onlyInfoRecord(t *testing.T, sink *logtest.Sink) logtest.Record {
	t.Helper()
	infos := recordsByLevel(sink.Records(), slog.LevelInfo)
	if len(infos) != 1 {
		t.Fatalf("want exactly 1 INFO spawn record, got %d: %+v", len(infos), infos)
	}
	return infos[0]
}

// assertClosedSpawnKeys fails if any captured record carries an attr key outside
// the closed spawn set.
func assertClosedSpawnKeys(t *testing.T, sink *logtest.Sink) {
	t.Helper()
	for _, r := range sink.Records() {
		for _, k := range r.Keys {
			if !closedSpawnAttrKeys[k] {
				t.Errorf("record %q (%s) carries non-closed attr key %q; closed set is %v", r.Msg, r.Level, k, closedSpawnAttrKeys)
			}
		}
	}
}

// TestBurstObservability_FullSuccessOpenedNofN asserts a full-success burst emits
// one INFO `spawn: opened N/N` counting the trigger self-attach, with the closed
// batch/terminal/bundle_id/resolution/opened/total attrs.
func TestBurstObservability_FullSuccessOpenedNofN(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"}) // external=[alpha,bravo], trigger=charlie, N=3
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	msg := spawnCompleteMsg{
		Batch:      "batch-xyz",
		Identity:   ghosttyIdentity(),
		Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("opened alpha")},
			{Session: "bravo", Ack: spawn.AckConfirmed, Result: spawn.Success("opened bravo")},
		},
	}
	rm, cmd := injectComplete(t, m, msg)
	if !isQuitCmd(cmd) {
		t.Fatal("precondition: a full-success terminal event must self-attach (tea.Quit)")
	}
	if rm.Selected() != "charlie" {
		t.Fatalf("precondition: full success must self-attach to the trigger, Selected()=%q", rm.Selected())
	}

	info := onlyInfoRecord(t, sink)
	if info.Msg != "opened 3/3" {
		t.Errorf("INFO msg = %q, want %q (opened N/N, trigger counted)", info.Msg, "opened 3/3")
	}
	if got := info.AttrString(t, "resolution"); got != "native" {
		t.Errorf("resolution = %q, want native", got)
	}
	if got := info.AttrString(t, "terminal"); got != "Ghostty" {
		t.Errorf("terminal = %q, want Ghostty", got)
	}
	if got := info.AttrString(t, "bundle_id"); got != "com.mitchellh.ghostty" {
		t.Errorf("bundle_id = %q, want com.mitchellh.ghostty", got)
	}
	if got := info.IntAttr(t, "opened"); got != 3 {
		t.Errorf("opened = %d, want 3 (2 confirmed externals + the trigger self-attach)", got)
	}
	if got := info.IntAttr(t, "total"); got != 3 {
		t.Errorf("total = %d, want 3 (N = external set + trigger)", got)
	}
	if got := info.AttrString(t, "batch"); got != "batch-xyz" {
		t.Errorf("batch = %q, want batch-xyz", got)
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_PartialFailureOpenedKofN asserts a partial failure (one
// external times out) emits `opened k/N` where k counts ONLY confirmed externals —
// the skipped trigger self-attach is NOT counted.
func TestBurstObservability_PartialFailureOpenedKofN(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"}) // external=[alpha,bravo], trigger=charlie, N=3
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	// alpha confirms; bravo (window 2) times out → k=1 confirmed external, trigger skipped.
	msg := spawnCompleteMsg{
		Batch:      "batch-xyz",
		Identity:   ghosttyIdentity(),
		Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("ok")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
		},
	}
	rm, cmd := injectComplete(t, m, msg)
	if cmd != nil {
		t.Fatal("precondition: a partial failure must NOT self-attach (no tea.Quit)")
	}
	if rm.Selected() != "" {
		t.Fatalf("precondition: a partial failure must not self-attach, Selected()=%q", rm.Selected())
	}

	info := onlyInfoRecord(t, sink)
	if info.Msg != "opened 1/3" {
		t.Errorf("INFO msg = %q, want %q (k=1 confirmed external, trigger not counted, total N=3)", info.Msg, "opened 1/3")
	}
	if got := info.IntAttr(t, "opened"); got != 1 {
		t.Errorf("opened = %d, want 1 (only the confirmed external; the skipped trigger is not counted)", got)
	}
	if got := info.IntAttr(t, "total"); got != 3 {
		t.Errorf("total = %d, want 3 (N, incl. the trigger self-attach target)", got)
	}
	if got := info.AttrString(t, "resolution"); got != "native" {
		t.Errorf("resolution = %q, want native", got)
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_PerExternalWindowSplitByOutcome asserts the per-window
// records split by outcome: a confirmed external window emits DEBUG "external window"
// while a non-permission failed window (here an AckTimeout) emits WARN "external
// window failed" — both carrying session + ack + the opaque driver detail. This is
// the picker-side witness of the shared spawn.LogWindowResults split (mirrored on the
// CLI); it closes the INFO-level invisibility gap where a failed window logged THAT it
// failed but not WHY.
func TestBurstObservability_PerExternalWindowSplitByOutcome(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	msg := spawnCompleteMsg{
		Batch:      "batch-xyz",
		Identity:   ghosttyIdentity(),
		Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("opened alpha detail")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.SpawnFailed("boom bravo detail")},
		},
	}
	injectComplete(t, m, msg)

	// alpha (confirmed) → DEBUG "external window".
	debugs := recordsByLevel(sink.Records(), slog.LevelDebug)
	if len(debugs) != 1 {
		t.Fatalf("want exactly 1 DEBUG spawn record (the confirmed window), got %d: %+v", len(debugs), debugs)
	}
	if debugs[0].Msg != "external window" {
		t.Errorf("DEBUG msg = %q, want %q", debugs[0].Msg, "external window")
	}
	if got := debugs[0].AttrString(t, "session"); got != "alpha" {
		t.Errorf("DEBUG session = %q, want alpha", got)
	}
	if got := debugs[0].AttrString(t, "ack"); got != "confirmed" {
		t.Errorf("DEBUG ack = %q, want confirmed", got)
	}
	if got := debugs[0].AttrString(t, "detail"); got != "opened alpha detail" {
		t.Errorf("DEBUG detail = %q, want the opaque driver detail", got)
	}

	// bravo (AckTimeout, non-permission) → WARN "external window failed" carrying the
	// opaque detail; ack=timeout distinguishes the mode.
	warns := recordsByLevel(sink.Records(), slog.LevelWarn)
	if len(warns) != 1 {
		t.Fatalf("want exactly 1 WARN spawn record (the failed window), got %d: %+v", len(warns), warns)
	}
	if warns[0].Msg != "external window failed" {
		t.Errorf("WARN msg = %q, want %q", warns[0].Msg, "external window failed")
	}
	if got := warns[0].AttrString(t, "session"); got != "bravo" {
		t.Errorf("WARN session = %q, want bravo", got)
	}
	if got := warns[0].AttrString(t, "ack"); got != "timeout" {
		t.Errorf("WARN ack = %q, want timeout", got)
	}
	if got := warns[0].AttrString(t, "detail"); got != "boom bravo detail" {
		t.Errorf("WARN detail = %q, want the opaque driver detail", got)
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_UnsupportedNoopNoPerWindow asserts the N≥2 unsupported
// no-op emits one INFO `resolution=unsupported` with terminal/bundle_id and NO
// per-window records (nothing was attempted).
func TestBurstObservability_UnsupportedNoopNoPerWindow(t *testing.T) {
	m := NewModelWithSessions(obsTwoSessions())
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	wireUnsupportedBurstSeams(&m, adapter, ack)
	// Enter multi-select during the async in-flight window (markTwo BEFORE
	// resolveDetection, so the §3 proactive entry block is inert), then resolve
	// unsupported — the Enter drives decideBurst's retained reactive no-op.
	m = markTwo(t, m)
	m = resolveDetection(t, m, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatal("precondition: com.apple.Terminal must resolve unsupported")
	}
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	m, _ = pressEnter(t, m)

	info := onlyInfoRecord(t, sink)
	if got := info.AttrString(t, "resolution"); got != "unsupported" {
		t.Errorf("resolution = %q, want unsupported", got)
	}
	if got := info.AttrString(t, "terminal"); got != "Apple Terminal" {
		t.Errorf("terminal = %q, want Apple Terminal", got)
	}
	if got := info.AttrString(t, "bundle_id"); got != "com.apple.Terminal" {
		t.Errorf("bundle_id = %q, want com.apple.Terminal", got)
	}
	if info.HasAttr("opened") || info.HasAttr("total") {
		t.Errorf("unsupported no-op must carry no opened/total counts: keys=%v", info.Keys)
	}
	if debugs := recordsByLevel(sink.Records(), slog.LevelDebug); len(debugs) != 0 {
		t.Errorf("unsupported no-op must emit NO per-window records, got %d DEBUG records", len(debugs))
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_PreflightAbortNamesGone asserts the pre-flight abort emits
// one INFO naming the gone session(s) with no per-window records.
func TestBurstObservability_PreflightAbortNamesGone(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	updated, _ := m.Update(spawnAbortMsg{Gone: []string{"bravo"}})
	_ = updated.(Model)

	info := onlyInfoRecord(t, sink)
	if info.Msg != "'bravo' is gone — nothing opened" {
		t.Errorf("INFO msg = %q, want %q (names the gone session)", info.Msg, "'bravo' is gone — nothing opened")
	}
	if debugs := recordsByLevel(sink.Records(), slog.LevelDebug); len(debugs) != 0 {
		t.Errorf("pre-flight abort must emit NO per-window records, got %d DEBUG records", len(debugs))
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_OnlyClosedSpawnAttrKeys drives full-success, partial,
// unsupported, and pre-flight-abort into one shared sink and asserts every captured
// record stays within the closed spawn attr-key set.
func TestBurstObservability_OnlyClosedSpawnAttrKeys(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	// Full success.
	full := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	full.spawnLogger = logger
	injectComplete(t, full, spawnCompleteMsg{
		Batch: "b1", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
		},
	})

	// Partial (with a permission window carrying opaque detail).
	partial := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	partial.spawnLogger = logger
	injectComplete(t, partial, spawnCompleteMsg{
		Batch: "b2", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckFailed, Result: spawn.PermissionRequired("evt -1743", "grant access")},
		},
	})

	// Unsupported no-op.
	unsupAck := &spawntest.FakeAckChannel{}
	unsup := NewModelWithSessions(obsTwoSessions())
	wireUnsupportedBurstSeams(&unsup, &spawntest.FakeAdapter{Ack: unsupAck}, unsupAck)
	// Enter multi-select in-flight (markTwo BEFORE resolveDetection → §3 entry block
	// inert), then resolve unsupported so the Enter drives the reactive no-op.
	unsup = markTwo(t, unsup)
	unsup = resolveDetection(t, unsup, appleTerminalIdentity())
	unsup.spawnLogger = logger
	unsup, _ = pressEnter(t, unsup)

	// Pre-flight abort.
	abort := newPendingBurstModel(t, []string{"alpha", "bravo"})
	abort.spawnLogger = logger
	abort.Update(spawnAbortMsg{Gone: []string{"alpha", "bravo"}})

	if len(sink.Records()) == 0 {
		t.Fatal("precondition: the four paths must have emitted at least one record")
	}
	assertClosedSpawnKeys(t, sink)
}

// wantPermissionBody is the exact rendered body the shared spawn.LogPermission
// produces and the picker's emitPermission must reproduce for the same identity /
// resolution / detail — the closed `spawn` permission event both the picker and the
// open burst's permission arm delegate to. internal/spawn's logemit_test.go pins
// spawn.LogPermission to this same literal, so a drift in either emitter fails its
// own golden and the picker + open-burst paths stay byte-identical.
const wantPermissionBody = "INFO permission required — nothing self-attached resolution=native terminal=Ghostty bundle_id=com.mitchellh.ghostty detail=evt -1743"

// TestBurstObservability_PermissionRequiredEmitsPermissionEvent asserts a picker
// permission-required burst emits exactly the emitPermission INFO event (closed
// resolution/terminal/bundle_id/detail attrs) and NOT the generic opened/total
// summary — matching spawn.LogPermission's skip-the-summary contract (the open burst
// takes the same path).
func TestBurstObservability_PermissionRequiredEmitsPermissionEvent(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"}) // external=[alpha,bravo], trigger=charlie
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	// alpha confirms; bravo hits the permission wall (AckFailed + OutcomePermissionRequired).
	msg := spawnCompleteMsg{
		Batch:      "batch-xyz",
		Identity:   ghosttyIdentity(),
		Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("ok")},
			{Session: "bravo", Ack: spawn.AckFailed, Result: spawn.PermissionRequired("evt -1743", "grant Automation for Ghostty")},
		},
	}
	injectComplete(t, m, msg)

	info := onlyInfoRecord(t, sink)
	if info.Msg != "permission required — nothing self-attached" {
		t.Errorf("INFO msg = %q, want the permission event", info.Msg)
	}
	if got := info.AttrString(t, "resolution"); got != "native" {
		t.Errorf("resolution = %q, want native", got)
	}
	if got := info.AttrString(t, "terminal"); got != "Ghostty" {
		t.Errorf("terminal = %q, want Ghostty", got)
	}
	if got := info.AttrString(t, "bundle_id"); got != "com.mitchellh.ghostty" {
		t.Errorf("bundle_id = %q, want com.mitchellh.ghostty", got)
	}
	if got := info.AttrString(t, "detail"); got != "evt -1743" {
		t.Errorf("detail = %q, want the opaque driver detail (evt -1743)", got)
	}
	// The permission event carries NO opened/total/batch summary attrs.
	if info.HasAttr("opened") || info.HasAttr("total") || info.HasAttr("batch") {
		t.Errorf("permission event must carry no opened/total/batch attrs: keys=%v", info.Keys)
	}
	// No generic opened summary INFO may be emitted on the permission arm.
	for _, r := range sink.Records() {
		if r.Level == slog.LevelInfo && strings.HasPrefix(r.Msg, "opened") {
			t.Errorf("permission arm must NOT emit the generic %q summary", r.Msg)
		}
	}
	assertClosedSpawnKeys(t, sink)
}

// TestBurstObservability_PartialFailureNoPermissionEmitsSummary asserts a partial
// failure WITHOUT a permission wall still emits the generic emitBurstSummary
// (opened k/N) and no permission event.
func TestBurstObservability_PartialFailureNoPermissionEmitsSummary(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

	// alpha confirms; bravo times out (no permission wall).
	msg := spawnCompleteMsg{
		Batch:      "batch-xyz",
		Identity:   ghosttyIdentity(),
		Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("ok")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
		},
	}
	injectComplete(t, m, msg)

	info := onlyInfoRecord(t, sink)
	if info.Msg != "opened 1/3" {
		t.Errorf("INFO msg = %q, want the generic opened k/N summary", info.Msg)
	}
	for _, r := range sink.Records() {
		if strings.HasPrefix(r.Msg, "permission required") {
			t.Errorf("a non-permission partial must NOT emit the permission event, got %q", r.Msg)
		}
	}
}

// TestEmitPermission_ParityWithCLI asserts the picker's emitPermission renders the
// exact same body (message + closed attr set) as the shared spawn.LogPermission (the
// same emitter the open burst's permission arm uses) for the same
// identity/resolution/detail — the cross-caller one-service lockstep.
func TestEmitPermission_ParityWithCLI(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	m := Model{spawnLogger: logger}

	m.emitPermission(ghosttyIdentity(), spawn.ResolutionNative, "evt -1743")

	if got := sink.Body(); got != wantPermissionBody {
		t.Errorf("emitPermission body =\n  %q\nwant\n  %q", got, wantPermissionBody)
	}
}

// TestBurstObservability_TotalIncludesTriggerOnEveryPath asserts total == N (the
// external set + the trigger self-attach target) on both batch-summary paths.
func TestBurstObservability_TotalIncludesTriggerOnEveryPath(t *testing.T) {
	// Full success → total N.
	full := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	fullLogger, fullSink := logtest.NewCaptureLogger(t)
	full.spawnLogger = fullLogger
	injectComplete(t, full, spawnCompleteMsg{
		Batch: "b1", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
		},
	})
	if got := onlyInfoRecord(t, fullSink).IntAttr(t, "total"); got != 3 {
		t.Errorf("full-success total = %d, want 3 (N incl. trigger)", got)
	}

	// Partial → total N (still counts the skipped trigger target).
	partial := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	partialLogger, partialSink := logtest.NewCaptureLogger(t)
	partial.spawnLogger = partialLogger
	injectComplete(t, partial, spawnCompleteMsg{
		Batch: "b2", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
		},
	})
	if got := onlyInfoRecord(t, partialSink).IntAttr(t, "total"); got != 3 {
		t.Errorf("partial total = %d, want 3 (N incl. the skipped trigger target)", got)
	}
}
