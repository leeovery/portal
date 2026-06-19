package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// keySpaceMsg synthesises a standalone Space keypress, matching the shape
// produced by bubbletea v1.
func keySpaceMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
}

// modelWithSeams returns a Model on the Sessions page seeded with the given
// sessions and the given enumerator/reader seam values. The Model is sized so
// SettingFilter() and SelectedItem() behave as in production.
func modelWithSeams(sessions []tmux.Session, enum TmuxEnumerator, reader ScrollbackReader) Model {
	items := ToListItems(sessions)
	l := newSessionList(items)
	l.SetSize(80, 24)
	pl := newProjectList()
	pl.SetSize(80, 24)
	return Model{
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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(nil, enum, reader)

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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(sessions, enum, reader)
	// Apply a filter that matches nothing so the visible list is empty and
	// SelectedItem() returns nil. Filter must be applied (committed), not
	// SettingFilter, so we route through the existing key handler.
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
	m := modelWithSeams(sessions, enum, reader)

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
	m := modelWithSeams(sessions, enum, reader)

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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(sessions, enum, reader)
	// Open the filter-input mode on the underlying bubbles/list so
	// SettingFilter() returns true. The list's "/" rune key opens it.
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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(nil, enum, reader)
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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{}
	m := modelWithSeams(nil, enum, reader)
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
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello")}
	m := modelWithSeams(sessions, enum, reader)

	// Transition to pagePreview via Space.
	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}

	// Send an arbitrary key — the top-level Update must route to preview's
	// Update without panicking and without changing activePage.
	updated2, _ := got.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated2)
	}
	if got2.activePage != pagePreview {
		t.Errorf("expected to remain on pagePreview after key, got %v", got2.activePage)
	}
}

// errStub is a minimal error type for tests that need an error sentinel.
type errStub string

func (e errStub) Error() string { return string(e) }
