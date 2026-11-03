package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func pressSpaceThenBail(t *testing.T, m Model, session string) (Model, tea.Cmd) {
	t.Helper()
	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after Space, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("test setup invariant: expected pagePreview after Space, got %v", got.activePage)
	}
	updated2, cmd := got.Update(previewAttachBailMsg{Session: session})
	got2, ok := updated2.(Model)
	if !ok {
		t.Fatalf("expected Model after bail msg, got %T", updated2)
	}
	return got2, cmd
}

func TestPreviewAttachBailFlipsToPageSessions(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions after bail, got %v", got.activePage)
	}
}

func TestPreviewAttachBailZerosPreviewModel(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	zero := previewModel{}
	if !reflect.DeepEqual(got.preview, zero) {
		t.Errorf("expected m.preview zeroed after bail, got %+v", got.preview)
	}
}

func TestPreviewAttachBailDispatchesRefreshCmd(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Answered: true, Attached: false},
		{Name: "bravo", Answered: true, Attached: false},
	}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	postKill := []Session{{Name: "bravo", Answered: true, Attached: false}}
	lister := &stepListerStub{steps: [][]Session{postKill}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	_, cmd := pressSpaceThenBail(t, m, "alpha")

	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler")
	}
	cmds := drainBatchCmds(cmd)
	if cmds == nil {
		t.Fatalf("expected tea.BatchMsg from bail cmd, got non-batch")
	}
	refreshed, ok := findRefreshedMsg(cmds)
	if !ok {
		t.Fatalf("expected previewSessionsRefreshedMsg in bail batch")
	}
	if lister.calls != 1 {
		t.Errorf("expected exactly 1 ListSessions call, got %d", lister.calls)
	}
	if refreshed.PreserveName != "alpha" {
		t.Errorf("expected PreserveName=%q from bail msg, got %q", "alpha", refreshed.PreserveName)
	}
}

func TestPreviewAttachBailPreservesSessionNameFromMessage(t *testing.T) {
	sessions := []Session{
		{Name: "alpha", Answered: true, Attached: false},
		{Name: "bravo", Answered: true, Attached: false},
	}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	_, cmd := pressSpaceThenBail(t, m, "bravo")

	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler")
	}
	cmds := drainBatchCmds(cmd)
	if cmds == nil {
		t.Fatalf("expected tea.BatchMsg from bail cmd, got non-batch")
	}
	refreshed, ok := findRefreshedMsg(cmds)
	if !ok {
		t.Fatalf("expected previewSessionsRefreshedMsg in bail batch")
	}
	if refreshed.PreserveName != "bravo" {
		t.Errorf("bail handler must read msg.Session: expected PreserveName=%q, got %q", "bravo", refreshed.PreserveName)
	}
}

func TestPreviewAttachBailNoListerStillEmitsTickCleanly(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	got, cmd := pressSpaceThenBail(t, m, "alpha")

	if got.activePage != PageSessions {
		t.Errorf("bail must still transition cleanly without a lister, got %v", got.activePage)
	}
	if got.flashText == "" {
		t.Errorf("expected flash to be set even without a lister, got empty")
	}
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler (tick is non-nil), got nil")
	}
	msg := cmd()
	if _, ok := msg.(flashTickMsg); !ok {
		t.Errorf("expected flashTickMsg directly from compacted Batch (only tick non-nil), got %T", msg)
	}
}

func TestPreviewAttachBailToleratesListerErrorSilently(t *testing.T) {
	first := []Session{
		{Name: "alpha", Answered: true, Attached: false},
		{Name: "bravo", Answered: true, Attached: false},
	}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{err: errors.New("boom")}
	m := modelWithSeamsAndLister(t, first, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "alpha")
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler")
	}
	cmds := drainBatchCmds(cmd)
	if cmds == nil {
		t.Fatalf("expected tea.BatchMsg from bail cmd, got non-batch")
	}
	refreshMsg, ok := findRefreshedMsg(cmds)
	if !ok {
		t.Fatalf("expected previewSessionsRefreshedMsg in bail batch")
	}
	updated, _ := got.Update(refreshMsg)
	final, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after refresh msg, got %T", updated)
	}
	if final.activePage != PageSessions {
		t.Errorf("expected PageSessions after refresh error, got %v", final.activePage)
	}
	names := visibleSessionNames(final)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Errorf("expected pre-refresh list preserved on lister error, got %v", names)
	}
}

func TestPreviewAttachBailEmptySessionNameStillTransitions(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "")

	if got.activePage != PageSessions {
		t.Errorf("expected PageSessions even with empty session, got %v", got.activePage)
	}
	if cmd == nil {
		t.Fatalf("expected non-nil cmd (lister wired)")
	}
	cmds := drainBatchCmds(cmd)
	if cmds == nil {
		t.Fatalf("expected tea.BatchMsg from bail cmd, got non-batch")
	}
	refreshed, ok := findRefreshedMsg(cmds)
	if !ok {
		t.Fatalf("expected previewSessionsRefreshedMsg in bail batch")
	}
	if refreshed.PreserveName != "" {
		t.Errorf("expected empty PreserveName forwarded, got %q", refreshed.PreserveName)
	}
}

func TestEscDismissPathUnchangedAfterBailHandlerAdded(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := modelWithSeamsAndLister(t, sessions, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected Esc dismiss to still land on PageSessions, got %v", got.activePage)
	}
	if lister.calls != 1 {
		t.Errorf("expected Esc dismiss to still trigger 1 ListSessions call, got %d", lister.calls)
	}
}
