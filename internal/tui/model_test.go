package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/ui"
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
			name:     "cursor does not go below last item including new option",
			sessions: threeSessions,
			keys: []tea.Msg{
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown},
				tea.KeyMsg{Type: tea.KeyDown}, // lands on [n] new in project...
				tea.KeyMsg{Type: tea.KeyDown}, // should not go further
				tea.KeyMsg{Type: tea.KeyDown}, // should not go further
			},
			wantCursorLine: 5, // 3 sessions + blank + divider + new option line
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

func TestNewInProjectOption(t *testing.T) {
	t.Run("session list includes new in project option", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		})
		view := m.View()

		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("view missing '[n] new in project...' option:\n%s", view)
		}
	})

	t.Run("new option appears below sessions with divider", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{
			{Name: "dev", Windows: 3, Attached: false},
			{Name: "work", Windows: 1, Attached: false},
		})
		view := m.View()

		lastSessionIdx := strings.Index(view, "work")
		dividerIdx := strings.Index(view, "─")
		newOptionIdx := strings.Index(view, "[n] new in project...")

		if lastSessionIdx == -1 || dividerIdx == -1 || newOptionIdx == -1 {
			t.Fatalf("missing elements in view:\n%s", view)
		}
		if dividerIdx <= lastSessionIdx {
			t.Errorf("divider (idx %d) should appear after last session (idx %d)", dividerIdx, lastSessionIdx)
		}
		if newOptionIdx <= dividerIdx {
			t.Errorf("new option (idx %d) should appear after divider (idx %d)", newOptionIdx, dividerIdx)
		}
	})

	t.Run("n key jumps to new option", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		view := m.View()
		// The [n] new in project... line should have the cursor
		lines := strings.Split(view, "\n")
		var newOptionLine string
		for _, line := range lines {
			if strings.Contains(line, "new in project") {
				newOptionLine = line
				break
			}
		}
		if !strings.Contains(newOptionLine, ">") {
			t.Errorf("n key should move cursor to new option line: %q", newOptionLine)
		}
	})

	t.Run("enter on new option switches to project picker", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
		)
		// Load sessions
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press n to jump to new option, then Enter
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Load projects into the picker
		model, _ = model.Update(ui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		view := model.View()
		if !strings.Contains(view, "Select a project") {
			t.Errorf("expected project picker view, got:\n%s", view)
		}
	})

	t.Run("esc in project picker returns to session list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to project picker
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: store.projects})

		// Esc should return to session list
		model, _ = model.Update(ui.BackMsg{})

		view := model.View()
		if !strings.Contains(view, "dev") {
			t.Errorf("expected session list with 'dev', got:\n%s", view)
		}
		if strings.Contains(view, "Select a project") {
			t.Errorf("should not show project picker after Esc:\n%s", view)
		}
	})

	t.Run("project selection triggers creation and returns session name", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{
			sessionName: "myapp-abc123",
		}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to project picker
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: store.projects})

		// Select a project
		_, cmd := model.Update(ui.ProjectSelectedMsg{Path: "/code/myapp"})
		if cmd == nil {
			t.Fatal("expected command from project selection, got nil")
		}

		// The command should trigger session creation
		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "myapp-abc123" {
			t.Errorf("expected session name %q, got %q", "myapp-abc123", createdMsg.SessionName)
		}
		if creator.createdDir != "/code/myapp" {
			t.Errorf("expected CreateFromDir called with %q, got %q", "/code/myapp", creator.createdDir)
		}
	})

	t.Run("empty session list still shows new option", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{})
		view := m.View()

		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("empty session list should show new option:\n%s", view)
		}
		if !strings.Contains(view, "No active sessions") {
			t.Errorf("empty session list should show no sessions message:\n%s", view)
		}
	})

	t.Run("combined empty state navigable", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		creator := &mockSessionCreator{}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			store,
			creator,
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// n should jump to new option
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		// Enter should switch to project picker
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		// Load empty projects
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: []project.Project{}})

		view := model.View()
		if !strings.Contains(view, "Select a project") {
			t.Errorf("expected project picker in combined empty state, got:\n%s", view)
		}
		if !strings.Contains(view, "No saved projects yet.") {
			t.Errorf("expected empty projects message, got:\n%s", view)
		}

		// Esc should return to session list
		model, _ = model.Update(ui.BackMsg{})
		view = model.View()
		if !strings.Contains(view, "No active sessions") {
			t.Errorf("expected session list after Esc, got:\n%s", view)
		}
	})
}

// mockProjectStore implements ui.ProjectStore for tui testing.
type mockProjectStore struct {
	projects []project.Project
	listErr  error
}

func (m *mockProjectStore) List() ([]project.Project, error) {
	return m.projects, m.listErr
}

func (m *mockProjectStore) CleanStale() (int, error) {
	return 0, nil
}

// mockSessionCreator implements tui.SessionCreator for testing.
type mockSessionCreator struct {
	sessionName string
	createdDir  string
	err         error
}

func (m *mockSessionCreator) CreateFromDir(dir string) (string, error) {
	m.createdDir = dir
	if m.err != nil {
		return "", m.err
	}
	return m.sessionName, nil
}

// mockDirLister implements ui.DirLister for tui testing.
type mockDirLister struct {
	entries map[string][]browser.DirEntry
}

func (m *mockDirLister) ListDirectories(path string, showHidden bool) ([]browser.DirEntry, error) {
	if entries, ok := m.entries[path]; ok {
		return entries, nil
	}
	return []browser.DirEntry{}, nil
}

func TestFileBrowserIntegration(t *testing.T) {
	t.Run("browse option opens file browser", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}, {Name: "docs"}},
			},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
			lister,
			"/home/user",
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to project picker
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: store.projects})

		// Trigger browse selection
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		view := model.View()
		// File browser should show the starting directory path
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser view with starting path, got:\n%s", view)
		}
		// Should show directory entries
		if !strings.Contains(view, "code") {
			t.Errorf("expected file browser to show directory entries, got:\n%s", view)
		}
		// Should NOT show project picker
		if strings.Contains(view, "Select a project") {
			t.Errorf("should not show project picker when file browser is open:\n%s", view)
		}
	})

	t.Run("selection creates session with browsed path", func(t *testing.T) {
		sessions := []tmux.Session{}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
			lister,
			"/home/user",
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Simulate browse flow: project picker -> file browser -> directory selected
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		// File browser emits BrowserDirSelectedMsg
		_, cmd := model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/code"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
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

	t.Run("selection registers project in store via session creator", func(t *testing.T) {
		sessions := []tmux.Session{}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "myproj-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
			lister,
			"/home/user",
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		// File browser selects directory
		_, cmd := model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/myproj"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}

		// Execute the command to trigger session creation
		cmd()

		// SessionCreator.CreateFromDir handles git resolution, project registration, and session creation.
		// Verify it was called with the browsed path.
		if creator.createdDir != "/home/user/myproj" {
			t.Errorf("expected CreateFromDir with %q, got %q", "/home/user/myproj", creator.createdDir)
		}
	})

	t.Run("cancel in file browser returns to project picker", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
		}
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}},
			},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
			lister,
			"/home/user",
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to project picker
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: store.projects})

		// Open file browser
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		// Cancel the file browser
		model, _ = model.Update(ui.BrowserCancelMsg{})

		view := model.View()
		// Should be back in project picker
		if !strings.Contains(view, "Select a project") {
			t.Errorf("expected project picker after cancel, got:\n%s", view)
		}
		if !strings.Contains(view, "myapp") {
			t.Errorf("expected project 'myapp' in picker, got:\n%s", view)
		}
	})

	t.Run("browse works from empty project list", func(t *testing.T) {
		sessions := []tmux.Session{}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "docs-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "docs"}},
			},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			store,
			creator,
			lister,
			"/home/user",
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to project picker (empty)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model, _ = model.Update(ui.ProjectsLoadedMsg{Projects: []project.Project{}})

		// In empty project list, cursor is on browse option. Trigger browse.
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		view := model.View()
		// Should be in file browser
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser with starting path, got:\n%s", view)
		}
		if !strings.Contains(view, "docs") {
			t.Errorf("expected file browser to show entries, got:\n%s", view)
		}

		// Selecting a directory should create a session
		_, cmd := model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/docs"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}
		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "docs-abc123" {
			t.Errorf("expected session name %q, got %q", "docs-abc123", createdMsg.SessionName)
		}
	})
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
}

func TestEmptyState(t *testing.T) {
	t.Run("empty sessions shows no active sessions message", func(t *testing.T) {
		m := tui.New(nil)
		// Simulate receiving an empty sessions list
		updated, _ := m.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})
		view := updated.View()
		if !strings.Contains(view, "No active sessions") {
			t.Errorf("expected view to contain 'No active sessions', got %q", view)
		}
		// Should still show the new option
		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("expected view to contain '[n] new in project...', got %q", view)
		}
	})

	t.Run("nil sessions shows no active sessions message", func(t *testing.T) {
		m := tui.New(nil)
		// Simulate receiving nil sessions (tmux server not running)
		updated, _ := m.Update(tui.SessionsMsg{Sessions: nil})
		view := updated.View()
		if !strings.Contains(view, "No active sessions") {
			t.Errorf("expected view to contain 'No active sessions', got %q", view)
		}
		// Should still show the new option
		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("expected view to contain '[n] new in project...', got %q", view)
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

func TestInsideTmuxSessionExclusion(t *testing.T) {
	t.Run("current session excluded from list inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "current-sess", Windows: 2, Attached: true},
			{Name: "bravo", Windows: 3, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("current-sess")
		view := m.View()

		lines := strings.Split(view, "\n")
		for _, line := range lines {
			if strings.Contains(line, "current-sess") && !strings.Contains(line, "Current:") {
				t.Errorf("current session should not appear in session list, found in line: %q", line)
			}
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("non-current session 'alpha' should appear in list, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("non-current session 'bravo' should appear in list, got:\n%s", view)
		}
	})

	t.Run("header renders current session name inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
			{Name: "my-project-x7k2m9", Windows: 2, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("my-project-x7k2m9")
		view := m.View()

		if !strings.Contains(view, "Current: my-project-x7k2m9") {
			t.Errorf("expected header 'Current: my-project-x7k2m9', got:\n%s", view)
		}
		// Header should appear before the session list
		headerIdx := strings.Index(view, "Current: my-project-x7k2m9")
		devIdx := strings.Index(view, "dev")
		if headerIdx >= devIdx {
			t.Errorf("header (idx %d) should appear before session 'dev' (idx %d)", headerIdx, devIdx)
		}
	})

	t.Run("no header rendered outside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 1, Attached: false},
			{Name: "work", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		view := m.View()

		if strings.Contains(view, "Current:") {
			t.Errorf("header should not appear outside tmux, got:\n%s", view)
		}
	})

	t.Run("empty state shows no other sessions when only current session", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "only-session", Windows: 1, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("only-session")
		view := m.View()

		if !strings.Contains(view, "No other sessions") {
			t.Errorf("expected 'No other sessions' when only current session exists, got:\n%s", view)
		}
		if strings.Contains(view, "No active sessions") {
			t.Errorf("should not show 'No active sessions' inside tmux, got:\n%s", view)
		}
	})

	t.Run("empty state shows no active sessions outside tmux", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{})
		view := m.View()

		if !strings.Contains(view, "No active sessions") {
			t.Errorf("expected 'No active sessions' outside tmux, got:\n%s", view)
		}
		if strings.Contains(view, "No other sessions") {
			t.Errorf("should not show 'No other sessions' outside tmux, got:\n%s", view)
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
		view := m.View()

		if !strings.Contains(view, "Current: current-one") {
			t.Errorf("expected header with current session name, got:\n%s", view)
		}
		for _, name := range []string{"alpha", "bravo", "charlie"} {
			if !strings.Contains(view, name) {
				t.Errorf("expected session %q in list, got:\n%s", name, view)
			}
		}
		// Ensure current-one only appears in the header, not in the session list area
		lines := strings.Split(view, "\n")
		for _, line := range lines {
			if strings.Contains(line, "current-one") && !strings.Contains(line, "Current:") {
				t.Errorf("current session should only appear in header, found in list line: %q", line)
			}
		}
	})

	t.Run("very long current session name renders in header without truncation", func(t *testing.T) {
		longName := "my-extremely-long-project-name-that-goes-on-and-on-forever-x7k2m9"
		sessions := []tmux.Session{
			{Name: longName, Windows: 1, Attached: true},
			{Name: "other", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux(longName)
		view := m.View()

		expected := "Current: " + longName
		if !strings.Contains(view, expected) {
			t.Errorf("expected full header %q in view, got:\n%s", expected, view)
		}
	})

	t.Run("new in project option visible when inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "current-sess", Windows: 1, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("current-sess")
		view := m.View()

		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("new in project option should be visible inside tmux, got:\n%s", view)
		}
	})

	t.Run("new in project option visible with sessions filtered inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "current-sess", Windows: 2, Attached: true},
		}
		m := tui.NewModelWithSessions(sessions).WithInsideTmux("current-sess")
		view := m.View()

		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("new in project option should be visible, got:\n%s", view)
		}
	})
}
