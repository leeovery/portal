package tui

// restore-host-terminal-windows-6-4 — full-success self-attach (net N) + marker
// self-clean.
//
// These white-box (package tui) tests drive a FULL-success N≥2 burst end-to-end —
// every external window confirms its token ack via the FakeAdapter/FakeAckChannel
// confirm-all path — and assert the terminal spawnCompleteMsg arm:
//   - Selected() == the trigger and the returned cmd is tea.Quit (driving the
//     existing AttachConnector/SwitchConnector via processTUIResult; NO adapter),
//   - the batch markers are self-cleaned by the burst goroutine BEFORE the
//     terminal message / before the self-attach handoff,
//   - no success flash / "N/N ✓" nag renders,
//   - includes-self and confirmed-while-attached-elsewhere both self-attach
//     (no special-casing, no dup guard),
//   - the non-all-confirmed path stays UNCHANGED (§6-3 record + clear pending, no
//     quit — the partial-failure/abort behaviour is tasks 6-6/6-7).
//
// The seam helpers (wireBurstSeams, allPresent, resolveDetection, markRow,
// spawnedSession, ghosttyIdentity) live in burst_dispatch_test.go. No t.Parallel:
// consistent with the rest of the tui test surface.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// setupConfirmingBurst builds a resolved-supported model with every named session
// marked (multi-select), wired to a FakeAdapter + FakeAckChannel whose default
// confirm-all path writes every window's token — so pressing Enter drives a
// FULL-success burst. It returns the model (ready for pressEnter), the adapter,
// and the ack channel for post-drain assertions.
func setupConfirmingBurst(t *testing.T, names []string) (Model, *spawntest.FakeAdapter, *spawntest.FakeAckChannel) {
	t.Helper()
	sessions := make([]tmux.Session, len(names))
	for i, n := range names {
		sessions[i] = tmux.Session{Name: n, Windows: i + 1}
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = pressSession(t, m, pressM)
	for i := range names {
		m = markRow(t, m, i)
	}
	if m.SelectedSessionCount() != len(names) {
		t.Fatalf("precondition: %d marked, got %d", len(names), m.SelectedSessionCount())
	}
	return m, adapter, ack
}

// driveBurstToTerminal runs the burst receiver chain from cmd, feeding each
// spawnProgressMsg back through Update to re-issue the receiver, and returns the
// model as it stood JUST BEFORE the terminal event plus the terminal tea.Msg
// (spawnCompleteMsg or spawnAbortMsg). The caller applies the terminal message
// itself, so it can assert pre-application state (e.g. the ack Clean ordering and
// that Selected() is still unset before the self-attach handoff).
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

// TestBurst_FullSuccess_SelfAttachesToTriggerAndQuits is the core assertion: a
// burst where every external window confirms sets Selected() to the trigger and
// returns tea.Quit — driving the picker's existing connector via processTUIResult
// (net N windows, never N+1).
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

// TestBurst_FullSuccess_CleansMarkersBeforeSelfAttachHandoff pins the
// self-clean-before-self-exec ordering: the burst goroutine records
// AckChannel.Clean(batch) STRICTLY before emitting the terminal spawnCompleteMsg,
// so by the time the terminal message is in hand — and BEFORE Selected() is set
// (the exec handoff) — the markers are already swept.
func TestBurst_FullSuccess_CleansMarkersBeforeSelfAttachHandoff(t *testing.T) {
	m, _, ack := setupConfirmingBurst(t, []string{"alpha", "bravo", "charlie"})

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)

	// Clean has already run (goroutine calls it before emitting the terminal
	// event), and Selected() is not yet set (only the terminal-message apply sets
	// it) — so Clean is strictly before the self-attach handoff.
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

// TestBurst_FullSuccess_RendersNoSuccessFlash guards the silent self-attach: a
// full-success burst sets NO flash — no "N/N ✓" nag.
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

// TestBurst_FullSuccess_IncludesSelfSelectionSelfAttaches covers the includes-self
// edge: the trigger is one of the marked sessions (no special-casing at the model
// layer). The trigger self-attaches; the rest of the marked set spawns externally
// and the trigger is never in the external open set.
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

// TestBurst_FullSuccess_ConfirmedWhileAttachedElsewhere covers the
// attached-elsewhere edge: a session already attached on another client still
// confirms via the token ack (the FakeAdapter writes OUR new window's token) — no
// dup guard — so the burst is full-success and self-attaches.
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

// TestBurst_NotAllConfirmed_RecordsAndClearsPendingWithoutQuit guards the
// unchanged §6-3 non-all-confirmed path: a single external spawn-failure (→
// AckFailed, classified WITHOUT an ack wait so the test stays fast) records the
// results, clears burst-pending, and does NOT self-attach or quit — the
// partial-failure/abort behaviour is tasks 6-6/6-7.
func TestBurst_NotAllConfirmed_RecordsAndClearsPendingWithoutQuit(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}
	ack := &spawntest.FakeAckChannel{}
	// external = [alpha]; a spawn-failed Result classifies AckFailed immediately
	// (no await), so the terminal message is not full-success.
	adapter := &spawntest.FakeAdapter{Ack: ack, Results: []spawn.Result{spawn.SpawnFailed("boom")}}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = pressSession(t, m, pressM)
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
		t.Error("the non-all-confirmed path returns a nil cmd (unchanged §6-3), got non-nil (no tea.Quit)")
	}
	if rm.BurstPending() {
		t.Error("the non-all-confirmed path must still clear burst-pending")
	}
	if len(rm.burstResults) != 1 {
		t.Errorf("burstResults must be recorded on the non-all-confirmed path; got %d", len(rm.burstResults))
	}
	if rm.burstBatch == "" {
		t.Error("burstBatch must be recorded on the non-all-confirmed path")
	}
}
