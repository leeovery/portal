package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// drainCmd executes a tea.Cmd to retrieve its tea.Msg, mirroring how the
// bubbletea event loop fans out commands. Used to round-trip
// previewDismissedMsg through Update without spinning up tea.Program.
func drainCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from preview Esc, got nil")
	}
	return cmd()
}

// pressSpaceThenEsc opens the preview via Space, then sends Esc, executes
// the returned cmd, and feeds the resulting message back into Update. The
// final Model is returned for assertions.
func pressSpaceThenEsc(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}
	updated2, cmd := got.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after Esc, got %T", updated2)
	}
	msg := drainCmd(t, cmd)
	updated3, _ := got2.Update(msg)
	got3, ok := updated3.(Model)
	if !ok {
		t.Fatalf("expected Model after dismiss msg, got %T", updated3)
	}
	return got3
}

func TestPreviewEscReturnsToSessionsPage(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	got := pressSpaceThenEsc(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions after Esc, got %v", got.activePage)
	}
}

func TestPreviewEscPreservesListCursor(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 1, Attached: false},
		{Name: "delta", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)
	m.sessionList.Select(3)
	if got := m.sessionList.Index(); got != 3 {
		t.Fatalf("test setup invariant: expected Index()=3 before Space, got %d", got)
	}

	got := pressSpaceThenEsc(t, m)

	if idx := got.sessionList.Index(); idx != 3 {
		t.Errorf("expected sessionList.Index()=3 after Space->Esc, got %d", idx)
	}
}

func TestPreviewEscPreservesNoFilterState(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)
	if m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=false before Space")
	}
	beforeFilterValue := m.sessionList.FilterValue()

	got := pressSpaceThenEsc(t, m)

	if got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=false after Space->Esc, got true")
	}
	if got.sessionList.FilterValue() != beforeFilterValue {
		t.Errorf("expected FilterValue()=%q, got %q", beforeFilterValue, got.sessionList.FilterValue())
	}
}

func TestPreviewEscPreservesCommittedFilter(t *testing.T) {
	sessions := []tmux.Session{
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
	m := modelWithSeams(sessions, enum, reader)
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before Space")
	}
	if got := m.sessionList.FilterValue(); got != "alpha" {
		t.Fatalf("test setup invariant: expected FilterValue()=%q before Space, got %q", "alpha", got)
	}

	got := pressSpaceThenEsc(t, m)

	if !got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=true after Space->Esc, got false")
	}
	if val := got.sessionList.FilterValue(); val != "alpha" {
		t.Errorf("expected FilterValue()=%q after Space->Esc, got %q", "alpha", val)
	}
}

func TestSecondEscClearsCommittedFilterViaListDefault(t *testing.T) {
	sessions := []tmux.Session{
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
	m := modelWithSeams(sessions, enum, reader)
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)

	afterFirstEsc := pressSpaceThenEsc(t, m)
	if afterFirstEsc.activePage != PageSessions {
		t.Fatalf("test setup invariant: expected PageSessions after first Esc, got %v", afterFirstEsc.activePage)
	}
	if !afterFirstEsc.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected committed filter to survive first Esc, got cleared")
	}

	// A second Esc on the Sessions page must reach bubbles/list's default
	// Esc handler, which clears the committed filter (FilterApplied -> Unfiltered).
	updated, _ := afterFirstEsc.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after second Esc, got %T", updated)
	}
	if got.sessionList.IsFiltered() {
		t.Errorf("expected committed filter cleared by second Esc, got IsFiltered()=true")
	}
	if got.sessionList.FilterState() == list.FilterApplied {
		t.Errorf("expected FilterState != FilterApplied after second Esc, got FilterApplied")
	}
}

func TestPreviewReopenAfterDismissConstructsFreshPreviewModel(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	// First open / dismiss cycle.
	afterFirst := pressSpaceThenEsc(t, m)
	if enum.calls != 1 {
		t.Fatalf("expected 1 enumerator call after first open, got %d", enum.calls)
	}
	if len(reader.calls) != 1 {
		t.Fatalf("expected 1 reader call after first open, got %d", len(reader.calls))
	}

	// Second open via Space — must trigger fresh enumeration AND fresh
	// tail-N read, evidenced by both call counts incrementing.
	updated, _ := afterFirst.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after second Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Errorf("expected pagePreview after second Space, got %v", got.activePage)
	}
	if enum.calls != 2 {
		t.Errorf("expected enumerator calls to bump to 2 after re-open, got %d", enum.calls)
	}
	if len(reader.calls) != 2 {
		t.Errorf("expected reader calls to bump to 2 after re-open, got %d", len(reader.calls))
	}
}
