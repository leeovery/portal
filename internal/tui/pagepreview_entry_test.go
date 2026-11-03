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

func historyCalls(f *backendtest.Fake) []string {
	var names []string
	for _, c := range f.CallsTo(backendtest.OpHistory) {
		names = append(names, c.Name)
	}
	return names
}

func TestSpaceOnSessionsPageTransitionsToPagePreviewWhenHighlighted(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd on successful preview transition, got %T", cmd)
	}
	if got := asModel(t, updated); got.activePage != pagePreview {
		t.Errorf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if got := historyCalls(history); len(got) != 1 || got[0] != "alpha" {
		t.Errorf("History read %v, want once for the highlighted alpha", got)
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

func TestSpaceOnSessionsPageRemainsOnSessionsWhenTheSessionIsGone(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFake(t, "hi"))

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	if got := asModel(t, updated); got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions for a session the backend no longer holds, got %v", got.activePage)
	}
}

func TestSpaceOnSessionsPageOpensAPreviewOfASessionNotAnswering(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "hi", sessions)
	history.StopAnswering("alpha")
	m := modelWithSeams(t, sessions, history)

	got := asModel(t, dropCmd(m.Update(keySpaceMsg())))

	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if body := previewBody(got.preview); body[len(body)-1] != previewNotAnswering {
		t.Errorf("body = %q, want %q", body, previewNotAnswering)
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
