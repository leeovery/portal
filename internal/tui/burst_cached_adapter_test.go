package tui

// restore-host-terminal-windows-10-1 — dispatchBurst must build the burster from
// the SINGLE detection-time resolve (the cached adapter), never a second m.resolve.
//
// A config-*script* recipe adapter re-stats the script on every resolve
// (spawn.newScriptRecipeAdapter), so a script deleted / de-executabled between
// page-entry detection and Enter makes a SECOND resolve return
// (nil, ResolutionUnsupported). The old dispatchBurst re-resolved and trusted that
// adapter non-nil, building spawn.NewBurster(nil, …) and nil-panicking the burst
// goroutine (a bare `go func()` with no recover) — crashing the whole picker rather
// than degrading to the honest unsupported no-op.
//
// These white-box (package tui) tests pin the single-resolve contract (dispatchBurst
// reads m.detectAdapter / m.detectResolution, never re-resolves) on BOTH burst entry
// points, and the defensive nil-adapter no-op.
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// wireTOCTOUResolveSeams wires the §6 burst seams with a COUNTING resolve seam that
// mimics the config-script TOCTOU: the FIRST resolve (detection) returns the given
// non-nil adapter + ResolutionNative (script exists+executable), every LATER resolve
// returns (nil, ResolutionUnsupported) (script deleted / exec-bit cleared). It
// returns a pointer to the call counter so a test can assert resolve fires exactly
// once per detection. It does NOT resolve detection — call resolveDetection for that.
func wireTOCTOUResolveSeams(m *Model, detectAdapter spawn.Adapter, ack spawn.AckChannelFull) *int {
	calls := new(int)
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		*calls++
		if *calls == 1 {
			return detectAdapter, spawn.ResolutionNative
		}
		return nil, spawn.ResolutionUnsupported
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnGetenv = func(string) string { return "/usr/bin" }
	return calls
}

// TestBurstDispatch_UsesCachedAdapter_AlreadyResolved is the core single-resolve
// assertion for the direct N≥2 Enter (detection already resolved): the seam is NOT
// called a second time and the burst opens through the CACHED detection-time adapter,
// never a fresh (here nil) re-resolve.
func TestBurstDispatch_UsesCachedAdapter_AlreadyResolved(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	detectAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	calls := wireTOCTOUResolveSeams(&m, detectAdapter, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	if *calls != 1 {
		t.Fatalf("precondition: detection must resolve exactly once, got %d", *calls)
	}
	if m.DetectUnsupported() {
		t.Fatal("precondition: the detection-time resolution must be supported (native)")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	if !m.BurstPending() {
		t.Fatal("a supported N≥2 Enter must dispatch the burst")
	}
	// The single-resolve contract: dispatchBurst must NOT re-resolve. Before the fix
	// it called m.resolve a second time (here returning (nil, unsupported)) and built
	// spawn.NewBurster(nil, …), nil-panicking the burst goroutine.
	if *calls != 1 {
		t.Errorf("m.resolve called %d times, want 1 (dispatchBurst must read the cached adapter, not re-resolve)", *calls)
	}

	// Drive the burst to its terminal outcome: the CACHED (non-nil) adapter opens the
	// one external window without a nil panic.
	m = drainBatchToModel(t, m, cmd)
	if len(detectAdapter.Calls) != 1 {
		t.Fatalf("the CACHED detection-time adapter must open the one external window; OpenWindow calls = %d, want 1", len(detectAdapter.Calls))
	}
}

// TestBurstDispatch_UsesCachedAdapter_DeferredEntry covers the deferred-detection
// entry point (terminalDetectedMsg → decideBurst → dispatchBurst): an N≥2 Enter
// pressed while detection is in flight DEFERS, and when detection resolves the
// deferred burst must build from that SAME single resolve, never a dispatch-time
// second resolve.
func TestBurstDispatch_UsesCachedAdapter_DeferredEntry(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	detectAdapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	calls := wireTOCTOUResolveSeams(&m, detectAdapter, ack)
	// Detection dispatched but not yet resolved (in-flight): the N≥2 Enter defers.
	m.detectDispatched = true

	m = markTwo(t, m)
	m, deferredCmd := pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not dispatch")
	}
	if *calls != 0 {
		t.Fatalf("no resolve may run while the deferred Enter awaits detection, got %d", *calls)
	}
	if deferredCmd != nil {
		t.Error("detection already in flight → no new detection cmd (nil)")
	}

	// Detection resolves supported → the deferred Enter resolution runs decideBurst →
	// dispatchBurst, which must build the burster from the SAME (single) resolve.
	updated, cmd := m.Update(terminalDetectedMsg{identity: ghosttyIdentity()})
	m = updated.(Model)
	if !m.BurstPending() {
		t.Fatal("the terminalDetectedMsg must resolve the deferred burst (supported → dispatch)")
	}
	if *calls != 1 {
		t.Errorf("m.resolve called %d times across the deferred path, want 1 (one detection resolve, no dispatch re-resolve)", *calls)
	}

	m = drainBatchToModel(t, m, cmd)
	if len(detectAdapter.Calls) != 1 {
		t.Fatalf("the cached detection-time adapter must open the one external window; OpenWindow calls = %d, want 1", len(detectAdapter.Calls))
	}
}

// TestBurstDispatch_NilCachedAdapter_RoutesToUnsupportedNoOp is the belt-and-braces
// guard: a model reaching dispatchBurst with a nil cached adapter (an inconsistent /
// undriven resolve — a supported resolution but no adapter) must route to the
// unsupported no-op, never construct spawn.NewBurster(nil, …).
func TestBurstDispatch_NilCachedAdapter_RoutesToUnsupportedNoOp(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	m := NewModelWithSessions(sessions)
	// An INCONSISTENT/undriven resolve: a supported resolution (native) but a nil
	// adapter — the belt-and-braces case the nil-guard must catch. A native resolution
	// leaves DetectUnsupported() false, so decideBurst falls THROUGH to dispatchBurst.
	m.detector = &fakeDetector{identity: ghosttyIdentity()}
	m.resolve = func(spawn.Identity) (spawn.Adapter, spawn.Resolution) {
		return nil, spawn.ResolutionNative
	}
	m.sessionExists = allPresent
	m.ackChannel = ack
	m.spawnExe = func() (string, error) { return "/abs/portal", nil }
	m.spawnGetenv = func(string) string { return "/usr/bin" }

	m = resolveDetection(t, m, ghosttyIdentity())
	if m.DetectUnsupported() {
		t.Fatal("precondition: a native resolution is supported, so decideBurst must fall through to dispatchBurst")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	// The nil-adapter guard must route to the unsupported no-op — never a burster.
	if m.BurstPending() {
		t.Error("a nil cached adapter must NOT enter burst-pending (routes to the unsupported no-op)")
	}
	if m.burstPipe != nil {
		t.Error("a nil cached adapter must construct NO burst pipe")
	}
	if isQuitCmd(cmd) {
		t.Error("the nil-adapter no-op must NOT tea.Quit")
	}
	if m.Selected() != "" {
		t.Errorf("the nil-adapter no-op must NOT self-attach; Selected() = %q", m.Selected())
	}
	if !m.MultiSelectActive() {
		t.Error("the nil-adapter no-op must stay in multi-select mode")
	}
	if m.SelectedSessionCount() != 2 {
		t.Errorf("the selection must be INTACT after the no-op; count = %d, want 2", m.SelectedSessionCount())
	}
	want := unsupportedFlashText(m.DetectedIdentity())
	if m.flashText != want {
		t.Errorf("flashText = %q, want %q (the nil-adapter guard mirrors decideBurst's unsupported no-op flash)", m.flashText, want)
	}
}
