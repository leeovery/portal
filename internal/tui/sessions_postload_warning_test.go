package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type postloadStubLister struct{ sessions []Session }

func (l postloadStubLister) ListSessions() ([]Session, error) { return l.sessions, nil }

func coldTUIModel(t *testing.T, sessions []Session) tea.Model {
	t.Helper()
	lister := postloadStubLister{sessions: sessions}
	receiver := func() tea.Msg { return nil }
	m := New(lister, WithServerStarted(true), WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

func warmStagingModel(t *testing.T) tea.Model {
	t.Helper()
	lister := postloadStubLister{sessions: []Session{}}
	m := New(lister, WithServerStarted(true))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

func transitionWithWarnings(model tea.Model, warnings []BootstrapWarning) (tea.Model, tea.Cmd) {
	model, _ = model.Update(LoadingMinElapsedMsg{})
	var cmd tea.Cmd
	model, cmd = model.Update(BootstrapCompleteMsg{Warnings: warnings})
	return model, cmd
}

func TestColdTUIWarnings_SurfaceAsPostLoadNoticeBand(t *testing.T) {
	warnings := []BootstrapWarning{
		{Lines: []string{"saver is down"}},
	}
	model, _ := transitionWithWarnings(coldTUIModel(t, nil), warnings)

	m := model.(Model)
	if m.ActivePage() != PageSessions {
		t.Fatalf("expected transition to PageSessions; got page %d", m.ActivePage())
	}

	role, message, ok := m.activeNoticeBand()
	if !ok {
		t.Fatal("expected a notice band to own the slot post-transition; got none")
	}
	if role != bandWarning {
		t.Errorf("post-load warning band role = %v, want bandWarning (orange/warning)", role)
	}
	if !strings.Contains(message, "saver is down") {
		t.Errorf("band message = %q, want it to carry the warning line", message)
	}
}

func TestColdTUIWarnings_SurfaceWhenMinElapsedArmTransitions(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}

	model := coldTUIModel(t, nil)
	model, _ = model.Update(BootstrapCompleteMsg{Warnings: warnings})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatal("expected still on PageLoading before min-elapsed")
	}
	model, _ = model.Update(LoadingMinElapsedMsg{})

	m := model.(Model)
	if m.ActivePage() != PageSessions {
		t.Fatalf("expected transition to PageSessions; got page %d", m.ActivePage())
	}
	role, _, ok := m.activeNoticeBand()
	if !ok || role != bandWarning {
		t.Errorf("min-elapsed-arm transition band role = %v ok = %v, want bandWarning true", role, ok)
	}
}

func TestColdTUIWarnings_RendersInSessionsViewChrome(t *testing.T) {
	const line = "the session saver is not running"
	warnings := []BootstrapWarning{{Lines: []string{line}}}
	model, _ := transitionWithWarnings(coldTUIModel(t, []Session{{Name: "dev", Answered: true}}), warnings)

	content := model.(Model).View().Content
	if !strings.Contains(content, line) {
		t.Errorf("post-load warning line %q not found in Sessions view chrome:\n%s", line, content)
	}
	if !strings.Contains(content, noticeBarGlyph) {
		t.Errorf("rendered Sessions view missing the %q notice left-bar:\n%s", noticeBarGlyph, content)
	}
}

func TestColdTUIWarnings_NoticeAppearsOnlyAfterPicker(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}

	model, _ := coldTUIModel(t, nil).Update(BootstrapCompleteMsg{Warnings: warnings})
	m := model.(Model)
	if m.ActivePage() != PageLoading {
		t.Fatalf("expected to still be on PageLoading; got page %d", m.ActivePage())
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("notice band must NOT own the slot while on the loading page")
	}
}

func TestColdTUIWarnings_ZeroWarningsNoNoticeNoFlush(t *testing.T) {
	var flushCalled bool
	restore := SetFlushWarningsToStderrForTest(func(_ []BootstrapWarning) {
		flushCalled = true
	})
	t.Cleanup(restore)

	model, cmd := transitionWithWarnings(coldTUIModel(t, nil), nil)
	m := model.(Model)

	if m.ActivePage() != PageSessions {
		t.Fatalf("expected transition to PageSessions; got page %d", m.ActivePage())
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("zero warnings must produce NO notice band")
	}
	if cmd != nil {
		cmd()
	}
	if flushCalled {
		t.Error("zero warnings must not flush to stderr (no spurious alt-screen toggle)")
	}
}

func TestColdTUIWarnings_NoStderrFlush(t *testing.T) {
	var flushCalled bool
	restore := SetFlushWarningsToStderrForTest(func(_ []BootstrapWarning) {
		flushCalled = true
	})
	t.Cleanup(restore)

	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, cmd := transitionWithWarnings(coldTUIModel(t, nil), warnings)
	if cmd != nil {
		_ = cmd()
	}
	_ = model
	if flushCalled {
		t.Error("cold/TUI path must not flush warnings to stderr — the in-TUI band replaces it")
	}
}

func TestColdTUIWarnings_TransientAutoClearsOnKeypress(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, _ := transitionWithWarnings(coldTUIModel(t, nil), warnings)

	if _, _, ok := model.(Model).activeNoticeBand(); !ok {
		t.Fatal("setup invariant: expected the notice band before the keypress")
	}

	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	m := model.(Model)
	if m.flashText != "" {
		t.Errorf("actionable key must clear the transient warning band: flashText = %q", m.flashText)
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("notice band must be cleared after the actionable keypress (transient, not persistent)")
	}
}

func TestColdTUIWarnings_MultipleWarningsOrderPreserved(t *testing.T) {
	warnings := []BootstrapWarning{
		{Lines: []string{"saver is down", "restart to recover"}},
		{Lines: []string{"sessions.json corrupt"}},
	}
	model, _ := transitionWithWarnings(coldTUIModel(t, nil), warnings)

	_, message, ok := model.(Model).activeNoticeBand()
	if !ok {
		t.Fatal("expected a notice band with multiple warnings")
	}

	wantOrder := []string{"saver is down", "restart to recover", "sessions.json corrupt"}
	lastIdx := -1
	for _, line := range wantOrder {
		idx := strings.Index(message, line)
		if idx < 0 {
			t.Fatalf("band message missing line %q; message = %q", line, message)
		}
		if idx <= lastIdx {
			t.Errorf("line %q out of order in band message %q", line, message)
		}
		lastIdx = idx
	}
}

func TestColdTUIWarnings_BestEffortStepDoesNotAbortBoot(t *testing.T) {
	warnings := []BootstrapWarning{
		{Lines: []string{"the session saver is not running"}},
	}
	model, _ := transitionWithWarnings(coldTUIModel(t, nil), warnings)

	m := model.(Model)
	if m.ActivePage() != PageSessions {
		t.Fatalf("a soft warning must NOT abort the boot; expected PageSessions, got page %d", m.ActivePage())
	}
	if _, _, ok := m.activeNoticeBand(); !ok {
		t.Error("the soft warning must surface as a post-load notice band")
	}
}

func TestWarmStagingWarnings_StillFlushToStderr(t *testing.T) {
	var captured [][]string
	restore := SetFlushWarningsToStderrForTest(func(warnings []BootstrapWarning) {
		for _, w := range warnings {
			captured = append(captured, append([]string{}, w.Lines...))
		}
	})
	t.Cleanup(restore)

	warnings := []BootstrapWarning{
		{Lines: []string{"saver down"}},
		{Lines: []string{"corrupt", "see log"}},
	}
	model, cmd := transitionWithWarnings(warmStagingModel(t), warnings)
	if cmd == nil {
		t.Fatal("warm/staging route must return the flushBufferedWarningsCmd")
	}
	cmd()

	if len(captured) != 2 {
		t.Fatalf("warm/staging flush captured %d warnings, want 2", len(captured))
	}
	if _, _, ok := model.(Model).activeNoticeBand(); ok {
		t.Error("warm/staging route must NOT surface an in-TUI notice band")
	}
}

func TestFormatWarningsFlash(t *testing.T) {
	if got := formatWarningsFlash(nil); got != "" {
		t.Errorf("formatWarningsFlash(nil) = %q, want empty", got)
	}
	if got := formatWarningsFlash([]BootstrapWarning{}); got != "" {
		t.Errorf("formatWarningsFlash(empty) = %q, want empty", got)
	}
	if got := formatWarningsFlash([]BootstrapWarning{{Lines: nil}}); got != "" {
		t.Errorf("formatWarningsFlash(warning with no lines) = %q, want empty", got)
	}

	warnings := []BootstrapWarning{
		{Lines: []string{"a1", "a2"}},
		{Lines: []string{"b1"}},
	}
	want := "a1\na2\nb1"
	if got := formatWarningsFlash(warnings); got != want {
		t.Errorf("formatWarningsFlash = %q, want %q", got, want)
	}
}
