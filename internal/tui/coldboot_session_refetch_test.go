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

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
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

// TestColdBoot_NPositive_LandsOnSessions is the AC1 reproduction: on the cold
// concurrent route, when N>0 sessions were restored, the picker must land on
// PageSessions with all restored names visible — no x press required.
//
// The exact bug ordering is reproduced inline (the shared
// driveColdBootToSessions driver does NOT deliver ProjectsLoadedMsg, so reusing
// it would leave projectsLoaded false and pass the test for the wrong reason —
// the latch would never fire). ProjectsLoadedMsg is delivered while on
// PageLoading BEFORE the transition: without it the evaluateDefaultPage latch
// can never fire on the stale interim list, so a pre-fix run would pass
// vacuously.
//
// Pre-fix this test FAILS: transitionFromLoading unconditionally sets
// sessionsLoaded=true + evaluateDefaultPage() against the stale EMPTY interim
// list → lands on PageProjects and latches defaultPageEvaluated, so the
// post-restore refetch's SessionsMsg cannot re-decide. Post-fix it PASSES:
// the cold route defers the landing to the refetch.
func TestColdBoot_NPositive_LandsOnSessions(t *testing.T) {
	// The driver feeds the Init/stale snapshot manually via SessionsMsg, so the
	// lister's ONLY real invocation is the post-restore refetch — it returns the
	// restored N>0 snapshot.
	stale := []tmux.Session{} // Init fires before Restore — empty pre-restore server.
	restored := []tmux.Session{
		{Name: "restored-alpha", Windows: 1},
		{Name: "restored-bravo", Windows: 2},
	}
	lister := &coldBootStepLister{steps: [][]tmux.Session{restored}}

	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithProjectStore(stubProjectStore{}),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Stale Init snapshot ingested while on PageLoading.
	model, _ = model.Update(SessionsMsg{Sessions: stale})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// MANDATORY: deliver ProjectsLoadedMsg while on PageLoading BEFORE the
	// transition. This sets projectsLoaded=true so evaluateDefaultPage can latch
	// on the stale interim list pre-fix — omitting it makes the test pass for the
	// wrong reason.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: nil})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after ProjectsLoadedMsg, got %v", model.(Model).ActivePage())
	}

	// Close both gates: min-display floor elapsed, then terminal complete.
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})

	// Drain the resulting batch (carries the refetch's SessionsMsg) so the
	// landing decision is made against the repaired post-restore list.
	final := drainBatchToModel(t, model.(Model), completeCmd)

	if final.ActivePage() != PageSessions {
		t.Fatalf("AC1: cold boot with N>0 restored sessions must land on PageSessions (no x required), got %v", final.ActivePage())
	}

	got := visibleSessionNames(final)
	want := []string{"restored-alpha", "restored-bravo"}
	if len(got) != len(want) {
		t.Fatalf("expected all %d restored names visible\n  want %v\n  got  %v", len(want), want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("restored session mismatch at idx %d: want %q got %q (full: %v)", i, want[i], got[i], got)
		}
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

// TestColdBoot_ZeroSessions_LandsOnProjects is the AC2 over-correction guard: the
// 1-1 fix defers the landing decision to the post-restore refetch's SessionsMsg,
// but it must NOT over-correct — a genuine zero-session cold boot (the refetch
// itself returns an EMPTY snapshot) must still land on PageProjects. The deferral
// changes WHEN the len(Items())>0 test runs, never WHAT it tests: empty → Projects.
//
// This is distinct from the stale-empty Init snapshot. Here BOTH the Init snapshot
// AND the post-restore refetch are empty — restore reconstructed nothing — so the
// deferred decision runs against a genuinely empty list and correctly chooses
// Projects. A meaningful (>=1) project record is delivered so Projects is a real
// landing surface rather than an empty page.
func TestColdBoot_ZeroSessions_LandsOnProjects(t *testing.T) {
	// The refetch is the lister's ONLY real invocation (the stale Init snapshot is
	// fed manually below) — it returns an EMPTY slice, the genuine zero-session case.
	stale := []tmux.Session{} // Init fires before Restore — empty pre-restore server.
	lister := &coldBootStepLister{steps: [][]tmux.Session{{}}}

	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithProjectStore(stubProjectStore{}),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Stale empty Init snapshot ingested while on PageLoading.
	model, _ = model.Update(SessionsMsg{Sessions: stale})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// MANDATORY: deliver ProjectsLoadedMsg (>=1 project so Projects is a meaningful
	// landing) while on PageLoading BEFORE the transition. This sets
	// projectsLoaded=true so the deferred evaluateDefaultPage can run once the
	// refetch lands.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after ProjectsLoadedMsg, got %v", model.(Model).ActivePage())
	}

	// Close both gates, then drain the refetch (carries the empty post-restore
	// SessionsMsg) so the landing decision is made against the repaired list.
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	final := drainBatchToModel(t, model.(Model), completeCmd)

	if final.ActivePage() != PageProjects {
		t.Fatalf("AC2: cold boot whose post-restore refetch returns ZERO sessions must land on PageProjects, got %v", final.ActivePage())
	}

	if got := visibleSessionNames(final); len(got) != 0 {
		t.Errorf("AC2: zero-session cold boot must have an empty session list, got %v", got)
	}
}

// TestColdBoot_InitialFilter_RoutesToSessions is the AC3 filter-routing guard
// (the filter co-defect): a cold boot carrying an initialFilter must route that
// filter to the SESSION list — and consume it there — once the deferred decision
// resolves the page to Sessions. Both the len(Items())>0 page test and the
// initialFilter application live inside the single evaluateDefaultPage() call, so
// deferring that call to the post-restore SessionsMsg routes the filter against
// the repaired list, never against the stale interim list.
//
// evaluateDefaultPage applies initialFilter to the session list only when
// activePage == PageSessions && !commandPending, then zeroes m.initialFilter
// unconditionally — so on the cold route the filter lands on Sessions (the
// deferred decision resolves to Sessions on the N>0 refetch) and the project list
// is left untouched. The chosen names match the filter so the visible list is
// non-empty (filtered-count behaviour is out of scope; the page decision is on
// raw len(Items())).
func TestColdBoot_InitialFilter_RoutesToSessions(t *testing.T) {
	// The refetch is the lister's ONLY real invocation; it returns an N>0 snapshot
	// where >=1 name matches the "alpha" filter so the visible list is non-empty.
	stale := []tmux.Session{} // Init fires before Restore — empty pre-restore server.
	restored := []tmux.Session{
		{Name: "restored-alpha", Windows: 1},
		{Name: "restored-bravo", Windows: 2},
	}
	lister := &coldBootStepLister{steps: [][]tmux.Session{restored}}

	// WithInitialFilter is a post-construction Model method — chain it onto the
	// New(...) result before driving the lifecycle.
	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithProjectStore(stubProjectStore{}),
	).WithInitialFilter("alpha")

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Stale Init snapshot ingested while on PageLoading.
	model, _ = model.Update(SessionsMsg{Sessions: stale})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// MANDATORY: deliver ProjectsLoadedMsg while on PageLoading BEFORE the
	// transition so projectsLoaded=true and the deferred decision can run.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after ProjectsLoadedMsg, got %v", model.(Model).ActivePage())
	}

	// Close both gates, then drain the refetch (carries the N>0 post-restore
	// SessionsMsg) so the deferred decision routes the filter against the repaired
	// list.
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	final := drainBatchToModel(t, model.(Model), completeCmd)

	if final.ActivePage() != PageSessions {
		t.Fatalf("AC3: cold boot with initialFilter and N>0 must land on PageSessions, got %v", final.ActivePage())
	}

	// The filter routed to the SESSION list and was applied there.
	if got := final.SessionListFilterValue(); got != "alpha" {
		t.Errorf("AC3: session list filter value must equal the initial filter, want %q got %q", "alpha", got)
	}
	if got := final.SessionListFilterState(); got != list.FilterApplied {
		t.Errorf("AC3: session list filter state must be FilterApplied, got %v", got)
	}

	// The filter did NOT route to the PROJECT list.
	if got := final.ProjectListFilterValue(); got != "" {
		t.Errorf("AC3: project list filter value must be untouched (empty), got %q", got)
	}
	if got := final.ProjectListFilterState(); got == list.FilterApplied {
		t.Errorf("AC3: project list filter state must NOT be FilterApplied, got %v", got)
	}

	// The initialFilter was consumed in the decision.
	if got := final.InitialFilter(); got != "" {
		t.Errorf("AC3: initialFilter must be zeroed after the deferred decision consumes it, got %q", got)
	}
}

// cold-boot-restore-lands-on-projects-1-3 — Warm-route parity guard.
//
// The 1-1 fix (transitionFromLoading() gates the synchronous landing decision on
// m.progressReceiver != nil — see internal/tui/model.go transitionFromLoading at
// ~1843-1850 and refetchSessionsAfterRestore at ~1818-1823) must not perturb the
// warm / CLI / synchronous route. These are TEST-ONLY parity / regression
// assertions against the existing post-1-1 code — no production change. They lock
// the zero-new-risk contract (spec §Constraints "Warm / CLI / direct-path
// untouched"; the warm-path startup sequence has prior-incident history —
// slow-open / zombie-session — so it must stay byte-identical to today): the warm
// route (progressReceiver == nil) still decides the landing page synchronously at
// transitionFromLoading() against its already-post-restore Init snapshot,
// dispatches no post-complete refetch, lands on Sessions for N>0 (AC4) and on
// Projects for zero sessions (AC5). They also lock AC6 — a commandPending launch
// lands on Projects and never reaches the modified transition.
//
// No t.Parallel (cmd-package convention + the rest of the tui test surface).

// TestWarmRoute_RefetchSessionsAfterRestore_Nil is the direct white-box predicate
// assertion: refetchSessionsAfterRestore() returns nil on the warm route
// (progressReceiver == nil) and non-nil on the cold route (progressReceiver
// wired). progressReceiver != nil is the sole authoritative discriminator (spec
// §Constraints "Canonical cold-route predicate"); pairing the two halves locks the
// predicate symmetry — the warm route does NO extra enumeration, the cold route
// always re-enumerates. (Model has a value receiver, so the method is called
// directly on the value.)
func TestWarmRoute_RefetchSessionsAfterRestore_Nil(t *testing.T) {
	lister := &coldBootStepLister{steps: [][]tmux.Session{{}}}

	// Warm route: serverStarted=true forces the loading page, but NO
	// progressReceiver — proving the receiver, not serverStarted, gates the
	// deferral/refetch.
	warm := New(lister, WithServerStarted(true))
	if cmd := warm.refetchSessionsAfterRestore(); cmd != nil {
		t.Errorf("warm route (progressReceiver == nil) must return a nil refetch cmd, got non-nil")
	}

	// Cold route: a wired progressReceiver makes the predicate true → non-nil.
	cold := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
	)
	if cmd := cold.refetchSessionsAfterRestore(); cmd == nil {
		t.Errorf("cold route (progressReceiver != nil) must return a non-nil refetch cmd, got nil")
	}
}

// TestWarmRoute_ZeroSessions_LandsOnProjects is the AC5 guard: a warm/synchronous
// boot whose Init snapshot is empty (zero sessions) lands on PageProjects, exactly
// as today. On the warm route transitionFromLoading() sets sessionsLoaded=true and
// runs evaluateDefaultPage() synchronously against the already-post-restore (here
// empty) Init snapshot, so the len(Items())>0 test chooses Projects. The
// transition must ALSO dispatch no post-complete refetch — the lister call count
// must not bump across the transition (mirrors TestWarmRoute_NoPostCompleteRefetch
// at ~309-342).
func TestWarmRoute_ZeroSessions_LandsOnProjects(t *testing.T) {
	// Empty Init snapshot — the synchronous route's Init fetch already saw it
	// (zero sessions restored). No refetch should consume a second step.
	lister := &coldBootStepLister{steps: [][]tmux.Session{{}}}

	// Warm route: serverStarted=true forces the loading page, NO progressReceiver.
	m := New(lister,
		WithServerStarted(true),
		WithProjectStore(stubProjectStore{}),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Empty Init snapshot ingested while on PageLoading.
	model, _ = model.Update(SessionsMsg{Sessions: nil})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after empty SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// Deliver ProjectsLoadedMsg (>=1 project so Projects is a meaningful landing)
	// while on PageLoading so evaluateDefaultPage() can resolve at the transition.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after ProjectsLoadedMsg, got %v", model.(Model).ActivePage())
	}

	// Close both gates: min-display floor elapsed, then terminal complete. On the
	// warm route transitionFromLoading() makes the synchronous landing decision
	// here and must dispatch no refetch.
	model, _ = model.Update(LoadingMinElapsedMsg{})
	callsBefore := lister.calls
	model, completeCmd := model.Update(BootstrapCompleteMsg{})

	final := model.(Model)
	if final.ActivePage() != PageProjects {
		t.Fatalf("AC5: warm boot with zero sessions must land on PageProjects, got %v", final.ActivePage())
	}

	// The warm transition handler must dispatch no post-complete refetch. No
	// warnings were staged and refetchSessionsAfterRestore() returns nil on the
	// warm route, so the batched cmd collapses to nil; even if non-nil, draining it
	// must not bump the lister call count.
	if completeCmd != nil {
		drainBatchToModel(t, final, completeCmd)
	}
	if lister.calls != callsBefore {
		t.Errorf("warm route must NOT re-fetch sessions on complete; ListSessions calls bumped from %d to %d", callsBefore, lister.calls)
	}
}

// TestCommandPending_LandsOnProjects_NoInterimFlash is the AC6 guard: a
// commandPending launch lands on PageProjects regardless of session count and is
// never observed on the interim PageSessions the deferral introduces.
//
// Spec invariant (§Constraints "commandPending does not intersect the deferral"):
// Init's commandPending branch (model.go ~1900-1902) returns BEFORE wiring
// loadingPadTick / progressReceiver re-issue (model.go ~1909-1936), so
// transitionFromLoading() is never invoked for a commandPending launch and no
// interim Sessions flash occurs — even though WithProgressReceiver is wired here,
// the commandPending short-circuit takes precedence. WithCommand
// (model.go ~632-639) sets commandPending = true and activePage = PageProjects.
func TestCommandPending_LandsOnProjects_NoInterimFlash(t *testing.T) {
	// A non-empty snapshot would otherwise route a normal launch to Sessions —
	// commandPending must override that and land on Projects regardless.
	sessions := []tmux.Session{{Name: "live-session", Windows: 1}}
	lister := &coldBootStepLister{steps: [][]tmux.Session{sessions}}

	// WithProgressReceiver is wired to prove the commandPending short-circuit
	// (Init's early return) takes precedence over the cold-route deferral.
	m := New(lister,
		WithServerStarted(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
	).WithCommand([]string{"echo", "hi"})

	// WithCommand set activePage = PageProjects immediately.
	if m.ActivePage() != PageProjects {
		t.Fatalf("setup invariant: WithCommand must set activePage = PageProjects, got %v", m.ActivePage())
	}

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Deliver ProjectsLoadedMsg (>=1 project) so the commandPending arm of
	// evaluateDefaultPage() can resolve. No loading-page dismissal machinery is
	// involved — the commandPending launch never enters that path.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one"}}})

	final := model.(Model)
	if final.ActivePage() != PageProjects {
		t.Fatalf("AC6: commandPending launch must land on PageProjects regardless of session count, got %v", final.ActivePage())
	}
}
