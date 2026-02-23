package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

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
				if !strings.Contains(firstLine, ">") {
					t.Errorf("first session should have cursor indicator: %q", firstLine)
				}
				if strings.Contains(secondLine, ">") {
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
				if !strings.Contains(view, ">") {
					t.Error("view missing cursor indicator")
				}
			},
		},
		{
			name: "long session name renders without truncation",
			sessions: []tmux.Session{
				{Name: "my-very-long-project-name-that-should-not-be-truncated-x7k2m9", Windows: 1, Attached: false},
			},
			checks: func(t *testing.T, view string) {
				t.Helper()
				if !strings.Contains(view, "my-very-long-project-name-that-should-not-be-truncated-x7k2m9") {
					t.Error("long session name was truncated")
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
			view := m.View()
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

		msg := cmd()
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
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
	t.Run("sessionsMsg populates sessions and sets cursor to zero", func(t *testing.T) {
		m := tui.New(nil)
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}

		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		view := updated.View()

		if !strings.Contains(view, "dev") {
			t.Error("view missing session 'dev' after sessionsMsg")
		}
		if !strings.Contains(view, "work") {
			t.Error("view missing session 'work' after sessionsMsg")
		}

		// Verify cursor is at first session
		lines := strings.Split(view, "\n")
		var devLine string
		for _, line := range lines {
			if strings.Contains(line, "dev") {
				devLine = line
				break
			}
		}
		if !strings.Contains(devLine, ">") {
			t.Errorf("cursor should be on first session after sessionsMsg: %q", devLine)
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

// cursorLine returns the index of the line containing the cursor indicator ">".
// Returns -1 if no cursor found.
func cursorLine(view string) int {
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, ">") {
			return i
		}
	}
	return -1
}

func TestKeyboardNavigation(t *testing.T) {
	threeSessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
		{Name: "charlie", Windows: 3, Attached: false},
	}

	tests := []struct {
		name           string
		sessions       []tmux.Session
		keys           []tea.Msg
		wantCursorLine int
	}{
		{
			name:     "down arrow moves cursor down",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
			},
			wantCursorLine: 1,
		},
		{
			name:     "j key moves cursor down",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
			},
			wantCursorLine: 1,
		},
		{
			name:     "up arrow moves cursor up",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyUp},
			},
			wantCursorLine: 0,
		},
		{
			name:     "k key moves cursor up",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}},
			},
			wantCursorLine: 0,
		},
		{
			name:     "cursor does not go below last item",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
			},
			wantCursorLine: 2,
		},
		{
			name:     "cursor does not go above first item",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyUp},
				tea.KeyMsg{Type: tea.KeyUp},
			},
			wantCursorLine: 0,
		},
		{
			name: "navigation is no-op with single session",
			sessions: []tmux.Session{
				{Name: "solo", Windows: 1, Attached: false},
			},
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyUp},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
				tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}},
			},
			wantCursorLine: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = tui.NewModelWithSessions(tt.sessions)
			for _, key := range tt.keys {
				m, _ = m.Update(key)
			}
			view := m.View()
			got := cursorLine(view)
			if got != tt.wantCursorLine {
				t.Errorf("cursor on line %d, want %d\nview:\n%s", got, tt.wantCursorLine, view)
			}
		})
	}

	t.Run("view highlights correct row after navigation", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		// Move cursor to "bravo" (index 1)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		view := m.View()

		lines := strings.Split(view, "\n")
		if len(lines) < 3 {
			t.Fatalf("expected 3 lines, got %d", len(lines))
		}
		// alpha should NOT have cursor
		if strings.Contains(lines[0], ">") {
			t.Errorf("alpha line should not have cursor: %q", lines[0])
		}
		// bravo SHOULD have cursor
		if !strings.Contains(lines[1], ">") {
			t.Errorf("bravo line should have cursor: %q", lines[1])
		}
		// charlie should NOT have cursor
		if strings.Contains(lines[2], ">") {
			t.Errorf("charlie line should not have cursor: %q", lines[2])
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
		key  tea.KeyMsg
	}{
		{
			name: "q key triggers quit",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		},
		{
			name: "Esc key triggers quit",
			key:  tea.KeyMsg{Type: tea.KeyEsc},
		},
		{
			name: "Ctrl+C triggers quit",
			key:  tea.KeyMsg{Type: tea.KeyCtrlC},
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
		key  tea.KeyMsg
	}{
		{
			name: "down arrow does not trigger quit",
			key:  tea.KeyMsg{Type: tea.KeyDown},
		},
		{
			name: "up arrow does not trigger quit",
			key:  tea.KeyMsg{Type: tea.KeyUp},
		},
		{
			name: "j key does not trigger quit",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}},
		},
		{
			name: "k key does not trigger quit",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}},
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

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

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

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

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

		quitKeys := []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyEsc},
			{Type: tea.KeyCtrlC},
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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Press Enter
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

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

func TestEmptyState(t *testing.T) {
	t.Run("empty sessions shows no active sessions message", func(t *testing.T) {
		m := tui.New(nil)
		// Simulate receiving an empty sessions list
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		view := updated.View()
		if view != "No active sessions" {
			t.Errorf("expected %q, got %q", "No active sessions", view)
		}
	})

	t.Run("nil sessions shows no active sessions message", func(t *testing.T) {
		m := tui.New(nil)
		// Simulate receiving nil sessions (tmux server not running)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: nil})
		view := updated.View()
		if view != "No active sessions" {
			t.Errorf("expected %q, got %q", "No active sessions", view)
		}
	})

	t.Run("non-empty sessions does not show empty message", func(t *testing.T) {
		m := tui.New(nil)
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: false},
		}
		updated, _ := m.Update(tui.SessionsMsg{Sessions: sessions})
		view := updated.View()
		if strings.Contains(view, "No active sessions") {
			t.Errorf("view should not contain empty state message when sessions exist: %q", view)
		}
	})

	t.Run("empty state is not shown before sessions are loaded", func(t *testing.T) {
		m := tui.New(nil)
		// Before any sessionsMsg, view should NOT show the empty state message
		view := m.View()
		if strings.Contains(view, "No active sessions") {
			t.Errorf("empty state message should not appear before sessions are loaded: %q", view)
		}
	})

	t.Run("quit works in empty state", func(t *testing.T) {
		quitKeys := []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyEsc},
			{Type: tea.KeyCtrlC},
		}

		for _, key := range quitKeys {
			m := tui.New(nil)
			// Load empty sessions
			updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
			// Press quit key
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
		// Load empty sessions
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		// Press Enter
		result, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
		// Should not trigger quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Error("enter in empty state should not trigger quit")
			}
		}
		// Selected should remain empty
		model, ok := result.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", result)
		}
		if model.Selected() != "" {
			t.Errorf("expected empty selected, got %q", model.Selected())
		}
	})
}
