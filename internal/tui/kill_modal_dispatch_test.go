package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func killDispatchModel(t *testing.T) (Model, *killerStub) {
	t.Helper()
	sessions := []Session{
		{Name: "alpha", Attached: true},
		{Name: "bravo", Attached: false},
	}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, historyFor(t, "hi", sessions), lister)
	killer := &killerStub{}
	m.sessionKiller = killer
	return m, killer
}

func TestKillKey_StoresName(t *testing.T) {
	m, _ := killDispatchModel(t)
	m.sessionList.Select(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	got := updated.(Model)

	if got.modal != modalKillConfirm {
		t.Fatalf("k must open the kill-confirm modal; modal = %v", got.modal)
	}
	if got.pendingKill.Name != "alpha" {
		t.Errorf("pendingKillName = %q, want %q", got.pendingKill.Name, "alpha")
	}
}

func TestKillConfirm_YConfirmsParity(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(1)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	afterK := updated.(Model)
	if afterK.pendingKill.Name != "bravo" {
		t.Fatalf("setup: expected bravo; got %q", afterK.pendingKill.Name)
	}

	updated2, cmd := afterK.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	afterY := updated2.(Model)

	if afterY.modal != modalNone {
		t.Errorf("y must close the modal; modal = %v", afterY.modal)
	}
	if afterY.pendingKill.Name != "" {
		t.Errorf("y must clear pendingKillName; got %q", afterY.pendingKill.Name)
	}
	if cmd == nil {
		t.Fatalf("y must emit the killAndRefresh cmd; got nil")
	}
	_ = cmd()
	if killer.killedName != "bravo" {
		t.Errorf("killAndRefresh must kill the stored name; KillSession(%q), want %q", killer.killedName, "bravo")
	}
}

func TestKillConfirm_EscCancelsClearsName(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(0)

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
	if afterEsc.pendingKill.Name != "" {
		t.Errorf("Esc must clear pendingKillName; got %q", afterEsc.pendingKill.Name)
	}
	if killer.killedName != "" {
		t.Errorf("Esc must not kill anything; KillSession called with %q", killer.killedName)
	}
}

func TestKillConfirm_NIgnored(t *testing.T) {
	m, killer := killDispatchModel(t)
	m.sessionList.Select(0)

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
	if afterN.pendingKill.Name != "alpha" {
		t.Errorf("n must not clear pendingKillName; got %q", afterN.pendingKill.Name)
	}
	if killer.killedName != "" {
		t.Errorf("n must not kill anything; KillSession called with %q", killer.killedName)
	}
}
