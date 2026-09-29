package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// pendingReaderStub answers each read with the next step's pending session
// names, holding on the last step once the script runs out.
type pendingReaderStub struct {
	steps [][]string
	err   error
	calls int
	log   *[]string
}

func (r *pendingReaderStub) ListPendingResumePanes() (tmux.PendingResumeView, error) {
	idx := r.calls
	r.calls++
	if r.log != nil {
		*r.log = append(*r.log, "pending")
	}
	if r.err != nil {
		return tmux.PendingResumeView{}, r.err
	}
	names := r.steps[min(idx, len(r.steps)-1)]
	view := tmux.PendingResumeView{Rows: []tmux.PendingResumeRow{}, Sessions: map[string]struct{}{}}
	for _, n := range names {
		view.Rows = append(view.Rows, tmux.PendingResumeRow{Pending: true, Session: n})
		view.Panes++
		view.Sessions[n] = struct{}{}
	}
	return view, nil
}

type loggingLister struct {
	inner SessionLister
	log   *[]string
}

func (l loggingLister) ListSessions() ([]tmux.Session, error) {
	*l.log = append(*l.log, "sessions")
	return l.inner.ListSessions()
}

func pendingReadModel(t *testing.T, mode prefs.SessionListMode, projects []project.Project, lister SessionLister, reader PendingResumeReader) Model {
	t.Helper()
	m := newRebuildTestModel(t, mode, nil, projects)
	m.sessionLister = lister
	if reader != nil {
		m.pendingReader = reader
	}
	return m
}

func loadSessions(t *testing.T, m Model, cmd tea.Cmd) (Model, SessionsMsg) {
	t.Helper()
	msg, ok := cmd().(SessionsMsg)
	if !ok {
		t.Fatalf("list load produced %T, want SessionsMsg", msg)
	}
	updated, next := m.Update(msg)
	got := updated.(Model)
	if slices.ContainsFunc(drainAll(next), isQuit) {
		t.Fatalf("applying the list load quit the picker")
	}
	return got, msg
}

func drainAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, drainAll(c)...)
	}
	return out
}

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}

// pendingDotRows maps each session name to one entry per rendered row it
// occupies, true where that row ends in the trailing indicator dot.
func pendingDotRows(m Model) map[string][]bool {
	names := make(map[string]struct{}, len(m.sessions))
	for _, s := range m.sessions {
		names[s.Name] = struct{}{}
	}
	rows := map[string][]bool{}
	for line := range strings.SplitSeq(ansi.Strip(m.sessionList.View()), "\n") {
		for field := range strings.FieldsSeq(line) {
			if _, ok := names[field]; ok {
				rows[field] = append(rows[field], strings.HasSuffix(strings.TrimRight(line, " "), rowIndicatorGlyph))
				break
			}
		}
	}
	return rows
}

func assertDots(t *testing.T, m Model, want map[string]bool) {
	t.Helper()
	rows := pendingDotRows(m)
	for name, wantDot := range want {
		got, ok := rows[name]
		if !ok {
			t.Errorf("session %q rendered no row; rows = %v", name, rows)
			continue
		}
		for i, dot := range got {
			if dot != wantDot {
				t.Errorf("session %q row %d dot = %v, want %v", name, i, dot, wantDot)
			}
		}
	}
}

var pendingReadSessions = []tmux.Session{
	{Name: "alpha", Windows: 1},
	{Name: "bravo", Windows: 2},
}

func TestSessionListLoad_TakesThePendingReadWithTheSessionEnumerationOnFirstLoad(t *testing.T) {
	var log []string
	lister := loggingLister{inner: &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}}, log: &log}
	reader := &pendingReaderStub{steps: [][]string{{"alpha"}}, log: &log}
	m := Build(Deps{Lister: lister, PendingReader: reader})
	m.sessionList.SetSize(80, 24)

	cmd := m.fetchSessionsCmd()
	if reader.calls != 0 {
		t.Fatalf("pending read taken before the load command ran: %d calls", reader.calls)
	}
	got, msg := loadSessions(t, m, cmd)

	if want := []string{"sessions", "pending"}; !slices.Equal(log, want) {
		t.Errorf("reads in the load command = %v, want %v", log, want)
	}
	if _, ok := msg.Pending["alpha"]; !ok || len(msg.Pending) != 1 {
		t.Errorf("SessionsMsg.Pending = %v, want {alpha}", msg.Pending)
	}
	assertDots(t, got, map[string]bool{"alpha": true, "bravo": false})
}

func TestSessionListLoad_ReReadsBothAfterAKillARenameAndOnPreviewDismissal(t *testing.T) {
	after := []tmux.Session{{Name: "bravo", Windows: 2}, {Name: "charlie", Windows: 1}}
	tests := []struct {
		name string
		run  func(t *testing.T, m Model) Model
	}{
		{
			name: "kill",
			run: func(t *testing.T, m Model) Model {
				m.sessionKiller = &killerStub{}
				m.sessionList.Select(0)
				updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
				updated, cmd := updated.(Model).Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
				got, _ := loadSessions(t, updated.(Model), cmd)
				return got
			},
		},
		{
			name: "rename",
			run: func(t *testing.T, m Model) Model {
				m.sessionRenamer = &mockRenamerStub{}
				m.sessionList.Select(0)
				updated, _ := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
				opened := updated.(Model)
				opened.renameInput.SetValue("charlie")
				updated, cmd := opened.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				got, _ := loadSessions(t, updated.(Model), cmd)
				return got
			},
		},
		{
			name: "preview dismissal",
			run: func(t *testing.T, m Model) Model {
				m.enumerator = &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}}}}
				m.reader = &recordingReader{bytes: []byte("hi")}
				m.sessionList.Select(0)
				return pressSpaceThenEscWithRefresh(t, m)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions, after}}
			reader := &pendingReaderStub{steps: [][]string{{"alpha"}, {"charlie"}}}
			m := pendingReadModel(t, prefs.ModeFlat, nil, lister, reader)
			m, _ = loadSessions(t, m, m.fetchSessionsCmd())

			got := tt.run(t, m)

			if lister.calls != 2 || reader.calls != 2 {
				t.Errorf("after %s: session reads = %d, pending reads = %d, want 2 each", tt.name, lister.calls, reader.calls)
			}
			assertDots(t, got, map[string]bool{"bravo": false, "charlie": true})
		})
	}
}

type mockRenamerStub struct{}

func (mockRenamerStub) RenameSession(string, string) error { return nil }

func TestSessionListLoad_DropsADotForASessionNoLongerPending(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}}
	reader := &pendingReaderStub{steps: [][]string{{"alpha", "bravo"}, {"bravo"}}}
	m := pendingReadModel(t, prefs.ModeFlat, nil, lister, reader)

	m, _ = loadSessions(t, m, m.fetchSessionsCmd())
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": true})

	m, _ = loadSessions(t, m, m.fetchSessionsCmd())
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": true})
}

func TestSessionListLoad_RendersTheListWithNoDotsWhenThePendingReadFails(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}}
	reader := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	m := pendingReadModel(t, prefs.ModeFlat, nil, lister, reader)
	m, _ = loadSessions(t, m, m.fetchSessionsCmd())
	assertDots(t, m, map[string]bool{"alpha": true})

	reader.err = errors.New("list-panes: server exited")
	for _, route := range []string{"list load", "preview dismissal"} {
		t.Run(route, func(t *testing.T) {
			var got Model
			if route == "list load" {
				got, _ = loadSessions(t, m, m.fetchSessionsCmd())
			} else {
				msg := m.refreshSessionsAfterPreviewCmd("")().(previewSessionsRefreshedMsg)
				if msg.Pending != nil {
					t.Errorf("failed read carried a set onto the preview refresh: %v", msg.Pending)
				}
				updated, _ := m.Update(msg)
				got = updated.(Model)
			}
			if !got.sessionsLoaded || !slices.Equal(visibleSessionNames(got), []string{"alpha", "bravo"}) {
				t.Errorf("list not rendered normally: loaded=%v rows=%v", got.sessionsLoaded, visibleSessionNames(got))
			}
			assertDots(t, got, map[string]bool{"alpha": false, "bravo": false})
		})
	}
}

func TestSessionListLoad_NeverCarriesAPendingErrorIntoTheSessionListError(t *testing.T) {
	sessionErr := errors.New("list-sessions: no server")
	pendingErr := errors.New("list-panes: no server")
	tests := []struct {
		name       string
		sessionErr error
		wantErr    error
	}{
		{name: "sessions read, pending fails", wantErr: nil},
		{name: "both fail", sessionErr: sessionErr, wantErr: sessionErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}, err: tt.sessionErr}
			reader := &pendingReaderStub{steps: [][]string{{"alpha"}}, err: pendingErr}
			m := pendingReadModel(t, prefs.ModeFlat, nil, lister, reader)
			m.sessionKiller = &killerStub{}
			m.sessionRenamer = mockRenamerStub{}

			for route, cmd := range map[string]tea.Cmd{
				"fetch":  m.fetchSessionsCmd(),
				"kill":   m.killAndRefresh("alpha"),
				"rename": m.renameAndRefresh("alpha", "charlie"),
			} {
				msg := cmd().(SessionsMsg)
				if !errors.Is(msg.Err, tt.wantErr) || (tt.wantErr == nil && msg.Err != nil) {
					t.Errorf("[%s] SessionsMsg.Err = %v, want %v", route, msg.Err, tt.wantErr)
				}
			}
			preview := m.refreshSessionsAfterPreviewCmd("")().(previewSessionsRefreshedMsg)
			if !errors.Is(preview.Err, tt.wantErr) || (tt.wantErr == nil && preview.Err != nil) {
				t.Errorf("[preview] Err = %v, want %v", preview.Err, tt.wantErr)
			}
		})
	}
}

func TestSessionListLoad_MarksNothingWhenTheSeamIsUnwired(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}}
	m := pendingReadModel(t, prefs.ModeFlat, nil, lister, nil)
	m.sessionKiller = &killerStub{}
	m.sessionRenamer = mockRenamerStub{}

	for route, cmd := range map[string]tea.Cmd{
		"fetch":  m.fetchSessionsCmd(),
		"kill":   m.killAndRefresh("alpha"),
		"rename": m.renameAndRefresh("alpha", "charlie"),
	} {
		msg := cmd().(SessionsMsg)
		if msg.Pending != nil || msg.Err != nil {
			t.Errorf("[%s] unwired seam: Pending = %v, Err = %v; want nil, nil", route, msg.Pending, msg.Err)
		}
	}
	if msg := m.refreshSessionsAfterPreviewCmd("")().(previewSessionsRefreshedMsg); msg.Pending != nil {
		t.Errorf("[preview] unwired seam: Pending = %v, want nil", msg.Pending)
	}

	got, _ := loadSessions(t, m, m.fetchSessionsCmd())
	assertDots(t, got, map[string]bool{"alpha": false, "bravo": false})
}

func TestSessionListLoad_IssuesNoSecondReadOnAGroupedRebuild(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Dir: dir}, {Name: "bravo", Windows: 2, Dir: dir}}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	reader := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	m := pendingReadModel(t, prefs.ModeFlat, projects, lister, reader)
	m, _ = loadSessions(t, m, m.fetchSessionsCmd())

	step := func(msg tea.Msg) {
		t.Helper()
		updated, cmd := m.Update(msg)
		m = updated.(Model)
		for _, follow := range drainAll(cmd) {
			if follow == nil {
				continue
			}
			updated, _ = m.Update(follow)
			m = updated.(Model)
		}
	}

	for range 3 {
		step(tea.KeyPressMsg{Code: 's', Text: "s"})
		assertDots(t, m, map[string]bool{"alpha": true, "bravo": false})
	}
	m.sessionListMode = prefs.ModeByTag
	step(ProjectsLoadedMsg{Projects: projects})
	assertDots(t, m, map[string]bool{"alpha": true, "bravo": false})
	step(tea.KeyPressMsg{Code: '/', Text: "/"})
	step(tea.KeyPressMsg{Code: 'a', Text: "a"})
	assertDots(t, m, map[string]bool{"alpha": true})

	if reader.calls != 1 || lister.calls != 1 {
		t.Errorf("reads after grouped rebuilds: pending = %d, sessions = %d, want 1 each", reader.calls, lister.calls)
	}
}

func TestSessionListLoad_ShowsTheDotOnEveryRowOfAMultiTagPendingSession(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Dir: dir}}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	reader := &pendingReaderStub{steps: [][]string{{"alpha"}}}
	m := pendingReadModel(t, prefs.ModeByTag, projects, lister, reader)

	m, _ = loadSessions(t, m, m.fetchSessionsCmd())

	rows := pendingDotRows(m)["alpha"]
	if len(rows) != 2 {
		t.Fatalf("multi-tag session rendered %d rows, want 2 (one per tag)", len(rows))
	}
	for i, dot := range rows {
		if !dot {
			t.Errorf("By-Tag row %d of the pending session carries no dot", i)
		}
	}
}

func TestSessionListLoad_LeavesMultiSelectMarksAsTheyAre(t *testing.T) {
	lister := &stepListerStub{steps: [][]tmux.Session{pendingReadSessions}}
	reader := &pendingReaderStub{steps: [][]string{{"bravo"}}}
	m := pendingReadModel(t, prefs.ModeFlat, nil, lister, reader)
	m.multiSelectMode = true
	m.selectedSessions = markedSet("alpha")

	m, _ = loadSessions(t, m, m.fetchSessionsCmd())

	if !m.IsSessionSelected("alpha") || m.SelectedSessionCount() != 1 {
		t.Errorf("marks after a load with a pending set: alpha=%v count=%d, want true 1", m.IsSessionSelected("alpha"), m.SelectedSessionCount())
	}
	assertDots(t, m, map[string]bool{"alpha": false, "bravo": true})
}
