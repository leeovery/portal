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
			nil,
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
			nil,
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
			nil,
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

	t.Run("project selection forwards command to session creator", func(t *testing.T) {
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
			nil,
			store,
			creator,
		).WithCommand([]string{"claude", "--resume"})
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

		// Execute the command to trigger session creation
		cmd()

		// Verify command was forwarded to session creator
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

	t.Run("no command set passes nil to session creator", func(t *testing.T) {
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
			nil,
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

		// Execute the command to trigger session creation
		cmd()

		// Verify nil command was passed (no command set on model)
		if creator.createdCommand != nil {
			t.Errorf("expected nil command, got %v", creator.createdCommand)
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
			nil,
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

// mockSessionKiller implements tui.SessionKiller for testing.
type mockSessionKiller struct {
	killedName string
	err        error
}

func (m *mockSessionKiller) KillSession(name string) error {
	m.killedName = name
	return m.err
}

// mockProjectStore implements ui.ProjectStore for tui testing.
type mockProjectStore struct {
	projects []project.Project
	listErr  error
}

func (m *mockProjectStore) List() ([]project.Project, error) {
	return m.projects, m.listErr
}

func (m *mockProjectStore) CleanStale() ([]project.Project, error) {
	return nil, nil
}

func (m *mockProjectStore) Remove(_ string) error {
	return nil
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
			nil,
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
			nil,
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
			nil,
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

	t.Run("file browser selection forwards command to session creator", func(t *testing.T) {
		sessions := []tmux.Session{}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: sessions},
			nil,
			store,
			creator,
			lister,
			"/home/user",
		).WithCommand([]string{"vim", "."})
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		// File browser selects directory
		_, cmd := model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/code"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}

		// Execute the command to trigger session creation
		cmd()

		// Verify command was forwarded
		wantCmd := []string{"vim", "."}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, wantCmd)
		}
		for i, arg := range creator.createdCommand {
			if arg != wantCmd[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, wantCmd[i])
			}
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
			nil,
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
			nil,
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

func TestKillSession(t *testing.T) {
	t.Run("K enters confirmation mode with session name", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K on the first session
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})

		view := model.View()
		if !strings.Contains(view, "Kill session 'alpha'? (y/n)") {
			t.Errorf("expected confirmation prompt for 'alpha', got:\n%s", view)
		}
	})

	t.Run("y in confirmation mode triggers kill and refresh", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

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

	t.Run("n in confirmation mode cancels", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then n
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		view := model.View()
		// Should be back to normal session list, no confirmation prompt
		if strings.Contains(view, "Kill session") {
			t.Errorf("confirmation prompt should be cleared after n, got:\n%s", view)
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("session 'alpha' should still be in list after cancel, got:\n%s", view)
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
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then Esc
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := model.View()
		if strings.Contains(view, "Kill session") {
			t.Errorf("confirmation prompt should be cleared after Esc, got:\n%s", view)
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
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then y to kill alpha
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Update the lister to return remaining sessions
		lister.sessions = remainingSessions

		// Execute the command
		msg := cmd()

		// Feed the result back into the model
		model, _ = model.Update(msg)

		view := model.View()
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
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to bravo (last session)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Press K then y to kill bravo
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Update lister
		lister.sessions = remainingSessions

		// Execute and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		// Cursor should be on alpha (index 0), which is now the last session
		lines := strings.Split(view, "\n")
		var alphaLine string
		for _, line := range lines {
			if strings.Contains(line, "alpha") {
				alphaLine = line
				break
			}
		}
		if !strings.Contains(alphaLine, ">") {
			t.Errorf("cursor should be on 'alpha' after killing last session, got:\n%s", view)
		}
	})

	t.Run("K on new-in-project option is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to the [n] option
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		// Press K — should be no-op
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})

		view := model.View()
		if strings.Contains(view, "Kill session") {
			t.Errorf("K on new option should be no-op, got:\n%s", view)
		}
	})

	t.Run("kill error returns error in SessionsMsg", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		killer := &mockSessionKiller{err: fmt.Errorf("session not found")}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then y to attempt kill
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

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
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Execute command and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		// SessionsMsg with error triggers quit, so the model should exit.
		// But the confirmation prompt should not still be showing.
		view := model.View()
		if strings.Contains(view, "Kill session") {
			t.Errorf("confirmation prompt should be cleared after kill error, got:\n%s", view)
		}
	})

	t.Run("killing last remaining session shows empty state", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "solo", Windows: 1, Attached: false},
		}
		killer := &mockSessionKiller{}
		lister := &mockSessionLister{sessions: sessions}
		m := tui.NewWithKiller(lister, killer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Update lister to return empty
		lister.sessions = []tmux.Session{}

		// Execute and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "No active sessions") {
			t.Errorf("expected empty state after killing last session, got:\n%s", view)
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

		m := tui.NewWithDeps(lister, killer, store, creator)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K — should enter confirmation mode (not no-op)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		view := model.View()
		if !strings.Contains(view, "Kill session 'alpha'? (y/n)") {
			t.Errorf("expected confirmation prompt via NewWithDeps, got:\n%s", view)
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
		dirLister := &mockDirLister{entries: map[string][]browser.DirEntry{}}

		m := tui.NewWithAllDeps(lister, killer, store, creator, dirLister, "/home/user")
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press K — should enter confirmation mode (not no-op)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
		view := model.View()
		if !strings.Contains(view, "Kill session 'alpha'? (y/n)") {
			t.Errorf("expected confirmation prompt via NewWithAllDeps, got:\n%s", view)
		}
	})
}

// mockSessionRenamer implements tui.SessionRenamer for testing.
type mockSessionRenamer struct {
	renamedOld string
	renamedNew string
	err        error
}

func (m *mockSessionRenamer) RenameSession(oldName, newName string) error {
	m.renamedOld = oldName
	m.renamedNew = newName
	return m.err
}

// newModelWithRenamer creates a model with lister, killer, and renamer for rename tests.
func newModelWithRenamer(lister *mockSessionLister, killer *mockSessionKiller, renamer *mockSessionRenamer) tui.Model {
	return tui.NewWithRenamer(lister, killer, renamer)
}

func TestRenameSession(t *testing.T) {
	t.Run("R enters rename mode with current name pre-filled", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press R on the first session
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		view := model.View()
		// Should show rename prompt with the session name
		if !strings.Contains(view, "Rename:") {
			t.Errorf("expected rename prompt, got:\n%s", view)
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("expected pre-filled name 'alpha' in rename prompt, got:\n%s", view)
		}
	})

	t.Run("Enter in rename mode calls rename-session and refreshes", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press R to enter rename mode
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		// Clear the text and type a new name
		// First, select all and delete (Ctrl+U clears line)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		// Type "new-alpha"
		for _, r := range "new-alpha" {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}

		// Press Enter to confirm
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

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

	t.Run("Esc in rename mode cancels without renaming", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press R to enter rename mode
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		// Press Esc to cancel
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := model.View()
		// Should be back to normal session list
		if strings.Contains(view, "Rename:") {
			t.Errorf("rename prompt should be cleared after Esc, got:\n%s", view)
		}
		if !strings.Contains(view, "alpha") {
			t.Errorf("session 'alpha' should still be in list after cancel, got:\n%s", view)
		}
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called, but got old=%q new=%q", renamer.renamedOld, renamer.renamedNew)
		}
	})

	t.Run("empty input does not trigger rename", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press R to enter rename mode
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		// Clear input
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})

		// Press Enter with empty input
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should not trigger rename (no command returned)
		if cmd != nil {
			t.Error("expected nil command for empty rename input")
		}

		// Should still be in rename mode or cancelled — renamer should not have been called
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called with empty input, but got old=%q", renamer.renamedOld)
		}
	})

	t.Run("R on new-in-project option is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to the [n] option
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		// Press R — should be no-op
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		view := model.View()
		if strings.Contains(view, "Rename:") {
			t.Errorf("R on new option should be no-op, got:\n%s", view)
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

		// Press R
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		// Clear and type new name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		for _, r := range "new-alpha" {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}

		// Press Enter
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Update lister to return renamed sessions
		lister.sessions = renamedSessions

		// Execute command and feed result back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "new-alpha") {
			t.Errorf("expected renamed session 'new-alpha' in list, got:\n%s", view)
		}
		if strings.Contains(view, "Rename:") {
			t.Errorf("rename prompt should be cleared after successful rename, got:\n%s", view)
		}
	})

	t.Run("renamed session appears with new name in list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		renamedSessions := []tmux.Session{
			{Name: "development", Windows: 3, Attached: true},
			{Name: "work", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// R, clear, type new name, Enter
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		for _, r := range "development" {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		lister.sessions = renamedSessions
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "development") {
			t.Errorf("expected 'development' in view, got:\n%s", view)
		}
		if strings.Contains(view, "dev") && !strings.Contains(view, "development") {
			t.Errorf("old name 'dev' should no longer appear, got:\n%s", view)
		}
	})

	t.Run("rename error from tmux is returned in SessionsMsg", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		renamer := &mockSessionRenamer{err: fmt.Errorf("duplicate session name")}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// R, clear, type new name, Enter
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		for _, r := range "bravo" {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

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

	t.Run("cursor stays at same index after rename", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		renamedSessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo-renamed", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		renamer := &mockSessionRenamer{}
		lister := &mockSessionLister{sessions: sessions}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to bravo (index 1)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// R, clear, type new name, Enter
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
		for _, r := range "bravo-renamed" {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		lister.sessions = renamedSessions
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		// Cursor should be on bravo-renamed (index 1)
		lines := strings.Split(view, "\n")
		var bravoLine string
		for _, line := range lines {
			if strings.Contains(line, "bravo-renamed") {
				bravoLine = line
				break
			}
		}
		if !strings.Contains(bravoLine, ">") {
			t.Errorf("cursor should be on 'bravo-renamed' after rename, got:\n%s", view)
		}
	})

	t.Run("R with no renamer configured is no-op", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)

		// Press R — should be no-op (no renamer)
		var model tea.Model = m
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

		view := model.View()
		if strings.Contains(view, "Rename:") {
			t.Errorf("R with no renamer should be no-op, got:\n%s", view)
		}
	})
}

func TestFilterMode(t *testing.T) {
	t.Run("slash activates filter mode", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Press / to enter filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		view := m.View()
		if !strings.Contains(view, "filter:") {
			t.Errorf("expected 'filter:' prompt in view after /, got:\n%s", view)
		}
	})

	t.Run("typing narrows session list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type "br"
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := m.View()
		if !strings.Contains(view, "bravo") {
			t.Errorf("filtered view should contain 'bravo', got:\n%s", view)
		}
		if strings.Contains(view, "alpha") {
			t.Errorf("filtered view should not contain 'alpha', got:\n%s", view)
		}
		if strings.Contains(view, "charlie") {
			t.Errorf("filtered view should not contain 'charlie', got:\n%s", view)
		}
		if !strings.Contains(view, "filter: br") {
			t.Errorf("expected 'filter: br' in view, got:\n%s", view)
		}
	})

	t.Run("fuzzy match filters correctly with subsequence matching", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "my-project", Windows: 1, Attached: false},
			{Name: "dev-work", Windows: 2, Attached: false},
			{Name: "portal-abc", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type "mpr" — subsequence of "my-project"
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := m.View()
		if !strings.Contains(view, "my-project") {
			t.Errorf("fuzzy match should include 'my-project' for 'mpr', got:\n%s", view)
		}
		if strings.Contains(view, "dev-work") {
			t.Errorf("fuzzy match should not include 'dev-work' for 'mpr', got:\n%s", view)
		}
	})

	t.Run("new-in-project option always visible during filter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type "xyz" (matches nothing)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})

		view := m.View()
		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("new-in-project should always be visible during filter, got:\n%s", view)
		}
		if strings.Contains(view, "alpha") {
			t.Errorf("alpha should not be visible with filter 'xyz', got:\n%s", view)
		}
	})

	t.Run("enter selects from filtered list", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type "br", then press Enter
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command, got nil")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg, got %T", msg)
		}

		model, ok := updated.(tui.Model)
		if !ok {
			t.Fatalf("expected tui.Model, got %T", updated)
		}
		if model.Selected() != "bravo" {
			t.Errorf("expected selected %q, got %q", "bravo", model.Selected())
		}
	})

	t.Run("shortcut keys are typeable in filter mode", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "nq-session", Windows: 1, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		// Type shortcut keys: n, K, R, q, k, j
		shortcutKeys := []rune{'n', 'K', 'R', 'q', 'k', 'j'}
		for _, r := range shortcutKeys {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}

		view := m.View()
		if !strings.Contains(view, "filter: nKRqkj") {
			t.Errorf("shortcut keys should be typeable in filter mode, expected 'filter: nKRqkj', got:\n%s", view)
		}
	})

	t.Run("cursor resets when filter text changes", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "anvil", Windows: 2, Attached: false},
			{Name: "bravo", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type "a" (matches alpha, anvil)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

		// Move cursor down to anvil
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Type another character — cursor should reset to 0
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

		view := m.View()
		// After typing "al", only alpha matches. Cursor should be at 0.
		lines := strings.Split(view, "\n")
		var alphaLine string
		for _, line := range lines {
			if strings.Contains(line, "alpha") {
				alphaLine = line
				break
			}
		}
		if !strings.Contains(alphaLine, ">") {
			t.Errorf("cursor should reset to first item when filter changes, got:\n%s", view)
		}
	})

	t.Run("no sessions match filter shows empty filtered list with new-in-project visible", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type something that matches nothing
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})

		view := m.View()
		// No sessions should be listed
		if strings.Contains(view, "alpha") {
			t.Errorf("no sessions should match 'zzz', got:\n%s", view)
		}
		if strings.Contains(view, "bravo") {
			t.Errorf("no sessions should match 'zzz', got:\n%s", view)
		}
		// [n] new in project should still be visible
		if !strings.Contains(view, "[n] new in project...") {
			t.Errorf("[n] new in project should be visible even when no sessions match, got:\n%s", view)
		}
		// Cursor should be on [n] since no sessions match (cursor 0 == len(matched) == 0)
		lines := strings.Split(view, "\n")
		var newLine string
		for _, line := range lines {
			if strings.Contains(line, "new in project") {
				newLine = line
				break
			}
		}
		if !strings.Contains(newLine, ">") {
			t.Errorf("[n] should have cursor when no sessions match, got:\n%s", view)
		}
	})

	t.Run("single character filter works correctly", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type "a" — should match "alpha", "bravo", "charlie" (all contain 'a')
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

		view := m.View()
		if !strings.Contains(view, "alpha") {
			t.Errorf("'alpha' should match filter 'a', got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("'bravo' should match filter 'a', got:\n%s", view)
		}
		if !strings.Contains(view, "charlie") {
			t.Errorf("'charlie' should match filter 'a', got:\n%s", view)
		}
	})

	t.Run("arrow keys navigate filtered results", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "anvil", Windows: 2, Attached: false},
			{Name: "bravo", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type "a" (matches alpha, anvil)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

		// Cursor should be on alpha (index 0)
		view := m.View()
		lines := strings.Split(view, "\n")
		var alphaLine string
		for _, line := range lines {
			if strings.Contains(line, "alpha") {
				alphaLine = line
				break
			}
		}
		if !strings.Contains(alphaLine, ">") {
			t.Errorf("cursor should be on alpha initially, got:\n%s", view)
		}

		// Press down to move to anvil
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		view = m.View()
		lines = strings.Split(view, "\n")
		var anvilLine string
		for _, line := range lines {
			if strings.Contains(line, "anvil") {
				anvilLine = line
				break
			}
		}
		if !strings.Contains(anvilLine, ">") {
			t.Errorf("cursor should be on anvil after down arrow, got:\n%s", view)
		}
	})

	t.Run("esc exits filter mode and clears filter", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type something
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Esc should exit filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := m.View()
		// Should show all sessions again
		if !strings.Contains(view, "alpha") {
			t.Errorf("alpha should be visible after exiting filter mode, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("bravo should be visible after exiting filter mode, got:\n%s", view)
		}
		// Filter prompt should be gone
		if strings.Contains(view, "filter:") {
			t.Errorf("filter prompt should be gone after Esc, got:\n%s", view)
		}
	})

	t.Run("backspace removes last filter character", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type "br"
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Backspace should remove 'r'
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

		view := m.View()
		if !strings.Contains(view, "filter: b") {
			t.Errorf("expected 'filter: b' after backspace, got:\n%s", view)
		}
		// Both alpha and bravo contain 'b' (bravo matches, alpha doesn't)
		if !strings.Contains(view, "bravo") {
			t.Errorf("bravo should match filter 'b', got:\n%s", view)
		}
	})

	t.Run("backspace on empty filter exits filter mode", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		// Backspace on empty filter should exit
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

		view := m.View()
		if strings.Contains(view, "filter:") {
			t.Errorf("backspace on empty filter should exit filter mode, got:\n%s", view)
		}
		// All sessions should be visible
		if !strings.Contains(view, "alpha") {
			t.Errorf("alpha should be visible after exiting filter, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("bravo should be visible after exiting filter, got:\n%s", view)
		}
	})

	t.Run("q in filter mode types q instead of quitting", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "quickfix", Windows: 1, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		// Type 'q' — should be treated as filter input, not quit
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("q in filter mode should not quit")
			}
		}

		view := m.View()
		if !strings.Contains(view, "filter: q") {
			t.Errorf("expected 'filter: q' in view, got:\n%s", view)
		}
	})

	t.Run("rapid backspace presses drain filter then exit mode", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and type "abc"
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

		// Backspace 3 times drains the filter text
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // "ab"
		view := m.View()
		if !strings.Contains(view, "filter: ab") {
			t.Errorf("expected 'filter: ab' after first backspace, got:\n%s", view)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // "a"
		view = m.View()
		if !strings.Contains(view, "filter: a") {
			t.Errorf("expected 'filter: a' after second backspace, got:\n%s", view)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // ""
		view = m.View()
		if !strings.Contains(view, "filter: ") {
			t.Errorf("expected empty filter after third backspace, got:\n%s", view)
		}

		// One more backspace exits filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		view = m.View()
		if strings.Contains(view, "filter:") {
			t.Errorf("fourth backspace should exit filter mode, got:\n%s", view)
		}
		// Full session list restored
		if !strings.Contains(view, "alpha") {
			t.Errorf("alpha should be visible after exiting filter, got:\n%s", view)
		}
		if !strings.Contains(view, "bravo") {
			t.Errorf("bravo should be visible after exiting filter, got:\n%s", view)
		}
	})

	t.Run("full session list restored after exit", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode, type "br" (only bravo visible)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := m.View()
		if strings.Contains(view, "alpha") || strings.Contains(view, "charlie") {
			t.Errorf("only bravo should be visible with filter 'br', got:\n%s", view)
		}

		// Exit via Esc
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		view = m.View()
		for _, name := range []string{"alpha", "bravo", "charlie"} {
			if !strings.Contains(view, name) {
				t.Errorf("session %q should be visible after exiting filter mode, got:\n%s", name, view)
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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

		// Exit via Esc
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// 'q' should now quit (shortcut restored)
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
		if cmd == nil {
			t.Fatal("q after exiting filter mode should trigger quit command")
		}
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Fatalf("expected tea.QuitMsg after q, got %T", msg)
		}
	})

	t.Run("esc with active filter clears and exits in one action", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
			{Name: "charlie", Windows: 3, Attached: false},
		}
		var m tea.Model = tui.NewModelWithSessions(sessions)

		// Enter filter mode and build up a filter
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

		// Verify filter is active and narrowing results
		view := m.View()
		if !strings.Contains(view, "filter: ch") {
			t.Errorf("expected active filter 'ch', got:\n%s", view)
		}

		// Single Esc should clear filter text AND exit filter mode
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		view = m.View()

		// Filter prompt should be gone
		if strings.Contains(view, "filter:") {
			t.Errorf("Esc should exit filter mode entirely, got:\n%s", view)
		}
		// All sessions restored
		for _, name := range []string{"alpha", "bravo", "charlie"} {
			if !strings.Contains(view, name) {
				t.Errorf("session %q should be visible after Esc, got:\n%s", name, view)
			}
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

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"})

		// Init should load projects (not sessions)
		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil command")
		}
		msg := cmd()

		// Should be a ProjectsLoadedMsg, not a SessionsMsg
		projectsMsg, ok := msg.(ui.ProjectsLoadedMsg)
		if !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		if len(projectsMsg.Projects) != 2 {
			t.Fatalf("expected 2 projects, got %d", len(projectsMsg.Projects))
		}

		// Feed projects back to model
		var model tea.Model = m
		model, _ = model.Update(projectsMsg)

		view := model.View()
		// Should show project picker, not session list
		if !strings.Contains(view, "Select a project") {
			t.Errorf("expected project picker view, got:\n%s", view)
		}
		if strings.Contains(view, "No active sessions") {
			t.Errorf("session list should not be shown in command-pending mode, got:\n%s", view)
		}
		if strings.Contains(view, "[n] new in project...") {
			t.Errorf("session list new option should not be shown in command-pending mode, got:\n%s", view)
		}
	})

	t.Run("banner shows command text", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "Command: claude") {
			t.Errorf("expected 'Command: claude' banner, got:\n%s", view)
		}
	})

	t.Run("banner shows multi-arg command joined", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude", "--resume", "--model", "opus"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "Command: claude --resume --model opus") {
			t.Errorf("expected full command in banner, got:\n%s", view)
		}
	})

	t.Run("session list not displayed in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{
				{Name: "existing", Windows: 2, Attached: false},
			}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
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

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude", "--resume"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Select a project
		_, cmd = model.Update(ui.ProjectSelectedMsg{Path: "/code/myapp"})
		if cmd == nil {
			t.Fatal("expected command from project selection, got nil")
		}
		cmd()

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

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press Esc - should quit, not go back to session list
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc in command-pending mode, got nil")
		}
		// The Esc in project picker normally emits BackMsg. In command-pending mode,
		// BackMsg should trigger quit instead of returning to session list.
		quitMsg := cmd()
		// Follow the message chain: BackMsg -> model handles it
		_, cmd = model.Update(quitMsg)
		if cmd == nil {
			t.Fatal("expected quit command after BackMsg in command-pending mode, got nil")
		}
		finalMsg := cmd()
		if _, ok := finalMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", finalMsg)
		}
	})

	t.Run("query filter pre-filled in project picker", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
				{Path: "/code/other", Name: "other"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"}).WithInitialFilter("myapp")

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		// Should show filter text pre-filled
		if !strings.Contains(view, "filter: myapp") {
			t.Errorf("expected pre-filled filter 'myapp', got:\n%s", view)
		}
	})

	t.Run("browse selection applies pending command", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}},
			},
		}

		m := tui.NewWithAllDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
			lister,
			"/home/user",
		).WithCommand([]string{"vim", "."})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Trigger browse
		model, _ = model.Update(ui.BrowseSelectedMsg{})

		// Select directory from browser
		_, cmd = model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/code"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}
		cmd()

		// Verify command was forwarded
		wantCmd := []string{"vim", "."}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, wantCmd)
		}
		for i, arg := range creator.createdCommand {
			if arg != wantCmd[i] {
				t.Errorf("command[%d] = %q, want %q", i, arg, wantCmd[i])
			}
		}
	})

	t.Run("no command starts in session list view", func(t *testing.T) {
		m := tui.New(&mockSessionLister{
			sessions: []tmux.Session{
				{Name: "dev", Windows: 1, Attached: false},
			},
		})

		cmd := m.Init()
		msg := cmd()

		// Should be a SessionsMsg
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
		}

		var model tea.Model = m
		model, _ = model.Update(sessionsMsg)

		view := model.View()
		if !strings.Contains(view, "dev") {
			t.Errorf("expected session 'dev' in view, got:\n%s", view)
		}
		if strings.Contains(view, "Command:") {
			t.Errorf("should not show command banner without pending command, got:\n%s", view)
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
		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{longCmd})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		expected := "Command: " + longCmd
		if !strings.Contains(view, expected) {
			t.Errorf("expected full command in banner %q, got:\n%s", expected, view)
		}
	})

	t.Run("no saved projects shows empty state with browse and command banner", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		creator := &mockSessionCreator{sessionName: "code-abc123"}

		m := tui.NewWithDeps(
			&mockSessionLister{sessions: []tmux.Session{}},
			nil,
			store,
			creator,
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "Command: claude") {
			t.Errorf("expected command banner in empty state, got:\n%s", view)
		}
		if !strings.Contains(view, "browse for directory...") {
			t.Errorf("expected browse option in empty state, got:\n%s", view)
		}
		if !strings.Contains(view, "No saved projects yet.") {
			t.Errorf("expected empty projects message, got:\n%s", view)
		}
	})
}
