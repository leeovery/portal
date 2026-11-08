package tui

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func obsTwoSessions() []Session {
	return []Session{
		{Name: "alpha"},
		{Name: "bravo"},
	}
}

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

func onlyInfoRecord(t *testing.T, sink *logtest.Sink) logtest.Record {
	t.Helper()
	return sink.Records().AtExactLevel(slog.LevelInfo).Only(t, "INFO spawn record")
}

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

func TestBurstObservability_FullSuccessOpenedNofN(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
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

// A burst that never started opened nothing, so it has no `opened 0/N`
// summary, as the CLI's has none; one WARN says why.
func TestBurstObservability_PreSpawnErrorEmitsNoSummary(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger
	boom := errors.New("os.Executable: boom")

	injectComplete(t, m, spawnCompleteMsg{Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative, Err: boom})

	if infos := sink.Records().AtExactLevel(slog.LevelInfo); len(infos) != 0 {
		t.Errorf("INFO records = %+v, want no batch summary for a burst that never started", infos)
	}
	warn := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the WARN saying why the burst never started")
	if warn.Msg != "burst not started" {
		t.Errorf("WARN msg = %q, want %q", warn.Msg, "burst not started")
	}
	if err := warn.ErrorAttr(t, "error"); !errors.Is(err, boom) {
		t.Errorf("WARN error = %v, want %v", err, boom)
	}
}

func TestBurstObservability_PartialFailureOpenedKofN(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

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

	debug := sink.Records().AtExactLevel(slog.LevelDebug).Only(t, "DEBUG spawn record")
	if debug.Msg != "external window" {
		t.Errorf("DEBUG msg = %q, want %q", debug.Msg, "external window")
	}
	if got := debug.AttrString(t, "session"); got != "alpha" {
		t.Errorf("DEBUG session = %q, want alpha", got)
	}
	if got := debug.AttrString(t, "ack"); got != "confirmed" {
		t.Errorf("DEBUG ack = %q, want confirmed", got)
	}
	if got := debug.AttrString(t, "detail"); got != "opened alpha detail" {
		t.Errorf("DEBUG detail = %q, want the opaque driver detail", got)
	}

	warn := sink.Records().AtExactLevel(slog.LevelWarn).Only(t, "WARN spawn record")
	if warn.Msg != "external window failed" {
		t.Errorf("WARN msg = %q, want %q", warn.Msg, "external window failed")
	}
	if got := warn.AttrString(t, "session"); got != "bravo" {
		t.Errorf("WARN session = %q, want bravo", got)
	}
	if got := warn.AttrString(t, "ack"); got != "timeout" {
		t.Errorf("WARN ack = %q, want timeout", got)
	}
	if got := warn.AttrString(t, "detail"); got != "boom bravo detail" {
		t.Errorf("WARN detail = %q, want the opaque driver detail", got)
	}
	assertClosedSpawnKeys(t, sink)
}

func TestBurstObservability_UnsupportedNoopNoPerWindow(t *testing.T) {
	m := NewModelWithSessions(obsTwoSessions())
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	wireUnsupportedBurstSeams(&m, adapter, ack)
	// markTwo must precede resolveDetection: entering multi-select after
	// detection resolves is blocked, and the reactive no-op never runs.
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
	if debugs := sink.Records().AtExactLevel(slog.LevelDebug); len(debugs) != 0 {
		t.Errorf("unsupported no-op must emit NO per-window records, got %d DEBUG records", len(debugs))
	}
	assertClosedSpawnKeys(t, sink)
}

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
	if debugs := sink.Records().AtExactLevel(slog.LevelDebug); len(debugs) != 0 {
		t.Errorf("pre-flight abort must emit NO per-window records, got %d DEBUG records", len(debugs))
	}
	assertClosedSpawnKeys(t, sink)
}

func TestBurstObservability_OnlyClosedSpawnAttrKeys(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)

	full := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	full.spawnLogger = logger
	injectComplete(t, full, spawnCompleteMsg{
		Batch: "b1", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
		},
	})

	partial := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	partial.spawnLogger = logger
	injectComplete(t, partial, spawnCompleteMsg{
		Batch: "b2", Identity: ghosttyIdentity(), Resolution: spawn.ResolutionNative,
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("d")},
			{Session: "bravo", Ack: spawn.AckFailed, Result: spawn.PermissionRequired("evt -1743", "grant access")},
		},
	})

	unsupAck := &spawntest.FakeAckChannel{}
	unsup := NewModelWithSessions(obsTwoSessions())
	wireUnsupportedBurstSeams(&unsup, &spawntest.FakeAdapter{Ack: unsupAck}, unsupAck)
	// markTwo before resolveDetection, as above.
	unsup = markTwo(t, unsup)
	unsup = resolveDetection(t, unsup, appleTerminalIdentity())
	unsup.spawnLogger = logger
	unsup, _ = pressEnter(t, unsup)

	abort := newPendingBurstModel(t, []string{"alpha", "bravo"})
	abort.spawnLogger = logger
	abort.Update(spawnAbortMsg{Gone: []string{"alpha", "bravo"}})

	if len(sink.Records()) == 0 {
		t.Fatal("precondition: the four paths must have emitted at least one record")
	}
	assertClosedSpawnKeys(t, sink)
}

const wantPermissionBody = "INFO permission required — nothing self-attached resolution=native terminal=Ghostty bundle_id=com.mitchellh.ghostty detail=evt -1743"

func TestBurstObservability_PermissionRequiredEmitsPermissionEvent(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

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
	if info.HasAttr("opened") || info.HasAttr("total") || info.HasAttr("batch") {
		t.Errorf("permission event must carry no opened/total/batch attrs: keys=%v", info.Keys)
	}
	for _, r := range sink.Records() {
		if r.Level == slog.LevelInfo && strings.HasPrefix(r.Msg, "opened") {
			t.Errorf("permission arm must NOT emit the generic %q summary", r.Msg)
		}
	}
	assertClosedSpawnKeys(t, sink)
}

func TestBurstObservability_PartialFailureNoPermissionEmitsSummary(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	logger, sink := logtest.NewCaptureLogger(t)
	m.spawnLogger = logger

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

func TestEmitPermission_ParityWithCLI(t *testing.T) {
	logger, sink := logtest.NewCaptureLogger(t)
	m := Model{spawnLogger: logger}

	m.emitPermission(ghosttyIdentity(), spawn.ResolutionNative, "evt -1743")

	if got := sink.Body(); got != wantPermissionBody {
		t.Errorf("emitPermission body =\n  %q\nwant\n  %q", got, wantPermissionBody)
	}
}

func TestBurstObservability_TotalIncludesTriggerOnEveryPath(t *testing.T) {
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
