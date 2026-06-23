package tui

// spectrum-tui-design-5-8 — Part B carry-forward fix.
//
// On the §10.2 concurrent cold-boot route the orchestrator runs in a goroutine
// CONCURRENTLY with the live event loop. fetchSessions fires once from Init
// (frame one) — BEFORE Restore (bootstrap step 6) has created the saved
// skeleton sessions. That Init snapshot is therefore STALE/EMPTY: the restored
// sessions do not exist yet when Init enumerates. Without a re-fetch the picker
// renders the stale Init snapshot post-transition — the prior-incident
// "empty-previews / slow-open" surface (§10.2 prior-incident history).
//
// The fix: on the terminal BootstrapCompleteMsg (cold/TUI route only, i.e. a
// non-nil progressReceiver), dispatch a fresh ListSessions so the Sessions page
// that appears post-transition reflects post-restore tmux state, NOT the empty
// Init snapshot. The warm/synchronous route (nil progressReceiver) must NOT
// re-fetch: its Init snapshot is already post-restore (PersistentPreRunE ran the
// orchestrator synchronously before the model was built), so a re-fetch there
// would be wasted work and a behaviour change.
//
// These tests live in package tui (white-box) so they can read the model's
// internal sessionList state via the visibleSessionNames helper and set
// progressReceiver directly.
//
// No t.Parallel: consistent with the cmd-package convention and the rest of the
// tui test surface (which mutates list state through Update).

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// coldBootStepLister returns a DIFFERENT session snapshot per call, modelling
// the concurrent cold boot: call 1 (Init fetchSessions, fired at frame one
// before Restore) sees the EMPTY pre-restore server; call 2 (the post-complete
// re-fetch this task adds) sees the restored sessions. calls counts invocations
// so a test can assert the re-fetch happened exactly once.
type coldBootStepLister struct {
	steps [][]tmux.Session
	calls int
}

func (l *coldBootStepLister) ListSessions() ([]tmux.Session, error) {
	idx := l.calls
	l.calls++
	if idx >= len(l.steps) {
		return l.steps[len(l.steps)-1], nil
	}
	return l.steps[idx], nil
}

// driveColdBootToSessions runs the cold/TUI loading lifecycle on m to the point
// just past transitionFromLoading, draining the re-fetch the terminal
// BootstrapCompleteMsg dispatches. It returns the final Model on PageSessions.
//
// Ordering mirrors production on the cold/TUI route:
//  1. Init's fetchSessions delivers the STALE (pre-restore) SessionsMsg while on
//     PageLoading — ingested but does not transition.
//  2. LoadingMinElapsedMsg sets minElapsed.
//  3. The terminal BootstrapCompleteMsg arrives — bootstrapComplete + minElapsed
//     both true → transitionFromLoading fires AND the post-complete re-fetch
//     command is dispatched.
//  4. The re-fetch's SessionsMsg (post-restore snapshot) is fed back through
//     Update on PageSessions, re-rendering the list.
func driveColdBootToSessions(t *testing.T, m Model, staleSnapshot []tmux.Session) Model {
	t.Helper()

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Step 1 — stale Init snapshot ingested while on PageLoading.
	model, _ = model.Update(SessionsMsg{Sessions: staleSnapshot})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// Step 2 — min-display floor elapsed.
	model, _ = model.Update(LoadingMinElapsedMsg{})

	// Step 3 — terminal complete event. On the cold/TUI route this both
	// transitions off PageLoading AND dispatches the post-complete re-fetch.
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("expected PageSessions after min+complete, got %v", model.(Model).ActivePage())
	}
	if completeCmd == nil {
		t.Fatal("expected a post-complete re-fetch command from BootstrapCompleteMsg on the cold/TUI route, got nil")
	}

	// Step 4 — feed the re-fetch result (post-restore snapshot) back through
	// Update so the list re-renders. The re-fetch cmd is batched with the
	// warnings-surface cmd; drain the batch so the SessionsMsg lands.
	return drainBatchToModel(t, model.(Model), completeCmd)
}

// drainBatchToModel resolves a (possibly tea.Batch) cmd into its constituent
// messages and feeds each back through Update. A tea.Batch returns a
// tea.BatchMsg (a slice of child cmds), so a plain single-step drain would not
// execute the re-fetch hiding inside it. This helper unwraps one batch level,
// runs each child cmd, and applies any resulting message — sufficient for the
// (warnings, re-fetch) batch the transition arms emit. Any further commands a
// child's Update returns (e.g. a filterItems cmd from SetItems) are drained
// through Update too.
func drainBatchToModel(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		cur := m
		for _, child := range batch {
			next := drainBatchToModel(t, cur, child)
			cur = next
		}
		return cur
	}
	updated, follow := m.Update(msg)
	um, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after draining batched msg, got %T", updated)
	}
	if follow != nil {
		return drainBatchToModel(t, um, follow)
	}
	return um
}

// TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions is the core Part B
// assertion: the cold-boot picker reflects sessions that appeared DURING restore
// (the post-restore snapshot), NOT the empty Init snapshot.
func TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions(t *testing.T) {
	// The driver feeds the Init/stale snapshot manually via SessionsMsg (Init's
	// frame-one fetch is simulated, not driven through the lister), so the
	// lister's ONLY real invocation is the post-complete re-fetch — it must
	// return the POST-restore snapshot.
	stale := []tmux.Session{} // Init fires before Restore — empty pre-restore server.
	restored := []tmux.Session{
		{Name: "restored-alpha", Windows: 1},
		{Name: "restored-bravo", Windows: 2},
	}
	lister := &coldBootStepLister{steps: [][]tmux.Session{restored}}

	// Cold/TUI route: serverStarted=true (loading page) + a non-nil
	// progressReceiver (the channel-owns-the-terminal-event marker). The receiver
	// itself is never invoked in this test — we drive the messages directly — but
	// its presence is what gates the re-fetch (production scopes the re-fetch to
	// the concurrent route).
	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
	)

	final := driveColdBootToSessions(t, m, stale)

	got := visibleSessionNames(final)
	want := []string{"restored-alpha", "restored-bravo"}
	if len(got) != len(want) {
		t.Fatalf("cold-boot picker must reflect the POST-restore snapshot, not the empty Init snapshot\n  want %v\n  got  %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("post-restore session mismatch at idx %d: want %q got %q (full: %v)", i, want[i], got[i], got)
		}
	}

	// The re-fetch must have fired exactly once (the driver simulates the Init
	// fetch via a manual SessionsMsg, so the lister's only call is the re-fetch).
	if lister.calls != 1 {
		t.Errorf("expected exactly 1 ListSessions call (the post-complete re-fetch), got %d", lister.calls)
	}
}

// TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed covers the FAST
// cold-boot ordering: BootstrapCompleteMsg arrives BEFORE LoadingMinElapsedMsg
// (a very fast cold boot where restore finishes before the 1.2s pad). The
// re-fetch must fire when the LATER of the two gates closes — here the
// LoadingMinElapsedMsg — so the post-restore snapshot still wins.
func TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed(t *testing.T) {
	stale := []tmux.Session{}
	restored := []tmux.Session{{Name: "fast-restored", Windows: 1}}
	// Only the re-fetch drives the lister (the stale Init snapshot is fed
	// manually below), so the lister returns the post-restore snapshot.
	lister := &coldBootStepLister{steps: [][]tmux.Session{restored}}

	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: stale})

	// Complete arrives FIRST (fast boot) — must NOT transition yet (minElapsed
	// false) and must NOT re-fetch prematurely.
	model, earlyCmd := model.Update(BootstrapCompleteMsg{})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("expected to stay on PageLoading when complete arrives before minElapsed, got %v", model.(Model).ActivePage())
	}
	if earlyCmd != nil {
		t.Fatalf("expected no re-fetch while still on PageLoading (minElapsed false), got %T", earlyCmd)
	}

	// minElapsed closes the second gate → transition + re-fetch.
	model, lateCmd := model.Update(LoadingMinElapsedMsg{})
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("expected PageSessions after min closes the second gate, got %v", model.(Model).ActivePage())
	}
	if lateCmd == nil {
		t.Fatal("expected a post-complete re-fetch command when LoadingMinElapsedMsg closes the gate, got nil")
	}

	final := drainBatchToModel(t, model.(Model), lateCmd)
	got := visibleSessionNames(final)
	if len(got) != 1 || got[0] != "fast-restored" {
		t.Errorf("fast cold-boot picker must reflect post-restore snapshot, want [fast-restored] got %v", got)
	}
}

// TestWarmRoute_NoPostCompleteRefetch pins the warm/synchronous parity: with a
// NIL progressReceiver (the synchronous warm/CLI route) the model must NOT
// dispatch a post-complete re-fetch. On that route PersistentPreRunE ran the
// orchestrator synchronously before the model was built, so the Init snapshot is
// already post-restore; a re-fetch would be a wasted call and a behaviour change.
func TestWarmRoute_NoPostCompleteRefetch(t *testing.T) {
	// A single snapshot — the synchronous route's Init fetch already saw it.
	sessions := []tmux.Session{{Name: "warm-already-live", Windows: 1}}
	lister := &coldBootStepLister{steps: [][]tmux.Session{sessions}}

	// Warm/synchronous route: serverStarted=true forces the loading page (the
	// 1.2s pad still applies on a synchronous boot), but NO progressReceiver — so
	// Init synthesizes BootstrapCompleteMsg itself and the model must keep today's
	// behaviour (no re-fetch).
	m := New(lister, WithServerStarted(true))

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: sessions})
	model, _ = model.Update(LoadingMinElapsedMsg{})

	// Terminal complete on the synchronous route must transition WITHOUT a
	// re-fetch cmd. No warnings were staged, so surfaceBufferedWarnings yields
	// nil and refetchSessionsAfterRestore yields nil on the warm route ⇒ the
	// batched cmd collapses to nil.
	callsBefore := lister.calls
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("expected PageSessions after min+complete on warm route, got %v", model.(Model).ActivePage())
	}
	if completeCmd != nil {
		t.Errorf("warm/synchronous route must NOT dispatch a post-complete re-fetch (or any cmd with no warnings); got non-nil cmd %T", completeCmd)
		// Drain it to surface any ListSessions side effect in the count below.
		drainBatchToModel(t, model.(Model), completeCmd)
	}
	if lister.calls != callsBefore {
		t.Errorf("warm/synchronous route must NOT re-fetch sessions on complete; ListSessions calls bumped from %d to %d", callsBefore, lister.calls)
	}
}
