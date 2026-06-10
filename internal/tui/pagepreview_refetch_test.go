package tui

import (
	"errors"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// AUDIT — Sessions-list re-fetch on pagePreview → pageSessions transition.
//
// Read sweep of internal/tui/model.go (as of task 4-5) for any tea.Cmd that
// re-populates the Sessions list:
//
//   - Init()                 — line 695, fetchSessions cmd, fired once at TUI
//                              startup. Does not fire on page transitions.
//   - killAndRefresh()       — line 1289, fires after a kill modal y-confirm
//                              only.
//   - renameAndRefresh()     — line 1343, fires after a rename modal Enter
//                              only.
//   - SessionsMsg handler    — line 745, applies a list snapshot but does not
//                              itself trigger a fresh ListSessions call.
//
// No tea.Cmd in model.go re-fetches sessions on PageProjects → PageSessions,
// or pagePreview → PageSessions transitions.
// There is no periodic refresh, no on-page-entry refresh, no "loadSessionsCmd"
// or "refreshSessions" dispatcher. Therefore: GAP.
//
// Resolution shipped with this task: previewDismissedMsg handler now returns a
// tea.Cmd that re-invokes m.sessionLister.ListSessions() and emits a
// previewSessionsRefreshedMsg. The handler:
//   1. Captures the currently-highlighted session name BEFORE flipping to
//      PageSessions, so the fresh list can re-anchor the cursor by name.
//   2. Re-applies SetItems (preserving filter via bubbles/list semantics).
//   3. If the captured name still exists in VisibleItems, Select that index;
//      else clamps cursor to the new maxCursorIndex (no panic when the
//      previous item was killed mid-preview).
//
// The previewDismissedMsg handler still preserves cursor/filter state per
// task 2-4 — the refresh is layered on top, not in place of, the existing
// dismiss semantics.

// stepListerStub implements SessionLister and emits a different list per call,
// modelling externally-killed-session-during-preview: the pre-Space list is
// observable separately from the post-dismiss list.
type stepListerStub struct {
	steps [][]tmux.Session
	err   error
	calls int
}

func (s *stepListerStub) ListSessions() ([]tmux.Session, error) {
	idx := s.calls
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if idx >= len(s.steps) {
		// Saturate at last step so any extra calls are deterministic.
		return s.steps[len(s.steps)-1], nil
	}
	return s.steps[idx], nil
}

// modelWithSeamsAndLister is modelWithSeams plus a wired SessionLister so the
// dismiss-time refresh dispatch has something to call. The model is otherwise
// identical to modelWithSeams.
func modelWithSeamsAndLister(sessions []tmux.Session, enum TmuxEnumerator, reader ScrollbackReader, lister SessionLister) Model {
	m := modelWithSeams(sessions, enum, reader)
	m.sessionLister = lister
	return m
}

// pressSpaceThenEscWithRefresh mirrors pressSpaceThenEsc but also drains the
// refresh tea.Cmd batched out of previewDismissedMsg. It returns the final
// Model after the refresh message has round-tripped through Update.
func pressSpaceThenEscWithRefresh(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}
	updated2, escCmd := got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after Esc, got %T", updated2)
	}
	dismissMsg := drainCmd(t, escCmd)
	updated3, refreshCmd := got2.Update(dismissMsg)
	got3, ok := updated3.(Model)
	if !ok {
		t.Fatalf("expected Model after dismiss msg, got %T", updated3)
	}
	if refreshCmd == nil {
		// No refresh dispatch is permitted only when no sessionLister was
		// wired. Tests that exercise the refresh always wire one.
		return got3
	}
	refreshMsg := refreshCmd()
	updated4, refilterCmd := got3.Update(refreshMsg)
	got4, ok := updated4.(Model)
	if !ok {
		t.Fatalf("expected Model after refresh msg, got %T", updated4)
	}
	final, ok := drainCmdThroughUpdate(t, got4, refilterCmd).(Model)
	if !ok {
		t.Fatalf("expected Model after refilter drain, got %T", final)
	}
	return final
}

// drainCmdThroughUpdate performs a single-step round-trip: it invokes the
// given tea.Cmd and feeds the resulting message back through the model's
// Update, returning the post-Update model. When cmd is nil (or invocation
// produces a nil message) the model is returned unchanged. This is a
// domain-agnostic helper — any caller that needs to observe the state
// transition produced by a deferred tea.Cmd can use it.
//
// Typical use: when the bubbles/list SetItems call is made while the list
// is in list.FilterApplied state, it synchronously nils filteredItems and
// returns a filterItems tea.Cmd that emits FilterMatchesMsg; only after
// that message is fed back through Update does VisibleItems() return the
// refiltered slice. Sessions-list refresh paths (e.g.
// previewSessionsRefreshedMsg or kill-refresh SessionsMsg) rely on this
// helper to perform the round-trip so assertions against VisibleItems()
// observe the refiltered list rather than the transient empty state. Nil
// cmd covers the boot / Unfiltered-list path where SetItems returns nil
// per bubbles@v1.0.0/list.go:385-397.
func drainCmdThroughUpdate(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	updated, _ := m.Update(msg)
	return updated
}

func TestPreviewEscRefetchesSessionsList(t *testing.T) {
	// modelWithSeamsAndLister seeds the initial Sessions list directly via
	// modelWithSeams (no Init() runs), so the first ListSessions invocation
	// in the test IS the dismiss-refresh. lister.steps therefore returns
	// the POST-kill list on its first (and only expected) call: alpha was
	// externally killed while preview was open.
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)
	// Cursor on bravo (the survivor) so the previously-selected session
	// still exists post-refresh.
	m.sessionList.Select(1)

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Fatalf("expected PageSessions after dismiss, got %v", got.activePage)
	}
	if lister.calls != 1 {
		t.Errorf("expected exactly 1 ListSessions call from dismiss-refresh dispatch, got %d", lister.calls)
	}
	names := visibleSessionNames(got)
	if len(names) != 1 || names[0] != "bravo" {
		t.Errorf("expected post-dismiss list = [bravo], got %v", names)
	}
}

func TestExternallyKilledSessionNotInListAfterDismiss(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	for _, n := range visibleSessionNames(got) {
		if n == "alpha" {
			t.Errorf("expected externally-killed alpha NOT in list after dismiss, got %v", visibleSessionNames(got))
		}
	}
}

func TestPreviewEscPreservesCursorWhenPreviousSessionStillExists(t *testing.T) {
	// Cursor on bravo (index 1) before Space; alpha killed during preview;
	// after refresh, list = {bravo}, cursor must land on bravo (index 0).
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)
	m.sessionList.Select(1)

	got := pressSpaceThenEscWithRefresh(t, m)

	si, ok := got.selectedSessionItem()
	if !ok {
		t.Fatalf("expected a selected session post-refresh, got none")
	}
	if si.Session.Name != "bravo" {
		t.Errorf("expected cursor on bravo (the still-existing previously-selected session), got %q", si.Session.Name)
	}
}

func TestPreviewEscCursorFallsBackToNeighbourWhenPreviousSessionGone(t *testing.T) {
	// Cursor on alpha (index 0); alpha killed during preview; after refresh
	// list = {bravo}, alpha is gone, cursor must land on a valid neighbour
	// (bravo, the only remaining session) without panic.
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)
	m.sessionList.Select(0)

	got := pressSpaceThenEscWithRefresh(t, m)

	si, ok := got.selectedSessionItem()
	if !ok {
		t.Fatalf("expected a selected session (the neighbour) post-refresh, got none")
	}
	if si.Session.Name != "bravo" {
		t.Errorf("expected cursor to fall back to bravo (the only remaining session), got %q", si.Session.Name)
	}
}

func TestPreviewEscRefreshIsObservablyNoOpWhenListUnchanged(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	// Refresh returns the same shape — observably no-op.
	lister := &stepListerStub{steps: [][]tmux.Session{first}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)
	m.sessionList.Select(1)

	got := pressSpaceThenEscWithRefresh(t, m)

	names := visibleSessionNames(got)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Errorf("expected unchanged list [alpha bravo] after no-op refresh, got %v", names)
	}
	si, ok := got.selectedSessionItem()
	if !ok || si.Session.Name != "bravo" {
		t.Errorf("expected cursor still on bravo after no-op refresh, got %v (ok=%v)", si.Session.Name, ok)
	}
}

func TestPreviewEscFilterStatePreservedAcrossDismissWithRefresh(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "alphabet", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{first}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before Space")
	}
	// Position cursor on the second filtered row ("alphabet") so the
	// post-dismiss cursor-index assertion has a non-zero target to lock in.
	m.sessionList.Select(1)
	wantCursorIndex := m.sessionList.Index()

	got := pressSpaceThenEscWithRefresh(t, m)

	if !got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=true after dismiss-with-refresh, got false")
	}
	if val := got.sessionList.FilterValue(); val != "alpha" {
		t.Errorf("expected FilterValue=%q after dismiss-with-refresh, got %q", "alpha", val)
	}
	if got.sessionList.FilterState() != list.FilterApplied {
		t.Errorf("expected FilterState=FilterApplied after dismiss-with-refresh, got %v", got.sessionList.FilterState())
	}
	// Wrong-axis miss site: assert on filteredItems via VisibleItems(),
	// not just on filter metadata. Order-sensitive slice equality is
	// mandatory — length-only would let row-substitution regressions
	// pass silently.
	wantNames := []string{"alpha", "alphabet"}
	gotNames := visibleSessionNames(got)
	if len(gotNames) != len(wantNames) {
		t.Errorf("expected VisibleItems=%v after dismiss-with-refresh, got %v", wantNames, gotNames)
	} else {
		for i := range wantNames {
			if gotNames[i] != wantNames[i] {
				t.Errorf("expected VisibleItems=%v after dismiss-with-refresh, got %v (mismatch at idx %d)", wantNames, gotNames, i)
				break
			}
		}
	}
	// Cursor must still point at the previously-highlighted filtered row.
	if gotIndex := got.sessionList.Index(); gotIndex != wantCursorIndex {
		t.Errorf("expected sessionList.Index()=%d (previously-highlighted filtered row) after dismiss-with-refresh, got %d", wantCursorIndex, gotIndex)
	}
}

// TestDrainCmdThroughUpdateNilCmdReturnsModelUnchanged locks the
// boot/unfiltered contract: SetItems against an Unfiltered list returns nil
// per bubbles@v1.0.0/list.go:385-397, so the helper MUST treat nil as a
// no-op (no panic, no perturbation of the model) and return the input
// model untouched.
func TestDrainCmdThroughUpdateNilCmdReturnsModelUnchanged(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeamsAndLister(first, enum, reader, &stepListerStub{steps: [][]tmux.Session{first}})
	before := visibleSessionNames(m)

	out := drainCmdThroughUpdate(t, m, nil)
	got, ok := out.(Model)
	if !ok {
		t.Fatalf("expected Model from drainCmdThroughUpdate, got %T", out)
	}
	after := visibleSessionNames(got)
	if len(before) != len(after) {
		t.Fatalf("expected VisibleItems unchanged on nil cmd, before=%v after=%v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("expected VisibleItems unchanged on nil cmd at idx %d, before=%q after=%q", i, before[i], after[i])
		}
	}
}

// TestDrainCmdThroughUpdateInvokesCmdAndFeedsResultThroughUpdate locks the
// active drain path: when given a non-nil cmd, the helper MUST invoke it
// and feed the produced message back through Update, returning the
// post-Update model. We synthesize a tea.Cmd that emits a known message
// and verify the message reaches Update by triggering an observable
// state transition (a tea.KeyMsg with Esc on the preview page flips
// activePage back to PageSessions and emits a previewDismissedMsg via
// the returned cmd; here we use a simpler probe — a tea.WindowSizeMsg —
// which Update consumes and stores on the model).
func TestDrainCmdThroughUpdateInvokesCmdAndFeedsResultThroughUpdate(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeamsAndLister(first, enum, reader, &stepListerStub{steps: [][]tmux.Session{first}})

	// Synthesize a cmd that emits a WindowSizeMsg — Update consumes this
	// and writes to m.termWidth/m.termHeight, giving us an observable
	// signal that the message was fed back through Update.
	probeCmd := func() tea.Msg { return tea.WindowSizeMsg{Width: 137, Height: 41} }

	out := drainCmdThroughUpdate(t, m, probeCmd)
	got, ok := out.(Model)
	if !ok {
		t.Fatalf("expected Model from drainCmdThroughUpdate, got %T", out)
	}
	if got.termWidth != 137 || got.termHeight != 41 {
		t.Errorf("expected drainCmdThroughUpdate to feed the cmd's message back through Update (termWidth=137 termHeight=41), got termWidth=%d termHeight=%d", got.termWidth, got.termHeight)
	}
}

func TestPreviewEscRefreshSilentOnListerError(t *testing.T) {
	// On lister error, the refresh must not crash the TUI nor blow away
	// the existing list. Defensive guard: the list survives with its
	// pre-refresh contents.
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{err: errors.New("boom")}
	m := modelWithSeamsAndLister(first, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected PageSessions even after lister error, got %v", got.activePage)
	}
	names := visibleSessionNames(got)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Errorf("expected pre-refresh list preserved on lister error, got %v", names)
	}
}

// visibleSessionNames extracts the rendered session names from m.sessionList
// in their visible (filter-applied) order. Used to make assertions robust
// against bubbles/list internal storage details.
func visibleSessionNames(m Model) []string {
	items := m.sessionList.VisibleItems()
	names := make([]string, 0, len(items))
	for _, it := range items {
		si, ok := it.(SessionItem)
		if !ok {
			continue
		}
		names = append(names, si.Session.Name)
	}
	return names
}
