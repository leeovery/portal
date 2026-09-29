package tui

import (
	"errors"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

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
		return s.steps[len(s.steps)-1], nil
	}
	return s.steps[idx], nil
}

func modelWithSeamsAndLister(t *testing.T, sessions []tmux.Session, enum TmuxEnumerator, reader ScrollbackReader, lister SessionLister) Model {
	t.Helper()
	m := modelWithSeams(t, sessions, enum, reader)
	m.sessionLister = lister
	return m
}

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
	updated2, escCmd := got.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
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
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	for _, n := range visibleSessionNames(got) {
		if n == "alpha" {
			t.Errorf("expected externally-killed alpha NOT in list after dismiss, got %v", visibleSessionNames(got))
		}
	}
}

func TestPreviewEscPreservesCursorWhenPreviousSessionStillExists(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
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
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
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
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{first}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
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
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{first}}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before Space")
	}
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
	if gotIndex := got.sessionList.Index(); gotIndex != wantCursorIndex {
		t.Errorf("expected sessionList.Index()=%d (previously-highlighted filtered row) after dismiss-with-refresh, got %d", wantCursorIndex, gotIndex)
	}
}

func TestDrainCmdThroughUpdateNilCmdReturnsModelUnchanged(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeamsAndLister(t, first, enum, reader, &stepListerStub{steps: [][]tmux.Session{first}})
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

func TestDrainCmdThroughUpdateInvokesCmdAndFeedsResultThroughUpdate(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeamsAndLister(t, first, enum, reader, &stepListerStub{steps: [][]tmux.Session{first}})

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
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{err: errors.New("boom")}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected PageSessions even after lister error, got %v", got.activePage)
	}
	names := visibleSessionNames(got)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Errorf("expected pre-refresh list preserved on lister error, got %v", names)
	}
}

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
