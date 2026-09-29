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

var (
	pageDownKey = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
	pageUpKey   = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
)

func markTwoFlatSessions(t *testing.T, m Model, idxA, idxB int) Model {
	t.Helper()
	m.sessionList.Select(idxA)
	m = enterMultiSelect(t, m)
	m.sessionList.Select(idxB)
	m = pressSession(t, m, pressM)
	if got := m.SelectedSessionCount(); got != 2 {
		t.Fatalf("precondition: expected 2 marked sessions, got %d", got)
	}
	return m
}

func TestMultiSelectMarksSurviveRegroup(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
		{Name: "charlie", Windows: 3},
	})
	m = markTwoFlatSessions(t, m, 0, 2)
	beforeMode := m.sessionListMode
	beforeCount := m.SelectedSessionCount()

	m = pressSession(t, m, keyS)

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

func TestMultiSelectMarksSurvivePaging(t *testing.T) {
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: fmt.Sprintf("sess-%02d", i)})
	}
	m := NewModelWithSessions(sessions)
	if m.sessionList.Paginator.TotalPages < 2 {
		t.Fatalf("test setup: want >1 page, got %d", m.sessionList.Paginator.TotalPages)
	}
	m = markTwoFlatSessions(t, m, 0, 1)
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

// Build, not a raw struct literal: the latter batches a cursor-blink cmd with
// the FilterMatchesMsg, which the shared drain does not unwrap.
func TestMultiSelectFilteredOutSessionStaysMarked(t *testing.T) {
	m := Build(Deps{Lister: fakeLister{}})
	m.termWidth = filteringReskinWidth
	m.termHeight = filteringReskinHeight
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	}, nil)
	m.sessionList.Select(1)
	m = enterMultiSelect(t, m)
	if !m.IsSessionSelected("bravo") {
		t.Fatalf("precondition: bravo must be marked")
	}
	if m.IsSessionSelected("alpha") {
		t.Fatalf("precondition: only bravo must be marked (alpha must stay unmarked)")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Fatalf("precondition: marked bravo must render a ● before filtering")
	}

	m = pressSlash(t, m)
	m = typeKeys(t, m, "alpha")
	if m.sessionList.FilterState() != list.Filtering {
		t.Fatalf("precondition: filter state = %v, want Filtering", m.sessionList.FilterState())
	}
	if names := visibleSessionNames(m); len(names) != 1 || names[0] != "alpha" {
		t.Fatalf("precondition: filter must narrow to [alpha], got %v", names)
	}
	if !m.IsSessionSelected("bravo") {
		t.Errorf("a filtered-out row must stay in the selection set; bravo dropped")
	}
	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("no ● should render while the only marked row is filtered out: %q", ansi.Strip(m.sessionList.View()))
	}

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

func TestMultiSelectPreviewRoundTripKeepsSelection(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	m = markTwoFlatSessions(t, m, 0, 2)

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

func TestMultiSelectPrunesExternallyKilledSession(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)

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
	if !strings.Contains(ansi.Strip(got.sessionList.View()), multiSelectMarker) {
		t.Errorf("survivor's ● must remain after the prune: %q", ansi.Strip(got.sessionList.View()))
	}
}

func TestMultiSelectMarkedSessionSurvivesBucketMove(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newSwitchViewTestModel(t, prefs.ModeByProject, nil, sessions, projects)

	rows := sessionRowIndices(m.sessionList.Items())
	if len(rows) != 1 {
		t.Fatalf("precondition: expected 1 session row, got %d", len(rows))
	}
	m.sessionList.Select(rows[0])
	m = enterMultiSelect(t, m)
	if !m.IsSessionSelected("portal-abc") {
		t.Fatalf("precondition: portal-abc must be marked")
	}
	if h := sessionRows(m.sessionList.Items())[0].GroupHeading; h != "Portal" {
		t.Fatalf("precondition: By Project heading = %q, want Portal", h)
	}

	m = pressSession(t, m, keyS)
	if m.sessionListMode != prefs.ModeByTag {
		t.Fatalf("precondition: expected ModeByTag after regroup, got %v", m.sessionListMode)
	}
	moved := sessionRows(m.sessionList.Items())
	if len(moved) != 1 || moved[0].GroupHeading != "work" {
		t.Fatalf("precondition: session must move to the work bucket; rows = %+v", moved)
	}

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
