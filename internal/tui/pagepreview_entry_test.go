package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backendtest"
)

func keySpaceMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

func modelWithSeams(t *testing.T, sessions []Session, history HistoryReader) Model {
	t.Helper()
	items := ToListItems(sessions)
	l := newSessionList(items)
	l.SetSize(80, 24)
	pl := newProjectList()
	pl.SetSize(80, 24)
	return Model{
		themeState:  themeState{active: testDarkTheme(t)},
		sessions:    sessions,
		sessionList: l,
		projectList: pl,
		activePage:  PageSessions,
		termWidth:   80,
		termHeight:  24,
		history:     newPreviewHistory(history),
	}
}

// historyFor holds every one of sessions, each with vt as its history.
func historyFor(t *testing.T, vt string, sessions []Session) *backendtest.Fake {
	t.Helper()
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return historyFake(t, vt, names...)
}

// landRead delivers what cmd returns, as the runtime does once a preview's
// read comes back.
func landRead(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected the preview's read, got no cmd")
	}
	return asModel(t, dropCmd(m.Update(cmd())))
}

// openPreviewOn presses Space on the highlighted session and lands its read.
func openPreviewOn(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.Update(keySpaceMsg())
	return landRead(t, asModel(t, updated), cmd)
}

func historyCalls(f *backendtest.Fake) []string {
	var names []string
	for _, c := range f.CallsTo(backendtest.OpHistory) {
		names = append(names, c.Name)
	}
	return names
}

func TestSpaceOnSessionsPageOpensThePreviewBeforeTheReadLands(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)

	updated, cmd := m.Update(keySpaceMsg())

	got := asModel(t, updated)
	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview at once, got %v", got.activePage)
	}
	if body := lastBodyRow(got.preview); body != previewLoading {
		t.Errorf("body before the read lands = %q, want %q", body, previewLoading)
	}
	if calls := historyCalls(history); len(calls) != 0 {
		t.Errorf("History read %v inside Update, want the read left to the cmd", calls)
	}

	got = landRead(t, got, cmd)

	if calls := historyCalls(history); len(calls) != 1 || calls[0] != "alpha" {
		t.Errorf("History read %v, want once for the highlighted alpha", calls)
	}
	if body := lastBodyRow(got.preview); body != "hi" {
		t.Errorf("body once the read lands = %q, want the history", body)
	}
}

func TestSpaceOnSessionsPageOpensNothingWithoutAHistoryReader(t *testing.T) {
	m := modelWithSeams(t, []Session{{Name: "alpha"}}, nil)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	if got := asModel(t, updated); got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions with no history reader, got %v", got.activePage)
	}
}

func TestSpaceOnSessionsPageNoOpWhenListEmpty(t *testing.T) {
	history := historyFake(t, "hi")
	m := modelWithSeams(t, nil, history)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	if got := asModel(t, updated); got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 0 {
		t.Errorf("History read %v on an empty list, want no read", got)
	}
}

func TestSpaceOnSessionsPageNoOpWhenSelectedItemNil(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)
	m.sessionList.SetFilterText("zzzzzzz")
	m.sessionList.SetFilterState(list.FilterApplied)

	if item := m.sessionList.SelectedItem(); item != nil {
		t.Fatalf("test setup invariant: expected SelectedItem()==nil after filtering to no matches, got %v", item)
	}

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	if got := asModel(t, updated); got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 0 {
		t.Errorf("History read %v with nothing selected, want no read", got)
	}
}

func TestSpaceOnSessionsPageSaysWhenTheSessionIsGone(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFake(t, "hi"))

	got := openPreviewOn(t, m)

	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if body := lastBodyRow(got.preview); body != previewSessionGone {
		t.Errorf("body = %q, want %q", body, previewSessionGone)
	}
}

func TestSpaceOnSessionsPageOpensAPreviewOfASessionNotAnswering(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "hi", sessions)
	history.StopAnswering("alpha")
	m := modelWithSeams(t, sessions, history)

	got := openPreviewOn(t, m)

	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if body := lastBodyRow(got.preview); body != previewNotAnswering {
		t.Errorf("body = %q, want %q", body, previewNotAnswering)
	}
}

func TestSpaceOnASessionListedNotAnsweringSaysSoWithoutDialling(t *testing.T) {
	sessions := []Session{{Name: "alpha", NotAnswering: true}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)

	updated, cmd := m.Update(keySpaceMsg())

	got := asModel(t, updated)
	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if body := lastBodyRow(got.preview); body != previewNotAnswering {
		t.Errorf("body = %q, want %q at once", body, previewNotAnswering)
	}
	if cmd != nil {
		t.Errorf("expected no read for a session the listing found not answering, got %T", cmd)
	}
	if calls := historyCalls(history); len(calls) != 0 {
		t.Errorf("History read %v, want none", calls)
	}
}

func TestPreviewRead_DroppedOnceTheUserHasSteppedOn(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	history := historyFor(t, "hi", sessions)
	history.SetHistory("alpha", []byte("alpha's history"))
	m := modelWithSeams(t, sessions, history)
	updated, alphaRead := m.Update(keySpaceMsg())
	m = asModel(t, updated)
	updated, stepCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = asModel(t, updated)
	updated, bravoRead := m.Update(drainCmd(t, stepCmd))
	m = asModel(t, updated)

	m = landRead(t, m, alphaRead)

	if m.preview.session != "bravo" {
		t.Fatalf("setup: preview on %q, want bravo", m.preview.session)
	}
	if body := lastBodyRow(m.preview); body != previewLoading {
		t.Errorf("bravo's body = %q after alpha's late read, want it still %q", body, previewLoading)
	}

	m = landRead(t, m, bravoRead)

	if body := lastBodyRow(m.preview); body != "hi" {
		t.Errorf("bravo's body = %q once its own read lands, want its history", body)
	}
}

func TestPreviewRead_DroppedOnceThePreviewHasClosed(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	updated, read := m.Update(keySpaceMsg())
	m = asModel(t, updated)
	updated, dismiss := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = asModel(t, dropCmd(asModel(t, updated).Update(drainCmd(t, dismiss))))

	m = landRead(t, m, read)

	if m.activePage != PageSessions {
		t.Errorf("a late read reopened the preview: page %v", m.activePage)
	}
	if m.preview.session != "" {
		t.Errorf("a late read drew into the closed preview: session %q", m.preview.session)
	}
}

func TestSpaceDuringSettingFilterDoesNotCallNewPreviewModel(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)
	updatedList, _ := m.sessionList.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.sessionList = updatedList

	if !m.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==true after pressing /")
	}

	updated, _ := m.Update(keySpaceMsg())

	if got := asModel(t, updated); got.activePage != PageSessions {
		t.Errorf("expected to remain on PageSessions while SettingFilter, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 0 {
		t.Errorf("History read %v while setting the filter, want no read", got)
	}
}

func TestSpaceOnLoadingPageDoesNotCallNewPreviewModel(t *testing.T) {
	history := historyFake(t, "hi")
	m := modelWithSeams(t, nil, history)
	m.activePage = PageLoading

	updated, _ := m.Update(keySpaceMsg())

	if got := asModel(t, updated); got.activePage != PageLoading {
		t.Errorf("expected to remain on PageLoading, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 0 {
		t.Errorf("History read %v from PageLoading, want no read", got)
	}
}

func TestSpaceOnProjectsPageDoesNotCallNewPreviewModel(t *testing.T) {
	history := historyFake(t, "hi")
	m := modelWithSeams(t, nil, history)
	m.activePage = PageProjects

	updated, _ := m.Update(keySpaceMsg())

	if got := asModel(t, updated); got.activePage != PageProjects {
		t.Errorf("expected to remain on PageProjects, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 0 {
		t.Errorf("History read %v from PageProjects, want no read", got)
	}
}

func TestPagePreviewRoutesUpdateToPreviewModel(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFor(t, "hello", sessions))

	got := asModel(t, dropCmd(m.Update(keySpaceMsg())))
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}

	got2 := asModel(t, dropCmd(got.Update(tea.KeyPressMsg{Code: tea.KeyDown})))
	if got2.activePage != pagePreview {
		t.Errorf("expected to remain on pagePreview after key, got %v", got2.activePage)
	}
}
