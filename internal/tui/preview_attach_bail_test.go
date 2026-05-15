package tui

import (
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// Tests for top-level handling of previewAttachBailMsg (and the terminal
// previewAttachErrorMsg). The bail handler mirrors the previewDismissedMsg
// shape: transition to PageSessions, zero m.preview, dispatch a sessions-list
// refresh keyed by the message's Session name. Phase 2 (task 2-5) layers an
// inline flash emission + tick onto that base — flash-specific assertions
// live in preview_attach_bail_flash_test.go. Tests here cover transitions,
// preview-zeroing, refresh-cmd dispatch, and Esc-dismiss regressions.

// pressSpaceThenBail opens the preview via Space, then feeds a
// previewAttachBailMsg directly into Update, mirroring how the cmd produced
// by previewAttachPipeline.Run resolves. The final Model and the tea.Cmd
// returned from the bail handler are surfaced for assertions.
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
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	if got.activePage != PageSessions {
		t.Errorf("expected activePage=PageSessions after bail, got %v", got.activePage)
	}
}

func TestPreviewAttachBailZerosPreviewModel(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	got, _ := pressSpaceThenBail(t, m, "alpha")

	zero := previewModel{}
	if !reflect.DeepEqual(got.preview, zero) {
		t.Errorf("expected m.preview zeroed after bail, got %+v", got.preview)
	}
}

func TestPreviewAttachBailDispatchesRefreshCmd(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	postKill := []tmux.Session{{Name: "bravo", Windows: 1, Attached: false}}
	lister := &stepListerStub{steps: [][]tmux.Session{postKill}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	_, cmd := pressSpaceThenBail(t, m, "alpha")

	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler")
	}
	// Bail handler now returns tea.Batch(refresh, tick) (Phase 2). Drain
	// the batch and locate the refresh msg.
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
	// The bail handler must read from msg.Session, not m.preview.session — the
	// preview is zeroed during the same Update call. Use a session name that
	// is NOT the one preview was opened on to prove the source.
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

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
	// Phase 2: bail returns tea.Batch(refresh, tick). When no SessionLister
	// is wired, refreshSessionsAfterPreviewCmd returns nil; bubbletea's
	// Batch compacts a single-non-nil-cmd input down to that cmd directly
	// (no BatchMsg wrapper). So the returned cmd produces a flashTickMsg
	// directly. The page still transitions cleanly and the flash is set.
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader) // no lister wired

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
	// Refresh-after-bail must tolerate lister errors the same way the dismiss
	// path does: drop the error silently, leave the existing list intact.
	first := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{err: errors.New("boom")}
	m := modelWithSeamsAndLister(first, enum, reader, lister)

	got, cmd := pressSpaceThenBail(t, m, "alpha")
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from bail handler")
	}
	// Bail returns tea.Batch(refresh, tick); fish the refresh cmd's msg
	// out of the batch and feed it through Update.
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
	// Defensive: empty msg.Session (e.g., empty-session guard in pipeline)
	// must still transition cleanly. PreserveName is forwarded empty;
	// reanchorSessionCursor returns early on empty.
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

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

// Regression: adding the bail handler must not perturb the existing Esc
// dismiss path. previewDismissedMsg must continue to flip to PageSessions
// and dispatch the refresh.
func TestEscDismissPathUnchangedAfterBailHandlerAdded(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := modelWithSeamsAndLister(sessions, enum, reader, lister)

	got := pressSpaceThenEscWithRefresh(t, m)

	if got.activePage != PageSessions {
		t.Errorf("expected Esc dismiss to still land on PageSessions, got %v", got.activePage)
	}
	if lister.calls != 1 {
		t.Errorf("expected Esc dismiss to still trigger 1 ListSessions call, got %d", lister.calls)
	}
}

func TestPreviewAttachErrorWithNilErrIsNoOp(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	_, cmd := m.Update(previewAttachErrorMsg{Err: nil})

	if cmd != nil {
		t.Errorf("expected nil cmd on previewAttachErrorMsg{Err: nil}, got %T", cmd)
	}
}

func TestPreviewAttachErrorWithNonNilErrQuits(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	_, cmd := m.Update(previewAttachErrorMsg{Err: errors.New("boom")})

	if cmd == nil {
		t.Fatalf("expected non-nil cmd on previewAttachErrorMsg{Err: non-nil}")
	}
	if msg := cmd(); msg != tea.Quit() {
		// tea.Quit is a function that returns tea.quitMsg. Comparison via the
		// returned message is the canonical bubbletea-test pattern.
		t.Errorf("expected tea.Quit() msg from error handler, got %T", msg)
	}
}
