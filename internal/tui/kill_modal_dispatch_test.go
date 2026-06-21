package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// killDispatchModel builds a Sessions model wired with a killer and a known
// session set (the cursor starts on the first row), for exercising the kill-confirm
// dispatch behaviour (NOT the render shell — that is covered by kill_modal_test.go).
func killDispatchModel(t *testing.T) (Model, *killerStub) {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	// killAndRefresh re-lists sessions after the kill, so a lister is required for
	// the `y` parity path (draining the cmd) not to nil-deref.
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)
	killer := &killerStub{}
	m.sessionKiller = killer
	return m, killer
}

// TestKillKey_StoresNameAndWindows asserts handleKillKey records BOTH the target's
// name and its window count (the count feeds the modal's `· N window(s)` line).
func TestKillKey_StoresNameAndWindows(t *testing.T) {
	m, _ := killDispatchModel(t)
	m.sessionList.Select(0) // alpha, 3 windows

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	got := updated.(Model)

	if got.modal != modalKillConfirm {
		t.Fatalf("k must open the kill-confirm modal; modal = %v", got.modal)
	}
	if got.pendingKillName != "alpha" {
		t.Errorf("pendingKillName = %q, want %q", got.pendingKillName, "alpha")
	}
	if got.pendingKillWindows != 3 {
		t.Errorf("pendingKillWindows = %d, want 3", got.pendingKillWindows)
	}
}

// TestKillConfirm_YConfirmsParity asserts `y` confirms exactly as before: it closes
// the modal, clears both pending fields, and emits the killAndRefresh cmd that calls
// KillSession with the stored name (behaviour parity — the reskin only restyles).
func TestKillConfirm_YConfirmsParity(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(1) // bravo, 1 window

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	afterK := updated.(Model)
	if afterK.pendingKillName != "bravo" || afterK.pendingKillWindows != 1 {
		t.Fatalf("setup: expected bravo/1; got %q/%d", afterK.pendingKillName, afterK.pendingKillWindows)
	}

	updated2, cmd := afterK.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	afterY := updated2.(Model)

	if afterY.modal != modalNone {
		t.Errorf("y must close the modal; modal = %v", afterY.modal)
	}
	if afterY.pendingKillName != "" {
		t.Errorf("y must clear pendingKillName; got %q", afterY.pendingKillName)
	}
	if afterY.pendingKillWindows != 0 {
		t.Errorf("y must clear pendingKillWindows; got %d", afterY.pendingKillWindows)
	}
	if cmd == nil {
		t.Fatalf("y must emit the killAndRefresh cmd; got nil")
	}
	// Draining the cmd kills the stored name (parity with killAndRefresh).
	_ = cmd()
	if killer.killedName != "bravo" {
		t.Errorf("killAndRefresh must kill the stored name; KillSession(%q), want %q", killer.killedName, "bravo")
	}
}

// TestKillConfirm_EscCancelsClearsBoth asserts `Esc` cancels: it closes the modal,
// emits no cmd (no kill), and clears BOTH pendingKillName and pendingKillWindows
// (no stale state on cancel).
func TestKillConfirm_EscCancelsClearsBoth(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(0) // alpha, 3 windows

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	afterK := updated.(Model)

	updated2, cmd := afterK.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	afterEsc := updated2.(Model)

	if afterEsc.modal != modalNone {
		t.Errorf("Esc must close the modal; modal = %v", afterEsc.modal)
	}
	if cmd != nil {
		t.Errorf("Esc must not emit a cmd (no kill); got non-nil")
	}
	if afterEsc.pendingKillName != "" {
		t.Errorf("Esc must clear pendingKillName; got %q", afterEsc.pendingKillName)
	}
	if afterEsc.pendingKillWindows != 0 {
		t.Errorf("Esc must clear pendingKillWindows; got %d", afterEsc.pendingKillWindows)
	}
	if killer.killedName != "" {
		t.Errorf("Esc must not kill anything; KillSession called with %q", killer.killedName)
	}
}

// TestKillConfirm_NIgnored asserts `n` is no longer a cancel key (§8.3 drops n):
// it is ignored — the modal stays open, no kill fires, the pending state is intact.
func TestKillConfirm_NIgnored(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(0) // alpha, 3 windows

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	afterK := updated.(Model)

	updated2, cmd := afterK.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	afterN := updated2.(Model)

	if afterN.modal != modalKillConfirm {
		t.Errorf("n must be ignored (modal stays open); modal = %v", afterN.modal)
	}
	if cmd != nil {
		t.Errorf("n must not emit a cmd; got non-nil")
	}
	if afterN.pendingKillName != "alpha" {
		t.Errorf("n must not clear pendingKillName; got %q", afterN.pendingKillName)
	}
	if afterN.pendingKillWindows != 3 {
		t.Errorf("n must not clear pendingKillWindows; got %d", afterN.pendingKillWindows)
	}
	if killer.killedName != "" {
		t.Errorf("n must not kill anything; KillSession called with %q", killer.killedName)
	}
}
