package tui

import (
	"context"
	"slices"
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

// deliver hands m the message a read already returned, as the runtime does
// with one that finished before the user moved on.
func deliver(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	if msg == nil {
		t.Fatalf("setup: the read returned no message to deliver")
	}
	return asModel(t, dropCmd(m.Update(msg)))
}

// pressStep presses ↓ on the preview and applies the step it emits, returning
// the read the step starts.
func pressStep(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	updated, stepCmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated, read := asModel(t, updated).Update(drainCmd(t, stepCmd))
	return asModel(t, updated), read
}

// dismissPreview presses Esc on the preview and applies the dismiss it emits.
func dismissPreview(t *testing.T, m Model) Model {
	t.Helper()
	updated, dismiss := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return asModel(t, dropCmd(asModel(t, updated).Update(drainCmd(t, dismiss))))
}

func TestPreviewRead_InFlightOnceTheUserHasSteppedOnIsDropped(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	history := historyFor(t, "hi", sessions)
	history.SetHistory("alpha", []byte("alpha's history"))
	m := modelWithSeams(t, sessions, history)
	updated, alphaRead := m.Update(keySpaceMsg())
	alphaBody := alphaRead()
	m, bravoRead := pressStep(t, asModel(t, updated))

	m = deliver(t, m, alphaBody)

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

func TestPreviewRead_InFlightOnceThePreviewHasClosedIsDropped(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	m := modelWithSeams(t, sessions, historyFor(t, "hi", sessions))
	updated, read := m.Update(keySpaceMsg())
	body := read()
	m = dismissPreview(t, asModel(t, updated))

	m = deliver(t, m, body)

	if m.activePage != PageSessions {
		t.Errorf("a late read reopened the preview: page %v", m.activePage)
	}
	if m.preview.session != "" {
		t.Errorf("a late read drew into the closed preview: session %q", m.preview.session)
	}
}

// The page is the preview again, so only the generation tells the first
// open's read from the reopen's.
func TestPreviewRead_InFlightAcrossADismissAndReopenOfTheSameSessionIsDropped(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "first", sessions)
	m := modelWithSeams(t, sessions, history)
	updated, firstRead := m.Update(keySpaceMsg())
	firstBody := firstRead()
	m = dismissPreview(t, asModel(t, updated))
	history.SetHistory("alpha", []byte("second"))
	updated, secondRead := m.Update(keySpaceMsg())
	m = asModel(t, updated)

	m = deliver(t, m, firstBody)

	if m.activePage != pagePreview || m.preview.session != "alpha" {
		t.Fatalf("setup: page %v on %q, want the reopened alpha", m.activePage, m.preview.session)
	}
	if body := lastBodyRow(m.preview); body != previewLoading {
		t.Errorf("reopened body = %q after the first open's read, want it still %q", body, previewLoading)
	}

	m = landRead(t, m, secondRead)

	if body := lastBodyRow(m.preview); body != "second" {
		t.Errorf("reopened body = %q once its own read lands, want the history as it is now", body)
	}
}

// With ↓ held, each step supersedes the last before its read runs: only the
// session the steps end on is dialled.
func TestPreviewRead_SupersededBeforeItRunsNeitherDialsNorDraws(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}, {Name: "charlie"}, {Name: "delta"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)
	updated, alphaRead := m.Update(keySpaceMsg())
	m, bravoRead := pressStep(t, asModel(t, updated))
	m, charlieRead := pressStep(t, m)
	m, deltaRead := pressStep(t, m)

	for name, read := range map[string]tea.Cmd{"alpha": alphaRead, "bravo": bravoRead, "charlie": charlieRead} {
		if msg := read(); msg != nil {
			t.Errorf("%s's superseded read returned %#v, want no message", name, msg)
		}
	}
	m = landRead(t, m, deltaRead)

	if calls := historyCalls(history); !slices.Equal(calls, []string{"delta"}) {
		t.Errorf("History read %v, want delta alone", calls)
	}
	if body := lastBodyRow(m.preview); body != "hi" {
		t.Errorf("delta's body = %q, want its history", body)
	}
}

func TestPreviewRead_EndedByTheDismissNeitherDialsNorDraws(t *testing.T) {
	sessions := []Session{{Name: "alpha"}}
	history := historyFor(t, "hi", sessions)
	m := modelWithSeams(t, sessions, history)
	updated, read := m.Update(keySpaceMsg())
	dismissPreview(t, asModel(t, updated))

	if msg := read(); msg != nil {
		t.Errorf("read after the dismiss returned %#v, want no message", msg)
	}
	if calls := historyCalls(history); len(calls) != 0 {
		t.Errorf("History read %v after the dismiss, want none", calls)
	}
}

// gatedHistory holds its answer until released, signalling once it is asked.
type gatedHistory struct {
	asked, release chan struct{}
}

func (g gatedHistory) History(context.Context, string, int) ([]byte, error) {
	close(g.asked)
	<-g.release
	return []byte("superseded history"), nil
}

func TestPreviewRead_SupersededMidReadDrawsNothing(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	gate := gatedHistory{asked: make(chan struct{}), release: make(chan struct{})}
	m := modelWithSeams(t, sessions, gate)
	updated, alphaRead := m.Update(keySpaceMsg())
	result := make(chan tea.Msg, 1)
	go func() { result <- alphaRead() }()
	<-gate.asked

	updated, stepCmd := asModel(t, updated).Update(tea.KeyPressMsg{Code: tea.KeyDown})
	asModel(t, updated).Update(drainCmd(t, stepCmd))
	close(gate.release)

	if msg := <-result; msg != nil {
		t.Errorf("read superseded mid-read returned %#v, want no message", msg)
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
