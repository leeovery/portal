package tui

// restore-host-terminal-windows-6-6 — partial-failure leave-what-opened +
// selection mutation.
//
// White-box (package tui) tests of the NON-all-confirmed arm of the terminal
// spawnCompleteMsg handler: a pre-spawn Burster.Run error, and a post-flight burst
// where one or more external windows fail / time out / hit the permission wall.
// They assert Portal:
//   - leaves every opened host window in place (there is NO teardown seam —
//     spawn.Adapter exposes only OpenWindow, so the adapter call count cannot grow),
//   - skips the trigger self-attach (Selected()=="" and no tea.Quit — the picker
//     stays in multi-select mode),
//   - unmarks EXACTLY the confirmed sessions (so a second Enter retries the
//     still-marked missing set) while keeping failed / un-acked / un-attempted
//     sessions marked,
//   - surfaces ONE transient flash — the driver's permission Guidance verbatim once
//     if any window hit the permission wall, else a one-line failed-window message
//     (the ⚠ glyph is added by the warning notice band, matching the
//     formatSessionGoneFlash convention — the message text carries no glyph).
//
// Fast-path scenarios (spawn-failed, permission → AckFailed with no ack wait, both
// classified immediately) are driven end-to-end through the real burster; the
// ack-timeout and pre-spawn-error scenarios inject a crafted terminal
// spawnCompleteMsg into a directly-constructed pending-burst model, avoiding the
// real ~8 s per-window ack timeout and any background goroutine to race under -race.
//
// Shared seam helpers (wireBurstSeams, allPresent, resolveDetection, markRow,
// spawnedSession, ghosttyIdentity, driveBurstToTerminal) live in the sibling burst
// test files. No t.Parallel: consistent with the rest of the tui test surface.

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
)

// newPendingBurstModel builds a multi-select model with every `names` session
// marked, then places it directly in the burst-pending state a real dispatch would
// leave — external = names[:last], trigger = names[last], burstPending — WITHOUT
// launching the async goroutine. A §6-6 handler test then injects a crafted terminal
// spawnCompleteMsg to exercise the arm deterministically and fast (no real per-window
// ack timeout, no background goroutine).
func newPendingBurstModel(t *testing.T, names []string) Model {
	t.Helper()
	m := NewModelWithSessions(sessionsFromNames(names))
	m = enterMultiSelectEmpty(t, m)
	for i := range names {
		m = markRow(t, m, i)
	}
	if m.SelectedSessionCount() != len(names) {
		t.Fatalf("precondition: %d marked, got %d", len(names), m.SelectedSessionCount())
	}
	last := len(names) - 1
	m.burstPending = true
	m.burstTrigger = names[last]
	m.burstExternal = slices.Clone(names[:last])
	m.burstTotal = len(names)
	return m
}

// injectComplete applies a terminal spawnCompleteMsg through Update and returns the
// resulting model plus the follow cmd (nil on the §6-6 partial path; tea.Quit only on
// full success).
func injectComplete(t *testing.T, m Model, msg spawnCompleteMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

// TestBurstPartialFailure_LeavesOpenedWindowsAndSkipsSelfAttach drives an end-to-end
// spawn-failed burst (alpha fails fast, bravo confirms) and asserts the §6-6 arm:
// the opened windows are left in place (no teardown seam exists), the self-attach is
// skipped (Selected()=="", no tea.Quit), and the picker stays in multi-select mode.
func TestBurstPartialFailure_LeavesOpenedWindowsAndSkipsSelfAttach(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	// external = [alpha, bravo]; alpha spawn-fails (AckFailed, classified with no ack
	// wait so the burst finishes fast), bravo opens + confirms.
	adapter := &spawntest.FakeAdapter{Ack: ack, Results: []spawn.Result{spawn.SpawnFailed("boom")}}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	openedBefore := len(adapter.Calls)

	rm, follow := injectComplete(t, mBefore, complete)

	if rm.Selected() != "" {
		t.Errorf("partial failure must skip the self-attach; Selected() = %q, want empty", rm.Selected())
	}
	if follow != nil {
		t.Error("partial failure must NOT return a cmd (no tea.Quit — the picker stays open)")
	}
	if !rm.MultiSelectActive() {
		t.Error("partial failure must stay in multi-select mode")
	}
	if rm.BurstPending() {
		t.Error("partial failure must clear burst-pending")
	}
	// No teardown: spawn.Adapter exposes only OpenWindow (no close seam), so the
	// opened windows are left in place — the adapter call count cannot grow after the
	// completion handler runs.
	if len(adapter.Calls) != openedBefore {
		t.Errorf("no opened window may be torn down; adapter calls grew %d → %d", openedBefore, len(adapter.Calls))
	}
	// Confirmed bravo unmarked; failed alpha + the trigger charlie stay marked.
	if rm.IsSessionSelected("bravo") {
		t.Error("the confirmed session bravo must be unmarked")
	}
	if !rm.IsSessionSelected("alpha") {
		t.Error("the failed session alpha must stay marked (a retry re-opens it)")
	}
	if !rm.IsSessionSelected("charlie") {
		t.Error("the trigger charlie must stay marked (its self-attach did not happen)")
	}
	// Assert through the shared renderer: the picker's bare flash body IS
	// spawn.PartialFailureMessage (the CLI carries the same body under its "spawn:"
	// prefix), so the spec's "same one-line message" parity is structural. The ⚠ is
	// added by the warning band, not this body.
	if want := spawn.PartialFailureMessage([]string{"alpha"}); rm.flashText != want {
		t.Errorf("flashText = %q, want %q (names the failed window; ⚠ added by the warning band)", rm.flashText, want)
	}
	if rm.flashKind != flashWarning {
		t.Error("the failed-window flash must be the warning variant")
	}
}

// TestBurstPartialFailure_PreSpawnError_GenericFlashSelectionUnchanged injects a
// Burster.Run pre-spawn error (empty Results) and asserts the generic flash, an
// UNCHANGED selection (nothing opened → nothing to unmark), cleared burst-pending,
// and no degenerate empty-named "failed to open" message.
func TestBurstPartialFailure_PreSpawnError_GenericFlashSelectionUnchanged(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie"})
	before := m.SelectedSessionCount()

	rm, follow := injectComplete(t, m, spawnCompleteMsg{Err: errors.New("os.Executable: boom"), Results: nil})

	if rm.flashText != burstPreSpawnErrorFlash {
		t.Errorf("flashText = %q, want the generic %q (opaque err rides only to the DEBUG log)", rm.flashText, burstPreSpawnErrorFlash)
	}
	if rm.flashKind != flashWarning {
		t.Error("the pre-spawn error flash must be the warning variant")
	}
	if strings.Contains(rm.flashText, "failed to open") {
		t.Errorf("a pre-spawn error must NOT surface the degenerate empty-named failed-to-open copy: %q", rm.flashText)
	}
	// Nothing opened → nothing to unmark: the selection is UNCHANGED.
	if rm.SelectedSessionCount() != before {
		t.Errorf("pre-spawn error must leave the selection unchanged; count %d → %d", before, rm.SelectedSessionCount())
	}
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		if !rm.IsSessionSelected(n) {
			t.Errorf("pre-spawn error must keep %q marked (selection unchanged)", n)
		}
	}
	if follow != nil {
		t.Error("pre-spawn error must not return a cmd (no self-attach, no tea.Quit)")
	}
	if rm.Selected() != "" {
		t.Errorf("pre-spawn error must not self-attach; Selected() = %q", rm.Selected())
	}
	if rm.BurstPending() {
		t.Error("pre-spawn error must clear burst-pending")
	}
	if !rm.MultiSelectActive() {
		t.Error("pre-spawn error must stay in multi-select mode")
	}
}

// TestBurstPartialFailure_UnmarksConfirmedKeepsFailedForRetry pins the retry
// contract: after the mutation the still-marked set is EXACTLY the retry set (the
// failed window + the un-attached trigger), and the confirmed windows are unmarked.
func TestBurstPartialFailure_UnmarksConfirmedKeepsFailedForRetry(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie", "delta"})
	// external = [alpha, bravo, charlie]; trigger = delta. alpha + charlie confirm,
	// bravo times out.
	msg := spawnCompleteMsg{
		Batch: "batch-xyz",
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
			{Session: "charlie", Ack: spawn.AckConfirmed, Result: spawn.Success("")},
		},
	}

	rm, _ := injectComplete(t, m, msg)

	want := map[string]bool{"alpha": false, "bravo": true, "charlie": false, "delta": true}
	for name, wantMarked := range want {
		if got := rm.IsSessionSelected(name); got != wantMarked {
			t.Errorf("%q marked = %v, want %v (confirmed unmarked; failed/trigger stay marked for retry)", name, got, wantMarked)
		}
	}
}

// TestBurstPartialFailure_AckTimeoutAndSpawnFailedClassifyIdentically asserts an ack
// timeout and an adapter spawn-failed both classify as failed (stay marked; only the
// confirmed window is unmarked) and are both named in the one-line flash.
func TestBurstPartialFailure_AckTimeoutAndSpawnFailedClassifyIdentically(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo", "charlie", "delta"})
	// external = [alpha, bravo, charlie]; trigger = delta. alpha confirms, bravo times
	// out (AckTimeout), charlie's adapter reports spawn-failed (AckFailed).
	msg := spawnCompleteMsg{
		Batch: "batch-xyz",
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckConfirmed, Result: spawn.Success("")},
			{Session: "bravo", Ack: spawn.AckTimeout, Result: spawn.Success("")},
			{Session: "charlie", Ack: spawn.AckFailed, Result: spawn.SpawnFailed("boom")},
		},
	}

	rm, _ := injectComplete(t, m, msg)

	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the ack-timed-out bravo must classify as failed and stay marked")
	}
	if !rm.IsSessionSelected("charlie") {
		t.Error("the spawn-failed charlie must classify as failed and stay marked")
	}
	if want := spawn.PartialFailureMessage([]string{"bravo", "charlie"}); rm.flashText != want {
		t.Errorf("flashText = %q, want %q (both failed windows named)", rm.flashText, want)
	}
}

// TestBurstPartialFailure_PermissionGuidanceOnceAffectedStaysMarked drives an
// end-to-end permission burst and asserts the driver's Result.Guidance is surfaced
// verbatim, once — not the generic failed-window flash — and the affected session
// stays marked.
func TestBurstPartialFailure_PermissionGuidanceOnceAffectedStaysMarked(t *testing.T) {
	const guidance = "grant Automation access to Ghostty in System Settings"
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	// alpha opens + confirms; bravo hits the permission wall (the burster then stops).
	adapter := &spawntest.FakeAdapter{
		Ack:     ack,
		Results: []spawn.Result{spawn.Success(""), spawn.PermissionRequired("evt -1743", guidance)},
	}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)

	m, cmd := pressEnter(t, m)
	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}

	rm, follow := injectComplete(t, mBefore, complete)

	if rm.flashText != guidance {
		t.Errorf("flashText = %q, want the driver Guidance %q verbatim", rm.flashText, guidance)
	}
	if strings.Contains(rm.flashText, "failed to open") {
		t.Errorf("a permission result must surface the Guidance, not the generic failed-window flash: %q", rm.flashText)
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the permission-affected bravo must stay marked (it is failed, not confirmed)")
	}
	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked")
	}
	if follow != nil {
		t.Error("a permission partial failure must not self-attach / quit")
	}
	if !rm.MultiSelectActive() {
		t.Error("a permission partial failure must stay in multi-select mode")
	}
}

// TestBurstPartialFailure_UnattemptedPostPermissionStayMarked asserts the windows
// after a permission wall are never attempted (not in Results) and therefore stay
// marked — the burst stopped on the wall.
func TestBurstPartialFailure_UnattemptedPostPermissionStayMarked(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
		{Name: "delta", Windows: 4},
	}
	ack := &spawntest.FakeAckChannel{}
	// alpha confirms, bravo hits the permission wall → the burster stops; charlie is
	// never attempted.
	adapter := &spawntest.FakeAdapter{
		Ack:     ack,
		Results: []spawn.Result{spawn.Success(""), spawn.PermissionRequired("evt -1743", "grant Automation for Ghostty")},
	}
	m := NewModelWithSessions(sessions)
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	for i := range sessions {
		m = markRow(t, m, i)
	}

	m, cmd := pressEnter(t, m)
	if got := m.BurstExternal(); !slices.Equal(got, []string{"alpha", "bravo", "charlie"}) {
		t.Fatalf("precondition: external = %v, want [alpha bravo charlie]", got)
	}

	mBefore, term := driveBurstToTerminal(t, m, cmd)
	complete, ok := term.(spawnCompleteMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnCompleteMsg", term)
	}
	if len(complete.Results) != 2 {
		t.Fatalf("the burst must stop after the permission wall; Results = %d, want 2 (charlie never attempted)", len(complete.Results))
	}
	if len(adapter.Calls) != 2 {
		t.Fatalf("only alpha + bravo may be opened; adapter calls = %d, want 2 (charlie never composed)", len(adapter.Calls))
	}

	rm, _ := injectComplete(t, mBefore, complete)

	if rm.IsSessionSelected("alpha") {
		t.Error("the confirmed alpha must be unmarked")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the permission-affected bravo must stay marked")
	}
	if !rm.IsSessionSelected("charlie") {
		t.Error("the un-attempted charlie (after the permission stop) must stay marked")
	}
	if !rm.IsSessionSelected("delta") {
		t.Error("the trigger delta must stay marked (no self-attach)")
	}
}

// TestBurstPartialFailure_StaysInMultiSelectMode is the focused mode-preservation
// guard: a partial failure never exits multi-select mode and never self-attaches.
func TestBurstPartialFailure_StaysInMultiSelectMode(t *testing.T) {
	m := newPendingBurstModel(t, []string{"alpha", "bravo"})
	// external = [alpha], trigger = bravo. alpha times out → partial failure.
	msg := spawnCompleteMsg{
		Batch: "batch-xyz",
		Results: []spawn.WindowResult{
			{Session: "alpha", Ack: spawn.AckTimeout, Result: spawn.Success("")},
		},
	}

	rm, follow := injectComplete(t, m, msg)

	if !rm.MultiSelectActive() {
		t.Error("a partial failure must stay in multi-select mode (do not exit)")
	}
	if rm.Selected() != "" || follow != nil {
		t.Errorf("a partial failure must not self-attach; Selected()=%q follow=%v", rm.Selected(), follow)
	}
	if rm.BurstPending() {
		t.Error("a partial failure must clear burst-pending")
	}
	if !rm.IsSessionSelected("alpha") {
		t.Error("the failed alpha must stay marked")
	}
	if !rm.IsSessionSelected("bravo") {
		t.Error("the trigger bravo must stay marked")
	}
}
