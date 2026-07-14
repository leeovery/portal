package tui

// restore-host-terminal-windows-8-3 — run pre-flight before the unsupported gate on
// the picker burst path.
//
// The CLI's runSpawn pre-flights the whole batch FIRST — ahead of the N≥2
// unsupported gate — so a gone session aborts with the more-actionable gone-session
// message even on an unsupported terminal. These white-box (package tui) tests pin
// the same ordering on the picker: decideBurst evaluates spawn.PreflightMissing over
// the marked set before the DetectUnsupported() atomic no-op, so a gone session on an
// unsupported terminal surfaces the abort banner + prunes the selection (matching
// handlePreflightAbort) instead of re-asserting the unsupported banner. Both entry
// points into decideBurst are covered: the already-resolved N≥2 Enter and the
// deferred-detection resolution.
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// assertUnsupportedPreflightAbort asserts the §8-3 outcome shared by both entry
// points: a gone session on an unsupported terminal takes the pre-flight-abort arm
// (gone banner + prune keeping survivors) and NOT the unsupported no-op — nothing
// spawned, no flash, still in multi-select mode, no self-attach.
func assertUnsupportedPreflightAbort(t *testing.T, m Model, adapter *spawntest.FakeAdapter) {
	t.Helper()
	if want := spawn.GoneMessage([]string{"bravo"}); m.abortBannerText != want {
		t.Errorf("abortBannerText = %q, want %q (pre-flight abort banner, not the unsupported no-op)", m.abortBannerText, want)
	}
	if m.flashText != "" {
		t.Errorf("the unsupported no-op flash must NOT fire when a session is gone; flashText = %q", m.flashText)
	}
	if _, ok := m.goneFlagged["bravo"]; !ok {
		t.Errorf("the gone session must be flagged; goneFlagged = %v", m.goneFlagged)
	}
	if m.IsSessionSelected("bravo") {
		t.Error("the gone session must be pruned from the selection")
	}
	if !m.IsSessionSelected("alpha") {
		t.Error("the survivor must stay marked (a second Enter proceeds with survivors)")
	}
	if m.SelectedSessionCount() != 1 {
		t.Errorf("selection count = %d, want 1 (gone pruned, survivor kept)", m.SelectedSessionCount())
	}
	if len(adapter.Calls) != 0 {
		t.Errorf("nothing may spawn on a pre-flight abort; adapter OpenWindow calls = %d", len(adapter.Calls))
	}
	if m.BurstPending() {
		t.Error("pre-flight abort must not enter burst-pending")
	}
	if !m.MultiSelectActive() {
		t.Error("pre-flight abort must stay in multi-select mode")
	}
}

// TestBurstUnsupported_PreflightAbortBeforeNoop drives the already-resolved N≥2
// Enter on a resolved-unsupported terminal (Apple Terminal) with one marked session
// externally killed. Pre-flight runs BEFORE the unsupported no-op, so the gone
// session surfaces the abort banner and is pruned — matching cmd/spawn.go — instead
// of the unsupported banner re-asserting.
func TestBurstUnsupported_PreflightAbortBeforeNoop(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	// bravo vanished between marking and Enter.
	m.sessionExists = func(name string) bool { return name != "bravo" }
	m = resolveDetection(t, m, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatal("precondition: com.apple.Terminal must resolve unsupported")
	}

	m = markTwo(t, m)
	m, cmd := pressEnter(t, m)

	assertUnsupportedPreflightAbort(t, m, adapter)
	if isQuitCmd(cmd) {
		t.Error("pre-flight abort must NOT tea.Quit")
	}
}

// TestBurstUnsupported_DeferredPreflightAbortBeforeNoop covers the deferred entry
// point: an N≥2 Enter pressed while detection is in flight DEFERS, and the
// deferred-Enter resolution (the terminalDetectedMsg arm → decideBurst) still runs
// pre-flight before the unsupported no-op when it resolves to an unsupported
// identity — so the same gone-session abort lands on the deferred path.
func TestBurstUnsupported_DeferredPreflightAbortBeforeNoop(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m.sessionExists = func(name string) bool { return name != "bravo" }
	// Detection dispatched but not resolved (in-flight).
	m.detectDispatched = true

	m = markTwo(t, m)
	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not act")
	}

	// Detection resolves to the unsupported Apple Terminal identity → the deferred
	// Enter resolution runs pre-flight BEFORE the unsupported no-op and aborts.
	updated, cmd2 := m.Update(terminalDetectedMsg{identity: appleTerminalIdentity()})
	m = updated.(Model)

	assertUnsupportedPreflightAbort(t, m, adapter)
	if isQuitCmd(cmd2) {
		t.Error("deferred pre-flight abort must NOT tea.Quit")
	}
}
