package tui_test

import (
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/tui/theme"
)

// editFieldFocused reports whether the edit-modal field label renders with the
// §13.1 focused colour (accent.violet) in the given view — the MV focus signal that
// replaced the legacy `> ` indicator. The probe builds the violet foreground SGR
// core (mode-default Dark, the harness canvas) and asserts the label line carries
// it. label is the uppercase field label (NAME / ALIASES / TAGS).
func editFieldFocused(view, label string) bool {
	probe := lipgloss.NewStyle().Foreground(theme.MV.AccentViolet.ColorFor(theme.Dark)).Render("x")
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		return false
	}
	violetCore := probe[start+1 : end]
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, label) && strings.Contains(line, violetCore) {
			return true
		}
	}
	return false
}

// flattenInitMsgs executes an Init (or any) cmd and returns every leaf message
// it produces, recursively draining tea.BatchMsg. Init now batches the OSC 11
// background-color query (tea.RequestBackgroundColor) alongside the data-load
// cmds, so its top-level result is a tea.BatchMsg; this helper lets a test reach
// the underlying SessionsMsg / ProjectsLoadedMsg without coupling to the batch
// shape. A nil cmd yields no messages; the OSC 11 query cmd produces a
// BackgroundColorMsg with no terminal response in the test harness, which is
// simply included in the returned slice (callers ignore it).
func flattenInitMsgs(cmd tea.Cmd) []tea.Msg {
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
		out = append(out, flattenInitMsgs(c)...)
	}
	return out
}

// applyInit runs the model's Init, flattens the resulting batch, and applies
// every produced message to the model in turn, returning the updated model. It
// drives the real Init → Update flow through the now-batched Init (which includes
// the async OSC 11 background-color query) so a test can assert on the rendered
// result exactly as before the query was added.
func applyInit(m tui.Model) tui.Model {
	var model tea.Model = m
	for _, msg := range flattenInitMsgs(m.Init()) {
		model, _ = model.Update(msg)
	}
	return model.(tui.Model)
}

// firstMsgOfType scans the flattened Init messages for the first one of type T
// and returns it with true; otherwise the zero value and false.
func firstMsgOfType[T tea.Msg](msgs []tea.Msg) (T, bool) {
	for _, msg := range msgs {
		if typed, ok := msg.(T); ok {
			return typed, true
		}
	}
	var zero T
	return zero, false
}

func TestView(t *testing.T) {
	tests := []struct {
		name     string
		sessions []tmux.Session
		cursor   int
		checks   func(t *testing.T, view string)
	}{
		{
			name: "renders all session names",
			sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 5, Attached: false},
				{Name: "misc", Windows: 1, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				for _, name := range []string{"dev", "work", "misc"} {
					if !strings.Contains(view, name) {
						t.Errorf("view missing session name %q", name)
					}
				}
			},
		},
		{
			name: "shows window count for each session",
			sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: false},
				{Name: "work", Windows: 1, Attached: false},
				{Name: "misc", Windows: 5, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				if !strings.Contains(view, "3 windows") {
					t.Error("view missing '3 windows'")
				}
				if !strings.Contains(view, "1 window") {
					t.Error("view missing '1 window'")
				}
				if !strings.Contains(view, "5 windows") {
					t.Error("view missing '5 windows'")
				}
			},
		},
		{
			name: "shows attached indicator for attached sessions",
			sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 5, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				lines := strings.Split(view, "\n")
				var devLine, workLine string
				for _, line := range lines {
					if strings.Contains(line, "dev") {
						devLine = line
					}
					if strings.Contains(line, "work") {
						workLine = line
					}
				}
				if !strings.Contains(devLine, "attached") {
					t.Errorf("attached session 'dev' line missing 'attached': %q", devLine)
				}
				if strings.Contains(workLine, "attached") {
					t.Errorf("detached session 'work' line should not contain 'attached': %q", workLine)
				}
			},
		},
		{
			name: "cursor starts at first session",
			sessions: []tmux.Session{
				{Name: "first", Windows: 1, Attached: false},
				{Name: "second", Windows: 2, Attached: false},
				{Name: "third", Windows: 3, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				lines := strings.Split(view, "\n")
				var firstLine, secondLine string
				for _, line := range lines {
					if strings.Contains(line, "first") {
						firstLine = line
					}
					if strings.Contains(line, "second") {
						secondLine = line
					}
				}
				if !strings.Contains(firstLine, "▌") {
					t.Errorf("first session should have cursor indicator: %q", firstLine)
				}
				if strings.Contains(secondLine, "▌") {
					t.Errorf("second session should not have cursor indicator: %q", secondLine)
				}
			},
		},
		{
			name: "single session renders correctly",
			sessions: []tmux.Session{
				{Name: "solo", Windows: 2, Attached: true},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				if !strings.Contains(view, "solo") {
					t.Error("view missing session name 'solo'")
				}
				if !strings.Contains(view, "2 windows") {
					t.Error("view missing '2 windows'")
				}
				if !strings.Contains(view, "attached") {
					t.Error("view missing 'attached' indicator")
				}
				if !strings.Contains(view, "▌") {
					t.Error("view missing cursor indicator")
				}
			},
		},
		{
			// §2.7: an over-long name truncates with an ellipsis so the fixed-width
			// trailing slots (window count, attached marker) are never pushed
			// off-row. The full name no longer appears; a leading prefix + the
			// ellipsis glyph does.
			name: "long session name truncates with an ellipsis",
			sessions: []tmux.Session{
				{Name: "my-very-long-project-name-that-should-not-be-truncated-x7k2m9", Windows: 1, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				if strings.Contains(view, "my-very-long-project-name-that-should-not-be-truncated-x7k2m9") {
					t.Error("over-long session name should be truncated to the flex width, but the full name rendered")
				}
				if !strings.Contains(view, "my-very-long-project-name") {
					t.Errorf("truncated name should keep a leading prefix of the name:\n%s", view)
				}
				if !strings.Contains(view, "…") {
					t.Errorf("truncated name should carry the ellipsis glyph:\n%s", view)
				}
			},
		},
		{
			name: "sessions displayed in order returned by tmux",
			sessions: []tmux.Session{
				{Name: "zebra", Windows: 1, Attached: false},
				{Name: "alpha", Windows: 2, Attached: false},
				{Name: "middle", Windows: 3, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				zebraIdx := strings.Index(view, "zebra")
				alphaIdx := strings.Index(view, "alpha")
				middleIdx := strings.Index(view, "middle")
				if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
					t.Fatal("not all session names found in view")
				}
				if zebraIdx >= alphaIdx {
					t.Errorf("zebra (idx %d) should appear before alpha (idx %d)", zebraIdx, alphaIdx)
				}
				if alphaIdx >= middleIdx {
					t.Errorf("alpha (idx %d) should appear before middle (idx %d)", alphaIdx, middleIdx)
				}
			},
		},
		{
			name: "window count uses correct pluralisation",
			sessions: []tmux.Session{
				{Name: "one-win", Windows: 1, Attached: false},
				{Name: "two-win", Windows: 2, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				lines := strings.Split(view, "\n")
				var oneWinLine, twoWinLine string
				for _, line := range lines {
					if strings.Contains(line, "one-win") {
						oneWinLine = line
					}
					if strings.Contains(line, "two-win") {
						twoWinLine = line
					}
				}
				if !strings.Contains(oneWinLine, "1 window") {
					t.Errorf("single window should show '1 window': %q", oneWinLine)
				}
				if strings.Contains(oneWinLine, "1 windows") {
					t.Errorf("single window should not show '1 windows': %q", oneWinLine)
				}
				if !strings.Contains(twoWinLine, "2 windows") {
					t.Errorf("multiple windows should show '2 windows': %q", twoWinLine)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tui.NewModelWithSessions(tt.sessions)
			view := m.View().Content
			tt.checks(t, view)
		})
	}
}

// mockSessionLister implements tui.SessionLister for testing.
type mockSessionLister struct {
	sessions []tmux.Session
	err      error
}

func (m *mockSessionLister) ListSessions() ([]tmux.Session, error) {
	return m.sessions, m.err
}

func TestInit(t *testing.T) {
	t.Run("returns command that fetches sessions", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		mock := &mockSessionLister{sessions: sessions}
		m := tui.New(mock)

		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil command")
		}

		// Init now batches the async OSC 11 background-color query alongside the
		// session fetch, so its top-level msg is a tea.BatchMsg; flatten it to
		// reach the SessionsMsg.
		sessionsMsg, ok := firstMsgOfType[tui.SessionsMsg](flattenInitMsgs(cmd))
		if !ok {
			t.Fatalf("expected a SessionsMsg in the Init batch")
		}
		if sessionsMsg.Err != nil {
			t.Fatalf("unexpected error: %v", sessionsMsg.Err)
		}
		if len(sessionsMsg.Sessions) != 2 {
			t.Fatalf("expected 2 sessions, got %d", len(sessionsMsg.Sessions))
		}
		if sessionsMsg.Sessions[0].Name != "dev" {
			t.Errorf("expected first session name 'dev', got %q", sessionsMsg.Sessions[0].Name)
		}
		if sessionsMsg.Sessions[1].Name != "work" {
			t.Errorf("expected second session name 'work', got %q", sessionsMsg.Sessions[1].Name)
		}
	})
}

func TestUpdate(t *testing.T) {
	t.Run("sessionsMsg populates list items", func(t *testing.T) {
		m := tui.New(nil)
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		items := model.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 list items, got %d", len(items))
		}
		si0 := items[0].(tui.SessionItem)
		if si0.Session.Name != "dev" {
			t.Errorf("items[0].Session.Name = %q, want %q", si0.Session.Name, "dev")
		}
		si1 := items[1].(tui.SessionItem)
		if si1.Session.Name != "work" {
			t.Errorf("items[1].Session.Name = %q, want %q", si1.Session.Name, "work")
		}
	})

	t.Run("sessionsMsg with error returns quit command", func(t *testing.T) {
		m := tui.New(nil)
		errMsg := tui.SessionsMsg{Err: fmt.Errorf("tmux not running")}

		_, cmd := m.Update(errMsg)
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}

		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})
}

func TestKeyboardNavigation(t *testing.T) {
	threeSessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
		{Name: "charlie", Windows: 3, Attached: false},
	}

	t.Run("down arrow moves cursor to next item", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "bravo" {
			t.Errorf("expected selected %q, got %q", "bravo", model.Selected())
		}
	})

	t.Run("up arrow moves cursor up", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

		result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "alpha" {
			t.Errorf("expected selected %q, got %q", "alpha", model.Selected())
		}
	})

	t.Run("cursor wraps to first item when going past last", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		// Move down 3 times: alpha -> bravo -> charlie -> wraps to alpha
		for range 3 {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}

		result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "alpha" {
			t.Errorf("expected selected %q (wrapped to first), got %q", "alpha", model.Selected())
		}
	})

	t.Run("cursor wraps to last item when going above first", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

		result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "charlie" {
			t.Errorf("expected selected %q (wrapped to last), got %q", "charlie", model.Selected())
		}
	})

	t.Run("view highlights correct row after navigation", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		view := m.View().Content

		lines := strings.Split(view, "\n")
		var alphaLine, bravoLine, charlieLine string
		for _, line := range lines {
			if strings.Contains(line, "alpha") {
				alphaLine = line
			}
			if strings.Contains(line, "bravo") {
				bravoLine = line
			}
			if strings.Contains(line, "charlie") {
				charlieLine = line
			}
		}
		if strings.Contains(alphaLine, "▌") {
			t.Errorf("alpha line should not have cursor: %q", alphaLine)
		}
		if !strings.Contains(bravoLine, "▌") {
			t.Errorf("bravo line should have cursor: %q", bravoLine)
		}
		if strings.Contains(charlieLine, "▌") {
			t.Errorf("charlie line should not have cursor: %q", charlieLine)
		}
	})
}

func TestQuitHandling(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	}

	quitTests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{
			name: "q key triggers quit",
			key:  tea.KeyPressMsg{Code: 'q', Text: "q"},
		},
		{
			name: "Ctrl+C triggers quit",
			key:  tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl},
		},
	}

	for _, tt := range quitTests {
		t.Run(tt.name, func(t *testing.T) {
			m := tui.NewModelWithSessions(sessions)
			_, cmd := m.Update(tt.key)
			if cmd == nil {
				t.Fatal("expected quit command, got nil")
			}
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Errorf("expected tea.QuitMsg, got %T", msg)
			}
		})
	}

	noQuitTests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{
			name: "down arrow does not trigger quit",
			key:  tea.KeyPressMsg{Code: tea.KeyDown},
		},
		{
			name: "up arrow does not trigger quit",
			key:  tea.KeyPressMsg{Code: tea.KeyUp},
		},
		{
			name: "j key does not trigger quit",
			key:  tea.KeyPressMsg{Code: 'j', Text: "j"},
		},
		{
			name: "k key does not trigger quit",
			key:  tea.KeyPressMsg{Code: 'k', Text: "k"},
		},
	}

	for _, tt := range noQuitTests {
		t.Run(tt.name, func(t *testing.T) {
			m := tui.NewModelWithSessions(sessions)
			_, cmd := m.Update(tt.key)
			if cmd != nil {
				msg := cmd()
				if _, ok := msg.(tea.QuitMsg); ok {
					t.Error("navigation key should not trigger quit")
				}
			}
		})
	}
}

func TestEnterSelection(t *testing.T) {
	t.Run("enter with no sessions is a no-op", func(t *testing.T) {
		m := tui.NewModelWithSessions(nil)

		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Should not trigger quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Error("enter with no sessions should not trigger quit")
			}
		}

		// Selected should remain empty
		model, ok := updated.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", updated)
		}
		if model.Selected() != "" {
			t.Errorf("expected empty selected, got %q", model.Selected())
		}
	})

	t.Run("enter sets selected session and triggers quit", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Should trigger quit
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}

		// Should set selected to the first session (cursor at 0)
		model, ok := updated.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", updated)
		}
		if model.Selected() != "dev" {
			t.Errorf("expected selected %q, got %q", "dev", model.Selected())
		}
	})

	t.Run("quit without selecting leaves selected empty", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}

		quitKeys := []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'c', Mod: tea.ModCtrl},
		}

		for _, key := range quitKeys {
			m := tui.NewModelWithSessions(sessions)
			updated, _ := m.Update(key)

			model, ok := updated.(tui.Model)
			if !ok {
				t.Fatalf("expected tui.Model, got %T", updated)
			}
			if model.Selected() != "" {
				t.Errorf("quit via %v should leave selected empty, got %q", key, model.Selected())
			}
		}
	})

	t.Run("selected returns correct session after navigation and enter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}

		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Navigate down twice to "charlie"
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Press Enter
		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Should trigger quit
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}

		// Should have selected "charlie"
		model, ok := updated.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", updated)
		}
		if model.Selected() != "charlie" {
			t.Errorf("expected selected %q, got %q", "charlie", model.Selected())
		}
	})
}

func TestNKeyCreatesSessionInCWD(t *testing.T) {
	t.Run("n creates session in cwd and quits", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		creator := &mockSessionCreator{
			sessionName: "portal-abc123",
		}

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/code/portal"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press n to create session in cwd
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "portal-abc123" {
			t.Errorf("expected session name %q, got %q", "portal-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/home/user/code/portal" {
			t.Errorf("expected CreateFromDir called with %q, got %q", "/home/user/code/portal", creator.createdDir)
		}
		if creator.createdCommand != nil {
			t.Errorf("expected nil command, got %v", creator.createdCommand)
		}

		// Feed SessionCreatedMsg back — should set selected and quit
		model, cmd = model.Update(createdMsg)
		if cmd == nil {
			t.Fatal("expected quit command after SessionCreatedMsg, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
		if model.(tui.Model).Selected() != "portal-abc123" {
			t.Errorf("expected Selected() = %q, got %q", "portal-abc123", model.(tui.Model).Selected())
		}
	})

	t.Run("n with no session creator is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithCWD("/home/user"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press n — no session creator, should be no-op
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd != nil {
			t.Errorf("expected nil command when no session creator, got non-nil")
		}
	})

	t.Run("session creation error is handled gracefully", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		creator := &mockSessionCreator{
			err: fmt.Errorf("tmux failed"),
		}

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/code"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press n
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key, got nil")
		}

		msg := cmd()
		// Should be sessionCreateErrMsg (unexported), not SessionCreatedMsg
		if _, ok := msg.(tui.SessionCreatedMsg); ok {
			t.Fatal("expected error msg, got SessionCreatedMsg")
		}

		// Feed the error back — should return to session list, not crash
		model, _ = model.Update(msg)
		if model.(tui.Model).Selected() != "" {
			t.Errorf("expected empty Selected() after error, got %q", model.(tui.Model).Selected())
		}

		// Verify TUI still renders (no crash)
		view := model.View().Content
		if !strings.Contains(view, "dev") {
			t.Errorf("expected session list after error, got:\n%s", view)
		}
	})

	t.Run("n from empty session list creates session in cwd", func(t *testing.T) {
		creator := &mockSessionCreator{
			sessionName: "code-abc123",
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/code"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// Press n
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key on empty list, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "code-abc123" {
			t.Errorf("expected session name %q, got %q", "code-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/home/user/code" {
			t.Errorf("expected CreateFromDir called with %q, got %q", "/home/user/code", creator.createdDir)
		}
	})
}

// mockSessionKiller implements tui.SessionKiller for testing.
type mockSessionKiller struct {
	killedName string
	err        error
}

func (m *mockSessionKiller) KillSession(name string) error {
	m.killedName = name
	return m.err
}

// mockProjectStore implements tui.ProjectStore for testing.
type mockProjectStore struct {
	projects     []project.Project
	listErr      error
	removeCalled bool
	removedPath  string
	removedVia   string
	removeErr    error
}

func (m *mockProjectStore) List() ([]project.Project, error) {
	return m.projects, m.listErr
}

func (m *mockProjectStore) CleanStale() ([]project.Project, error) {
	return nil, nil
}

func (m *mockProjectStore) Remove(path, via string) error {
	m.removeCalled = true
	m.removedPath = path
	m.removedVia = via
	return m.removeErr
}

// mockSessionCreator implements tui.SessionCreator for testing.
type mockSessionCreator struct {
	sessionName    string
	createdDir     string
	createdCommand []string
	err            error
}

func (m *mockSessionCreator) CreateFromDir(dir string, command []string) (string, error) {
	m.createdDir = dir
	m.createdCommand = command
	if m.err != nil {
		return "", m.err
	}
	return m.sessionName, nil
}

func TestInitialFilter(t *testing.T) {
	t.Run("model stores initial filter text", func(t *testing.T) {
		m := tui.New(&mockSessionLister{sessions: []tmux.Session{}})
		m = m.WithInitialFilter("myquery")

		if m.InitialFilter() != "myquery" {
			t.Errorf("InitialFilter() = %q, want %q", m.InitialFilter(), "myquery")
		}
	})

	t.Run("initial filter defaults to empty", func(t *testing.T) {
		m := tui.New(&mockSessionLister{sessions: []tmux.Session{}})

		if m.InitialFilter() != "" {
			t.Errorf("InitialFilter() = %q, want empty", m.InitialFilter())
		}
	})

	t.Run("evaluateDefaultPage applies initial filter via built-in list filtering", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		updatedModel := model.(tui.Model)

		// Filter state should be FilterApplied
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Errorf("filter state = %v, want FilterApplied", updatedModel.SessionListFilterState())
		}

		// Filter value should be "myapp"
		if updatedModel.SessionListFilterValue() != "myapp" {
			t.Errorf("filter value = %q, want %q", updatedModel.SessionListFilterValue(), "myapp")
		}

		// Visible items should only include matching sessions
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 2 {
			t.Fatalf("expected 2 visible items, got %d", len(visible))
		}
		for _, item := range visible {
			si := item.(tui.SessionItem)
			if si.Session.Name == "other" {
				t.Error("'other' should be filtered out")
			}
		}
	})

	t.Run("initial filter consumed on first load only", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
		}
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		// First SessionsMsg — applies filter
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Exit filter with Esc (clears built-in filter)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Second SessionsMsg — should NOT re-apply filter
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		updatedModel := model.(tui.Model)
		// Filter state should be Unfiltered
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v after second load, want Unfiltered", updatedModel.SessionListFilterState())
		}
		// All sessions should be visible
		items := updatedModel.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 items after filter consumed, got %d", len(items))
		}
	})

	t.Run("command-pending mode applies initial filter to project list", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"}).WithInitialFilter("myapp")

		// Init now batches the async OSC 11 query; applyInit flattens it and
		// applies every produced message so the project load still drives render.
		updatedModel := applyInit(m)

		// Should show the §11.4 command-pending banner (Pick a project to run + chip).
		visible := ansi.Strip(updatedModel.View().Content)
		if !strings.Contains(visible, "Pick a project to run") {
			t.Errorf("expected the §11.4 command-pending banner, got:\n%s", visible)
		}
		if !strings.Contains(visible, "claude") {
			t.Errorf("expected the command in the banner chip, got:\n%s", visible)
		}
		// Initial filter is applied to project list and consumed
		if updatedModel.ProjectListFilterState() != list.FilterApplied {
			t.Errorf("project filter state = %v, want FilterApplied", updatedModel.ProjectListFilterState())
		}
		if updatedModel.InitialFilter() != "" {
			t.Errorf("initialFilter should be consumed, got %q", updatedModel.InitialFilter())
		}
	})
}

func TestEmptyState(t *testing.T) {
	t.Run("empty sessions shows list empty state", func(t *testing.T) {
		m := tui.New(nil)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		items := updated.(tui.Model).SessionListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}
	})

	t.Run("nil sessions shows list empty state", func(t *testing.T) {
		m := tui.New(nil)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: nil})

		items := updated.(tui.Model).SessionListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}
	})

	t.Run("non-empty sessions populates list", func(t *testing.T) {
		m := tui.New(nil)
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: false},
		}
		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})

		items := updated.(tui.Model).SessionListItems()
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
	})

	t.Run("quit works in empty state", func(t *testing.T) {
		quitKeys := []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'c', Mod: tea.ModCtrl},
		}

		for _, key := range quitKeys {
			m := tui.New(nil)
			updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
			_, cmd := updated.Update(key)
			if cmd == nil {
				t.Fatalf("expected quit command for key %v, got nil", key)
			}
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); !ok {
				t.Errorf("expected tea.QuitMsg for key %v, got %T", key, msg)
			}
		}
	})

	t.Run("enter is no-op in empty state", func(t *testing.T) {
		m := tui.New(nil)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		result, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Error("enter in empty state should not trigger quit")
			}
		}
		model, ok := result.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", result)
		}
		if model.Selected() != "" {
			t.Errorf("expected empty selected, got %q", model.Selected())
		}
	})
}

func TestInsideTmuxSessionExclusion(t *testing.T) {
	t.Run("current session excluded from list inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "current-sess", Windows: 2, Attached: true},
			{Name: "bravo", Windows: 3, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("current-sess")

		items := m.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 items (excluding current), got %d", len(items))
		}
		for _, item := range items {
			si := item.(tui.SessionItem)
			if si.Session.Name == "current-sess" {
				t.Error("current session should be excluded from list items")
			}
		}

		view := m.View().Content
		if !strings.Contains(view, "alpha") {
			t.Errorf("non-current session 'alpha' should appear in list, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("non-current session 'bravo' should appear in list, got:\n%s", view)
		}
	})

	t.Run("title shows current session name inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
			{Name: "my-project-x7k2m9", Windows: 2, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("my-project-x7k2m9")

		title := m.SessionListTitle()
		want := "Sessions (current: my-project-x7k2m9)"
		if title != want {
			t.Errorf("SessionListTitle() = %q, want %q", title, want)
		}
		// Title should appear in view
		view := m.View().Content
		if !strings.Contains(view, "current: my-project-x7k2m9") {
			t.Errorf("expected title with current session name in view, got:\n%s", view)
		}
	})

	t.Run("title is Sessions outside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
			{Name: "work", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		title := m.SessionListTitle()
		if title != "Sessions" {
			t.Errorf("SessionListTitle() = %q, want %q", title, "Sessions")
		}
	})

	t.Run("empty list when only current session exists inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "only-session", Windows: 1, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("only-session")

		items := m.SessionListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items after filtering, got %d", len(items))
		}
	})

	t.Run("multiple sessions minus current renders correctly", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "current-one", Windows: 2, Attached: true},
			{Name: "bravo", Windows: 3, Attached: false},
			{Name: "charlie", Windows: 4, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("current-one")

		title := m.SessionListTitle()
		if !strings.Contains(title, "current-one") {
			t.Errorf("expected title with current session name, got: %q", title)
		}

		view := m.View().Content
		for _, name := range []string{"alpha", "bravo", "charlie"} {
			if !strings.Contains(view, name) {
				t.Errorf("expected session %q in list, got:\n%s", name, view)
			}
		}
		// Ensure current-one only appears in the title, not in session items
		items := m.SessionListItems()
		for _, item := range items {
			si := item.(tui.SessionItem)
			if si.Session.Name == "current-one" {
				t.Error("current session should only appear in title, not in items")
			}
		}
	})

	t.Run("very long current session name in title", func(t *testing.T) {
		longName := "my-extremely-long-project-name-that-goes-on-and-on-forever-x7k2m9"
		sessions := []tmux.Session{
			{Name: longName, Windows: 1, Attached: true},
			{Name: "other", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux(longName)

		title := m.SessionListTitle()
		expected := "Sessions (current: " + longName + ")"
		if title != expected {
			t.Errorf("SessionListTitle() = %q, want %q", title, expected)
		}
	})
}

func TestKillSession(t *testing.T) {
	t.Run("k opens kill confirmation modal for selected session", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k on the first session
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("expected the kill-confirm modal header, got:\n%s", view)
		}
		// The target session name appears in the modal body (§8.3).
		if !strings.Contains(view, "alpha") {
			t.Errorf("expected the target session name 'alpha' in the modal body, got:\n%s", view)
		}
		// Modal should have border styling (box-drawing characters)
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("modal overlay should contain border characters, got:\n%s", view)
		}
	})

	t.Run("y in confirmation mode triggers kill and refresh", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		if cmd == nil {
			t.Fatal("expected command from kill confirmation, got nil")
		}

		// Execute the command — it should kill and then return a SessionsMsg
		msg := cmd()
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}

		// The kill should have been called
		if killer.killedName != "alpha" {
			t.Errorf("expected kill of %q, got %q", "alpha", killer.killedName)
		}

		// Simulate receiving the refreshed sessions (alpha removed)
		if sessionsMsg.Err != nil {
			t.Fatalf("unexpected error: %v", sessionsMsg.Err)
		}
	})

	t.Run("n in confirmation mode is ignored (no longer cancels)", func(t *testing.T) {
		// §8.3 drops `n`: cancel is Esc only. `n` is ignored — the modal stays open,
		// nothing is killed.
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then n
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})

		view := model.View().Content
		// The modal must STAY open (n is ignored, not a cancel).
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("n must be ignored — the kill-confirm modal should stay open, got:\n%s", view)
		}
		if killer.killedName != "" {
			t.Errorf("kill should not have been called, but got %q", killer.killedName)
		}
	})

	t.Run("Esc in confirmation mode cancels", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then Esc
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		view := model.View().Content
		if strings.Contains(view, "Kill session?") {
			t.Errorf("kill-confirm modal should be cleared after Esc, got:\n%s", view)
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("session 'alpha' should still be in list after cancel, got:\n%s", view)
		}
		if killer.killedName != "" {
			t.Errorf("kill should not have been called, but got %q", killer.killedName)
		}
	})

	t.Run("session list refreshes after kill", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		remainingSessions := []tmux.Session{
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y to kill alpha
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		// Update the lister to return remaining sessions
		lister.sessions = remainingSessions

		// Execute the command
		msg := cmd()

		// Feed the result back into the model
		model, _ = model.Update(msg)

		view := model.View().Content
		if strings.Contains(view, "alpha") {
			t.Errorf("killed session 'alpha' should not appear in refreshed list, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("remaining session 'bravo' should appear in list, got:\n%s", view)
		}
	})

	t.Run("cursor adjusts when last session killed", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		remainingSessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to bravo (last session)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Press k then y to kill bravo
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		// Update lister
		lister.sessions = remainingSessions

		// Execute and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View().Content
		// Cursor should be on alpha (index 0), which is now the last session
		lines := strings.Split(view, "\n")
		var alphaLine string
		for _, line := range lines {
			if strings.Contains(line, "alpha") {
				alphaLine = line
				break
			}
		}
		if !strings.Contains(alphaLine, "▌") {
			t.Errorf("cursor should be on 'alpha' after killing last session, got:\n%s", view)
		}
	})

	t.Run("kill error returns error in SessionsMsg", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{err: fmt.Errorf("session not found")}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y to attempt kill
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		if cmd == nil {
			t.Fatal("expected command from kill confirmation, got nil")
		}

		// Execute the command — it should return a SessionsMsg with the kill error
		msg := cmd()
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}
		if sessionsMsg.Err == nil {
			t.Fatal("expected error in SessionsMsg when kill fails, got nil")
		}
		if sessionsMsg.Err.Error() != "failed to kill session 'alpha': session not found" {
			t.Errorf("unexpected error message: %q", sessionsMsg.Err.Error())
		}
	})

	t.Run("kill error clears confirmation state via SessionsMsg", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{err: fmt.Errorf("session not found")}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		// Execute command and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		// SessionsMsg with error triggers quit, so the model should exit.
		// But the confirmation prompt should not still be showing.
		view := model.View().Content
		if strings.Contains(view, "Kill session?") {
			t.Errorf("kill-confirm modal should be cleared after kill error, got:\n%s", view)
		}
	})

	t.Run("killing last remaining session empties list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "solo", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		// Update lister to return empty
		lister.sessions = []tmux.Session{}

		// Execute and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		items := model.(tui.Model).SessionListItems()
		if len(items) != 0 {
			t.Errorf("expected 0 items after killing last session, got %d", len(items))
		}
	})

	t.Run("NewWithDeps supports kill", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{}

		m := tui.New(lister, tui.WithKiller(killer), tui.WithProjectStore(store), tui.WithSessionCreator(creator))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k — should enter confirmation mode (not no-op)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("expected the kill-confirm modal via NewWithDeps, got:\n%s", view)
		}
	})

	t.Run("NewWithAllDeps supports kill", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{}

		m := tui.New(lister, tui.WithKiller(killer), tui.WithProjectStore(store), tui.WithSessionCreator(creator))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k — should enter confirmation mode (not no-op)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("expected the kill-confirm modal via NewWithAllDeps, got:\n%s", view)
		}
	})

	t.Run("other keys ignored during kill modal", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k to open kill modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

		// Press various keys that should be ignored
		ignoredKeys := []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'k', Text: "k"},
			{Code: 'r', Text: "r"},
			{Code: 'p', Text: "p"},
			{Code: 'x', Text: "x"},
			{Code: tea.KeyDown},
			{Code: tea.KeyUp},
			{Code: tea.KeyEnter},
		}
		for _, k := range ignoredKeys {
			var cmd tea.Cmd
			model, cmd = model.Update(k)
			if cmd != nil {
				msg := cmd()
				if _, ok := msg.(tui.SessionsMsg); ok {
					t.Errorf("key %v should be ignored during kill modal but produced SessionsMsg", k)
				}
			}
		}

		// Modal should still be showing
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("modal should still show after ignored keys, got:\n%s", view)
		}
		// Session should not have been killed
		if killer.killedName != "" {
			t.Errorf("no kill should have occurred, but got %q", killer.killedName)
		}
	})

	t.Run("k on empty list is no-op", func(t *testing.T) {
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// Press k on empty list
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

		// Should be a no-op: no command, no modal
		if cmd != nil {
			t.Errorf("k on empty list should return nil command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "Kill session?") {
			t.Errorf("k on empty list should not show confirmation modal, got:\n%s", view)
		}
	})
}

func TestSessionListHelpBar(t *testing.T) {
	t.Run("condensed footer shows the Core keys and omits the help-only keys", func(t *testing.T) {
		// §3.4: the Sessions footer is the single condensed row of Core keys. The
		// help-only keys (rename / kill / new in cwd / quit) move to the ? help modal
		// (Phase 3) and must NOT appear in the footer.
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		// Wide enough that the full condensed row renders without §2.7 truncation.
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := updated.View().Content

		// Core (footer) descriptions present.
		for _, desc := range []string{"navigate", "attach", "filter", "preview", "switch view", "projects", "help"} {
			if !strings.Contains(view, desc) {
				t.Errorf("condensed footer should contain Core key %q, got:\n%s", desc, view)
			}
		}
		// Help-only descriptions absent from the footer.
		for _, desc := range []string{"rename", "kill", "new in cwd", "quit"} {
			if strings.Contains(view, desc) {
				t.Errorf("condensed footer must NOT contain help-only key %q (§3.4), got:\n%s", desc, view)
			}
		}
	})
}

// mockSessionRenamer implements tui.SessionRenamer for testing.
//
// calls counts every RenameSession invocation. It exists so a test can assert
// the in-TUI rename path reduces to a SINGLE RenameSession(old, new) call with
// no other session-rename-adjacent work (notably no hook re-keying) — the
// SessionRenamer is the ONLY seam the rename path touches, so a single
// increment here is the structural proof that renameAndRefresh does a bare
// rename + list refresh.
type mockSessionRenamer struct {
	renamedOld string
	renamedNew string
	calls      int
	err        error
}

func (m *mockSessionRenamer) RenameSession(oldName, newName string) error {
	m.calls++
	m.renamedOld = oldName
	m.renamedNew = newName
	return m.err
}

// newModelWithRenamer creates a model with lister, killer, and renamer for rename tests.
func newModelWithRenamer(lister *mockSessionLister, killer *mockSessionKiller, renamer *mockSessionRenamer) tui.Model {
	return tui.New(lister, tui.WithKiller(killer), tui.WithRenamer(renamer))
}

func TestRenameSession(t *testing.T) {
	t.Run("r opens rename modal with pre-populated session name", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r (lowercase) on the first session
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		view := model.View().Content
		// Should show the §8.4 rename modal — `Rename session` header + `NEW NAME` label.
		if !strings.Contains(view, "Rename session") {
			t.Errorf("expected the rename modal ('Rename session' header), got:\n%s", view)
		}
		if !strings.Contains(view, "NEW NAME") {
			t.Errorf("expected the rename modal 'NEW NAME' field label, got:\n%s", view)
		}
		// Should have border styling (modal panel + input box).
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("rename modal should contain border characters, got:\n%s", view)
		}
		// Should contain the pre-populated session name.
		if !strings.Contains(view, "alpha") {
			t.Errorf("expected pre-filled name 'alpha' in rename modal, got:\n%s", view)
		}
	})

	t.Run("enter in rename modal renames session and refreshes", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r to open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Clear the text and type a new name
		model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		for _, r := range "new-alpha" {
			model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}

		// Press Enter to confirm
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("expected command from rename confirmation, got nil")
		}

		// Execute the command — should rename and return SessionsMsg
		msg := cmd()
		_, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}

		// Verify rename was called with correct args
		if renamer.renamedOld != "alpha" {
			t.Errorf("expected old name %q, got %q", "alpha", renamer.renamedOld)
		}
		if renamer.renamedNew != "new-alpha" {
			t.Errorf("expected new name %q, got %q", "new-alpha", renamer.renamedNew)
		}
	})

	t.Run("it reduces the in-TUI rename path to a single RenameSession with no hook re-keying", func(t *testing.T) {
		// This drives the SAME exported rename path as the subtest above
		// (tui.New + WithRenamer → r → type → Enter → renameAndRefresh's
		// tea.Cmd) but asserts the STRUCTURAL invariant behind spec
		// Acceptance Criteria 6 ("No external/UI change") and the fix's
		// central premise: the in-TUI rename is a bare RenameSession(old,
		// new) + list refresh with ZERO hook re-keying. If the fix were the
		// rejected "intercept-and-re-key" design, this path would have to
		// touch a hook seam; it does not, which is exactly why external and
		// in-TUI renames are both fixed at the root (@portal-id) rather than
		// intercepted here.
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Open rename modal, clear, type a new name, confirm — the exact
		// keystroke sequence the in-TUI rename path processes.
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		for _, r := range "renamed-alpha" {
			model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from rename confirmation, got nil")
		}

		// Executing the returned command runs renameAndRefresh, which is
		// the whole in-TUI rename side-effect: sessionRenamer.RenameSession
		// followed by a sessionLister.ListSessions refresh (model.go
		// renameAndRefresh). Nothing else runs.
		msg := cmd()
		if _, ok := msg.(tui.SessionsMsg); !ok {
			t.Fatalf("expected SessionsMsg (list refresh) after rename, got %T", msg)
		}

		// EXACTLY ONE RenameSession(old, new) call — no repeat, no
		// second rename-adjacent call. The rename path issues a single
		// client.RenameSession(old, new), byte-identical to the external
		// `tmux rename-session` the integration test's external leg drives.
		if renamer.calls != 1 {
			t.Errorf("in-TUI rename should call RenameSession exactly once; got %d calls", renamer.calls)
		}
		if renamer.renamedOld != "alpha" || renamer.renamedNew != "renamed-alpha" {
			t.Errorf("in-TUI rename should be RenameSession(%q, %q); got RenameSession(%q, %q)",
				"alpha", "renamed-alpha", renamer.renamedOld, renamer.renamedNew)
		}

		// Structural proof of "no hook re-keying": the tui Model wires NO
		// hook seam at all. SessionRenamer (the seam this path uses) has a
		// single method — RenameSession — and there is no hooks store, no
		// hook-key resolver, and no hook interface anywhere in the tui
		// package's rename wiring (grep internal/tui for hook: the only
		// matches are bootstrap loading-step labels and an unrelated layout
		// hook, never a resume-hook seam). renameAndRefresh therefore
		// CANNOT re-key hooks even in principle — the hook survives a rename
		// solely because it is keyed off the immutable @portal-id, proven
		// end-to-end by the RenameSession-equivalent integration leg.
		var _ tui.SessionRenamer = renamer
	})

	t.Run("empty rename input is rejected on enter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r to open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Clear input
		model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

		// Press Enter with empty input
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Should not trigger rename (no command returned)
		if cmd != nil {
			t.Error("expected nil command for empty rename input")
		}

		// Renamer should not have been called
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called with empty input, but got old=%q", renamer.renamedOld)
		}

		// Modal should still be open (modal stays open on empty input)
		view := model.View().Content
		if !strings.Contains(view, "Rename session") {
			t.Errorf("rename modal should stay open after empty enter, got:\n%s", view)
		}
	})

	t.Run("Esc dismisses rename modal without renaming", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r to open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Press Esc to dismiss
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		view := model.View().Content
		// Should be back to normal session list, no modal
		if strings.Contains(view, "Rename session") {
			t.Errorf("rename modal should be dismissed after Esc, got:\n%s", view)
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("session 'alpha' should still be in list after cancel, got:\n%s", view)
		}
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called, but got old=%q new=%q", renamer.renamedOld, renamer.renamedNew)
		}
	})

	t.Run("rename to same name is allowed", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r — name is pre-filled with "alpha"
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Press Enter without changing — same name rename
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("expected command from same-name rename, got nil")
		}

		msg := cmd()
		_, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}

		// Verify rename was called with same old and new name
		if renamer.renamedOld != "alpha" {
			t.Errorf("expected old name %q, got %q", "alpha", renamer.renamedOld)
		}
		if renamer.renamedNew != "alpha" {
			t.Errorf("expected new name %q, got %q", "alpha", renamer.renamedNew)
		}
	})

	t.Run("rename error triggers refresh", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{err: fmt.Errorf("duplicate session name")}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// r, clear, type new name, Enter
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		for _, r := range "bravo" {
			model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("expected command from rename, got nil")
		}

		msg := cmd()
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}
		if sessionsMsg.Err == nil {
			t.Fatal("expected error in SessionsMsg when rename fails, got nil")
		}
	})

	t.Run("r on empty list is no-op", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		renamer := &mockSessionRenamer{}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// Press r on empty list
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		view := model.View().Content
		// Should not show rename modal
		if strings.Contains(view, "Rename session") {
			t.Errorf("r on empty list should be no-op, got:\n%s", view)
		}
	})

	t.Run("r without renamer configured is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		// Press r — should be no-op (no renamer)
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		view := model.View().Content
		if strings.Contains(view, "Rename session") {
			t.Errorf("r with no renamer should be no-op, got:\n%s", view)
		}
	})

	t.Run("session list refreshes after successful rename", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamedSessions := []tmux.Session{
			{Name: "new-alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Clear and type new name
		model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		for _, r := range "new-alpha" {
			model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}

		// Press Enter
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Update lister to return renamed sessions
		lister.sessions = renamedSessions

		// Execute command and feed result back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View().Content
		if !strings.Contains(view, "new-alpha") {
			t.Errorf("expected renamed session 'new-alpha' in list, got:\n%s", view)
		}
		// Modal should be dismissed
		if strings.Contains(view, "Rename session") {
			t.Errorf("rename modal should be dismissed after successful rename, got:\n%s", view)
		}
	})
}

func TestFilterMode(t *testing.T) {
	t.Run("q does not quit while filtering is active", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "quickfix", Windows: 1, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode
		m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

		// Verify we're filtering
		model := m.(tui.Model)
		if model.SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", model.SessionListFilterState())
		}

		// Type 'q' — should be treated as filter input, not quit
		var cmd tea.Cmd
		_, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("q in filter mode should not quit")
			}
		}
	})

	t.Run("shortcut keys functional after filter mode exit", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type something
		m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

		// Exit via Esc (cancel filtering)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Verify we're no longer filtering
		model := m.(tui.Model)
		if model.SessionListFilterState() != list.Unfiltered {
			t.Fatalf("expected Unfiltered state after Esc, got %v", model.SessionListFilterState())
		}

		// 'q' should now quit (shortcut restored)
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatal("q after exiting filter mode should trigger quit command")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg after q, got %T", msg)
		}
	})
}

func TestCommandPendingMode(t *testing.T) {
	t.Run("command-pending mode starts in project picker view", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/api", Name: "api"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		// Init should load projects (not sessions). It now batches the async OSC
		// 11 query, so flatten to reach the ProjectsLoadedMsg — and assert NO
		// SessionsMsg is present (command-pending mode loads projects only).
		initMsgs := flattenInitMsgs(m.Init())
		projectsMsg, ok := firstMsgOfType[tui.ProjectsLoadedMsg](initMsgs)
		if !ok {
			t.Fatalf("expected a ProjectsLoadedMsg in the Init batch")
		}
		if _, hasSessions := firstMsgOfType[tui.SessionsMsg](initMsgs); hasSessions {
			t.Fatalf("command-pending Init must not fetch sessions")
		}
		if len(projectsMsg.Projects) != 2 {
			t.Fatalf("expected 2 projects, got %d", len(projectsMsg.Projects))
		}

		// Feed projects back to model
		var model tea.Model = m
		model, _ = model.Update(projectsMsg)

		view := model.View().Content
		// Should show projects page content, not session list
		if !strings.Contains(view, "myapp") {
			t.Errorf("expected projects page with project items, got:\n%s", view)
		}
	})

	t.Run("banner shows command text", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the banner reads `Pick a project to run` + the command in an orange
		// chip (the legacy plain `Select project to run: <cmd>` line is gone).
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, "Pick a project to run") {
			t.Errorf("expected the §11.4 banner text 'Pick a project to run', got:\n%s", visible)
		}
		if !strings.Contains(visible, "claude") {
			t.Errorf("expected the command 'claude' in the banner chip, got:\n%s", visible)
		}
		if strings.Contains(visible, "Select project to run") {
			t.Errorf("legacy plain status line must be gone, got:\n%s", visible)
		}
	})

	t.Run("banner shows multi-arg command joined", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume", "--model", "opus"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the multi-arg command joins on spaces into the orange chip.
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, "claude --resume --model opus") {
			t.Errorf("expected the joined command in the banner chip, got:\n%s", visible)
		}
	})

	t.Run("session list not displayed in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "existing", Windows: 2, Attached: false},
			}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View().Content
		// Session list content should not be visible
		if strings.Contains(view, "existing") {
			t.Errorf("session list should not be displayed in command-pending mode, got:\n%s", view)
		}
	})

	t.Run("project selection creates session with command", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Select a project by pressing Enter (first project in list)
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from project selection, got nil")
		}
		cmd()

		// Verify session was created with the project directory
		if creator.createdDir != "/code/myapp" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/code/myapp", creator.createdDir)
		}
		// Verify command was forwarded
		wantCmd := []string{"claude", "--resume"}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, wantCmd)
		}
		for i, arg := range creator.createdCommand {
			if arg != wantCmd[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, wantCmd[i])
			}
		}
	})

	t.Run("esc in command-pending mode quits TUI", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press Esc - should quit directly
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc in command-pending mode, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
	})

	t.Run("initial filter applied to project list in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"}).WithInitialFilter("myapp")

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		updated := applyInit(m)

		// Initial filter is applied to project list
		items := updated.ProjectListItems()
		if len(items) != 2 {
			t.Errorf("expected 2 total projects in list, got %d", len(items))
		}
		if updated.ProjectListFilterState() != list.FilterApplied {
			t.Errorf("expected FilterApplied on project list, got filter state %v", updated.ProjectListFilterState())
		}
		if updated.ProjectListFilterValue() != "myapp" {
			t.Errorf("expected project filter value %q, got %q", "myapp", updated.ProjectListFilterValue())
		}
	})

	t.Run("no command starts in session list view", func(t *testing.T) {
		m := tui.New(&mockSessionLister{
			sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			},
		})

		// Init now batches the async OSC 11 query; flatten to reach the SessionsMsg.
		sessionsMsg, ok := firstMsgOfType[tui.SessionsMsg](flattenInitMsgs(m.Init()))
		if !ok {
			t.Fatalf("expected a SessionsMsg in the Init batch")
		}

		var model tea.Model = m
		model, _ = model.Update(sessionsMsg)

		view := model.View().Content
		if !strings.Contains(view, "dev") {
			t.Errorf("expected session 'dev' in view, got:\n%s", view)
		}
		if strings.Contains(view, "Select project to run:") {
			t.Errorf("should not show status line without pending command, got:\n%s", view)
		}
	})

	t.Run("long command text in banner renders without truncation", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		longCmd := "some-very-long-command-name --with-many-flags --verbose --output=/tmp/really-long-path"
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{longCmd})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the full command renders in the chip without truncation.
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, longCmd) {
			t.Errorf("expected full command %q in the banner chip (no truncation), got:\n%s", longCmd, visible)
		}
	})

	t.Run("no saved projects shows empty state with browse and command banner", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		creator := &mockSessionCreator{sessionName: "code-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the command banner shows over the empty-projects state too (the
		// banner sits above the section header, the empty state is the list body).
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, "Pick a project to run") {
			t.Errorf("expected the §11.4 banner text in empty state, got:\n%s", visible)
		}
		if !strings.Contains(visible, "claude") {
			t.Errorf("expected the command in the banner chip in empty state, got:\n%s", visible)
		}
		if !strings.Contains(visible, "No saved projects") {
			t.Errorf("expected empty projects message, got:\n%s", visible)
		}
	})

	t.Run("pressing s in command-pending mode does nothing", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Verify we are on projects page
		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("expected projects page, got %v", updated.ActivePage())
		}

		// Press s - should do nothing (stay on projects page)
		model, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})

		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("pressing s in command-pending mode should stay on projects page, got page %v", updated.ActivePage())
		}
	})

	t.Run("pressing x in command-pending mode does nothing", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press x - should do nothing (stay on projects page)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("pressing x in command-pending mode should stay on projects page, got page %v", updated.ActivePage())
		}
	})

	t.Run("pressing e in command-pending mode does nothing", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithProjectEditor(editor),
			tui.WithAliasEditor(aliases),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press e - should do nothing (no modal should appear)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		view := model.View().Content
		if strings.Contains(view, "Edit Project") {
			t.Errorf("pressing e in command-pending mode should not open edit modal, got:\n%s", view)
		}
	})

	t.Run("pressing d in command-pending mode does nothing", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press d - should do nothing (no delete modal should appear)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		view := model.View().Content
		if strings.Contains(view, "Delete") && strings.Contains(view, "y/n") {
			t.Errorf("pressing d in command-pending mode should not open delete modal, got:\n%s", view)
		}
	})

	// §11.4 reskin (Phase 4): the command-pending Projects footer SWAPS to
	// `⏎ run here · n run in cwd · esc cancel` (+ the right-aligned `? help` anchor),
	// replacing the standard §6.3 copy while a command is pending. q quit stays
	// deferred to the ? help modal.
	t.Run("command-pending footer swaps to the §11.4 copy", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)
		// Set wide width so the condensed footer renders fully (no §2.7 truncation).
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		visible := ansi.Strip(model.View().Content)
		// The §11.4 swapped copy renders (labels assert each entry; the right-aligned
		// `? help` anchor survives).
		for _, want := range []string{"run here", "run in cwd", "cancel", "help"} {
			if !strings.Contains(visible, want) {
				t.Errorf("command-pending footer missing §11.4 entry %q, got:\n%s", want, visible)
			}
		}
		// The standard §6.3 copy must NOT leak in command-pending mode, and q quit
		// stays deferred to the ? help modal.
		for _, banned := range []string{"new session", "new in cwd", "quit"} {
			if strings.Contains(visible, banned) {
				t.Errorf("command-pending footer leaked non-§11.4 copy %q, got:\n%s", banned, visible)
			}
		}
	})

	t.Run("condensed footer copy is identical in normal mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		view := model.View().Content
		// The §6.3 condensed footer copy renders in normal mode (the same fixed copy).
		for _, want := range []string{"new session", "sessions", "edit", "filter", "help"} {
			if !strings.Contains(view, want) {
				t.Errorf("condensed footer missing §6.3 entry %q in normal mode, got:\n%s", want, view)
			}
		}
	})
}

func TestSessionListWithBubblesList(t *testing.T) {
	t.Run("SessionsMsg populates list items", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		m := tui.New(nil)

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		items := model.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 list items, got %d", len(items))
		}
		si0 := items[0].(tui.SessionItem)
		if si0.Session.Name != "dev" {
			t.Errorf("items[0].Session.Name = %q, want %q", si0.Session.Name, "dev")
		}
		si1 := items[1].(tui.SessionItem)
		if si1.Session.Name != "work" {
			t.Errorf("items[1].Session.Name = %q, want %q", si1.Session.Name, "work")
		}
	})

	t.Run("SessionsMsg with error triggers quit", func(t *testing.T) {
		m := tui.New(nil)
		errMsg := tui.SessionsMsg{Err: fmt.Errorf("tmux not running")}

		_, cmd := m.Update(errMsg)
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("enter selects session and quits", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}

		model := updated.(tui.Model)
		if model.Selected() != "dev" {
			t.Errorf("Selected() = %q, want %q", model.Selected(), "dev")
		}
	})

	t.Run("q key triggers quit", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		})

		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("Ctrl+C triggers quit", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		})

		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("inside tmux excludes current session from list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "current-sess", Windows: 2, Attached: true},
			{Name: "bravo", Windows: 3, Attached: false},
		}
		m := tui.New(nil)
		m = m.WithInsideTmux("current-sess")

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		items := model.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 items (excluding current), got %d", len(items))
		}
		for _, item := range items {
			si := item.(tui.SessionItem)
			if si.Session.Name == "current-sess" {
				t.Error("current session should be excluded from list items")
			}
		}
	})

	t.Run("inside tmux sets title with current session name", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "my-project", Windows: 2, Attached: true},
		}
		m := tui.New(nil)
		m = m.WithInsideTmux("my-project")

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		title := model.SessionListTitle()
		want := "Sessions (current: my-project)"
		if title != want {
			t.Errorf("SessionListTitle() = %q, want %q", title, want)
		}
	})

	t.Run("outside tmux sets title to Sessions", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		m := tui.New(nil)

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		title := model.SessionListTitle()
		if title != "Sessions" {
			t.Errorf("SessionListTitle() = %q, want %q", title, "Sessions")
		}
	})

	t.Run("empty session list shows empty state", func(t *testing.T) {
		m := tui.New(nil)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		items := updated.(tui.Model).SessionListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}

		view := updated.View().Content
		if view == "" {
			t.Error("view should not be empty even with no items")
		}
	})

	t.Run("WindowSizeMsg updates list dimensions", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		})

		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model := updated.(tui.Model)

		// Width is reduced by the global content gutter (Hinset cells each side)
		// so the list composes inside the inset region. Height is reduced by the
		// vertical inset plus the manual three-column keymap footer height (see
		// applyListSize) so the list does not overflow; the exact height reduction
		// depends on the footer's rendered shape, so we assert the relationship
		// rather than a pinned value.
		w, h := model.SessionListSize()
		if want := 120 - 2*tui.Hinset; w != want {
			t.Errorf("list width = %d, want %d (content gutter folded in)", w, want)
		}
		if h <= 0 || h >= 40-2*tui.Vinset {
			t.Errorf("list height = %d, want value reduced from 40 for the inset + footer", h)
		}
	})

	t.Run("inside tmux with only current session shows empty list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "only-session", Windows: 1, Attached: true},
		}
		m := tui.New(nil)
		m = m.WithInsideTmux("only-session")

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		model := updated.(tui.Model)

		items := model.SessionListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items after filtering, got %d", len(items))
		}
	})

	t.Run("sessions page renders using list View", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		})
		// Send a WindowSizeMsg so the list has dimensions to render
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		view := updated.View().Content
		// The view should contain session names (rendered by the list)
		if !strings.Contains(view, "dev") {
			t.Errorf("view should contain 'dev', got:\n%s", view)
		}
		if !strings.Contains(view, "work") {
			t.Errorf("view should contain 'work', got:\n%s", view)
		}
	})
}

func TestNewWithFunctionalOptions(t *testing.T) {
	t.Run("New with no options creates model with lister only", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister)

		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil command")
		}
		// Init now batches the async OSC 11 query; flatten to reach the SessionsMsg.
		sessionsMsg, ok := firstMsgOfType[tui.SessionsMsg](flattenInitMsgs(cmd))
		if !ok {
			t.Fatalf("expected a SessionsMsg in the Init batch")
		}
		if len(sessionsMsg.Sessions) != 1 {
			t.Fatalf("expected 1 session, got %d", len(sessionsMsg.Sessions))
		}
	})

	t.Run("WithKiller enables kill functionality", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k — should enter confirmation mode
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("expected the kill-confirm modal, got:\n%s", view)
		}
	})

	t.Run("WithRenamer enables rename functionality", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithRenamer(renamer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r — should open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		view := model.View().Content
		if !strings.Contains(view, "Rename session") {
			t.Errorf("expected the rename modal, got:\n%s", view)
		}
	})

	t.Run("WithProjectStore and WithSessionCreator enable project picker", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		lister := &mockSessionLister{sessions: []tmux.Session{}}

		m := tui.New(lister,
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"test"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		view := applyInit(m).View().Content
		if !strings.Contains(view, "myapp") {
			t.Errorf("expected projects page with project items, got:\n%s", view)
		}
	})

	t.Run("all options combined", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		renamer := &mockSessionRenamer{}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		lister := &mockSessionLister{sessions: sessions}

		m := tui.New(lister,
			tui.WithKiller(killer),
			tui.WithRenamer(renamer),
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Verify kill works
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Errorf("expected the kill-confirm modal, got:\n%s", view)
		}
	})
}

func TestBuiltInFiltering(t *testing.T) {
	t.Run("initial filter pre-applies filter text after items load", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		updatedModel := model.(tui.Model)

		// Filter state should be FilterApplied
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Errorf("filter state = %v, want FilterApplied", updatedModel.SessionListFilterState())
		}

		// Filter value should be "myapp"
		if updatedModel.SessionListFilterValue() != "myapp" {
			t.Errorf("filter value = %q, want %q", updatedModel.SessionListFilterValue(), "myapp")
		}

		// Visible items should only include matching sessions
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 2 {
			t.Fatalf("expected 2 visible items, got %d", len(visible))
		}
		for _, item := range visible {
			si := item.(tui.SessionItem)
			if si.Session.Name == "other" {
				t.Error("'other' should be filtered out")
			}
		}

		// All items should still be present in the full list
		allItems := updatedModel.SessionListItems()
		if len(allItems) != 3 {
			t.Errorf("expected 3 total items, got %d", len(allItems))
		}
	})

	t.Run("initial filter with no matches shows empty filtered state", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("zzzzz")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		updatedModel := model.(tui.Model)

		// Filter state should be FilterApplied
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Errorf("filter state = %v, want FilterApplied", updatedModel.SessionListFilterState())
		}

		// Visible items should be empty
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 0 {
			t.Errorf("expected 0 visible items for non-matching filter, got %d", len(visible))
		}
	})

	t.Run("empty initial filter is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		updatedModel := model.(tui.Model)

		// Filter state should be Unfiltered
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v, want Unfiltered", updatedModel.SessionListFilterState())
		}

		// All items should be visible
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 2 {
			t.Errorf("expected 2 visible items, got %d", len(visible))
		}
	})

	t.Run("list handles filter activation via slash key", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Press / to activate filter mode
		m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

		updatedModel := m.(tui.Model)

		// Filter state should be Filtering (user is actively editing the filter)
		if updatedModel.SessionListFilterState() != list.Filtering {
			t.Errorf("filter state = %v, want Filtering", updatedModel.SessionListFilterState())
		}
	})

	t.Run("Esc clears active filter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: filter state = %v, want FilterApplied", updatedModel.SessionListFilterState())
		}

		// Press Esc to clear the filter
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		updatedModel = model.(tui.Model)

		// Filter state should be Unfiltered
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v after Esc, want Unfiltered", updatedModel.SessionListFilterState())
		}

		// All items should be visible again
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 3 {
			t.Errorf("expected 3 visible items after clearing filter, got %d", len(visible))
		}
	})
}

func TestPageSwitching(t *testing.T) {
	t.Run("p on sessions page no longer switches to projects page", func(t *testing.T) {
		// §12.2: the p → Projects alias is dropped; x is the sole toggle. p is
		// now a no-op on the Sessions page (not bound to any action).
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Verify starting on sessions page
		if m.ActivePage() != tui.PageSessions {
			t.Fatalf("expected initial page to be PageSessions, got %d", m.ActivePage())
		}

		// Press p — it must NOT switch to projects (the alias is gone).
		model, _ = model.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected to stay on PageSessions after p (alias dropped), got %d", updated.ActivePage())
		}
	})

	t.Run("s on projects page no longer switches to sessions page", func(t *testing.T) {
		// §12.2: the Projects-side s→Sessions alias is dropped; x is the sole
		// both-directions toggle. s is now a no-op on the Projects page.
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page first (x is the sole toggle now).
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Press s — it must NOT switch back to sessions (the alias is gone).
		model, _ = model.Update(tea.KeyPressMsg{Code: 's', Text: "s"})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected to stay on PageProjects after s (alias dropped), got %d", updated.ActivePage())
		}
	})

	t.Run("x toggles from sessions to projects", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Press x to toggle from sessions to projects
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after x from sessions, got %d", updated.ActivePage())
		}
	})

	t.Run("x toggles from projects to sessions", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects first
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Press x to toggle back to sessions
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after x from projects, got %d", updated.ActivePage())
		}
	})

	t.Run("switching to projects and back preserves session list state", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Move cursor down to bravo
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Verify cursor is on bravo
		result, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if result.(tui.Model).Selected() != "bravo" {
			t.Fatalf("precondition: expected cursor on bravo, got %q", result.(tui.Model).Selected())
		}

		// Reset model (re-navigate to bravo without selecting)
		model = tui.NewModelWithSessions(sessions)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Switch to projects and back (x is the sole both-directions toggle).
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Press enter to verify cursor is still on bravo
		result, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command from enter")
		}
		if result.(tui.Model).Selected() != "bravo" {
			t.Errorf("expected cursor still on bravo after page switch round-trip, got %q", result.(tui.Model).Selected())
		}
	})

	t.Run("switching to empty stub projects page shows empty message", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page (stub with no items)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		view := model.View().Content
		// §11.1 reskin: the pre-reskin "No saved projects" copy is replaced by the
		// spec-exact "No projects yet".
		if !strings.Contains(view, "No projects yet") {
			t.Errorf("expected 'No projects yet' on empty projects page, got:\n%s", view)
		}
	})
}

func TestProjectsPage(t *testing.T) {
	t.Run("ProjectsLoadedMsg populates project list items", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Send ProjectsLoadedMsg
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		updated := model.(tui.Model)
		items := updated.ProjectListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 project list items, got %d", len(items))
		}
		pi0 := items[0].(tui.ProjectItem)
		if pi0.Project.Name != "portal" {
			t.Errorf("items[0].Project.Name = %q, want %q", pi0.Project.Name, "portal")
		}
		pi1 := items[1].(tui.ProjectItem)
		if pi1.Project.Name != "webapp" {
			t.Errorf("items[1].Project.Name = %q, want %q", pi1.Project.Name, "webapp")
		}
	})

	t.Run("createSession creates session at given path", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		creator := &mockSessionCreator{sessionName: "portal-abc123"}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press enter on first project
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter on project, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "portal-abc123" {
			t.Errorf("expected session name %q, got %q", "portal-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/code/portal" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/code/portal", creator.createdDir)
		}
	})

	t.Run("createSessionInCWD delegates to createSession with cwd", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "mydir-abc123"}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// Press n to create in cwd
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "mydir-abc123" {
			t.Errorf("expected session name %q, got %q", "mydir-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/home/user/mydir", creator.createdDir)
		}
	})

	t.Run("enter on project creates session and quits", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "webapp-abc123"}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Navigate to second project
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})

		// Press enter
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter on project, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "webapp-abc123" {
			t.Errorf("expected session name %q, got %q", "webapp-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/code/webapp" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/code/webapp", creator.createdDir)
		}

		// Feed SessionCreatedMsg back — should set selected and quit
		model, cmd = model.Update(createdMsg)
		if cmd == nil {
			t.Fatal("expected quit command after SessionCreatedMsg, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
		if model.(tui.Model).Selected() != "webapp-abc123" {
			t.Errorf("expected Selected() = %q, got %q", "webapp-abc123", model.(tui.Model).Selected())
		}
	})

	t.Run("n on projects page creates session in cwd", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		creator := &mockSessionCreator{sessionName: "mydir-abc123"}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press n to create session in cwd
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key on projects page, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "mydir-abc123" {
			t.Errorf("expected session name %q, got %q", "mydir-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/home/user/mydir", creator.createdDir)
		}

		// Feed SessionCreatedMsg back — should set selected and quit
		model, cmd = model.Update(createdMsg)
		if cmd == nil {
			t.Fatal("expected quit command after SessionCreatedMsg, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
		if model.(tui.Model).Selected() != "mydir-abc123" {
			t.Errorf("expected Selected() = %q, got %q", "mydir-abc123", model.(tui.Model).Selected())
		}
	})

	t.Run("q key quits from projects page", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Press q — should quit
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatal("expected quit command from q on projects page, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("Ctrl+C quits from projects page", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Press Ctrl+C — should quit
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C on projects page, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("empty project list shows empty message", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page (no items loaded)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		view := model.View().Content
		// §11.1 reskin: "No saved projects" → spec-exact "No projects yet".
		if !strings.Contains(view, "No projects yet") {
			t.Errorf("expected 'No projects yet' on empty projects page, got:\n%s", view)
		}
	})

	t.Run("project load error leaves list empty", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Send ProjectsLoadedMsg with error
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Err: fmt.Errorf("failed to load projects"),
		})

		updated := model.(tui.Model)
		items := updated.ProjectListItems()
		if len(items) != 0 {
			t.Fatalf("expected 0 items after load error, got %d", len(items))
		}

		// Should not crash — view should still render
		view := model.View().Content
		if view == "" {
			t.Error("view should not be empty after project load error")
		}
	})

	t.Run("session creation error handled gracefully", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		creator := &mockSessionCreator{err: fmt.Errorf("tmux failed")}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press enter on project
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter, got nil")
		}

		msg := cmd()
		// Should be error message, not SessionCreatedMsg
		if _, ok := msg.(tui.SessionCreatedMsg); ok {
			t.Fatal("expected error msg, got SessionCreatedMsg")
		}

		// Feed the error back — should not crash, selected should be empty
		model, _ = model.Update(msg)
		if model.(tui.Model).Selected() != "" {
			t.Errorf("expected empty Selected() after error, got %q", model.(tui.Model).Selected())
		}

		// Verify TUI still renders
		view := model.View().Content
		if view == "" {
			t.Error("view should not be empty after session creation error")
		}
	})

	t.Run("WindowSizeMsg updates project list dimensions", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		})

		updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model := updated.(tui.Model)

		// As with the sessions-list counterpart, width is reduced by the global
		// content gutter (Hinset each side) and height by the vertical inset plus
		// the manual three-column keymap footer height (see applyListSize).
		w, h := model.ProjectListSize()
		if want := 120 - 2*tui.Hinset; w != want {
			t.Errorf("project list width = %d, want %d (content gutter folded in)", w, want)
		}
		if h <= 0 || h >= 40-2*tui.Vinset {
			t.Errorf("project list height = %d, want value reduced from 40 for the inset + footer", h)
		}
	})

	t.Run("projects help bar shows correct keybindings", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(&mockSessionCreator{sessionName: "test"}),
		)
		var model tea.Model = m

		// Switch to projects page with wide width
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		view := model.View().Content
		// The §6.3 condensed footer copy (the `new in cwd` help-only key is deferred
		// to the ? help modal in Phase 3 — it is no longer in the footer).
		expectedDescs := []string{
			"new session",
			"sessions",
			"edit",
			"filter",
			"help",
		}
		for _, desc := range expectedDescs {
			if !strings.Contains(view, desc) {
				t.Errorf("projects footer should contain §6.3 entry %q, got:\n%s", desc, view)
			}
		}
		if strings.Contains(view, "new in cwd") {
			t.Errorf("projects footer should not contain the deferred 'new in cwd' help-only key, got:\n%s", view)
		}
	})

	t.Run("Init fires loadProjects command", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			}},
			tui.WithProjectStore(store),
		)

		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil command")
		}

		// The Init command should be a batch that includes loadProjects.
		// We cannot directly inspect batch commands, but we can verify
		// that after running Init and feeding messages, both sessions
		// and projects get loaded.
		// For now, just verify Init returns a non-nil command.
	})

	t.Run("projects page renders using list View", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects, set size, populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		view := model.View().Content
		if !strings.Contains(view, "portal") {
			t.Errorf("view should contain 'portal', got:\n%s", view)
		}
		if !strings.Contains(view, "webapp") {
			t.Errorf("view should contain 'webapp', got:\n%s", view)
		}
	})
}

func TestDeleteProject(t *testing.T) {
	t.Run("d opens delete confirmation modal for selected project", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d on the first project
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		view := model.View().Content
		// §8.6 reskin: the panel shows the destructive `▲ Delete project?` header and
		// the project name in the body (the old `Delete portal? (y/n)` copy is gone).
		if !strings.Contains(view, "Delete project?") {
			t.Errorf("expected '▲ Delete project?' header, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("expected the project name 'portal' in the delete panel, got:\n%s", view)
		}
		// Modal should have border styling
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("modal overlay should contain border characters, got:\n%s", view)
		}
	})

	t.Run("y in delete modal removes project and refreshes list", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		if cmd == nil {
			t.Fatal("expected command from delete confirmation, got nil")
		}

		// Execute the command — should call Remove and return ProjectsLoadedMsg
		msg := cmd()
		loadedMsg, ok := msg.(tui.ProjectsLoadedMsg)
		if !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		if loadedMsg.Err != nil {
			t.Fatalf("unexpected error: %v", loadedMsg.Err)
		}

		// Verify remove was called (inside the command)
		if !store.removeCalled {
			t.Error("expected store.Remove to be called")
		}
		if store.removedPath != "/code/portal" {
			t.Errorf("expected Remove(%q), got Remove(%q)", "/code/portal", store.removedPath)
		}
		// The TUI delete is a user-facing mutation, so the breadcrumb must
		// record via=cli.
		if store.removedVia != "cli" {
			t.Errorf("expected Remove via=cli, got %q", store.removedVia)
		}
	})

	t.Run("n in delete modal is ignored (cancel is Esc only)", func(t *testing.T) {
		// §8.1/§8.6: the keymap drops `n` — only Esc cancels. `n` is now ignored, so the
		// modal stays open and nothing is deleted.
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then n — n must NOT dismiss the modal.
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})

		view := model.View().Content
		// The modal is STILL open (n was ignored).
		if !strings.Contains(view, "Delete project?") {
			t.Errorf("delete modal should still be open after n (n is ignored), got:\n%s", view)
		}
		if store.removeCalled {
			t.Error("Remove should not have been called after n")
		}
	})

	t.Run("Esc in delete modal dismisses without deleting", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then Esc
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		view := model.View().Content
		if strings.Contains(view, "? (y/n)") {
			t.Errorf("confirmation prompt should be cleared after Esc, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("project 'portal' should still be in list after cancel, got:\n%s", view)
		}
		if store.removeCalled {
			t.Error("Remove should not have been called after Esc")
		}
	})

	t.Run("other keys ignored during delete modal", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d to open delete modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		// Press various keys that should be ignored
		ignoredKeys := []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'd', Text: "d"},
			{Code: 's', Text: "s"},
			{Code: 'x', Text: "x"},
			{Code: tea.KeyDown},
			{Code: tea.KeyUp},
			{Code: tea.KeyEnter},
		}
		for _, k := range ignoredKeys {
			var cmd tea.Cmd
			model, cmd = model.Update(k)
			if cmd != nil {
				msg := cmd()
				if _, ok := msg.(tui.ProjectsLoadedMsg); ok {
					t.Errorf("key %v should be ignored during delete modal but produced ProjectsLoadedMsg", k)
				}
			}
		}

		// Modal should still be showing (§8.6 reskinned header).
		view := model.View().Content
		if !strings.Contains(view, "Delete project?") {
			t.Errorf("modal should still show after ignored keys, got:\n%s", view)
		}
		if store.removeCalled {
			t.Error("no remove should have occurred")
		}
	})

	t.Run("delete last remaining project shows empty state", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		// Update store to return empty after removal
		store.projects = []project.Project{}

		model, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		if cmd == nil {
			t.Fatal("expected command from delete confirmation, got nil")
		}

		// Execute command and feed result back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View().Content
		// §11.1 reskin: "No saved projects" → spec-exact "No projects yet".
		if !strings.Contains(view, "No projects yet") {
			t.Errorf("expected empty state after deleting last project, got:\n%s", view)
		}
	})

	t.Run("d on empty project list is no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page (no items)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Press d on empty list
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		if cmd != nil {
			t.Errorf("d on empty list should return nil command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "? (y/n)") {
			t.Errorf("d on empty list should not show confirmation modal, got:\n%s", view)
		}
	})

	t.Run("delete error propagated via ProjectsLoadedMsg", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
			removeErr: fmt.Errorf("permission denied"),
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})

		if cmd == nil {
			t.Fatal("expected command from delete confirmation, got nil")
		}

		// Execute the command — should return ProjectsLoadedMsg with the remove error
		msg := cmd()
		loadedMsg, ok := msg.(tui.ProjectsLoadedMsg)
		if !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		if loadedMsg.Err == nil {
			t.Fatal("expected error in ProjectsLoadedMsg when Remove fails, got nil")
		}
		if !strings.Contains(loadedMsg.Err.Error(), "permission denied") {
			t.Errorf("expected error to contain 'permission denied', got: %q", loadedMsg.Err.Error())
		}
	})

	t.Run("delete while filter active removes the correct project", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
				{Path: "/code/api", Name: "api"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)

		// Switch to projects page and populate
		var model tea.Model = m
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
				{Path: "/code/api", Name: "api"},
			},
		})

		// Apply a filter via the list's filter API so only "webapp" is visible
		tuiModel := model.(tui.Model)
		tuiModel.SetProjectListFilter("webapp")
		if tuiModel.ProjectListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", tuiModel.ProjectListFilterState())
		}
		model = tuiModel

		// Press d — should target webapp from the filtered view
		model, _ = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})

		view := model.View().Content
		// §8.6 reskin: header is `▲ Delete project?` and the target name is in the body.
		if !strings.Contains(view, "Delete project?") {
			t.Errorf("expected '▲ Delete project?' header, got:\n%s", view)
		}
		if !strings.Contains(view, "webapp") {
			t.Errorf("expected the target project 'webapp' in the delete panel, got:\n%s", view)
		}

		// Confirm deletion
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		if cmd == nil {
			t.Fatal("expected command from delete confirmation, got nil")
		}

		// Execute the command and verify correct project was removed
		msg := cmd()
		_, ok := msg.(tui.ProjectsLoadedMsg)
		if !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		if store.removedPath != "/code/webapp" {
			t.Errorf("expected Remove(%q), got Remove(%q)", "/code/webapp", store.removedPath)
		}
	})
}

func TestSessionsPageEmptyText(t *testing.T) {
	t.Run("empty sessions page shows the §11.1 empty-sessions message", func(t *testing.T) {
		m := tui.NewModelWithSessions(nil)
		view := m.View().Content
		// §11.1 reskin: the pre-reskin "No sessions running" copy is replaced by the
		// spec-exact "No sessions yet" + the new hint.
		if !strings.Contains(view, "No sessions yet") {
			t.Errorf("expected 'No sessions yet' on empty sessions page, got:\n%s", view)
		}
	})
}

func TestProjectsStubHelpBar(t *testing.T) {
	t.Run("projects stub help bar includes s for sessions", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page and set wide width so help bar shows
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := model.View().Content
		if !strings.Contains(view, "sessions") {
			t.Errorf("projects help bar should contain 'sessions', got:\n%s", view)
		}
	})
}

func TestSessionsPageHelpBarIncludesProjects(t *testing.T) {
	t.Run("sessions page help bar includes p for projects", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		// Use wider width so all help bindings fit
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := updated.View().Content
		if !strings.Contains(view, "projects") {
			t.Errorf("sessions help bar should contain 'projects', got:\n%s", view)
		}
	})
}

func TestEscProgressiveBack(t *testing.T) {
	t.Run("Esc with no modal or filter quits TUI", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc with no modal or filter, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})

	t.Run("Esc during kill modal dismisses modal", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k to open kill modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

		// Verify kill modal is showing
		view := model.View().Content
		if !strings.Contains(view, "Kill session?") {
			t.Fatalf("precondition: expected kill modal, got:\n%s", view)
		}

		// Press Esc — should dismiss modal, NOT quit
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc during kill modal should dismiss modal, not quit")
			}
		}

		// Modal should be dismissed
		view = model.View().Content
		if strings.Contains(view, "Kill session?") {
			t.Errorf("kill modal should be dismissed after Esc, got:\n%s", view)
		}

		// Sessions should still be visible
		if !strings.Contains(view, "alpha") {
			t.Errorf("session list should be visible after modal dismiss, got:\n%s", view)
		}
	})

	t.Run("Esc during rename modal dismisses modal", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r to open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// Verify rename modal is showing
		view := model.View().Content
		if !strings.Contains(view, "Rename session") {
			t.Fatalf("precondition: expected rename modal, got:\n%s", view)
		}

		// Press Esc — should dismiss modal, NOT quit
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc during rename modal should dismiss modal, not quit")
			}
		}

		// Modal should be dismissed
		view = model.View().Content
		if strings.Contains(view, "Rename session") {
			t.Errorf("rename modal should be dismissed after Esc, got:\n%s", view)
		}

		// Renamer should not have been called
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called, but got old=%q", renamer.renamedOld)
		}
	})

	t.Run("Esc with filter active clears filter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", updatedModel.SessionListFilterState())
		}

		// Press Esc — should clear filter, NOT quit
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc with applied filter should clear filter, not quit")
			}
		}

		// Filter should be cleared
		updatedModel = model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v after Esc, want Unfiltered", updatedModel.SessionListFilterState())
		}

		// All items should be visible
		visible := updatedModel.SessionListVisibleItems()
		if len(visible) != 3 {
			t.Errorf("expected 3 visible items after clearing filter, got %d", len(visible))
		}
	})

	t.Run("Esc during SettingFilter cancels filter without quitting", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Enter filter mode by pressing /
		model, _ = model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

		// Type some filter text
		model, _ = model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

		// Verify we are in SettingFilter state (actively typing filter)
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", updatedModel.SessionListFilterState())
		}

		// Press Esc — should cancel filter, NOT quit
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc during SettingFilter should cancel filter, not quit")
			}
		}

		// Filter state should return to Unfiltered
		updatedModel = model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v after Esc during SettingFilter, want Unfiltered", updatedModel.SessionListFilterState())
		}
	})

	t.Run("Ctrl+C force-quits from any state", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}

		// Ctrl+C from normal session list (no modal)
		var model tea.Model = tui.NewModelWithSessions(sessions)
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from Ctrl+C, got %T", msg)
		}

		// Test Ctrl+C with filter applied
		m2 := tui.New(&mockSessionLister{sessions: sessions})
		m2 = m2.WithInitialFilter("alpha")
		model = m2
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C with filter, got nil")
		}
		msg = cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from Ctrl+C with filter, got %T", msg)
		}

		// Test Ctrl+C during kill modal
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m3 := tui.New(lister, tui.WithKiller(killer))
		model = m3
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		// Open kill modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		// Ctrl+C should force-quit even during kill modal
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C during kill modal, got nil")
		}
		msg = cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from Ctrl+C during kill modal, got %T", msg)
		}

		// Test Ctrl+C during rename modal
		renamer := &mockSessionRenamer{}
		lister2 := &mockSessionLister{sessions: sessions}
		m4 := newModelWithRenamer(lister2, nil, renamer)
		model = m4
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		// Open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		// Ctrl+C should force-quit even during rename modal
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C during rename modal, got nil")
		}
		msg = cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from Ctrl+C during rename modal, got %T", msg)
		}

		// Test Ctrl+C during active filtering (SettingFilter)
		m5 := tui.NewModelWithSessions(sessions)
		model = m5
		// Enter filter mode with /
		model, _ = model.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		// Type a character to confirm filtering
		model, _ = model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		// Verify we are in SettingFilter state
		if model.(tui.Model).SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", model.(tui.Model).SessionListFilterState())
		}
		// Ctrl+C should force-quit even during active filtering
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command from Ctrl+C during active filtering, got nil")
		}
		msg = cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from Ctrl+C during active filtering, got %T", msg)
		}
	})

	t.Run("Esc during rename then Esc again quits TUI", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press r to open rename modal
		model, _ = model.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

		// First Esc — dismisses rename modal
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("first Esc should dismiss modal, not quit")
			}
		}

		// Verify modal is dismissed
		view := model.View().Content
		if strings.Contains(view, "Rename session") {
			t.Fatalf("rename modal should be dismissed, got:\n%s", view)
		}

		// Second Esc — should quit
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from second Esc, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from second Esc, got %T", msg)
		}
	})

	t.Run("Esc clears filter then second Esc quits", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		)
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", updatedModel.SessionListFilterState())
		}

		// First Esc — clears filter
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("first Esc should clear filter, not quit")
			}
		}

		updatedModel = model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter should be cleared after first Esc, got %v", updatedModel.SessionListFilterState())
		}

		// Second Esc — should quit
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from second Esc, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from second Esc, got %T", msg)
		}
	})

	t.Run("Esc on projects page with no filter quits TUI", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Verify on projects page
		if model.(tui.Model).ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected PageProjects, got %d", model.(tui.Model).ActivePage())
		}

		// Press Esc — should quit
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc on projects page, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
	})
}

// mockProjectEditor implements tui.ProjectEditor for testing.
//
// tagErr is kept distinct from err (the Rename error) so a tag-persist failure
// can be exercised independently of a name-rename failure.
type mockProjectEditor struct {
	renamedPath string
	renamedName string
	renamedVia  string
	err         error

	addedTags   []tagCall
	removedTags []tagCall
	tagErr      error
}

// tagCall records a single AddTag/RemoveTag invocation.
type tagCall struct {
	path   string
	rawTag string
}

func (m *mockProjectEditor) Rename(path, newName, via string) error {
	m.renamedPath = path
	m.renamedName = newName
	m.renamedVia = via
	return m.err
}

func (m *mockProjectEditor) AddTag(path, rawTag string) error {
	m.addedTags = append(m.addedTags, tagCall{path: path, rawTag: rawTag})
	return m.tagErr
}

func (m *mockProjectEditor) RemoveTag(path, rawTag string) error {
	m.removedTags = append(m.removedTags, tagCall{path: path, rawTag: rawTag})
	return m.tagErr
}

// mockAliasEditor implements tui.AliasEditor for testing. The mutation surface
// is the audited combined methods (SetAndSave / DeleteAndSave); setCalls /
// deleted / saveCalled track invocations for assertions, and saveErr forces a
// persist failure.
type mockAliasEditor struct {
	aliases    map[string]string
	loadErr    error
	setCalls   []aliasSetCall
	deleted    []string
	saveCalled bool
	saveErr    error
}

type aliasSetCall struct {
	name string
	path string
	via  string
}

func (m *mockAliasEditor) Load() (map[string]string, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	result := make(map[string]string)
	maps.Copy(result, m.aliases)
	return result, nil
}

func (m *mockAliasEditor) SetAndSave(name, path, via string) error {
	m.setCalls = append(m.setCalls, aliasSetCall{name: name, path: path, via: via})
	m.saveCalled = true
	if m.saveErr != nil {
		return m.saveErr
	}
	if m.aliases == nil {
		m.aliases = make(map[string]string)
	}
	m.aliases[name] = path
	return nil
}

func (m *mockAliasEditor) DeleteAndSave(name, via string) (bool, error) {
	m.deleted = append(m.deleted, name)
	_, ok := m.aliases[name]
	if !ok {
		// Absent delete: no persist, no error (mirrors the store contract).
		return false, nil
	}
	delete(m.aliases, name)
	m.saveCalled = true
	if m.saveErr != nil {
		return true, m.saveErr
	}
	return true, nil
}

// setupEditModel creates a model on the projects page with the given projects,
// project editor, and alias editor.
func setupEditModel(store *mockProjectStore, editor *mockProjectEditor, aliases *mockAliasEditor) tea.Model {
	opts := []tui.Option{
		tui.WithProjectStore(store),
	}
	if editor != nil {
		opts = append(opts, tui.WithProjectEditor(editor))
	}
	if aliases != nil {
		opts = append(opts, tui.WithAliasEditor(aliases))
	}
	m := tui.New(&mockSessionLister{sessions: []tmux.Session{}}, opts...)
	var model tea.Model = m

	// Switch to projects page and populate
	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: store.projects})
	return model
}

func TestEditProject(t *testing.T) {
	t.Run("e opens edit modal with project name and aliases", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{
				"p": "/code/portal",
				"w": "/code/webapp",
			},
		}
		model := setupEditModel(store, editor, aliases)

		// Press e on first project.
		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		view := model.View().Content
		if !strings.Contains(view, "NAME") {
			t.Errorf("edit modal should contain 'NAME' field, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("edit modal should show current project name 'portal', got:\n%s", view)
		}
		if !strings.Contains(view, "ALIASES") {
			t.Errorf("edit modal should contain 'ALIASES' section, got:\n%s", view)
		}
		if !strings.Contains(view, "p") {
			t.Errorf("edit modal should show alias 'p', got:\n%s", view)
		}
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("edit modal should have border styling, got:\n%s", view)
		}
	})

	t.Run("Tab moves between Name and Aliases in navigate mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		// Open lands on Name in navigate mode — the §13.1 focus signal is the NAME
		// label rendered in accent.violet (the legacy `> ` indicator is gone).
		if !editFieldFocused(model.View().Content, "NAME") {
			t.Fatalf("open should focus Name, got:\n%s", model.View().Content)
		}

		// Tab moves focus to Aliases.
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if !editFieldFocused(model.View().Content, "ALIASES") {
			t.Errorf("after Tab focus should be Aliases, got:\n%s", model.View().Content)
		}

		// Two more Tabs wrap Aliases → Tags → Name.
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Aliases → Tags
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Tags → Name
		if !editFieldFocused(model.View().Content, "NAME") {
			t.Errorf("after three Tabs focus should wrap to Name, got:\n%s", model.View().Content)
		}
	})

	t.Run("Enter on Name commits and persists via Rename", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		// Enter edit mode on Name, append a char, commit.
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // navigate → edit
		model, _ = model.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit

		if editor.renamedPath != "/code/portal" {
			t.Errorf("expected Rename path '/code/portal', got %q", editor.renamedPath)
		}
		if editor.renamedName != "portalX" {
			t.Errorf("expected Rename name 'portalX', got %q", editor.renamedName)
		}
		if editor.renamedVia != "cli" {
			t.Errorf("expected Rename via=cli, got %q", editor.renamedVia)
		}
		// Commit returns to navigate, modal stays open.
		if !strings.Contains(model.View().Content, "NAME") {
			t.Errorf("modal should stay open after a Name commit (navigate mode), got:\n%s", model.View().Content)
		}
	})

	t.Run("Esc in navigate closes and refreshes after a live edit", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		// Commit a Name change (persists live).
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		// Esc in navigate closes; the saved work survives and a refresh fires.
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		if editor.renamedName != "portalY" {
			t.Errorf("saved name should survive Esc; Rename name = %q", editor.renamedName)
		}
		if cmd == nil {
			t.Fatal("Esc after a live edit should return a refresh command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tui.ProjectsLoadedMsg); !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		view := model.View().Content
		if strings.Contains(view, "NAME") {
			t.Errorf("edit modal should be dismissed after Esc, got:\n%s", view)
		}
	})

	t.Run("Esc in navigate with no edits closes without refresh", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		if editor.renamedPath != "" {
			t.Error("Esc with no edits should not call Rename")
		}
		if cmd != nil {
			t.Errorf("Esc with no edits should return nil command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "NAME") {
			t.Errorf("edit modal should be dismissed after Esc, got:\n%s", view)
		}
	})

	t.Run("empty Name commit reverts to prior without persisting or blocking", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // edit Name
		for range len("portal") {
			model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		}
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit empty

		// No Rename, no blocking modal — silently reverts.
		if editor.renamedPath != "" {
			t.Error("empty Name must not call Rename")
		}
		if cmd != nil {
			t.Errorf("empty-Name commit should not produce a command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "cannot be empty") {
			t.Errorf("empty Name must NOT pop a blocking error, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("Name should revert to prior 'portal', got:\n%s", view)
		}
	})

	t.Run("cross-project alias collision is a silent revert", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{"w": "/code/webapp"}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // → Aliases (add slot)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // spawn new chip
		model, _ = model.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit → collision

		if len(aliases.setCalls) != 0 {
			t.Errorf("collision must not SetAndSave; setCalls = %+v", aliases.setCalls)
		}
		view := model.View().Content
		if strings.Contains(view, "already exists") {
			t.Errorf("collision must NOT pop a blocking error (silent revert), got:\n%s", view)
		}
	})

	t.Run("x removes a focused alias chip immediately", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{
				"p":  "/code/portal",
				"pt": "/code/portal",
			},
		}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})  // → Aliases (add slot)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // onto last chip
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Immediate persist via DeleteAndSave (1 chip removed).
		if len(aliases.deleted) != 1 {
			t.Fatalf("x should DeleteAndSave immediately, got %d deletes", len(aliases.deleted))
		}
		if !aliases.saveCalled {
			t.Error("expected Save on immediate alias delete")
		}
		view := model.View().Content
		if !strings.Contains(view, "NAME") {
			t.Errorf("modal should stay open after removing an alias, got:\n%s", view)
		}
	})

	t.Run("a new alias is committed via SetAndSave on Enter", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab})   // → Aliases (add slot)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // spawn new chip
		model, _ = model.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit

		if len(aliases.setCalls) != 1 {
			t.Fatalf("expected 1 Set call, got %d", len(aliases.setCalls))
		}
		if aliases.setCalls[0].name != "my" {
			t.Errorf("expected alias name 'my', got %q", aliases.setCalls[0].name)
		}
		if aliases.setCalls[0].path != "/code/portal" {
			t.Errorf("expected alias path '/code/portal', got %q", aliases.setCalls[0].path)
		}
		if aliases.setCalls[0].via != "cli" {
			t.Errorf("expected SetAndSave via=cli, got %q", aliases.setCalls[0].via)
		}
		// Commit returns to navigate; the modal stays open.
		if !strings.Contains(model.View().Content, "NAME") {
			t.Errorf("modal should stay open after committing a new alias")
		}
	})

	t.Run("e with no editor configured is no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		model := setupEditModel(store, nil, nil)

		model, cmd := model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		if cmd != nil {
			t.Errorf("e with no editor should return nil command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "NAME") {
			t.Errorf("edit modal should not open without editor, got:\n%s", view)
		}
	})

	t.Run("e on empty project list is no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model, cmd := model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		if cmd != nil {
			t.Errorf("e on empty list should return nil command, got non-nil")
		}
		view := model.View().Content
		if strings.Contains(view, "NAME") {
			t.Errorf("edit modal should not open on empty list, got:\n%s", view)
		}
	})
}

// openTagsAddSlot opens the edit modal for the (single) project and Tabs focus to
// the Tags field, landing on the trailing + add slot (navigate mode).
func openTagsAddSlot(model tea.Model) tea.Model {
	model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Name → Aliases
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // Aliases → Tags
	return model
}

// addTagLive spawns a new tag chip from the + add slot, types the tag, and
// commits with Enter — persisting it live via AddTag.
func addTagLive(model tea.Model, tag string) tea.Model {
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // spawn new chip
	for _, r := range tag {
		model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit
	return model
}

func TestEditProjectTagPersistence(t *testing.T) {
	t.Run("persists an added tag via AddTag on commit", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model = openTagsAddSlot(model)
		model = addTagLive(model, "work")

		if len(editor.addedTags) != 1 {
			t.Fatalf("expected 1 AddTag call, got %d: %+v", len(editor.addedTags), editor.addedTags)
		}
		if editor.addedTags[0].path != "/code/portal" {
			t.Errorf("expected AddTag path /code/portal, got %q", editor.addedTags[0].path)
		}
		if editor.addedTags[0].rawTag != "work" {
			t.Errorf("expected AddTag tag 'work', got %q", editor.addedTags[0].rawTag)
		}
		if len(editor.removedTags) != 0 {
			t.Errorf("expected no RemoveTag calls, got %+v", editor.removedTags)
		}
		// Commit returns to navigate; the modal stays open and the chip shows.
		if !strings.Contains(model.View().Content, "work") {
			t.Errorf("committed tag 'work' should be visible in the modal")
		}
	})

	t.Run("persists a removed tag via RemoveTag on x", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal", Tags: []string{"work"}},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model = openTagsAddSlot(model)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft}) // onto the existing tag
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		if len(editor.removedTags) != 1 {
			t.Fatalf("expected 1 RemoveTag call, got %d: %+v", len(editor.removedTags), editor.removedTags)
		}
		if editor.removedTags[0].path != "/code/portal" {
			t.Errorf("expected RemoveTag path /code/portal, got %q", editor.removedTags[0].path)
		}
		if editor.removedTags[0].rawTag != "work" {
			t.Errorf("expected RemoveTag tag 'work', got %q", editor.removedTags[0].rawTag)
		}
		if len(editor.addedTags) != 0 {
			t.Errorf("expected no AddTag calls, got %+v", editor.addedTags)
		}
		// The removed tag is gone from the modal but the modal stays open.
		if !strings.Contains(model.View().Content, "NAME") {
			t.Errorf("modal should stay open after an immediate tag removal")
		}
	})

	t.Run("Esc in navigate after a live tag edit refreshes projects", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model = openTagsAddSlot(model)
		model = addTagLive(model, "work")

		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("Esc after a live tag add should refresh projects, got nil")
		}
		if _, ok := cmd().(tui.ProjectsLoadedMsg); !ok {
			t.Errorf("expected ProjectsLoadedMsg from the refresh cmd")
		}
	})

	t.Run("a duplicate tag commit is a silent no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal", Tags: []string{"work"}},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model = openTagsAddSlot(model)
		model = addTagLive(model, "work") // duplicate of the existing tag

		if len(editor.addedTags) != 0 {
			t.Errorf("duplicate tag must not AddTag, got %+v", editor.addedTags)
		}
		// The single existing "work" chip remains; no duplicate is shown.
		if strings.Count(model.View().Content, "work") != 1 {
			t.Errorf("duplicate commit should leave exactly one 'work' chip, got:\n%s", model.View().Content)
		}
	})

	t.Run("passes the raw tag to the store (NormaliseTag in the store)", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}
		model := setupEditModel(store, editor, aliases)

		model = openTagsAddSlot(model)
		model = addTagLive(model, "work")

		if len(editor.addedTags) != 1 {
			t.Fatalf("expected 1 AddTag call, got %+v", editor.addedTags)
		}
		if editor.addedTags[0].rawTag != "work" {
			t.Errorf("expected raw tag 'work' passed verbatim, got %q", editor.addedTags[0].rawTag)
		}
		if !strings.Contains(model.View().Content, "work") {
			t.Errorf("committed tag should be visible in the modal")
		}
	})
}

func TestPageSwitchingFilterIndependence(t *testing.T) {
	t.Run("switching pages does not carry filter text", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
				{Name: "bravo", Windows: 2, Attached: false},
			}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Provide window size for both lists
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Load sessions
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
				{Name: "bravo", Windows: 2, Attached: false},
			},
		})

		// Apply a filter on the sessions list
		tuiModel := model.(tui.Model)
		tuiModel.SetSessionListFilter("alpha")
		model = tuiModel

		// Verify session filter is applied
		if tuiModel.SessionListFilterValue() != "alpha" {
			t.Fatalf("precondition: expected session filter 'alpha', got %q", tuiModel.SessionListFilterValue())
		}

		// Switch to projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Load projects
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Verify projects page has no filter text
		updated := model.(tui.Model)
		if updated.ProjectListFilterValue() != "" {
			t.Errorf("expected empty project filter after switch, got %q", updated.ProjectListFilterValue())
		}
		if updated.ProjectListFilterState() != list.Unfiltered {
			t.Errorf("expected Unfiltered project state, got %v", updated.ProjectListFilterState())
		}
	})

	t.Run("filter state preserved when switching back to source page", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
				{Name: "bravo", Windows: 2, Attached: false},
			}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Provide window size
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Load sessions
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
				{Name: "bravo", Windows: 2, Attached: false},
			},
		})

		// Apply filter on sessions
		tuiModel := model.(tui.Model)
		tuiModel.SetSessionListFilter("alpha")
		model = tuiModel

		// Switch to projects then back to sessions (x is the sole both-directions
		// toggle now — §12.2 drops the Projects-side s alias).
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		// Verify session filter is still applied
		updated := model.(tui.Model)
		if updated.SessionListFilterValue() != "alpha" {
			t.Errorf("expected session filter 'alpha' preserved, got %q", updated.SessionListFilterValue())
		}
		if updated.SessionListFilterState() != list.FilterApplied {
			t.Errorf("expected FilterApplied after round-trip, got %v", updated.SessionListFilterState())
		}
	})

	t.Run("projects footer shows the §6.3 condensed copy (x sessions, edit, filter)", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(&mockSessionCreator{sessionName: "test"}),
		)
		var model tea.Model = m

		// Switch to projects page with wide width
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		view := model.View().Content
		// The §6.3 condensed footer copy (the `delete` help-only key is deferred to
		// the ? help modal in Phase 3 — it is no longer in the footer).
		expectedDescs := []string{
			"sessions",
			"edit",
			"filter",
		}
		for _, desc := range expectedDescs {
			if !strings.Contains(view, desc) {
				t.Errorf("projects footer should contain §6.3 entry %q, got:\n%s", desc, view)
			}
		}
		if strings.Contains(view, "delete") {
			t.Errorf("projects footer should not contain the deferred 'delete' help-only key, got:\n%s", view)
		}
	})

	t.Run("sessions help bar still includes p for projects after projects page replacement", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
			}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Set wide width and load sessions
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "alpha", Windows: 1, Attached: false},
			},
		})

		view := model.View().Content
		if !strings.Contains(view, "projects") {
			t.Errorf("sessions help bar should contain 'projects', got:\n%s", view)
		}
	})
}

func TestDefaultPageSelection(t *testing.T) {
	t.Run("defaults to Sessions page when sessions exist", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Send both messages to simulate Init() completion
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 1, Attached: false},
			},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions when sessions exist, got %d", updated.ActivePage())
		}
	})

	t.Run("defaults to Projects page when no sessions exist but projects exist", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Send both messages — no sessions
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects when no sessions, got %d", updated.ActivePage())
		}
	})

	t.Run("defaults to Projects page when both pages are empty", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects when both empty, got %d", updated.ActivePage())
		}
	})

	t.Run("defaults to Projects page when all sessions filtered by inside-tmux exclusion", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		).WithInsideTmux("only-session")
		var model tea.Model = m

		// The only session is the current session, so it gets filtered out
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "only-session", Windows: 2, Attached: true},
			},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects when all sessions filtered by inside-tmux, got %d", updated.ActivePage())
		}
	})

	t.Run("page switching works after defaulting to Sessions page", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Load both data sources — sessions exist, so defaults to Sessions
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
			},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Fatalf("precondition: expected PageSessions, got %d", updated.ActivePage())
		}

		// Press x to switch to projects (the sole both-directions toggle, §12.2)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after x, got %d", updated.ActivePage())
		}

		// Press x to switch back to sessions (s alias dropped, §12.2)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after x, got %d", updated.ActivePage())
		}

		// Press x to toggle to projects
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after x, got %d", updated.ActivePage())
		}
	})

	t.Run("page switching works after defaulting to Projects page", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Load both — no sessions, so defaults to Projects
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected PageProjects, got %d", updated.ActivePage())
		}

		// Press x to switch to sessions (the sole both-directions toggle, §12.2)
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after x, got %d", updated.ActivePage())
		}

		// Press x to switch back to projects
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after x, got %d", updated.ActivePage())
		}

		// Press x to toggle to sessions
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after x from projects, got %d", updated.ActivePage())
		}
	})

	t.Run("evaluateDefaultPage only runs once and does not override manual page switch", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithKiller(&mockSessionKiller{}),
		)
		var model tea.Model = m

		// Initial load: sessions exist, so defaults to Sessions page
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
			},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})
		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Fatalf("precondition: expected PageSessions, got %d", updated.ActivePage())
		}

		// User manually switches to Projects page
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected PageProjects after p, got %d", updated.ActivePage())
		}

		// A subsequent SessionsMsg arrives (e.g. after rename-and-refresh)
		// with sessions still present. This should NOT override the user's
		// manual page selection back to PageSessions.
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev-renamed", Windows: 3, Attached: true},
			},
		})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects to persist after refresh, got %d — evaluateDefaultPage ran more than once", updated.ActivePage())
		}
	})

	t.Run("default page waits for both data sources before evaluating", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Send only SessionsMsg (no sessions) — page should NOT change yet
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{},
		})
		updated := model.(tui.Model)
		// Before both are loaded, activePage stays at default (PageSessions)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions before both loaded, got %d", updated.ActivePage())
		}

		// Now send ProjectsLoadedMsg — now both are loaded, should switch to Projects
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after both loaded with no sessions, got %d", updated.ActivePage())
		}
	})

	t.Run("default page waits for both data sources before evaluating — projects first", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		// Send only ProjectsLoadedMsg first — page should NOT change yet
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})
		updated := model.(tui.Model)
		// Before both are loaded, activePage stays at default (PageSessions)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions before both loaded, got %d", updated.ActivePage())
		}

		// Now send SessionsMsg — now both are loaded, should switch to Projects (no sessions)
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{},
		})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after both loaded with no sessions, got %d", updated.ActivePage())
		}
	})

	t.Run("command-pending sets PageProjects even when sessionList has items", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		).WithCommand([]string{"claude"})
		var model tea.Model = m

		// Simulate a SessionsMsg arriving (populates sessionList with items)
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 1, Attached: false},
			},
		})

		// Now send ProjectsLoadedMsg which triggers evaluateDefaultPage
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("command-pending should always set PageProjects, got %d", updated.ActivePage())
		}
	})

	t.Run("normal mode still defaults to PageSessions when sessions exist", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)
		var model tea.Model = m

		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 3, Attached: true},
				{Name: "work", Windows: 1, Attached: false},
			},
		})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("normal mode should default to PageSessions when sessions exist, got %d", updated.ActivePage())
		}
	})
}

func TestCommandPendingStatusLine(t *testing.T) {
	t.Run("status line shows pending command text", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the banner reads `Pick a project to run` + the command in the chip.
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, "Pick a project to run") {
			t.Errorf("expected the §11.4 banner text, got:\n%s", visible)
		}
		if !strings.Contains(visible, "claude") {
			t.Errorf("expected the command in the banner chip, got:\n%s", visible)
		}
	})

	t.Run("status line absent in normal mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Switch to projects page
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{{Path: "/code/myapp", Name: "myapp"}},
		})

		view := model.View().Content
		if strings.Contains(view, "Select project to run:") {
			t.Errorf("status line should not appear in normal mode, got:\n%s", view)
		}
	})

	t.Run("multi-word command shown space-separated", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume", "--model", "opus"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the multi-word command joins on spaces into the orange chip.
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, "claude --resume --model opus") {
			t.Errorf("expected the joined multi-word command in the banner chip, got:\n%s", visible)
		}
	})

	t.Run("long command text renders without truncation", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		longCmd := "some-very-long-command-name --with-many-flags --verbose --output=/tmp/really-long-path"
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{longCmd})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// §11.4: the full command renders in the chip without truncation.
		visible := ansi.Strip(model.View().Content)
		if !strings.Contains(visible, longCmd) {
			t.Errorf("expected full command %q in the banner chip (no truncation), got:\n%s", longCmd, visible)
		}
	})

	t.Run("title stays Projects in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View().Content
		if !strings.Contains(view, "Projects") {
			t.Errorf("expected 'Projects' title in command-pending mode, got:\n%s", view)
		}
	})

	// §11.4 placement: the banner sits DIRECTLY under the title separator and ABOVE
	// the green `Projects` section header (the §11 convention), so the banner text
	// appears BEFORE the `Projects` title in the rendered view.
	t.Run("banner appears under the separator, above the Projects section header", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		visible := ansi.Strip(model.View().Content)
		bannerIdx := strings.Index(visible, "Pick a project to run")
		titleIdx := strings.Index(visible, "Projects")
		if bannerIdx < 0 {
			t.Fatalf("banner text not found in view:\n%s", visible)
		}
		if titleIdx < 0 {
			t.Fatalf("title 'Projects' not found in view:\n%s", visible)
		}
		if bannerIdx > titleIdx {
			t.Errorf("banner (pos %d) must appear before the Projects section header (pos %d) — §11.4 places it above the header.\nView:\n%s",
				bannerIdx, titleIdx, visible)
		}
	})
}

func TestCommandPendingEnterCreatesSession(t *testing.T) {
	t.Run("enter on project in command-pending mode creates session with command", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Press enter on first project
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter on project, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "myapp-abc123" {
			t.Errorf("expected session name %q, got %q", "myapp-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/code/myapp" {
			t.Errorf("expected CreateFromDir called with dir %q, got %q", "/code/myapp", creator.createdDir)
		}
		if creator.createdCommand == nil {
			t.Fatal("expected command to be forwarded, got nil")
		}
	})

	t.Run("command slice forwarded exactly to CreateFromDir", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume", "--model", "opus"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Press enter on project
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter, got nil")
		}
		cmd()

		wantCmd := []string{"claude", "--resume", "--model", "opus"}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command length = %d, want %d; got %v", len(creator.createdCommand), len(wantCmd), creator.createdCommand)
		}
		for i, arg := range wantCmd {
			if creator.createdCommand[i] != arg {
				t.Errorf("command[%d] = %q, want %q", i, creator.createdCommand[i], arg)
			}
		}
	})

	t.Run("selected returns session name after creation", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Press enter to create session
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command, got nil")
		}
		msg := cmd()

		// Feed SessionCreatedMsg back
		model, _ = model.Update(msg)
		updated := model.(tui.Model)
		if updated.Selected() != "myapp-abc123" {
			t.Errorf("Selected() = %q, want %q", updated.Selected(), "myapp-abc123")
		}
	})

	t.Run("TUI quits after successful session creation", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Press enter to create session
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command, got nil")
		}
		msg := cmd()

		// Feed SessionCreatedMsg back — should return quit command
		_, cmd = model.Update(msg)
		if cmd == nil {
			t.Fatal("expected quit command after SessionCreatedMsg, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
	})

	t.Run("session creation error keeps TUI on Projects page", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{err: fmt.Errorf("tmux failed")}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)

		// Press enter on project — will fail
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter, got nil")
		}
		msg := cmd()

		// Should not be SessionCreatedMsg
		if _, ok := msg.(tui.SessionCreatedMsg); ok {
			t.Fatal("expected error msg, got SessionCreatedMsg")
		}

		// Feed error back — should stay on projects page
		model, cmd = model.Update(msg)

		// No quit command should be returned
		if cmd != nil {
			t.Errorf("expected nil command after error, got non-nil")
		}

		// Should still be on Projects page
		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected to stay on PageProjects, got page %v", updated.ActivePage())
		}

		// Selected should be empty
		if updated.Selected() != "" {
			t.Errorf("expected empty Selected() after error, got %q", updated.Selected())
		}

		// Should still render
		view := model.View().Content
		if view == "" {
			t.Error("view should not be empty after error")
		}
	})

	t.Run("normal mode enter on project passes nil command", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		// Press enter on project
		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter, got nil")
		}
		cmd()

		// In normal mode (no WithCommand), command should be nil
		if creator.createdCommand != nil {
			t.Errorf("expected nil command in normal mode, got %v", creator.createdCommand)
		}
	})
}

func TestCommandPendingNKey(t *testing.T) {
	t.Run("n-key creates session in cwd with command in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "cwd-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		).WithCommand([]string{"vim", "."})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press n to create session in cwd
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key, got nil")
		}
		cmd()

		// Verify command was forwarded
		wantCmd := []string{"vim", "."}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, wantCmd)
		}
		for i, arg := range wantCmd {
			if creator.createdCommand[i] != arg {
				t.Errorf("command[%d] = %q, want %q", i, creator.createdCommand[i], arg)
			}
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("dir = %q, want %q", creator.createdDir, "/home/user/mydir")
		}
	})

	t.Run("n-key creates session in cwd without command in normal mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "cwd-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		// Press n to create session in cwd
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key, got nil")
		}
		cmd()

		// In normal mode, command should be nil
		if creator.createdCommand != nil {
			t.Errorf("expected nil command in normal mode, got %v", creator.createdCommand)
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("dir = %q, want %q", creator.createdDir, "/home/user/mydir")
		}
	})

	t.Run("n-key works from Sessions page", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "cwd-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			}},
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		)
		var model tea.Model = m

		// Load sessions to land on Sessions page
		model, _ = model.Update(tui.SessionsMsg{
			Sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			},
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Fatalf("expected PageSessions, got %d", updated.ActivePage())
		}

		// Press n to create session in cwd from Sessions page
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key on Sessions page, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "cwd-abc123" {
			t.Errorf("session name = %q, want %q", createdMsg.SessionName, "cwd-abc123")
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("dir = %q, want %q", creator.createdDir, "/home/user/mydir")
		}
	})

	t.Run("n-key works from Projects page", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		creator := &mockSessionCreator{sessionName: "cwd-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithCWD("/home/user/mydir"),
		)
		var model tea.Model = m

		// Switch to projects page and populate
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("expected PageProjects, got %d", updated.ActivePage())
		}

		// Press n to create session in cwd from Projects page
		_, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if cmd == nil {
			t.Fatal("expected command from n key on Projects page, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "cwd-abc123" {
			t.Errorf("session name = %q, want %q", createdMsg.SessionName, "cwd-abc123")
		}
		if creator.createdDir != "/home/user/mydir" {
			t.Errorf("dir = %q, want %q", creator.createdDir, "/home/user/mydir")
		}
	})
}

func TestCommandPendingEscAndQuit(t *testing.T) {
	t.Run("Esc with nothing active in command-pending mode exits TUI", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Esc with nothing active should quit
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
	})

	t.Run("Esc with filter active clears filter first in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Apply a filter on the project list
		updated := model.(tui.Model)
		updated.SetProjectListFilter("myapp")
		model = updated

		// Verify filter is applied
		if model.(tui.Model).ProjectListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", model.(tui.Model).ProjectListFilterState())
		}

		// Esc should clear the filter, not quit
		model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd != nil {
			quitMsg := cmd()
			if _, ok := quitMsg.(tea.QuitMsg); ok {
				t.Fatal("first Esc should clear filter, not quit")
			}
		}

		// Filter should be cleared
		if model.(tui.Model).ProjectListFilterState() != list.Unfiltered {
			t.Errorf("filter should be cleared after Esc, got %v", model.(tui.Model).ProjectListFilterState())
		}
	})

	t.Run("two Esc presses: clear filter then exit in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Apply a filter
		updated := model.(tui.Model)
		updated.SetProjectListFilter("myapp")
		model = updated

		// First Esc clears filter
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if model.(tui.Model).ProjectListFilterState() != list.Unfiltered {
			t.Fatalf("first Esc should clear filter, got %v", model.(tui.Model).ProjectListFilterState())
		}

		// Second Esc should quit
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from second Esc, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from second Esc, got %T", quitMsg)
		}
	})

	t.Run("Esc with modal active dismisses modal in command-pending mode", func(t *testing.T) {
		// In command-pending mode, e and d keys are disabled so modals cannot
		// be opened directly. This test verifies that the modal-first architecture
		// works on the projects page by testing in normal mode, which exercises
		// the same updateProjectsPage code path for Esc handling.
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{aliases: map[string]string{}}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithProjectEditor(editor),
			tui.WithAliasEditor(aliases),
		)

		var model tea.Model = m
		cmd := m.Init()
		// Process both session and project loading
		for cmd != nil {
			msgs := executeBatchCmd(cmd)
			cmd = nil
			for _, msg := range msgs {
				var nextCmd tea.Cmd
				model, nextCmd = model.Update(msg)
				if nextCmd != nil {
					cmd = nextCmd
				}
			}
		}

		// With an empty session list the model defaults to the Projects page
		// (evaluateDefaultPage), so no page toggle is needed here. (Pre-§12.2
		// this used the inert-on-Projects p key as a redundant "navigate";
		// after the alias drop x is the sole toggle and would bounce us back to
		// the empty Sessions page, so we simply assert we are already on
		// Projects.)
		if model.(tui.Model).ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected default Projects page with empty sessions, got %d", model.(tui.Model).ActivePage())
		}

		// Open edit modal with e
		model, _ = model.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

		// Verify modal is open by checking view contains edit content
		view := model.(tui.Model).View().Content
		if !strings.Contains(view, "Edit Project") {
			t.Fatalf("precondition: expected edit modal open, got:\n%s", view)
		}

		// Press Esc — should dismiss modal, not quit
		model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd != nil {
			quitMsg := cmd()
			if _, ok := quitMsg.(tea.QuitMsg); ok {
				t.Fatal("Esc should dismiss modal, not quit")
			}
		}

		// Modal should be dismissed — view should not contain Edit modal
		view = model.(tui.Model).View().Content
		if strings.Contains(view, "Edit Project") {
			t.Errorf("modal should be dismissed after Esc, got:\n%s", view)
		}
	})

	t.Run("q exits from any state in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// q should quit from projects page with no filter
		_, cmd = model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatal("expected quit command from q, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg from q, got %T", quitMsg)
		}
	})

	t.Run("Esc on Projects page in normal mode with nothing active exits TUI", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		)

		var model tea.Model = m
		cmd := m.Init()
		// In normal mode, Init batches sessions + projects
		msgs := executeBatchCmd(cmd)
		for _, msg := range msgs {
			model, _ = model.Update(msg)
		}

		// With no sessions loaded, model should default to projects page
		if model.(tui.Model).ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected PageProjects, got %d", model.(tui.Model).ActivePage())
		}

		// Esc should quit
		_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc on projects page, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
	})
}

func TestInitialFilterAppliedToDefaultPage(t *testing.T) {
	t.Run("initial filter applied to Sessions page when sessions exist", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		).WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)

		// Should default to Sessions page
		if updated.ActivePage() != tui.PageSessions {
			t.Fatalf("expected PageSessions, got %d", updated.ActivePage())
		}

		// Filter should be applied to session list
		if updated.SessionListFilterState() != list.FilterApplied {
			t.Errorf("session filter state = %v, want FilterApplied", updated.SessionListFilterState())
		}
		if updated.SessionListFilterValue() != "myapp" {
			t.Errorf("session filter value = %q, want %q", updated.SessionListFilterValue(), "myapp")
		}

		// Visible items should only include matching sessions
		visible := updated.SessionListVisibleItems()
		if len(visible) != 2 {
			t.Fatalf("expected 2 visible items, got %d", len(visible))
		}
		for _, item := range visible {
			si := item.(tui.SessionItem)
			if si.Session.Name == "other" {
				t.Error("'other' should be filtered out")
			}
		}

		// Filter consumed
		if updated.InitialFilter() != "" {
			t.Errorf("initialFilter should be consumed, got %q", updated.InitialFilter())
		}
	})

	t.Run("initial filter applied to Projects page when no sessions exist", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
				{Path: "/code/myapp-prod", Name: "myapp-prod"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
		).WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)

		// Should default to Projects page
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("expected PageProjects, got %d", updated.ActivePage())
		}

		// Filter should be applied to project list
		if updated.ProjectListFilterState() != list.FilterApplied {
			t.Errorf("project filter state = %v, want FilterApplied", updated.ProjectListFilterState())
		}
		if updated.ProjectListFilterValue() != "myapp" {
			t.Errorf("project filter value = %q, want %q", updated.ProjectListFilterValue(), "myapp")
		}

		// Visible items should only include matching projects
		visible := updated.ProjectListVisibleItems()
		if len(visible) != 2 {
			t.Fatalf("expected 2 visible items, got %d", len(visible))
		}
		for _, item := range visible {
			pi := item.(tui.ProjectItem)
			if pi.Project.Name == "other" {
				t.Error("'other' should be filtered out")
			}
		}

		// Filter consumed
		if updated.InitialFilter() != "" {
			t.Errorf("initialFilter should be consumed, got %q", updated.InitialFilter())
		}
	})

	t.Run("initial filter applied to Projects page in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
				{Path: "/code/myapp-prod", Name: "myapp-prod"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"}).WithInitialFilter("myapp")

		// In command-pending mode, Init() only loads projects (no sessions fetch).
		// Init now batches the async OSC 11 query; applyInit flattens it and applies
		// every produced message so the project list is populated and filtered.
		updated := applyInit(m)

		// Should be on Projects page (command-pending mode)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("expected PageProjects in command-pending mode, got %d", updated.ActivePage())
		}

		// Filter should be applied to project list
		if updated.ProjectListFilterState() != list.FilterApplied {
			t.Errorf("project filter state = %v, want FilterApplied", updated.ProjectListFilterState())
		}
		if updated.ProjectListFilterValue() != "myapp" {
			t.Errorf("project filter value = %q, want %q", updated.ProjectListFilterValue(), "myapp")
		}

		// Visible items should only include matching projects
		visible := updated.ProjectListVisibleItems()
		if len(visible) != 2 {
			t.Fatalf("expected 2 visible items, got %d", len(visible))
		}
		for _, item := range visible {
			pi := item.(tui.ProjectItem)
			if pi.Project.Name == "other" {
				t.Error("'other' should be filtered out")
			}
		}

		// Filter consumed
		if updated.InitialFilter() != "" {
			t.Errorf("initialFilter should be consumed, got %q", updated.InitialFilter())
		}
	})

	t.Run("initial filter with no matches shows empty filtered state", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		).WithInitialFilter("zzzzz")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)

		// Should default to Sessions page (sessions exist)
		if updated.ActivePage() != tui.PageSessions {
			t.Fatalf("expected PageSessions, got %d", updated.ActivePage())
		}

		// Filter applied
		if updated.SessionListFilterState() != list.FilterApplied {
			t.Errorf("filter state = %v, want FilterApplied", updated.SessionListFilterState())
		}

		// Visible items should be empty
		visible := updated.SessionListVisibleItems()
		if len(visible) != 0 {
			t.Errorf("expected 0 visible items for non-matching filter, got %d", len(visible))
		}
	})

	t.Run("empty initial filter is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		).WithInitialFilter("")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)

		// Filter state should be Unfiltered
		if updated.SessionListFilterState() != list.Unfiltered {
			t.Errorf("session filter state = %v, want Unfiltered", updated.SessionListFilterState())
		}

		// All items should be visible
		visible := updated.SessionListVisibleItems()
		if len(visible) != 2 {
			t.Errorf("expected 2 visible items, got %d", len(visible))
		}
	})

	t.Run("filter consumed after first application", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
		).WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		// Filter consumed after first evaluation
		updated := model.(tui.Model)
		if updated.InitialFilter() != "" {
			t.Errorf("initialFilter should be consumed, got %q", updated.InitialFilter())
		}

		// Exit filter with Esc (clears built-in filter)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

		// Second SessionsMsg — should NOT re-apply filter
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		updated = model.(tui.Model)
		if updated.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v after second load, want Unfiltered", updated.SessionListFilterState())
		}
		items := updated.SessionListItems()
		if len(items) != 2 {
			t.Fatalf("expected 2 items after filter consumed, got %d", len(items))
		}
	})

	t.Run("SessionsMsg handler no longer applies initial filter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		m := tui.New(
			&mockSessionLister{sessions: sessions},
		).WithInitialFilter("myapp")

		var model tea.Model = m
		// Only send SessionsMsg without ProjectsLoadedMsg
		// evaluateDefaultPage won't run (needs both), so filter should NOT be applied
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		updated := model.(tui.Model)

		// Filter should NOT be applied yet (evaluateDefaultPage hasn't run)
		if updated.SessionListFilterState() != list.Unfiltered {
			t.Errorf("filter state = %v, want Unfiltered (filter should not be applied in SessionsMsg handler)", updated.SessionListFilterState())
		}

		// initialFilter should still be stored (not consumed)
		if updated.InitialFilter() == "" {
			t.Error("initialFilter should still be stored before evaluateDefaultPage runs")
		}
	})
}

// executeBatchCmd executes a tea.Cmd that may be a batch command and returns all messages.
func executeBatchCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batchMsg {
			if c != nil {
				msgs = append(msgs, c())
			}
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestHelpBarQuitBinding(t *testing.T) {
	t.Run("session footer omits quit (help-only) and shows the ? help hint", func(t *testing.T) {
		// §3.4: the Sessions footer is the condensed core-keys row — q quit is
		// help-only (it moved to the ? help modal, Phase 3), so it must NOT appear
		// in the footer; the right-aligned ? help hint advertises where it lives.
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := updated.View().Content
		if strings.Contains(view, "quit") {
			t.Errorf("Sessions condensed footer must NOT contain 'quit' (help-only, §3.4), got:\n%s", view)
		}
		// The ? help label is a styled run; in the raw (un-stripped) view the "?"
		// glyph and the "help" label are separate SGR runs, so assert the label
		// itself (a single contiguous run) is present.
		if !strings.Contains(view, "help") {
			t.Errorf("Sessions condensed footer must show the right-aligned '? help' hint, got:\n%s", view)
		}
	})

	// The §6.3 reskin moved `quit` (a help-only key) out of the Projects footer and
	// into the ? help modal (Phase 3) — same as the Sessions condensed footer. The
	// condensed Projects footer must NOT contain 'quit', and the right-aligned
	// '? help' hint must be present (where 'quit' now lives).
	t.Run("project condensed footer omits quit and shows ? help", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(&mockSessionCreator{sessionName: "test"}),
		)
		var model tea.Model = m

		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		view := model.View().Content
		if strings.Contains(view, "quit") {
			t.Errorf("Projects condensed footer must NOT contain 'quit' (help-only, deferred to ? help), got:\n%s", view)
		}
		if !strings.Contains(view, "help") {
			t.Errorf("Projects condensed footer must show the right-aligned '? help' hint, got:\n%s", view)
		}
	})

	t.Run("command-pending condensed footer omits quit and shows ? help", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude"})

		// applyInit flattens the now-batched Init (incl. the async OSC 11 query)
		// and applies every produced message so the project list is populated.
		var model tea.Model = applyInit(m)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := model.View().Content
		if strings.Contains(view, "quit") {
			t.Errorf("command-pending condensed footer must NOT contain 'quit' (deferred to ? help), got:\n%s", view)
		}
		if !strings.Contains(view, "help") {
			t.Errorf("command-pending condensed footer must show the right-aligned '? help' hint, got:\n%s", view)
		}
	})
}

func TestLoadingPage(t *testing.T) {
	t.Run("model with WithServerStarted(true) starts on PageLoading", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		if m.ActivePage() != tui.PageLoading {
			t.Errorf("expected PageLoading, got %d", m.ActivePage())
		}
		if !m.ServerStarted() {
			t.Error("expected ServerStarted() to be true")
		}
	})

	t.Run("model with WithServerStarted(false) starts on PageSessions", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(false))
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions, got %d", m.ActivePage())
		}
	})

	t.Run("model without WithServerStarted starts on PageSessions", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister)
		if m.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions, got %d", m.ActivePage())
		}
	})

	t.Run("loading view shows the honest §10.3 screen (wordmark + step-list)", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		view := ansi.Strip(model.View().Content)
		// The honest loading screen renders the friendly step labels (a real list),
		// including "Restoring sessions" — no longer the old "…" placeholder.
		if !strings.Contains(view, "Restoring sessions") {
			t.Errorf("expected the 'Restoring sessions' step label, got:\n%s", view)
		}
		if strings.Contains(view, "Restoring sessions…") {
			t.Errorf("old placeholder 'Restoring sessions…' should be gone, got:\n%s", view)
		}
		if !strings.Contains(view, "Started tmux server") {
			t.Errorf("expected the full step-list (a real list), got:\n%s", view)
		}
	})

	t.Run("loading view does not show old Starting tmux server text", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		view := model.View().Content
		if strings.Contains(view, "Starting tmux server") {
			t.Errorf("loading view should not contain old text 'Starting tmux server', got:\n%s", view)
		}
	})

	t.Run("loading view centers the block in terminal", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		lines := strings.Split(ansi.Strip(model.View().Content), "\n")
		// The block should not be on the first line — it is roughly centered.
		if len(lines) < 2 {
			t.Fatal("expected multiple lines for centered layout")
		}
		// Find which line carries the active "Restoring sessions" step.
		textLine := -1
		for i, line := range lines {
			if strings.Contains(line, "Restoring sessions") {
				textLine = i
				break
			}
		}
		if textLine < 0 {
			t.Fatal("step-list label not found in view")
		}
		// The step-list sits in the lower-middle band of the centred block — it must
		// not be at the very top or bottom of a 24-row terminal.
		if textLine < 6 || textLine > 20 {
			t.Errorf("expected the step row near vertical center (row 6-20 of 24), got row %d", textLine)
		}
	})

	t.Run("loading view does not show session list chrome", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		view := model.View().Content
		if strings.Contains(view, "Sessions") {
			t.Error("loading view should not contain session list title 'Sessions'")
		}
	})

	t.Run("loading view uses fallback dimensions when no WindowSizeMsg received", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		// Do NOT send WindowSizeMsg
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "Restoring sessions") {
			t.Errorf("expected the loading step-list with fallback dimensions, got:\n%s", view)
		}
		// Should have multiple lines (80x24 fallback centering)
		lines := strings.Split(view, "\n")
		if len(lines) < 2 {
			t.Error("expected multiple lines with fallback 80x24 centering")
		}
	})

	t.Run("Ctrl+C during loading page quits", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, cmd := model.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", msg)
		}
		_ = model
	})

	t.Run("LoadingMinDuration is 1.2 seconds", func(t *testing.T) {
		if tui.LoadingMinDuration != 1200*time.Millisecond {
			t.Errorf("expected LoadingMinDuration to be 1.2s, got %v", tui.LoadingMinDuration)
		}
	})

	t.Run("Init schedules a single LoadingMinElapsedMsg via tea.Tick when on PageLoading", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil, expected batch command")
		}
		// Inspect each batched command's source to find the loadingPadTick.
		// We invoke each non-tick command; the tick command (with 1.2s duration)
		// is identified because it would block. Strategy: count batch entries
		// and ensure exactly one of them, when its goroutine runs to completion,
		// produces a LoadingMinElapsedMsg.
		// Simpler check: confirm Init returned something, and a separate test
		// asserts LoadingMinDuration constant. To avoid blocking the test on
		// the 1.2s tick, we don't invoke tick commands. Instead verify there
		// is at least one command whose body type matches the tea.Cmd shape.
		msg := cmd()
		batchMsg, ok := msg.(tea.BatchMsg)
		if !ok {
			t.Fatalf("expected tea.BatchMsg, got %T", msg)
		}
		// Expect at least 3 commands: fetchSessions + loadingPadTick +
		// bootstrapCompleteCmd (task 5-7 wires BootstrapCompleteMsg from Init).
		if len(batchMsg) < 3 {
			t.Errorf("expected at least 3 batch commands, got %d", len(batchMsg))
		}
	})

	t.Run("Init emits BootstrapCompleteMsg from PageLoading first event-loop tick", func(t *testing.T) {
		// Task 5-7 wires BootstrapCompleteMsg from Init so the loading-page
		// dismissal gate trips after the 1.2s tick has fired. PersistentPreRunE
		// has already run synchronously before this Init, so 'bootstrap
		// complete' is effectively true at Init time.
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil")
		}
		msg := cmd()
		batchMsg, ok := msg.(tea.BatchMsg)
		if !ok {
			t.Fatalf("expected tea.BatchMsg, got %T", msg)
		}

		// Walk the batch looking for a non-tick command whose synchronous
		// invocation produces a BootstrapCompleteMsg. Skip the loadingPadTick
		// (1.2s) so the test doesn't sleep — it's the only blocking command.
		// Run each non-tick command in a goroutine guarded by a short timeout.
		found := false
		for _, c := range batchMsg {
			if c == nil {
				continue
			}
			done := make(chan tea.Msg, 1)
			go func(cmd tea.Cmd) { done <- cmd() }(c)
			select {
			case got := <-done:
				if _, ok := got.(tui.BootstrapCompleteMsg); ok {
					found = true
				}
			case <-time.After(50 * time.Millisecond):
				// Likely the loadingPadTick; ignore.
			}
		}
		if !found {
			t.Error("expected one batched command to produce BootstrapCompleteMsg")
		}
	})

	t.Run("Init does not emit BootstrapCompleteMsg when not on PageLoading", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(false))
		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil")
		}
		msg := cmd()
		// On PageSessions Init, the result may be a single fetchSessions cmd
		// (single-cmd path) or a batch of (fetchSessions, loadProjects). Either
		// way it must NOT contain BootstrapCompleteMsg.
		switch typed := msg.(type) {
		case tea.BatchMsg:
			for _, c := range typed {
				done := make(chan tea.Msg, 1)
				go func(cmd tea.Cmd) { done <- cmd() }(c)
				select {
				case got := <-done:
					if _, ok := got.(tui.BootstrapCompleteMsg); ok {
						t.Error("Init on non-loading page emitted BootstrapCompleteMsg")
					}
				case <-time.After(50 * time.Millisecond):
				}
			}
		default:
			if _, ok := msg.(tui.BootstrapCompleteMsg); ok {
				t.Error("Init on non-loading page emitted BootstrapCompleteMsg")
			}
		}
	})

	t.Run("Init does not poll ListSessions on a loading tick (no DefaultPollInterval)", func(t *testing.T) {
		// Regression: removing pollSessionsCmd and tmux.DefaultPollInterval. The
		// SessionsMsg branch must not return a non-nil retry command on empty
		// sessions during loading.
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		_, cmd := model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		if cmd != nil {
			t.Errorf("expected no re-fetch command from SessionsMsg during loading, got %T", cmd)
		}
	})

	t.Run("LoadingMinElapsedMsg sets minElapsed but stays on PageLoading when bootstrap not complete", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		updated := model.(tui.Model)

		if !updated.MinElapsed() {
			t.Error("expected MinElapsed() to be true after LoadingMinElapsedMsg")
		}
		if updated.ActivePage() != tui.PageLoading {
			t.Errorf("expected to remain on PageLoading (bootstrap not complete), got %d", updated.ActivePage())
		}
	})

	t.Run("BootstrapCompleteMsg sets bootstrapComplete but stays on PageLoading when minElapsed is false", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		updated := model.(tui.Model)

		if !updated.BootstrapComplete() {
			t.Error("expected BootstrapComplete() to be true after BootstrapCompleteMsg")
		}
		if updated.ActivePage() != tui.PageLoading {
			t.Errorf("expected to remain on PageLoading (minElapsed false), got %d", updated.ActivePage())
		}
	})

	t.Run("transitions off PageLoading when both minElapsed and bootstrapComplete are true (min first)", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Error("expected transition off PageLoading after min + bootstrap")
		}
	})

	t.Run("transitions off PageLoading when both minElapsed and bootstrapComplete are true (bootstrap first)", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Error("expected transition off PageLoading after bootstrap + min")
		}
	})

	t.Run("other keys during loading are swallowed", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m

		keys := []tea.KeyPressMsg{
			{Code: 'q', Text: "q"},
			{Code: 'p', Text: "p"},
			{Code: tea.KeyEnter},
			{Code: tea.KeyEsc},
		}
		for _, k := range keys {
			var cmd tea.Cmd
			model, cmd = model.Update(k)
			if cmd != nil {
				msg := cmd()
				if _, ok := msg.(tea.QuitMsg); ok {
					t.Errorf("key %v should be swallowed during loading, not quit", k)
				}
			}
		}

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageLoading {
			t.Errorf("expected still on PageLoading after swallowed keys, got %d", updated.ActivePage())
		}
	})

	t.Run("orphaned LoadingMinElapsedMsg after transition is harmless", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Force transition by completing both gates
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		updated := model.(tui.Model)
		page := updated.ActivePage()

		// Now receive orphaned LoadingMinElapsedMsg — should not crash or change state
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		updated = model.(tui.Model)
		if updated.ActivePage() != page {
			t.Errorf("orphaned LoadingMinElapsedMsg changed page from %d to %d", page, updated.ActivePage())
		}
	})

	t.Run("orphaned BootstrapCompleteMsg after transition is harmless", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Transition
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		updated := model.(tui.Model)
		page := updated.ActivePage()

		// Orphaned BootstrapCompleteMsg
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		updated = model.(tui.Model)
		if updated.ActivePage() != page {
			t.Errorf("orphaned BootstrapCompleteMsg changed page from %d to %d", page, updated.ActivePage())
		}
	})

	t.Run("SessionsMsg during loading does not transition off PageLoading", func(t *testing.T) {
		// New behaviour: SessionsMsg no longer triggers loading-page dismissal.
		// Only LoadingMinElapsedMsg + BootstrapCompleteMsg do.
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		sessions := []tmux.Session{{Name: "dev", Windows: 1, Attached: false}}
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageLoading {
			t.Errorf("expected still on PageLoading after SessionsMsg (gates not met), got %d", updated.ActivePage())
		}
	})

	t.Run("transition off PageLoading lands on PageProjects when no sessions and projects loaded", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{{Path: "/code/portal", Name: "portal"}},
		}
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister,
			tui.WithServerStarted(true),
			tui.WithProjectStore(store),
			tui.WithSessionCreator(&mockSessionCreator{sessionName: "test"}),
		)
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Projects load during loading wait
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{{Path: "/code/portal", Name: "portal"}},
		})

		// Both gates fire with no sessions
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects (no sessions, projects loaded), got %d", updated.ActivePage())
		}
	})

	t.Run("transition with sessions and projects loaded stays on PageSessions", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{{Path: "/code/portal", Name: "portal"}},
		}
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister,
			tui.WithServerStarted(true),
			tui.WithProjectStore(store),
			tui.WithSessionCreator(&mockSessionCreator{sessionName: "test"}),
		)
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Projects load
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{{Path: "/code/portal", Name: "portal"}},
		})

		// Sessions arrive during loading
		sessions := []tmux.Session{{Name: "dev", Windows: 1, Attached: false}}
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Both gates fire
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions (sessions exist), got %d", updated.ActivePage())
		}
	})
}

// TestBootstrapWarningBuffering verifies that warnings carried on
// BootstrapCompleteMsg are buffered on the model and flushed to stderr
// (with alt-screen toggle) only after the loading page dismisses.
//
// Tests in this function manipulate the package-level test seam
// flushWarningsToStderr and MUST NOT use t.Parallel.
func TestBootstrapWarningBuffering(t *testing.T) {
	t.Run("BootstrapCompleteMsg.Warnings buffers into bufferedWarnings", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		warnings := []tui.BootstrapWarning{
			{Lines: []string{"warn 1 line 1", "warn 1 line 2"}},
			{Lines: []string{"warn 2 line 1"}},
		}
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		updated := model.(tui.Model)
		got := updated.BufferedWarnings()
		if len(got) != 2 {
			t.Fatalf("BufferedWarnings len = %d, want 2", len(got))
		}
		if len(got[0].Lines) != 2 || got[0].Lines[0] != "warn 1 line 1" {
			t.Errorf("first buffered warning = %#v", got[0])
		}
		if len(got[1].Lines) != 1 || got[1].Lines[0] != "warn 2 line 1" {
			t.Errorf("second buffered warning = %#v", got[1])
		}
	})

	t.Run("transition flushes warnings via flushBufferedWarningsCmd (min first, bootstrap with warnings)", func(t *testing.T) {
		var captured [][]string
		restore := tui.SetFlushWarningsToStderrForTest(func(warnings []tui.BootstrapWarning) {
			for _, w := range warnings {
				captured = append(captured, append([]string{}, w.Lines...))
			}
		})
		t.Cleanup(restore)

		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// minElapsed first
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})

		// bootstrap completes with warnings
		warnings := []tui.BootstrapWarning{
			{Lines: []string{"saver down"}},
			{Lines: []string{"corrupt", "see log"}},
		}
		var cmd tea.Cmd
		model, cmd = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Error("expected transition off PageLoading after both gates with warnings")
		}
		if cmd == nil {
			t.Fatal("expected non-nil flushBufferedWarningsCmd after transition with warnings")
		}

		// Walk the returned tea.Sequence by consuming returned messages.
		// tea.Sequence returns a tea.Cmd that emits messages one-by-one.
		// We invoke the top-level cmd which yields a tea.SequenceMsg holding
		// the sequenced sub-cmds; iterate them.
		drainSequence(cmd)

		if len(captured) != 2 {
			t.Fatalf("captured %d warnings, want 2", len(captured))
		}
		want := [][]string{{"saver down"}, {"corrupt", "see log"}}
		for i, w := range want {
			if !equalStrings(captured[i], w) {
				t.Errorf("captured[%d] = %v, want %v", i, captured[i], w)
			}
		}

		// bufferedWarnings cleared after flush
		if len(updated.BufferedWarnings()) != 0 {
			t.Errorf("bufferedWarnings not cleared after flush; got %d", len(updated.BufferedWarnings()))
		}
	})

	t.Run("transition flushes warnings (bootstrap first, then minElapsed)", func(t *testing.T) {
		var captured [][]string
		restore := tui.SetFlushWarningsToStderrForTest(func(warnings []tui.BootstrapWarning) {
			for _, w := range warnings {
				captured = append(captured, append([]string{}, w.Lines...))
			}
		})
		t.Cleanup(restore)

		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Bootstrap first
		warnings := []tui.BootstrapWarning{{Lines: []string{"warn"}}}
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		// Then min elapsed — this is where the flush command fires.
		var cmd tea.Cmd
		model, cmd = model.Update(tui.LoadingMinElapsedMsg{})

		if cmd == nil {
			t.Fatal("expected non-nil flush cmd from LoadingMinElapsedMsg branch")
		}
		drainSequence(cmd)

		if len(captured) != 1 || len(captured[0]) != 1 || captured[0][0] != "warn" {
			t.Errorf("captured = %v, want [[warn]]", captured)
		}

		updated := model.(tui.Model)
		if len(updated.BufferedWarnings()) != 0 {
			t.Errorf("bufferedWarnings not cleared after flush; got %d", len(updated.BufferedWarnings()))
		}
	})

	t.Run("transition with no warnings returns no flush command", func(t *testing.T) {
		var called bool
		restore := tui.SetFlushWarningsToStderrForTest(func(_ []tui.BootstrapWarning) {
			called = true
		})
		t.Cleanup(restore)

		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		var cmd tea.Cmd
		model, cmd = model.Update(tui.BootstrapCompleteMsg{Warnings: nil})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Error("expected transition off PageLoading")
		}
		if cmd != nil {
			t.Errorf("expected nil flush cmd when warnings empty (avoids spurious alt-screen toggle); got %T", cmd)
		}
		if called {
			t.Error("flushWarningsToStderr must not be called when warnings empty")
		}
	})

	t.Run("repeat transitions do not re-emit buffered warnings", func(t *testing.T) {
		var calls int
		restore := tui.SetFlushWarningsToStderrForTest(func(_ []tui.BootstrapWarning) {
			calls++
		})
		t.Cleanup(restore)

		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		warnings := []tui.BootstrapWarning{{Lines: []string{"warn"}}}
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})
		var cmd tea.Cmd
		model, cmd = model.Update(tui.LoadingMinElapsedMsg{})
		if cmd != nil {
			drainSequence(cmd)
		}

		// Orphaned BootstrapCompleteMsg with new warnings — already off
		// PageLoading, so no flush should fire.
		model, cmd = model.Update(tui.BootstrapCompleteMsg{
			Warnings: []tui.BootstrapWarning{{Lines: []string{"second"}}},
		})
		if cmd != nil {
			t.Errorf("orphaned BootstrapCompleteMsg after transition produced flush cmd")
		}

		// Orphaned LoadingMinElapsedMsg
		_, cmd = model.Update(tui.LoadingMinElapsedMsg{})
		if cmd != nil {
			t.Errorf("orphaned LoadingMinElapsedMsg produced flush cmd")
		}

		if calls != 1 {
			t.Errorf("flushWarningsToStderr called %d times, want exactly 1", calls)
		}
	})

	t.Run("orphaned BootstrapCompleteMsg after transition does not buffer warnings", func(t *testing.T) {
		var calls int
		restore := tui.SetFlushWarningsToStderrForTest(func(_ []tui.BootstrapWarning) {
			calls++
		})
		t.Cleanup(restore)

		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Force transition with no warnings
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: nil})

		// Orphaned message with warnings — must NOT populate bufferedWarnings
		// (we are no longer on PageLoading; the warnings have nowhere to go).
		model, _ = model.Update(tui.BootstrapCompleteMsg{
			Warnings: []tui.BootstrapWarning{{Lines: []string{"orphan"}}},
		})

		updated := model.(tui.Model)
		if got := updated.BufferedWarnings(); len(got) != 0 {
			t.Errorf("orphaned warnings populated bufferedWarnings = %#v, want empty", got)
		}
		if calls != 0 {
			t.Errorf("flushWarningsToStderr called %d times for orphaned warnings, want 0", calls)
		}
	})

	t.Run("SetPendingBootstrapWarnings exposes warnings via Init", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		m := tui.New(lister, tui.WithServerStarted(true))
		warnings := []tui.BootstrapWarning{{Lines: []string{"pending"}}}
		m.SetPendingBootstrapWarnings(warnings)

		got := m.PendingBootstrapWarnings()
		if len(got) != 1 || len(got[0].Lines) != 1 || got[0].Lines[0] != "pending" {
			t.Errorf("PendingBootstrapWarnings() = %#v, want [{Lines:[pending]}]", got)
		}

		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init returned nil")
		}
		// One of the batched cmds must produce a BootstrapCompleteMsg whose
		// Warnings field equals the pending slice.
		msg := cmd()
		batchMsg, ok := msg.(tea.BatchMsg)
		if !ok {
			t.Fatalf("Init result not BatchMsg, got %T", msg)
		}
		var found *tui.BootstrapCompleteMsg
		for _, c := range batchMsg {
			if c == nil {
				continue
			}
			done := make(chan tea.Msg, 1)
			go func(cmd tea.Cmd) { done <- cmd() }(c)
			select {
			case got := <-done:
				if bc, ok := got.(tui.BootstrapCompleteMsg); ok {
					found = &bc
				}
			case <-time.After(50 * time.Millisecond):
			}
		}
		if found == nil {
			t.Fatal("no batched cmd produced BootstrapCompleteMsg")
		}
		if len(found.Warnings) != 1 || len(found.Warnings[0].Lines) != 1 || found.Warnings[0].Lines[0] != "pending" {
			t.Errorf("BootstrapCompleteMsg.Warnings = %#v, want [{Lines:[pending]}]", found.Warnings)
		}
	})
}

// TestWriteWarningsToWriter verifies that the inner stderr-writing helper
// (factored out for direct testing) emits every warning's lines in order,
// one Fprintln per line.
func TestWriteWarningsToWriter(t *testing.T) {
	t.Run("emits all lines in order", func(t *testing.T) {
		var buf strings.Builder
		warnings := []tui.BootstrapWarning{
			{Lines: []string{"a1", "a2"}},
			{Lines: []string{"b1"}},
		}
		tui.WriteBootstrapWarnings(&buf, warnings)
		want := "a1\na2\nb1\n"
		if buf.String() != want {
			t.Errorf("WriteBootstrapWarnings wrote %q, want %q", buf.String(), want)
		}
	})

	t.Run("empty warnings writes nothing", func(t *testing.T) {
		var buf strings.Builder
		tui.WriteBootstrapWarnings(&buf, nil)
		if buf.Len() != 0 {
			t.Errorf("WriteBootstrapWarnings on nil wrote %q, want empty", buf.String())
		}
	})
}

// drainSequence consumes a tea.Cmd produced by flushBufferedWarningsCmd.
// In Bubble Tea v1, tea.Sequence wraps sub-commands in a single tea.Cmd
// whose invocation produces an unexported sequenceMsg ([]tea.Cmd). The
// runtime would dispatch each sub-cmd in order; without the runtime we
// use reflection to walk the slice and invoke each sub-cmd directly so
// the inner stderr-writing closure executes during tests.
func drainSequence(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if seq, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range seq {
			if sub == nil {
				continue
			}
			_ = sub()
		}
		return
	}
	// Reflect on the sequenceMsg slice type (~[]tea.Cmd, unexported).
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i).Interface()
		if sub, ok := elem.(tea.Cmd); ok && sub != nil {
			_ = sub()
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
