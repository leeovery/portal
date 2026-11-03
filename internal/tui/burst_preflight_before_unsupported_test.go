package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

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

func TestBurstUnsupported_PreflightAbortBeforeNoop(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Answered: true},
		{Name: "bravo", Answered: true},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m.sessionExists = func(name string) bool { return name != "bravo" }
	m = markTwo(t, m)
	m = resolveDetection(t, m, appleTerminalIdentity())
	if !m.DetectUnsupported() {
		t.Fatal("precondition: com.apple.Terminal must resolve unsupported")
	}

	m, cmd := pressEnter(t, m)

	assertUnsupportedPreflightAbort(t, m, adapter)
	if isQuitCmd(cmd) {
		t.Error("pre-flight abort must NOT tea.Quit")
	}
}

func TestBurstUnsupported_DeferredPreflightAbortBeforeNoop(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Answered: true},
		{Name: "bravo", Answered: true},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireUnsupportedBurstSeams(&m, adapter, ack)
	m.sessionExists = func(name string) bool { return name != "bravo" }
	m.detectDispatched = true

	m = markTwo(t, m)
	m, _ = pressEnter(t, m)
	if m.BurstPending() {
		t.Fatal("N≥2 Enter while detection is in flight must DEFER, not act")
	}

	updated, cmd2 := m.Update(terminalDetectedMsg{identity: appleTerminalIdentity()})
	m = updated.(Model)

	assertUnsupportedPreflightAbort(t, m, adapter)
	if isQuitCmd(cmd2) {
		t.Error("deferred pre-flight abort must NOT tea.Quit")
	}
}
