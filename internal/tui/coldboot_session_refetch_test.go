package tui

import (
	"errors"
	"fmt"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
)

// idleBootstrap is a bootstrap behind the picker that sends nothing, for a
// test that drives the loading page's messages itself.
func idleBootstrap() tea.Msg { return nil }

type coldBootStepLister struct {
	steps [][]Session
	calls int
}

func (l *coldBootStepLister) ListSessions() ([]Session, error) {
	idx := l.calls
	l.calls++
	if idx >= len(l.steps) {
		return l.steps[len(l.steps)-1], nil
	}
	return l.steps[idx], nil
}

func driveColdBootToSessions(t *testing.T, m Model, staleSnapshot []Session) Model {
	t.Helper()

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(SessionsMsg{Sessions: staleSnapshot})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	model, _ = model.Update(LoadingMinElapsedMsg{})

	model, completeCmd := model.Update(BootstrapCompleteMsg{})
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("expected PageSessions after min+complete, got %v", model.(Model).ActivePage())
	}
	if completeCmd == nil {
		t.Fatal("expected a post-complete re-fetch command from BootstrapCompleteMsg on the cold/TUI route, got nil")
	}

	return drainBatchToModel(t, model.(Model), completeCmd)
}

func oneProjectLoaded() []project.Project {
	return []project.Project{{Path: "/p/one", Name: "one"}}
}

func twoRestoredSessions() []Session {
	return []Session{
		{Name: "restored-alpha"},
		{Name: "restored-bravo"},
	}
}

func twoRestoredSessionNames() []string {
	sessions := twoRestoredSessions()
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names
}

func assertVisibleSessionNames(t *testing.T, m Model, want []string, context string) {
	t.Helper()
	got := visibleSessionNames(m)
	if len(got) != len(want) {
		t.Fatalf("%s\n  want %v\n  got  %v", context, want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("restored session mismatch at idx %d: want %q got %q (full: %v)", i, want[i], got[i], got)
		}
	}
}

func driveColdBootToTransition(t *testing.T, m Model, staleSnapshot []Session) (Model, tea.Cmd) {
	t.Helper()

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(SessionsMsg{Sessions: staleSnapshot})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	// This must land before the transition: with projectsLoaded false the
	// landing latch never fires and the test passes vacuously.
	model, _ = model.Update(ProjectsLoadedMsg{Projects: oneProjectLoaded()})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after ProjectsLoadedMsg, got %v", model.(Model).ActivePage())
	}

	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, completeCmd := model.Update(BootstrapCompleteMsg{})

	return model.(Model), completeCmd
}

func driveColdBootWithProjects(t *testing.T, m Model, staleSnapshot []Session) Model {
	t.Helper()
	interim, completeCmd := driveColdBootToTransition(t, m, staleSnapshot)
	return drainBatchToModel(t, interim, completeCmd)
}

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

func TestColdBoot_PostCompleteRefetch_ReflectsRestoredSessions(t *testing.T) {
	stale := []Session{}
	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
	)

	final := driveColdBootToSessions(t, m, stale)

	got := visibleSessionNames(final)
	want := twoRestoredSessionNames()
	if len(got) != len(want) {
		t.Fatalf("cold-boot picker must reflect the POST-restore snapshot, not the empty Init snapshot\n  want %v\n  got  %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("post-restore session mismatch at idx %d: want %q got %q (full: %v)", i, want[i], got[i], got)
		}
	}

	if lister.calls != 1 {
		t.Errorf("expected exactly 1 ListSessions call (the post-complete re-fetch), got %d", lister.calls)
	}
}

func TestColdBoot_PostCompleteRefetch_CompleteBeforeMinElapsed(t *testing.T) {
	stale := []Session{}
	restored := []Session{{Name: "fast-restored"}}
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: stale})

	model, earlyCmd := model.Update(BootstrapCompleteMsg{})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("expected to stay on PageLoading when complete arrives before minElapsed, got %v", model.(Model).ActivePage())
	}
	if earlyCmd != nil {
		t.Fatalf("expected no re-fetch while still on PageLoading (minElapsed false), got %T", earlyCmd)
	}

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

func TestColdBoot_NPositive_LandsOnSessions(t *testing.T) {
	stale := []Session{}
	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	)

	final := driveColdBootWithProjects(t, m, stale)

	if final.ActivePage() != PageSessions {
		t.Fatalf("AC1: cold boot with N>0 restored sessions must land on PageSessions (no x required), got %v", final.ActivePage())
	}

	want := twoRestoredSessionNames()
	assertVisibleSessionNames(t, final, want,
		fmt.Sprintf("expected all %d restored names visible", len(want)))
}

func TestColdBoot_ZeroSessions_LandsOnProjects(t *testing.T) {
	stale := []Session{}
	lister := &coldBootStepLister{steps: [][]Session{{}}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	)

	final := driveColdBootWithProjects(t, m, stale)

	if final.ActivePage() != PageProjects {
		t.Fatalf("AC2: cold boot whose post-restore refetch returns ZERO sessions must land on PageProjects, got %v", final.ActivePage())
	}

	if got := visibleSessionNames(final); len(got) != 0 {
		t.Errorf("AC2: zero-session cold boot must have an empty session list, got %v", got)
	}
}

func TestColdBoot_InitialFilter_RoutesToSessions(t *testing.T) {
	stale := []Session{}
	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	).WithInitialFilter("alpha")

	final := driveColdBootWithProjects(t, m, stale)

	if final.ActivePage() != PageSessions {
		t.Fatalf("AC3: cold boot with initialFilter and N>0 must land on PageSessions, got %v", final.ActivePage())
	}

	if got := final.SessionListFilterValue(); got != "alpha" {
		t.Errorf("AC3: session list filter value must equal the initial filter, want %q got %q", "alpha", got)
	}
	if got := final.SessionListFilterState(); got != list.FilterApplied {
		t.Errorf("AC3: session list filter state must be FilterApplied, got %v", got)
	}

	if got := final.ProjectListFilterValue(); got != "" {
		t.Errorf("AC3: project list filter value must be untouched (empty), got %q", got)
	}
	if got := final.ProjectListFilterState(); got == list.FilterApplied {
		t.Errorf("AC3: project list filter state must NOT be FilterApplied, got %v", got)
	}

	if got := final.InitialFilter(); got != "" {
		t.Errorf("AC3: initialFilter must be zeroed after the deferred decision consumes it, got %q", got)
	}
}

func TestCommandPending_LandsOnProjects_NoInterimFlash(t *testing.T) {
	sessions := []Session{{Name: "live-session"}}
	lister := &coldBootStepLister{steps: [][]Session{sessions}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
	).WithCommand([]string{"echo", "hi"})

	if m.ActivePage() != PageProjects {
		t.Fatalf("setup invariant: WithCommand must set activePage = PageProjects, got %v", m.ActivePage())
	}

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(ProjectsLoadedMsg{Projects: oneProjectLoaded()})

	final := model.(Model)
	if final.ActivePage() != PageProjects {
		t.Fatalf("AC6: commandPending launch must land on PageProjects regardless of session count, got %v", final.ActivePage())
	}
}

func TestColdBoot_InterimPage_IsValidSessions(t *testing.T) {
	stale := []Session{}
	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	)

	interim, completeCmd := driveColdBootToTransition(t, m, stale)

	if interim.ActivePage() != PageSessions {
		t.Fatalf("AC7: interim page (after transition, before refetch SessionsMsg) must be PageSessions (a valid picker page, never PageLoading/undefined), got %v", interim.ActivePage())
	}

	if got := visibleSessionNames(interim); len(got) != 0 {
		t.Errorf("AC7: interim Sessions list must be the accepted briefly-empty state before the refetch lands (not special-cased), got %v", got)
	}

	final := drainBatchToModel(t, interim, completeCmd)

	if final.ActivePage() != PageSessions {
		t.Fatalf("AC1: final landing after the refetch must be PageSessions, got %v", final.ActivePage())
	}

	want := twoRestoredSessionNames()
	assertVisibleSessionNames(t, final, want,
		fmt.Sprintf("expected all %d restored names visible", len(want)))
}

// SessionsMsg is injected directly rather than through drainBatchToModel: the
// drain would resolve the refetch before the late ProjectsLoadedMsg lands.
func TestColdBoot_LateProjectsLoadedMsg_StillLandsOnSessions(t *testing.T) {
	stale := []Session{}
	restored := twoRestoredSessions()
	lister := &coldBootStepLister{steps: [][]Session{restored}}

	m := New(lister,
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model, _ = model.Update(SessionsMsg{Sessions: stale})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatalf("setup invariant: expected PageLoading after stale SessionsMsg, got %v", model.(Model).ActivePage())
	}

	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{})

	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("ordering: interim page after transition must be PageSessions (projectsLoaded false), got %v", model.(Model).ActivePage())
	}

	model, _ = model.Update(ProjectsLoadedMsg{Projects: oneProjectLoaded()})
	if model.(Model).ActivePage() != PageSessions {
		t.Fatalf("ordering: a late ProjectsLoadedMsg in the interim window must NOT latch Projects against the stale list (evaluateDefaultPage early-returns on !sessionsLoaded), page must stay PageSessions, got %v", model.(Model).ActivePage())
	}

	model, _ = model.Update(SessionsMsg{Sessions: restored})
	final := model.(Model)

	if final.ActivePage() != PageSessions {
		t.Fatalf("ordering: final page after the decision-bearing SessionsMsg must be PageSessions with N>0 restored sessions, got %v", final.ActivePage())
	}

	want := twoRestoredSessionNames()
	assertVisibleSessionNames(t, final, want,
		fmt.Sprintf("expected all %d restored names visible after the late-ProjectsLoadedMsg interleave", len(want)))
}

// coldBootInterim is a cold-boot picker past its loading page, showing stale
// rows ahead of the post-restore refetch, with the landing page still owed.
func coldBootInterim(t *testing.T, stale []Session) Model {
	t.Helper()
	m := New(&coldBootStepLister{steps: [][]Session{stale}},
		WithBootstrapping(idleBootstrap),
		WithProjectStore(stubProjectStore{}),
	)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: stale})
	model, _ = model.Update(ProjectsLoadedMsg{Projects: oneProjectLoaded()})
	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{})

	interim := model.(Model)
	if interim.ActivePage() != PageSessions || interim.sessionsLoaded {
		t.Fatalf("setup invariant: interim page = %v, sessionsLoaded = %v; want PageSessions with the landing owed", interim.ActivePage(), interim.sessionsLoaded)
	}
	return interim
}

// The interim list was shown, so a refetch that fails keeps it, flashes, and
// settles the landing exactly as a refetch answering with the same rows would.
func TestColdBoot_RefetchError_FlashesAndSettlesOnTheInterim(t *testing.T) {
	stale := []Session{}

	answered, _ := coldBootInterim(t, stale).Update(SessionsMsg{Sessions: stale, afterRestore: true})
	updated, cmd := coldBootInterim(t, stale).Update(SessionsMsg{Err: errors.New("zmx: listing timed out"), afterRestore: true})
	got, want := updated.(Model), answered.(Model)

	if quits(cmd) {
		t.Error("a failing refetch quit the picker after the interim list was shown")
	}
	if !got.sessionsLoaded || !got.defaultPageEvaluated {
		t.Errorf("sessionsLoaded = %v, defaultPageEvaluated = %v; want the landing settled", got.sessionsLoaded, got.defaultPageEvaluated)
	}
	if got.ActivePage() != want.ActivePage() || got.ActivePage() != PageProjects {
		t.Errorf("page = %v, want %v as an answered refetch lands: an empty list lands on Projects", got.ActivePage(), want.ActivePage())
	}
	if got.flashText != "zmx: listing timed out" {
		t.Errorf("flash = %q, want the listing's own words", got.flashText)
	}
	if got.ListError() != nil {
		t.Errorf("ListError() = %v, want nil once a listing answered", got.ListError())
	}
}
