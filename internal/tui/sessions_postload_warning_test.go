package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Tests for task 5-7 (§10.5): on the concurrent cold/TUI route soft bootstrap
// warnings ride the progress channel onto the terminal BootstrapCompleteMsg and,
// on transition to the Sessions picker, surface as a POST-LOAD notice band routed
// through the §11 single-slot arbiter — a TRANSIENT orange/warning band that
// auto-clears on the next actionable keypress (the §11.2 flash lifecycle), NOT a
// stderr alt-screen flush. Zero warnings → no band, no flush. The warm/staging
// route (no progress receiver) keeps the existing flushBufferedWarningsCmd path
// byte-for-byte unchanged (TestBootstrapWarningBuffering covers that).
//
// White-box (package tui) so the band state can be asserted through the same
// unexported seams the §11.2 flash-reskin tests use (flashText / flashKind /
// activeNoticeBand). These tests mutate no package-level state, but live in the
// cmd-discipline family — no t.Parallel.

// postloadStubLister is a minimal SessionLister for the cold/TUI transition tests.
type postloadStubLister struct{ sessions []tmux.Session }

func (l postloadStubLister) ListSessions() ([]tmux.Session, error) { return l.sessions, nil }

// coldTUIModel builds a loading-page model on the concurrent cold/TUI route — a
// non-nil progress receiver is what discriminates that route from the warm/staging
// path. A no-op receiver suffices: these tests never drive the channel, they drive
// BootstrapCompleteMsg directly to exercise the post-load surfacing.
func coldTUIModel(t *testing.T, sessions []tmux.Session) tea.Model {
	t.Helper()
	lister := postloadStubLister{sessions: sessions}
	receiver := func() tea.Msg { return nil }
	m := New(lister, WithServerStarted(true), WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

// warmStagingModel builds a loading-page model on the warm/staging route — NO
// progress receiver, so the flushBufferedWarningsCmd path stays in force.
func warmStagingModel(t *testing.T) tea.Model {
	t.Helper()
	lister := postloadStubLister{sessions: []tmux.Session{}}
	m := New(lister, WithServerStarted(true))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

// transitionWithWarnings drives the two loading gates with the given warnings on
// the terminal complete event, landing the model on PageSessions. Returns the
// post-transition model and the cmd from the transition step.
func transitionWithWarnings(model tea.Model, warnings []BootstrapWarning) (tea.Model, tea.Cmd) {
	model, _ = model.Update(LoadingMinElapsedMsg{})
	var cmd tea.Cmd
	model, cmd = model.Update(BootstrapCompleteMsg{Warnings: warnings})
	return model, cmd
}

// TestColdTUIWarnings_SurfaceAsPostLoadNoticeBand asserts that soft warnings
// carried on BootstrapCompleteMsg surface as a notice band on the Sessions page
// AFTER the picker appears — never over the loading page, never via a stderr flush.
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

// TestColdTUIWarnings_SurfaceWhenMinElapsedArmTransitions asserts the band also
// surfaces when the transition happens in the LoadingMinElapsedMsg arm (complete
// arrives first, then min-elapsed) — the other of the two transition sites. Both
// gates must route warnings to the in-TUI band identically.
func TestColdTUIWarnings_SurfaceWhenMinElapsedArmTransitions(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}

	model := coldTUIModel(t, nil)
	// Complete first (buffers warnings, no transition yet — min not elapsed).
	model, _ = model.Update(BootstrapCompleteMsg{Warnings: warnings})
	if model.(Model).ActivePage() != PageLoading {
		t.Fatal("expected still on PageLoading before min-elapsed")
	}
	// Then min-elapsed — this arm performs the transition + surfacing.
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

// TestColdTUIWarnings_RendersInSessionsViewChrome closes the loop end-to-end: the
// warning carried on BootstrapCompleteMsg renders as a band line in the Sessions
// page chrome (m.View().Content) after the transition — proving it surfaces in the
// picker chrome, not merely in arbiter state. The §11.2 reskin tests cover the
// band's full styling; here we only assert the line is present post-transition.
func TestColdTUIWarnings_RendersInSessionsViewChrome(t *testing.T) {
	const line = "the session saver is not running"
	warnings := []BootstrapWarning{{Lines: []string{line}}}
	model, _ := transitionWithWarnings(coldTUIModel(t, []tmux.Session{{Name: "dev", Windows: 1}}), warnings)

	content := model.(Model).View().Content
	if !strings.Contains(content, line) {
		t.Errorf("post-load warning line %q not found in Sessions view chrome:\n%s", line, content)
	}
	// The §11 left-bar glyph must be present (the band, not a stray render).
	if !strings.Contains(content, noticeBarGlyph) {
		t.Errorf("rendered Sessions view missing the %q notice left-bar:\n%s", noticeBarGlyph, content)
	}
}

// TestColdTUIWarnings_NoticeAppearsOnlyAfterPicker asserts the band does NOT
// appear over the loading page — it surfaces only on the post-transition Sessions
// page. While still on PageLoading (only the complete msg, no min-elapsed) the
// slot is empty.
func TestColdTUIWarnings_NoticeAppearsOnlyAfterPicker(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}

	// Complete arrives but min-elapsed has NOT — still on loading page.
	model, _ := coldTUIModel(t, nil).Update(BootstrapCompleteMsg{Warnings: warnings})
	m := model.(Model)
	if m.ActivePage() != PageLoading {
		t.Fatalf("expected to still be on PageLoading; got page %d", m.ActivePage())
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("notice band must NOT own the slot while on the loading page")
	}
}

// TestColdTUIWarnings_ZeroWarningsNoNoticeNoFlush asserts zero warnings produce no
// band AND no stderr flush — preserving today's no-spurious-toggle property.
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
		// Run any returned cmd to be sure it does not flush.
		cmd()
	}
	if flushCalled {
		t.Error("zero warnings must not flush to stderr (no spurious alt-screen toggle)")
	}
}

// TestColdTUIWarnings_NoStderrFlush asserts the cold/TUI path does NOT flush
// warnings to stderr (the in-TUI band replaces the alt-screen flush). The
// flushWarningsToStderr seam must never fire on this route, even with warnings.
func TestColdTUIWarnings_NoStderrFlush(t *testing.T) {
	var flushCalled bool
	restore := SetFlushWarningsToStderrForTest(func(_ []BootstrapWarning) {
		flushCalled = true
	})
	t.Cleanup(restore)

	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, cmd := transitionWithWarnings(coldTUIModel(t, nil), warnings)
	if cmd != nil {
		// Run the transition cmd and any message it produces — the band setup
		// schedules an auto-clear tick, not a flush; running it must not flush.
		_ = cmd()
	}
	_ = model
	if flushCalled {
		t.Error("cold/TUI path must not flush warnings to stderr — the in-TUI band replaces it")
	}
}

// TestColdTUIWarnings_TransientAutoClearsOnKeypress asserts the post-load notice
// is TRANSIENT: the next actionable keypress clears it via the §11.2
// flashGen/isActionableKey lifecycle (it is not a standing persistent band).
func TestColdTUIWarnings_TransientAutoClearsOnKeypress(t *testing.T) {
	warnings := []BootstrapWarning{{Lines: []string{"saver is down"}}}
	model, _ := transitionWithWarnings(coldTUIModel(t, nil), warnings)

	if _, _, ok := model.(Model).activeNoticeBand(); !ok {
		t.Fatal("setup invariant: expected the notice band before the keypress")
	}

	// Any actionable key clears the transient band as a side effect.
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	m := model.(Model)
	if m.flashText != "" {
		t.Errorf("actionable key must clear the transient warning band: flashText = %q", m.flashText)
	}
	if _, _, ok := m.activeNoticeBand(); ok {
		t.Error("notice band must be cleared after the actionable keypress (transient, not persistent)")
	}
}

// TestColdTUIWarnings_MultipleWarningsOrderPreserved asserts every warning surfaces
// and the band message preserves orchestrator-observation order across multiple
// warnings (and multiple lines per warning).
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

	// Every line present, in order, before the next.
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

// TestColdTUIWarnings_BestEffortStepDoesNotAbortBoot asserts a best-effort-step
// warning (SaverDown / CorruptSessionsJSON shape) rides the channel and surfaces
// post-load WITHOUT aborting the boot — the model lands on the picker, never the
// fatal error frame (contrast with the 5-6 fatal path).
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

// TestWarmStagingWarnings_StillFlushToStderr asserts the warm/staging route (no
// progress receiver) keeps flushing warnings to stderr via flushBufferedWarningsCmd
// — byte-for-byte unchanged — and does NOT surface an in-TUI band.
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
	// The warm route surfaces via stderr, NOT an in-TUI band.
	if _, _, ok := model.(Model).activeNoticeBand(); ok {
		t.Error("warm/staging route must NOT surface an in-TUI notice band")
	}
}

// TestFormatWarningsFlash asserts the warning→band-message flattening: every
// warning's lines, in order, joined by newlines; empty/nil → "".
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
