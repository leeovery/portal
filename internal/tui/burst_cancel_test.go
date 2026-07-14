package tui

// restore-host-terminal-windows-6-8 — Ctrl-C / Esc cancellation mid-burst.
//
// White-box (package tui) tests of cancelBurst + the post-cancel terminal arm.
// They cover the two contracts:
//
//  1. The FIX for the concurrency defect found in review: the burst's TERMINAL
//     event must be delivered RELIABLY after a cancel (the naked terminal send in
//     burstProgressPipe.send), so burstPending is ALWAYS cleared and the picker
//     never permanently input-locks. Under the previous ctx-guarded terminal send a
//     cancelled ctx raced the terminal send and dropped it ~50% of the time,
//     wedging the picker. driveCancelToTerminal's burstChannelClosedMsg guard is the
//     regression tripwire — a dropped terminal closes the channel before any
//     spawnCompleteMsg and fails the test.
//
//  2. The cancellation behaviour: Ctrl-C/Esc mid-burst cancels the goroutine's ctx
//     (no tea.Quit — returns to multi-select), keeps burstPending true until the
//     goroutine's terminal event lands, and the completion handler applies the same
//     leave-what-opened selection mutation as a partial failure — SILENTLY (no
//     flash). Cancel before the first spawn keeps every marked session marked; cancel
//     after some opened unmarks only the confirmed sessions.
//
// The mutation/suppression assertions inject a crafted terminal spawnCompleteMsg into
// a directly-constructed pending-burst model (newPendingBurstModel, in
// burst_partial_failure_test.go) for determinism; the delivery/self-clean assertions
// drive a REAL burst goroutine and cancel it, exercising the send path under -race.
//
// Shared seam helpers (wireBurstSeams, allPresent, resolveDetection, markRow,
// ghosttyIdentity, newPendingBurstModel, injectComplete, isQuitCmd, sessionsFromNames)
// live in the sibling burst test files. No t.Parallel: consistent with the rest of the
// tui test surface.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
)

// cancellablePendingModel builds a marked multi-select model forced into
// burst-pending with a live pipe + a recording cancel func, WITHOUT a real
// goroutine, so cancelBurst's wiring (invoke cancel, set burstCancelled, re-issue the
// receiver, no quit) can be exercised in isolation. The returned *bool flips true
// when the recorded cancel func fires. The pipe is pre-loaded with a benign terminal
// event so the re-issued receiver resolves immediately (there is no goroutine here) —
// letting isQuitCmd probe the returned cmd without blocking, proving it is the
// receiver and never tea.Quit.
func cancellablePendingModel(t *testing.T, names ...string) (Model, *bool) {
	t.Helper()
	m := newPendingBurstModel(t, names)
	m.termWidth = 80
	m.termHeight = 24
	cancelled := false
	pipe := newBurstProgressPipe()
	pipe.ch <- burstProgress{Done: true}
	m.burstPipe = pipe
	m.burstCancel = func() { cancelled = true }
	return m, &cancelled
}

// driveCancelToTerminal runs the burst receiver chain from cmd, feeding each
// spawnProgressMsg back through Update to re-issue the receiver, and APPLIES the
// terminal event, returning the resulting model. A burstChannelClosedMsg observed
// BEFORE any terminal event means the terminal send was dropped — the exact
// concurrency defect the naked terminal send fixes — so it fails loudly.
func driveCancelToTerminal(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for range 200 {
		if cmd == nil {
			t.Fatal("cancelBurst must return the receiver so the terminal event is drained")
		}
		msg := cmd()
		updated, follow := m.Update(msg)
		m = updated.(Model)
		switch msg.(type) {
		case spawnProgressMsg:
			cmd = follow
		case spawnCompleteMsg, spawnAbortMsg:
			return m
		case burstChannelClosedMsg:
			t.Fatal("burst channel closed WITHOUT delivering the terminal event — the terminal send was dropped on cancel (the naked-terminal-send regression)")
		default:
			t.Fatalf("unexpected burst message %T", msg)
		}
	}
	t.Fatal("burst did not terminate within the step budget")
	return m
}

// realCancellableBurst wires a resolved-supported model whose windows OPEN but never
// confirm their token (Confirm all false) — so the burst would poll to timeout, and
// only a cancel makes it stop fast — and dispatches it, returning the pending model,
// the receiver cmd, and the ack channel.
func realCancellableBurst(t *testing.T, names ...string) (Model, tea.Cmd, *spawntest.FakeAckChannel) {
	t.Helper()
	ack := &spawntest.FakeAckChannel{}
	confirm := make([]bool, len(names)) // all false → no window ever confirms
	adapter := &spawntest.FakeAdapter{Ack: ack, Confirm: confirm}
	m := NewModelWithSessions(sessionsFromNames(names))
	m.termWidth = 80
	m.termHeight = 24
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())
	m = pressSession(t, m, pressM)
	for i := range names {
		m = markRow(t, m, i)
	}
	m, cmd := pressEnter(t, m)
	if !m.BurstPending() {
		t.Fatal("precondition: the burst must be pending after dispatch")
	}
	return m, cmd, ack
}

// TestBurstCancel_CtrlCReturnsToMultiSelectNotQuit — "it cancels the burst and
// returns to multi-select mode (not quit) on Ctrl-C mid-burst".
func TestBurstCancel_CtrlCReturnsToMultiSelectNotQuit(t *testing.T) {
	m, cancelled := cancellablePendingModel(t, "alpha", "bravo", "charlie")

	updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)

	if !*cancelled {
		t.Error("Ctrl-C mid-burst must invoke burstCancel (cancel the goroutine ctx)")
	}
	if isQuitCmd(cmd) {
		t.Error("Ctrl-C mid-burst must NOT tea.Quit — the picker returns to multi-select mode")
	}
	if !m.multiSelectMode {
		t.Error("Ctrl-C mid-burst must stay in multi-select mode")
	}
	if !m.burstCancelled {
		t.Error("cancelBurst must set burstCancelled so the completion handler suppresses the flash + quit")
	}
	if !m.BurstPending() {
		t.Error("burstPending must stay true until the goroutine's terminal event lands")
	}
	if cmd == nil {
		t.Error("cancelBurst must return the receiver cmd so the terminal event is still drained")
	}
}

// TestBurstCancel_EscReturnsToMultiSelectNotQuit — "it cancels the burst and returns
// to multi-select mode on Esc mid-burst".
func TestBurstCancel_EscReturnsToMultiSelectNotQuit(t *testing.T) {
	m, cancelled := cancellablePendingModel(t, "alpha", "bravo")

	updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if !*cancelled {
		t.Error("Esc mid-burst must invoke burstCancel")
	}
	if isQuitCmd(cmd) {
		t.Error("Esc mid-burst must not quit")
	}
	if !m.multiSelectMode {
		t.Error("Esc mid-burst must route to cancelBurst, NOT exit multi-select mode")
	}
	if !m.burstCancelled {
		t.Error("cancelBurst must set burstCancelled")
	}
	if !m.BurstPending() {
		t.Error("burstPending must stay true until the terminal event")
	}
	if cmd == nil {
		t.Error("cancelBurst must return the receiver cmd")
	}
}

// TestBurstCancel_BeforeFirstSpawnKeepsAllMarkedSilent — "it opens nothing and keeps
// all marked when cancelled before the first spawn". The goroutine breaks on the
// between-windows ctx check before opening anything → an empty-Results terminal event;
// the completion handler unmarks nothing and stays silent.
func TestBurstCancel_BeforeFirstSpawnKeepsAllMarkedSilent(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	m.burstCancelled = true
	before := m.SelectedSessionCount()

	rm, follow := injectComplete(t, m, spawnCompleteMsg{Batch: "b1", Results: nil})

	for _, n := range []string{"alpha", "bravo", "charlie"} {
		if !rm.IsSessionSelected(n) {
			t.Errorf("%q must stay marked after a cancel before the first spawn (nothing opened)", n)
		}
	}
	if got := rm.SelectedSessionCount(); got != before {
		t.Errorf("cancel before first spawn must leave the selection unchanged; count %d → %d", before, got)
	}
	if rm.flashText != "" {
		t.Errorf("cancel is user-initiated → silent; flashText = %q, want empty", rm.flashText)
	}
	if follow != nil {
		t.Error("cancel must not return a cmd (no self-attach, no tea.Quit)")
	}
	if rm.Selected() != "" {
		t.Errorf("cancel must not self-attach; Selected() = %q", rm.Selected())
	}
	if rm.BurstPending() {
		t.Error("the terminal event must clear burstPending (no permanent input-lock)")
	}
	if !rm.MultiSelectActive() {
		t.Error("cancel stays in multi-select mode")
	}
	if rm.burstCancelled {
		t.Error("burstCancelled must reset after the terminal event")
	}
}

// TestBurstCancel_AfterSomeOpenedUnmarksConfirmedKeepsRest — "it leaves opened
// windows and unmarks only the opened sessions when cancelled after some opened".
// The confirmed windows are unmarked (a retry must not re-open them); the ack-abandoned
// and un-attempted windows plus the trigger stay marked — silently.
func TestBurstCancel_AfterSomeOpenedUnmarksConfirmedKeepsRest(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie", "delta"})
	// external = [alpha, bravo, charlie]; trigger = delta. alpha confirmed before the
	// cancel; bravo's ack poll was abandoned by the cancel (AckTimeout); charlie + delta
	// never attempted.
	m.burstCancelled = true
	msg := spawnCompleteMsg{
		Batch: "b1",
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
		},
	}

	rm, follow := injectComplete(t, m, msg)

	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked (its window opened; a retry must not re-open it)")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the ack-abandoned bravo must stay marked (a retry re-opens it)")
	}
	if !rm.IsSessionSelected("charlie") {
		t.Error("the un-attempted charlie must stay marked")
	}
	if !rm.IsSessionSelected("delta") {
		t.Error("the trigger delta must stay marked (no self-attach)")
	}
	if rm.flashText != "" {
		t.Errorf("cancel is silent; flashText = %q, want empty", rm.flashText)
	}
	if follow != nil {
		t.Error("cancel must not self-attach / quit")
	}
	if rm.BurstPending() {
		t.Error("the terminal event must clear burstPending")
	}
}

// TestBurstCancel_AllConfirmedRaceDoesNotSelfAttach guards the race where a cancel
// arrives AFTER every external window already confirmed: the terminal event is
// all-confirmed, but burstCancelled must still route it to the leave-what-opened arm —
// NO self-attach, NO quit — because the user pressed Ctrl-C.
func TestBurstCancel_AllConfirmedRaceDoesNotSelfAttach(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo"}) // external = [alpha], trigger = bravo
	m.burstCancelled = true
	msg := spawnCompleteMsg{
		Batch: "b1",
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")},
		},
	}

	rm, follow := injectComplete(t, m, msg)

	if follow != nil {
		t.Error("a cancel that races an all-confirmed terminal must NOT self-attach / quit")
	}
	if rm.Selected() != "" {
		t.Errorf("must not self-attach; Selected() = %q", rm.Selected())
	}
	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked (leave-what-opened)")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the trigger bravo must stay marked")
	}
	if rm.flashText != "" {
		t.Errorf("cancel is silent; flashText = %q", rm.flashText)
	}
	if rm.BurstPending() {
		t.Error("burstPending must be cleared")
	}
}

// TestBurstCancel_SelfCleansBatchMarkersOnCancelPath — "it self-cleans the batch
// markers on the cancel path". A REAL cancelled burst still runs the goroutine's
// Clean(batch) on its terminal step.
func TestBurstCancel_SelfCleansBatchMarkersOnCancelPath(t *testing.T) {
	m, cmd, ack := realCancellableBurst(t, "alpha", "bravo", "charlie")

	updated, drainCmd := m.cancelBurst()
	m = updated.(Model)
	// Progress events (if any) may still be in flight on the receiver we abandon here;
	// keep the original receiver chain running until the terminal event.
	_ = cmd
	m = driveCancelToTerminal(t, m, drainCmd)

	if len(ack.Cleaned) != 1 {
		t.Errorf("the cancel path must self-clean the batch exactly once; ack.Cleaned = %v", ack.Cleaned)
	}
	if m.BurstPending() {
		t.Error("the delivered terminal event must clear burstPending on the cancel path")
	}
}

// TestBurstCancel_TerminalEventAlwaysDeliveredAfterCancel is the focused regression
// for the concurrency defect: after a real cancel the terminal event is ALWAYS
// delivered (never dropped by a ctx-vs-send race), so burstPending is cleared and the
// picker is never permanently input-locked. Run under -race (and -count) for the
// concurrency path.
func TestBurstCancel_TerminalEventAlwaysDeliveredAfterCancel(t *testing.T) {
	m, cmd, _ := realCancellableBurst(t, "alpha", "bravo", "charlie", "delta")
	_ = cmd

	updated, drainCmd := m.cancelBurst()
	m = updated.(Model)
	if drainCmd == nil {
		t.Fatal("cancelBurst must return the receiver so the terminal event is drained")
	}

	m = driveCancelToTerminal(t, m, drainCmd)

	if m.BurstPending() {
		t.Error("no permanent input-lock: the reliably-delivered terminal event must clear burstPending")
	}
	if m.burstCancelled {
		t.Error("burstCancelled must reset once the terminal event lands")
	}
	if !m.MultiSelectActive() {
		t.Error("cancellation returns to multi-select mode")
	}
}

// TestBurstCancel_CtrlCLiveWhileInputLockedCancelsNotQuits — "it keeps Ctrl-C live
// while input-locked and cancels rather than quits". A row action (Enter) is swallowed
// (input-locked) while Ctrl-C stays live and cancels — proving the cancellation
// carve-out sits inside the input-lock.
func TestBurstCancel_CtrlCLiveWhileInputLockedCancelsNotQuits(t *testing.T) {
	m, cancelled := cancellablePendingModel(t, "alpha", "bravo")

	// A second Enter is swallowed (input-locked) — no cancel, no quit, still pending.
	updated, enterCmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
	locked := updated.(Model)
	if enterCmd != nil {
		t.Error("Enter must be swallowed while input-locked (nil cmd)")
	}
	if *cancelled {
		t.Error("Enter must not cancel the burst")
	}
	if !locked.BurstPending() {
		t.Error("Enter must leave the burst pending")
	}

	// Ctrl-C stays live even though the picker is otherwise inert — it cancels, not quits.
	updated, ctrlCmd := locked.updateSessionList(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(Model)
	if !*cancelled {
		t.Error("Ctrl-C must stay live while input-locked and invoke burstCancel")
	}
	if isQuitCmd(ctrlCmd) {
		t.Error("Ctrl-C while input-locked must cancel, NOT quit")
	}
	if !m.multiSelectMode {
		t.Error("Ctrl-C while input-locked must stay in multi-select mode")
	}
	if !m.burstCancelled {
		t.Error("Ctrl-C while input-locked must flag burstCancelled")
	}
}

// TestBurstPartialFailureFlash_DegenerateEmptyFailedNoFlash pins the degenerate-flash
// guard at the pure-function level: no failed windows and no permission wall yields NO
// flash text (not the leading-space " failed to open — others left open").
func TestBurstPartialFailureFlash_DegenerateEmptyFailedNoFlash(t *testing.T) {
	got := burstPartialFailureFlash(
		[]spawn.WindowResult{{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")}},
		nil,
	)
	if got != "" {
		t.Errorf("degenerate (no failed windows, no permission wall) must yield no flash; got %q", got)
	}
}

// TestBurstPartialFailure_DegenerateEmptyFailedRendersNoBand covers the model-level
// guard: a non-cancel partial with no failed window (only a confirmed one, fewer
// results than external) sets NO flash band.
func TestBurstPartialFailure_DegenerateEmptyFailedRendersNoBand(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	// external = [alpha, bravo]; trigger = charlie. alpha confirmed; bravo never
	// attempted (fewer results than external → the partial arm), NOT cancelled.
	msg := spawnCompleteMsg{
		Batch:   "b1",
		Results: []spawn.WindowResult{{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")}},
	}

	rm, _ := injectComplete(t, m, msg)

	if rm.flashText != "" {
		t.Errorf("a degenerate empty-failed partial must render no flash band; flashText = %q", rm.flashText)
	}
	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the un-attempted bravo must stay marked")
	}
}
