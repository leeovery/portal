package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

type killerStub struct {
	killedName string
	err        error
}

func (k *killerStub) KillSession(name string) error {
	k.killedName = name
	return k.err
}

func TestKillRefreshUnderFilterPreservesFilteredList(t *testing.T) {
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "alphabet", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	postKill := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	killer := &killerStub{}

	m := modelWithSeamsAndLister(t, first, enum, reader, lister)
	m.sessionKiller = killer

	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if !m.sessionList.IsFiltered() {
		t.Fatalf("test setup invariant: expected IsFiltered()=true before kill keystrokes")
	}
	m.sessionList.Select(1)
	si, ok := m.selectedSessionItem()
	if !ok || si.Session.Name != "alphabet" {
		t.Fatalf("test setup invariant: expected cursor on %q, got ok=%v name=%q", "alphabet", ok, si.Session.Name)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	afterK, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after 'k', got %T", updated)
	}
	if afterK.modal != modalKillConfirm {
		t.Fatalf("expected modalKillConfirm after 'k', got %v", afterK.modal)
	}
	if afterK.pendingKillName != "alphabet" {
		t.Fatalf("expected pendingKillName=%q after 'k', got %q", "alphabet", afterK.pendingKillName)
	}

	updated2, killCmd := afterK.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	afterY, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after 'y', got %T", updated2)
	}
	if killCmd == nil {
		t.Fatalf("expected non-nil cmd from kill confirmation, got nil")
	}

	msg := killCmd()
	if killer.killedName != "alphabet" {
		t.Errorf("expected KillSession(%q), got %q", "alphabet", killer.killedName)
	}
	sessionsMsg, ok := msg.(SessionsMsg)
	if !ok {
		t.Fatalf("expected SessionsMsg from killAndRefresh cmd, got %T", msg)
	}
	if sessionsMsg.Err != nil {
		t.Fatalf("unexpected SessionsMsg error: %v", sessionsMsg.Err)
	}

	updated3, refilterCmd := afterY.Update(sessionsMsg)
	afterRefresh, ok := updated3.(Model)
	if !ok {
		t.Fatalf("expected Model after SessionsMsg, got %T", updated3)
	}
	finalAny := drainCmdThroughUpdate(t, afterRefresh, refilterCmd)
	got, ok := finalAny.(Model)
	if !ok {
		t.Fatalf("expected Model after refilter drain, got %T", finalAny)
	}

	if !got.sessionList.IsFiltered() {
		t.Errorf("expected IsFiltered()=true after kill-refresh, got false")
	}
	if val := got.sessionList.FilterValue(); val != "alpha" {
		t.Errorf("expected FilterValue=%q after kill-refresh, got %q", "alpha", val)
	}
	if got.sessionList.FilterState() != list.FilterApplied {
		t.Errorf("expected FilterState=FilterApplied after kill-refresh, got %v", got.sessionList.FilterState())
	}

	wantNames := []string{"alpha"}
	gotNames := visibleSessionNames(got)
	if len(gotNames) != len(wantNames) {
		t.Fatalf("expected VisibleItems=%v after kill-refresh, got %v", wantNames, gotNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("expected VisibleItems=%v after kill-refresh, got %v (mismatch at idx %d)", wantNames, gotNames, i)
			break
		}
	}
}
