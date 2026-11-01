package tui

import (
	"errors"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Enumerates every session as one single-pane window, except the sessions
// listed in failing, which answer an error as a session killed outside the
// picker does.
type stepEnumerator struct {
	failing map[string]bool
}

func (s *stepEnumerator) ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) {
	if s.failing[session] {
		return nil, errors.New("can't find session")
	}
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}, nil
}

func stepSessions(names ...string) []tmux.Session {
	sessions := make([]tmux.Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, tmux.Session{Name: n, Windows: 1})
	}
	return sessions
}

func stepModel(t *testing.T, enum TmuxEnumerator, names ...string) Model {
	t.Helper()
	return modelWithSeams(t, stepSessions(names...), enum, &recordingReader{bytes: []byte("hi")})
}

func asModel(t *testing.T, tm tea.Model) Model {
	t.Helper()
	m, ok := tm.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", tm)
	}
	return m
}

func openPreviewAt(t *testing.T, m Model, index int) Model {
	t.Helper()
	m.sessionList.Select(index)
	m = asModel(t, dropCmd(m.Update(keySpaceMsg())))
	if m.activePage != pagePreview {
		t.Fatalf("setup: expected pagePreview after Space, got %v", m.activePage)
	}
	return m
}

func dropCmd(tm tea.Model, _ tea.Cmd) tea.Model { return tm }

// Presses the key on the preview page and feeds the step message it emits back
// through the parent, as the runtime does.
func pressPreviewStep(t *testing.T, m Model, code rune) Model {
	t.Helper()
	updated, cmd := m.Update(tea.KeyPressMsg{Code: code})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected a step cmd from the arrow on the preview page, got nil")
	}
	msg := cmd()
	if _, ok := msg.(previewStepSessionMsg); !ok {
		t.Fatalf("expected previewStepSessionMsg, got %T", msg)
	}
	return asModel(t, dropCmd(m.Update(msg)))
}

func assertPreviewing(t *testing.T, m Model, wantSession string, wantIndex int) {
	t.Helper()
	if m.activePage != pagePreview {
		t.Fatalf("expected the preview to stay open, got page %v", m.activePage)
	}
	if m.preview.session != wantSession {
		t.Errorf("preview.session = %q, want %q", m.preview.session, wantSession)
	}
	if got := m.sessionList.Index(); got != wantIndex {
		t.Errorf("sessionList.Index() = %d, want %d (the list cursor follows the preview)", got, wantIndex)
	}
}

func TestPreviewStep_DownOpensTheNextSession(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie"), 0)

	m = pressPreviewStep(t, m, tea.KeyDown)
	assertPreviewing(t, m, "bravo", 1)

	m = pressPreviewStep(t, m, tea.KeyDown)
	assertPreviewing(t, m, "charlie", 2)
}

func TestPreviewStep_UpOpensThePreviousSession(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie"), 2)

	m = pressPreviewStep(t, m, tea.KeyUp)

	assertPreviewing(t, m, "bravo", 1)
}

func TestPreviewStep_WrapsPastEitherEnd(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie"), 2)

	m = pressPreviewStep(t, m, tea.KeyDown)
	assertPreviewing(t, m, "alpha", 0)

	m = pressPreviewStep(t, m, tea.KeyUp)
	assertPreviewing(t, m, "charlie", 2)
}

func TestPreviewStep_ResetsWindowPaneAndScrollForTheNewSession(t *testing.T) {
	enum := &stubEnumerator{groups: []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "one", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "two", Panes: []tmux.WindowPane{{Index: 0}}},
	}}
	m := openPreviewAt(t, modelWithSeams(t, stepSessions("alpha", "bravo"), enum, &recordingReader{bytes: []byte("hi")}), 0)
	m = asModel(t, dropCmd(m.Update(tea.KeyPressMsg{Code: tea.KeyRight})))
	if m.preview.windowIdx != 1 {
		t.Fatalf("setup: expected windowIdx=1 after →, got %d", m.preview.windowIdx)
	}

	m = pressPreviewStep(t, m, tea.KeyDown)

	assertPreviewing(t, m, "bravo", 1)
	if m.preview.windowIdx != 0 || m.preview.paneIdx != 0 {
		t.Errorf("stepped preview focus = window %d pane %d, want 0/0", m.preview.windowIdx, m.preview.paneIdx)
	}
	if !m.preview.viewport.AtBottom() {
		t.Errorf("stepped preview must anchor at the bottom, got YOffset=%d", m.preview.viewport.YOffset())
	}
}

func TestPreviewStep_CarriesTheActiveThemeAndColourMode(t *testing.T) {
	m := stepModel(t, &stepEnumerator{}, "alpha", "bravo")
	m.colourless = true
	m = openPreviewAt(t, m, 0)

	m = pressPreviewStep(t, m, tea.KeyDown)

	if m.preview.th.Canvas != m.themeState.active.Canvas {
		t.Errorf("stepped preview canvas = %v, want the active theme's %v", m.preview.th.Canvas, m.themeState.active.Canvas)
	}
	if !m.preview.colourless {
		t.Error("stepped preview lost the colourless flag")
	}
}

func TestPreviewStep_SkipsGroupHeadings(t *testing.T) {
	m := stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie")
	m.sessionList.SetItems([]list.Item{
		HeaderItem{Heading: "one", Count: 2, Key: "one"},
		SessionItem{Session: tmux.Session{Name: "alpha", Windows: 1}, GroupKey: "one"},
		SessionItem{Session: tmux.Session{Name: "bravo", Windows: 1}, GroupKey: "one"},
		HeaderItem{Heading: "two", Count: 1, Key: "two"},
		SessionItem{Session: tmux.Session{Name: "charlie", Windows: 1}, GroupKey: "two"},
	})
	m = openPreviewAt(t, m, 2)

	m = pressPreviewStep(t, m, tea.KeyDown)
	assertPreviewing(t, m, "charlie", 4)

	// Wrapping off the bottom passes the leading heading too.
	m = pressPreviewStep(t, m, tea.KeyDown)
	assertPreviewing(t, m, "alpha", 1)

	m = pressPreviewStep(t, m, tea.KeyUp)
	assertPreviewing(t, m, "charlie", 4)
}

func TestPreviewStep_PassesOverASessionThatNoLongerEnumerates(t *testing.T) {
	enum := &stepEnumerator{failing: map[string]bool{"bravo": true}}
	m := openPreviewAt(t, stepModel(t, enum, "alpha", "bravo", "charlie"), 0)

	m = pressPreviewStep(t, m, tea.KeyDown)

	assertPreviewing(t, m, "charlie", 2)
}

func TestPreviewStep_StaysPutWhenNoOtherSessionOpens(t *testing.T) {
	t.Run("single session", func(t *testing.T) {
		m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha"), 0)

		m = pressPreviewStep(t, m, tea.KeyDown)

		assertPreviewing(t, m, "alpha", 0)
	})
	t.Run("every other session gone", func(t *testing.T) {
		enum := &stepEnumerator{failing: map[string]bool{"bravo": true, "charlie": true}}
		m := openPreviewAt(t, stepModel(t, enum, "alpha", "bravo", "charlie"), 0)

		m = pressPreviewStep(t, m, tea.KeyUp)

		assertPreviewing(t, m, "alpha", 0)
	})
}

func TestPreviewStep_WalksOnlyTheFilteredSessions(t *testing.T) {
	m := stepModel(t, &stepEnumerator{}, "alpha", "bravo", "alphabet")
	m.sessionList.SetFilterText("alpha")
	m.sessionList.SetFilterState(list.FilterApplied)
	if n := len(m.sessionList.VisibleItems()); n != 2 {
		t.Fatalf("setup: expected 2 visible sessions under the filter, got %d", n)
	}
	m = openPreviewAt(t, m, 0)
	start := m.preview.session

	m = pressPreviewStep(t, m, tea.KeyDown)
	if m.preview.session == start || m.preview.session == "bravo" {
		t.Fatalf("step under filter opened %q; want the other match", m.preview.session)
	}

	m = pressPreviewStep(t, m, tea.KeyDown)
	if m.preview.session != start {
		t.Errorf("second step under filter opened %q; want a wrap back to %q", m.preview.session, start)
	}
	if !m.sessionList.IsFiltered() || m.sessionList.FilterValue() != "alpha" {
		t.Errorf("stepping touched the filter: filtered=%t value=%q", m.sessionList.IsFiltered(), m.sessionList.FilterValue())
	}
}

func TestPreviewStep_DismissLandsOnTheSessionLastPreviewed(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie"), 0)
	m = pressPreviewStep(t, m, tea.KeyDown)
	m = pressPreviewStep(t, m, tea.KeyDown)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = asModel(t, dropCmd(asModel(t, updated).Update(drainCmd(t, cmd))))

	if m.activePage != PageSessions {
		t.Fatalf("expected PageSessions after dismiss, got %v", m.activePage)
	}
	if si, ok := m.selectedSessionItem(); !ok || si.Session.Name != "charlie" {
		t.Errorf("cursor after dismiss = %+v, want charlie", si)
	}
}

func TestPreviewStep_StepsFromThePreviewedSessionWhenTheCursorDrifted(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo", "charlie", "delta"), 1)
	// A late refresh re-anchored the cursor elsewhere while bravo stays open.
	m.sessionList.Select(3)

	m = pressPreviewStep(t, m, tea.KeyDown)

	assertPreviewing(t, m, "charlie", 2)
}

func TestPreviewStep_MessageAfterDismissIsIgnored(t *testing.T) {
	m := openPreviewAt(t, stepModel(t, &stepEnumerator{}, "alpha", "bravo"), 0)
	m.activePage = PageSessions

	m = asModel(t, dropCmd(m.Update(previewStepSessionMsg{delta: +1})))

	if m.activePage != PageSessions {
		t.Errorf("a stale step reopened the preview: page %v", m.activePage)
	}
	if got := m.sessionList.Index(); got != 0 {
		t.Errorf("a stale step moved the cursor to %d", got)
	}
}

func TestPreviewStep_HelpOverlaySwallowsTheArrows(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	for _, code := range []rune{tea.KeyUp, tea.KeyDown} {
		if _, cmd := pressPreviewKey(t, m, tea.KeyPressMsg{Code: code}); cmd != nil {
			t.Errorf("arrow %d under the help overlay emitted a cmd; the overlay owns every key", code)
		}
	}
}
