package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

func setupConfirmingBurst(t *testing.T, names []string) (Model, *spawntest.FakeAdapter, *spawntest.FakeAckChannel) {
	t.Helper()
	m, adapter, ack := markedSupportedBurstModel(t, names)
	if m.SelectedSessionCount() != len(names) {
		t.Fatalf("precondition: %d marked, got %d", len(names), m.SelectedSessionCount())
	}
	return m, adapter, ack
}

func driveBurstToTerminal(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Msg) {
	t.Helper()
	for range 50 {
		if cmd == nil {
			t.Fatal("burst receiver chain ended without a terminal message")
		}
		msg := cmd()
		if _, ok := msg.(spawnProgressMsg); ok {
			updated, follow := m.Update(msg)
			m = updated.(Model)
			cmd = follow
			continue
		}
		return m, msg
	}
	t.Fatal("burst did not terminate within the step budget")
	return m, nil
}

func TestBurst_FullSuccess_SelfAttachesToTriggerAndQuits(t *testing.T) {
	m, adapter, _ := setupConfirmingBurst(t, []string{"alpha", "bravo", "charlie"})

	m, cmd := pressEnter(t, m)
	if !m.BurstPending() {
		t.Fatal("precondition: burst must be pending after dispatch")
	}

	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	if len(complete.Results) != 2 {
		t.Fatalf("precondition: want 2 external results, got %d", len(complete.Results))
	}
	for i, r := range complete.Results {
		if r.Ack != spawn.AckConfirmed {
			t.Fatalf("precondition: result[%d].Ack = %q, want confirmed", i, r.Ack)
		}
	}

	updated, follow := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.Selected() != "charlie" {
		t.Errorf("Selected() = %q, want charlie (self-attach to the trigger)", rm.Selected())
	}
	if !isQuitCmd(follow) {
		t.Error("full success must return tea.Quit (drives the existing connector via processTUIResult)")
	}
	if rm.BurstPending() {
		t.Error("full success must clear burst-pending")
	}
	if len(adapter.Calls) != 2 {
		t.Fatalf("OpenWindow called %d times, want 2 (N-1 external; the trigger self-attaches)", len(adapter.Calls))
	}
	for _, call := range adapter.Calls {
		if spawnedSession(t, call) == "charlie" {
			t.Error("the trigger (charlie) must self-attach, never be externally opened")
		}
	}
}

func TestBurst_FullSuccess_CleansMarkersBeforeSelfAttachHandoff(t *testing.T) {
	m, _, ack := setupConfirmingBurst(t, []string{"alpha", "bravo", "charlie"})

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)

	if len(ack.Cleaned) != 1 {
		t.Fatalf("batch markers must be cleaned before the terminal spawnCompleteMsg; Clean calls = %d, want 1", len(ack.Cleaned))
	}
	if mBefore.Selected() != "" {
		t.Fatalf("precondition: Selected() must be unset before the terminal message is applied, got %q", mBefore.Selected())
	}

	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	updated, _ := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.Selected() != "charlie" {
		t.Errorf("Selected() = %q, want charlie after the self-attach", rm.Selected())
	}
	if len(ack.Cleaned) != 1 {
		t.Errorf("the self-attach must not re-Clean; Clean calls = %d, want 1", len(ack.Cleaned))
	}
}

func TestBurst_FullSuccess_RendersNoSuccessFlash(t *testing.T) {
	m, _, _ := setupConfirmingBurst(t, []string{"alpha", "bravo", "charlie"})

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}

	updated, _ := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.flashText != "" {
		t.Errorf("full success must set NO flash (silent self-attach, no N/N ✓ nag); flashText = %q", rm.flashText)
	}
}

func TestBurst_FullSuccess_IncludesSelfSelectionSelfAttaches(t *testing.T) {
	m, adapter, _ := setupConfirmingBurst(t, []string{"alpha", "bravo"})

	m, cmd := pressEnter(t, m)
	trigger := m.BurstTrigger()

	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}

	updated, follow := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.Selected() != trigger {
		t.Errorf("Selected() = %q, want the trigger %q (self-attach, no special-casing)", rm.Selected(), trigger)
	}
	if !isQuitCmd(follow) {
		t.Error("includes-self full success must quit to self-attach")
	}
	if len(adapter.Calls) != 1 {
		t.Errorf("the rest of the marked set (N-1 = 1) must spawn externally; OpenWindow calls = %d", len(adapter.Calls))
	}
	for _, call := range adapter.Calls {
		if spawnedSession(t, call) == trigger {
			t.Errorf("the trigger %q must self-attach, never be externally opened", trigger)
		}
	}
}

func TestBurst_FullSuccess_ConfirmedWhileAttachedElsewhere(t *testing.T) {
	m, _, _ := setupConfirmingBurst(t, []string{"alpha", "bravo"})

	m, cmd := pressEnter(t, m)
	trigger := m.BurstTrigger()

	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	if complete.Results[0].Ack != spawn.AckConfirmed {
		t.Fatalf("the token ack must confirm our window regardless of other clients; Ack = %q", complete.Results[0].Ack)
	}

	updated, follow := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.Selected() != trigger {
		t.Errorf("Selected() = %q, want the trigger %q (no dup guard)", rm.Selected(), trigger)
	}
	if !isQuitCmd(follow) {
		t.Error("a confirmed-elsewhere window must still self-attach (tea.Quit)")
	}
}

func TestBurst_NotAllConfirmed_ClearsPendingWithoutQuit(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Answered: true},
		{Name: "bravo", Answered: true},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack, Results: []spawn.Result{spawn.SpawnFailed("boom")}}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	if complete.Results[0].Ack != spawn.AckFailed {
		t.Fatalf("precondition: the failed external window must classify AckFailed, got %q", complete.Results[0].Ack)
	}

	updated, follow := mBefore.Update(complete)
	rm := updated.(Model)

	if rm.Selected() != "" {
		t.Errorf("a non-all-confirmed burst must NOT self-attach; Selected() = %q, want empty", rm.Selected())
	}
	if follow != nil {
		t.Error("the non-all-confirmed path returns a nil cmd (unchanged), got non-nil (no tea.Quit)")
	}
	if rm.BurstPending() {
		t.Error("the non-all-confirmed path must still clear burst-pending")
	}
	if !rm.IsSessionSelected("alpha") {
		t.Error("the spawn-failed alpha must stay marked for a retry")
	}
	if !rm.MultiSelectActive() {
		t.Error("a non-all-confirmed burst must stay in multi-select mode")
	}
}
