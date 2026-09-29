package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

func keySpaceMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

func modelWithSeams(t *testing.T, sessions []tmux.Session, enum TmuxEnumerator, reader ScrollbackReader) Model {
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
		enumerator:  enum,
		reader:      reader,
	}
}

func TestSpaceOnSessionsPageTransitionsToPagePreviewWhenHighlighted(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd on successful preview transition, got %T", cmd)
	}
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Errorf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if enum.calls != 1 {
		t.Errorf("expected enumerator to be called exactly once, got %d", enum.calls)
	}
	if enum.lastArg != "alpha" {
		t.Errorf("expected enumerator called with %q (highlighted session), got %q", "alpha", enum.lastArg)
	}
}

func TestSpaceOnSessionsPageNoOpWhenListEmpty(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, nil, enum, reader)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions, got %v", got.activePage)
	}
	if enum.calls != 0 {
		t.Errorf("expected NewPreviewModel NOT called on empty list (enumerator calls), got %d", enum.calls)
	}
}

func TestSpaceOnSessionsPageNoOpWhenSelectedItemNil(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, sessions, enum, reader)
	m.sessionList.SetFilterText("zzzzzzz")
	m.sessionList.SetFilterState(list.FilterApplied)

	if item := m.sessionList.SelectedItem(); item != nil {
		t.Fatalf("test setup invariant: expected SelectedItem()==nil after filtering to no matches, got %v", item)
	}

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions, got %v", got.activePage)
	}
	if enum.calls != 0 {
		t.Errorf("expected enumerator NOT called when SelectedItem()==nil, got %d", enum.calls)
	}
}

func TestSpaceOnSessionsPageRemainsOnSessionsWhenEnumerationFails(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{err: errStub("boom")}
	reader := &recordingReader{}
	m := modelWithSeams(t, sessions, enum, reader)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions on enumeration failure, got %v", got.activePage)
	}
	if enum.calls != 1 {
		t.Errorf("expected enumerator called exactly once, got %d", enum.calls)
	}
}

func TestSpaceOnSessionsPageRemainsOnSessionsWhenEnumerationEmpty(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{groups: nil}
	reader := &recordingReader{}
	m := modelWithSeams(t, sessions, enum, reader)

	updated, cmd := m.Update(keySpaceMsg())

	if cmd != nil {
		t.Errorf("expected nil cmd, got %T", cmd)
	}
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions on empty enumeration, got %v", got.activePage)
	}
}

func TestSpaceDuringSettingFilterDoesNotCallNewPreviewModel(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, sessions, enum, reader)
	updatedList, _ := m.sessionList.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.sessionList = updatedList

	if !m.sessionList.SettingFilter() {
		t.Fatalf("test setup invariant: expected SettingFilter()==true after pressing /")
	}

	updated, _ := m.Update(keySpaceMsg())

	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageSessions {
		t.Errorf("expected to remain on PageSessions while SettingFilter, got %v", got.activePage)
	}
	if enum.calls != 0 {
		t.Errorf("expected enumerator NOT called while SettingFilter, got %d", enum.calls)
	}
}

func TestSpaceOnLoadingPageDoesNotCallNewPreviewModel(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, nil, enum, reader)
	m.activePage = PageLoading

	updated, _ := m.Update(keySpaceMsg())

	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageLoading {
		t.Errorf("expected to remain on PageLoading, got %v", got.activePage)
	}
	if enum.calls != 0 {
		t.Errorf("expected enumerator NOT called from PageLoading, got %d", enum.calls)
	}
}

func TestSpaceOnProjectsPageDoesNotCallNewPreviewModel(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(t, nil, enum, reader)
	m.activePage = PageProjects

	updated, _ := m.Update(keySpaceMsg())

	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != PageProjects {
		t.Errorf("expected to remain on PageProjects, got %v", got.activePage)
	}
	if enum.calls != 0 {
		t.Errorf("expected enumerator NOT called from PageProjects, got %d", enum.calls)
	}
}

func TestPagePreviewRoutesUpdateToPreviewModel(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello")}
	m := modelWithSeams(t, sessions, enum, reader)

	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}

	updated2, _ := got.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated2)
	}
	if got2.activePage != pagePreview {
		t.Errorf("expected to remain on pagePreview after key, got %v", got2.activePage)
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }
