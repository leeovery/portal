package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// multi_select_sticky_test.go pins task 5.6: the §5 multi-select set is keyed on
// Session.Name (model state independent of the list items), so marks are STICKY
// across filtering, paging, regrouping, and the Space-preview round-trip. The one
// active mutation is the externally-killed prune on the sessions-refresh
// chokepoint (applySessions): a session absent from the refreshed list is dropped
// from the set while every survivor is kept.
//
// No t.Parallel() — the package injects mocks via package-level mutable state.

// pageDownKey / pageUpKey are the §12.2 arrow-only paging keys (Ctrl+↓ / Ctrl+↑,
// bound by pinArrowOnlyNav to NextPage / PrevPage).
var (
	pageDownKey = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
	pageUpKey   = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
)

// markTwoFlatSessions enters multi-select mode on a flat model and marks the
// sessions at the two given row indices, asserting the resulting count. It
// leaves the cursor on the second marked row.
func markTwoFlatSessions(t *testing.T, m Model, idxA, idxB int) Model {
	t.Helper()
	m = enterMultiSelect(t, m)
	m.sessionList.Select(idxA)
	m = pressSession(t, m, pressM)
	m.sessionList.Select(idxB)
	m = pressSession(t, m, pressM)
	if got := m.SelectedSessionCount(); got != 2 {
		t.Fatalf("precondition: expected 2 marked sessions, got %d", got)
	}
	return m
}

// TestMultiSelectMarksSurviveRegroup covers stickiness across an s-regroup: after
// cycling the grouping mode the same sessions stay selected and the banner count
// (len(selectedSessions)) is unchanged.
func TestMultiSelectMarksSurviveRegroup(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	})
	m = markTwoFlatSessions(t, m, 0, 2) // mark alpha + charlie
	beforeMode := m.sessionListMode
	beforeCount := m.SelectedSessionCount()

	m = pressSession(t, m, keyS) // regroup Flat → By Project

	if m.sessionListMode == beforeMode {
		t.Fatalf("precondition: s must cycle the grouping mode; mode unchanged at %v", beforeMode)
	}
	if !m.MultiSelectActive() {
		t.Errorf("regroup must not exit multi-select mode")
	}
	if !m.IsSessionSelected("alpha") || !m.IsSessionSelected("charlie") {
		t.Errorf("marks must survive an s-regroup; alpha=%v charlie=%v",
			m.IsSessionSelected("alpha"), m.IsSessionSelected("charlie"))
	}
	if got := m.SelectedSessionCount(); got != beforeCount {
		t.Errorf("banner count changed across regroup: got %d, want %d", got, beforeCount)
	}
}

// TestMultiSelectMarksSurvivePaging covers stickiness across paging: navigating
// across pages (Ctrl+↓ / Ctrl+↑) does not clear the set.
func TestMultiSelectMarksSurvivePaging(t *testing.T) {
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: fmt.Sprintf("sess-%02d", i)})
	}
	m := NewModelWithSessions(sessions)
	if m.sessionList.Paginator.TotalPages < 2 {
		t.Fatalf("test setup: want >1 page, got %d", m.sessionList.Paginator.TotalPages)
	}
	m = markTwoFlatSessions(t, m, 0, 1) // mark sess-00 + sess-01 on page 0
	if m.sessionList.Paginator.Page != 0 {
		t.Fatalf("precondition: expected to start on page 0, got %d", m.sessionList.Paginator.Page)
	}

	m = pressSession(t, m, pageDownKey)
	if m.sessionList.Paginator.Page == 0 {
		t.Fatalf("precondition: Ctrl+↓ must advance the page")
	}
	if !m.IsSessionSelected("sess-00") || !m.IsSessionSelected("sess-01") || m.SelectedSessionCount() != 2 {
		t.Errorf("paging forward cleared the set: sess-00=%v sess-01=%v count=%d",
			m.IsSessionSelected("sess-00"), m.IsSessionSelected("sess-01"), m.SelectedSessionCount())
	}

	m = pressSession(t, m, pageUpKey)
	if !m.IsSessionSelected("sess-00") || !m.IsSessionSelected("sess-01") || m.SelectedSessionCount() != 2 {
		t.Errorf("paging back cleared the set: sess-00=%v sess-01=%v count=%d",
			m.IsSessionSelected("sess-00"), m.IsSessionSelected("sess-01"), m.SelectedSessionCount())
	}
	if !m.MultiSelectActive() {
		t.Errorf("paging must not exit multi-select mode")
	}
}

// TestMultiSelectFilteredOutSessionStaysMarked covers the filter round-trip: a row
// filtered out by an active query stays in the set and its ● reappears when the
// filter clears.
func TestMultiSelectFilteredOutSessionStaysMarked(t *testing.T) {
	// Build the full production model (mirroring filteringTestModel) so the live
	// filter input narrows VisibleItems via the standard typeKeys drain: a raw
	// struct-literal model batches a cursor-blink cmd with the FilterMatchesMsg,
	// which the shared drain does not unwrap, so it would never narrow.
	m := Build(Deps{Lister: fakeLister{}, Appearance: prefs.AppearanceDark})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})
	// Enter mode and mark bravo (index 1).
	m = enterMultiSelect(t, m)
	m.sessionList.Select(1)
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("bravo") {
		t.Fatalf("precondition: bravo must be marked")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Fatalf("precondition: marked bravo must render a ● before filtering")
	}

	// Filter to "alpha" — bravo is filtered out of the visible set.
	m = pressSlash(t, m)
	m = typeKeys(t, m, "alpha")
	if m.sessionList.FilterState() != list.Filtering {
		t.Fatalf("precondition: filter state = %v, want Filtering", m.sessionList.FilterState())
	}
	if names := visibleSessionNames(m); len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("precondition: filter must narrow to [alpha], got %v", names)
	}
	// The set is untouched by filtering even though bravo's row is hidden.
	if !m.IsSessionSelected("bravo") {
		t.Errorf("a filtered-out row must stay in the selection set; bravo dropped")
	}
	// bravo is hidden, so no ● renders while the query excludes it.
	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("no ● should render while the only marked row is filtered out: %q", ansi.Strip(m.sessionList.View()))
	}

	// Clear the filter (focused-filter Esc) — bravo reappears with its ●.
	m = pressSession(t, m, keyEsc)
	if m.sessionList.FilterState() != list.Unfiltered {
		t.Fatalf("precondition: Esc must clear the filter; state = %v", m.sessionList.FilterState())
	}
	if !m.MultiSelectActive() {
		t.Errorf("clearing the filter must not exit multi-select mode")
	}
	if !m.IsSessionSelected("bravo") {
		t.Errorf("bravo must still be marked after the filter clears")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("bravo's ● must reappear when the filter clears: %q", ansi.Strip(m.sessionList.View()))
	}
}

// TestMultiSelectPreviewRoundTripKeepsSelection covers the Space-preview
// round-trip with no external kill: dismissing the preview returns to the
// Sessions page still in multi-select mode with the whole selection intact.
func TestMultiSelectPreviewRoundTripKeepsSelection(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	// The refresh returns the unchanged list — nothing killed during preview.
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	m = markTwoFlatSessions(t, m, 0, 2) // mark alpha + charlie

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Errorf("preview round-trip must return to the Sessions page; active page = %d", got.activePage)
	}
	if !got.MultiSelectActive() {
		t.Errorf("preview round-trip must return in multi-select mode")
	}
	if !got.IsSessionSelected("alpha") || !got.IsSessionSelected("charlie") {
		t.Errorf("selection must survive the preview round-trip; alpha=%v charlie=%v",
			got.IsSessionSelected("alpha"), got.IsSessionSelected("charlie"))
	}
	if c := got.SelectedSessionCount(); c != 2 {
		t.Errorf("selection count after preview round-trip = %d, want 2", c)
	}
	if !strings.Contains(ansi.Strip(got.sessionList.View()), multiSelectMarker) {
		t.Errorf("markers must be re-rendered after the preview round-trip: %q", ansi.Strip(got.sessionList.View()))
	}
}

// TestMultiSelectPrunesExternallyKilledSession covers the one genuinely-new
// behaviour: a session marked and then externally killed during the Space
// preview is pruned from the selection on the post-dismiss refresh, while every
// surviving marked session stays selected.
func TestMultiSelectPrunesExternallyKilledSession(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	// alpha is externally killed while preview is open — the refresh returns
	// the survivors only.
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(first, enum, reader, lister)

	// Mark alpha (index 0, killed) and charlie (index 2, survivor).
	m = markTwoFlatSessions(t, m, 0, 2)
	if !m.IsSessionSelected("alpha") || !m.IsSessionSelected("charlie") {
		t.Fatalf("precondition: expected alpha and charlie marked")
	}

	got := pressSpaceThenEscWithRefresh(t, m)

	if !got.MultiSelectActive() {
		t.Errorf("the round-trip refresh must stay in multi-select mode")
	}
	if got.IsSessionSelected("alpha") {
		t.Errorf("the externally-killed alpha must be pruned from the selection")
	}
	if !got.IsSessionSelected("charlie") {
		t.Errorf("the surviving charlie must stay selected after the prune")
	}
	if c := got.SelectedSessionCount(); c != 1 {
		t.Errorf("selection count after prune = %d, want 1 (killed dropped, survivor kept)", c)
	}
	// The pruned session's row is gone from the list and its ● with it; the
	// survivor still renders a ●.
	if !strings.Contains(ansi.Strip(got.sessionList.View()), multiSelectMarker) {
		t.Errorf("survivor's ● must remain after the prune: %q", ansi.Strip(got.sessionList.View()))
	}
}

// TestMultiSelectMarkedSessionSurvivesBucketMove covers the bucket-independence
// edge: a marked session that moves to a different heading on an s-regroup
// (By-Project → By-Tag) stays marked (the set is keyed on Session.Name, not on
// the render bucket).
func TestMultiSelectMarkedSessionSurvivesBucketMove(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newSwitchViewTestModel(prefs.ModeByProject, nil, sessions, projects)

	// Enter mode, land the cursor on the (single) session row, and mark it.
	m = enterMultiSelect(t, m)
	rows := sessionRowIndices(m.sessionList.Items())
	if len(rows) != 1 {
		t.Fatalf("precondition: expected 1 session row, got %d", len(rows))
	}
	m.sessionList.Select(rows[0])
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("portal-abc") {
		t.Fatalf("precondition: portal-abc must be marked")
	}
	// By Project → the session sits under the project heading "Portal".
	if h := sessionRows(m.sessionList.Items())[0].GroupHeading; h != "Portal" {
		t.Fatalf("precondition: By Project heading = %q, want Portal", h)
	}

	// Regroup By Project → By Tag: the session moves into the "work" bucket.
	m = pressSession(t, m, keyS)
	if m.sessionListMode != prefs.ModeByTag {
		t.Fatalf("precondition: expected ModeByTag after regroup, got %v", m.sessionListMode)
	}
	moved := sessionRows(m.sessionList.Items())
	if len(moved) != 1 || moved[0].GroupHeading != "work" {
		t.Fatalf("precondition: session must move to the work bucket; rows = %+v", moved)
	}

	// The mark survives the bucket move (keyed on name, bucket-independent).
	if !m.IsSessionSelected("portal-abc") {
		t.Errorf("a marked session must stay marked when it moves buckets on regroup")
	}
	if !m.MultiSelectActive() {
		t.Errorf("the bucket move must not exit multi-select mode")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("the ● must render on the session in its new bucket: %q", ansi.Strip(m.sessionList.View()))
	}
}
