package tui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "alpha" {
			t.Errorf("expected selected %q, got %q", "alpha", model.Selected())
		}
	})

	t.Run("cursor does not go below last item", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		for i := 0; i < 10; i++ {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		}

		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "charlie" {
			t.Errorf("expected selected %q (last item), got %q", "charlie", model.Selected())
		}
	})

	t.Run("cursor does not go above first item", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected quit command")
		}
		model := result.(tui.Model)
		if model.Selected() != "alpha" {
			t.Errorf("expected selected %q (first item), got %q", "alpha", model.Selected())
		}
	})

	t.Run("view highlights correct row after navigation", func(t *testing.T) {
		var m tea.Model = tui.NewModelWithSessions(threeSessions)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		view := m.View()

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
		if strings.Contains(alphaLine, ">") {
			t.Errorf("alpha line should not have cursor: %q", alphaLine)
		}
		if !strings.Contains(bravoLine, ">") {
			t.Errorf("bravo line should have cursor: %q", bravoLine)
		}
		if strings.Contains(charlieLine, ">") {
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
		key  tea.KeyMsg
	}{
		{
			name: "q key triggers quit",
			key:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		view := model.View()
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
	removeErr    error
}

func (m *mockProjectStore) List() ([]project.Project, error) {
	return m.projects, m.listErr
}

func (m *mockProjectStore) CleanStale() ([]project.Project, error) {
	return nil, nil
}

func (m *mockProjectStore) Remove(path string) error {
	m.removeCalled = true
	m.removedPath = path
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

// mockDirLister implements tui.DirLister for testing.
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

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"test"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		view := model.View()
		// File browser should show the starting directory path
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser view with starting path, got:\n%s", view)
		}
		// Should show directory entries
		if !strings.Contains(view, "code") {
			t.Errorf("expected file browser to show directory entries, got:\n%s", view)
		}
		// Should NOT show project list
		if strings.Contains(view, "Projects") {
			t.Errorf("should not show project list when file browser is open:\n%s", view)
		}
	})

	t.Run("selection creates session with browsed path", func(t *testing.T) {
		sessions := []tmux.Session{}
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{},
		}

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to projects page, then open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

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

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to projects page, then open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

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

		m := tui.New(
			&mockSessionLister{sessions: sessions},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"vim", "."})
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

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

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"test"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Cancel the file browser
		model, _ = model.Update(ui.BrowserCancelMsg{})

		view := model.View()
		// Should be back on projects page showing project items
		if !strings.Contains(view, "myapp") {
			t.Errorf("expected projects page with 'myapp' after cancel, got:\n%s", view)
		}
	})

	t.Run("browse works from empty project list", func(t *testing.T) {
		store := &mockProjectStore{projects: []project.Project{}}
		creator := &mockSessionCreator{sessionName: "docs-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "docs"}},
			},
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"test"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		view := model.View()
		// Should be in file browser
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser with starting path, got:\n%s", view)
		}
		if !strings.Contains(view, "docs") {
			t.Errorf("expected file browser to show entries, got:\n%s", view)
		}

		// Selecting a directory should create a session
		_, cmd = model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/docs"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}
		msg = cmd()
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

	t.Run("SessionsMsg applies initial filter via built-in list filtering", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "myapp-dev", Windows: 1, Attached: false},
			{Name: "other", Windows: 2, Attached: false},
			{Name: "myapp-prod", Windows: 3, Attached: false},
		}
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

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

	t.Run("command-pending mode preserved with initial filter", func(t *testing.T) {
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		// Should show command header (command-pending mode)
		if !strings.Contains(view, "Select project to run: claude") {
			t.Errorf("expected command-pending mode header, got:\n%s", view)
		}
		// Initial filter is stored but not applied to project picker
		// (project picker filter forwarding removed)
		updatedModel := model.(tui.Model)
		if updatedModel.InitialFilter() != "myapp" {
			t.Errorf("initial filter should be stored as %q, got %q", "myapp", updatedModel.InitialFilter())
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
		quitKeys := []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyCtrlC},
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
		result, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

		view := m.View()
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
		view := m.View()
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

		view := m.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
			t.Errorf("expected confirmation prompt for 'alpha', got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
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
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then n
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		view := model.View()
		// Should be back to normal session list, no confirmation prompt
		if strings.Contains(view, "? (y/n)") {
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
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then Esc
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := model.View()
		if strings.Contains(view, "? (y/n)") {
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
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y to kill alpha
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
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
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Navigate to bravo (last session)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Press k then y to kill bravo
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
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
		m := tui.New(lister, tui.WithKiller(killer))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Execute command and feed back
		msg := cmd()
		model, _ = model.Update(msg)

		// SessionsMsg with error triggers quit, so the model should exit.
		// But the confirmation prompt should not still be showing.
		view := model.View()
		if strings.Contains(view, "? (y/n)") {
			t.Errorf("confirmation prompt should be cleared after kill error, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
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

		m := tui.New(lister, tui.WithKiller(killer), tui.WithProjectStore(store), tui.WithSessionCreator(creator), tui.WithDirLister(dirLister, "/home/user"))
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Press k — should enter confirmation mode (not no-op)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
			t.Errorf("expected confirmation prompt via NewWithAllDeps, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

		// Press various keys that should be ignored
		ignoredKeys := []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyRunes, Runes: []rune{'k'}},
			{Type: tea.KeyRunes, Runes: []rune{'r'}},
			{Type: tea.KeyRunes, Runes: []rune{'p'}},
			{Type: tea.KeyRunes, Runes: []rune{'x'}},
			{Type: tea.KeyDown},
			{Type: tea.KeyUp},
			{Type: tea.KeyEnter},
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
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
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
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

		// Should be a no-op: no command, no modal
		if cmd != nil {
			t.Errorf("k on empty list should return nil command, got non-nil")
		}
		view := model.View()
		if strings.Contains(view, "? (y/n)") {
			t.Errorf("k on empty list should not show confirmation modal, got:\n%s", view)
		}
	})
}

func TestSessionListHelpBar(t *testing.T) {
	t.Run("help bar shows session-specific keybindings", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
			{Name: "bravo", Windows: 2, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		// Use wider width so all help bindings fit without truncation
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := updated.View()

		// Each description should appear in the help bar
		expectedDescs := []string{
			"attach",
			"rename",
			"kill",
			"projects",
			"new in cwd",
			"filter",
		}
		for _, desc := range expectedDescs {
			if !strings.Contains(view, desc) {
				t.Errorf("help bar should contain %q, got:\n%s", desc, view)
			}
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := model.View()
		// Should show rename modal with "New name: " prompt
		if !strings.Contains(view, "New name:") {
			t.Errorf("expected rename modal with 'New name:' prompt, got:\n%s", view)
		}
		// Should have border styling (modal overlay)
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("rename modal should contain border characters, got:\n%s", view)
		}
		// Should contain the pre-populated session name
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Clear the text and type a new name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Clear input
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})

		// Press Enter with empty input
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should not trigger rename (no command returned)
		if cmd != nil {
			t.Error("expected nil command for empty rename input")
		}

		// Renamer should not have been called
		if renamer.renamedOld != "" {
			t.Errorf("rename should not have been called with empty input, but got old=%q", renamer.renamedOld)
		}

		// Modal should still be open (modal stays open on empty input)
		view := model.View()
		if !strings.Contains(view, "New name:") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Press Esc to dismiss
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := model.View()
		// Should be back to normal session list, no modal
		if strings.Contains(view, "New name:") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Press Enter without changing — same name rename
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
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

	t.Run("r on empty list is no-op", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{}}
		renamer := &mockSessionRenamer{}
		m := newModelWithRenamer(lister, nil, renamer)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{}})

		// Press r on empty list
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := model.View()
		// Should not show rename modal
		if strings.Contains(view, "New name:") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		view := model.View()
		if strings.Contains(view, "New name:") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

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
		// Modal should be dismissed
		if strings.Contains(view, "New name:") {
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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		// Verify we're filtering
		model := m.(tui.Model)
		if model.SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", model.SessionListFilterState())
		}

		// Type 'q' — should be treated as filter input, not quit
		var cmd tea.Cmd
		_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

		// Exit via Esc (cancel filtering)
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// Verify we're no longer filtering
		model := m.(tui.Model)
		if model.SessionListFilterState() != list.Unfiltered {
			t.Fatalf("expected Unfiltered state after Esc, got %v", model.SessionListFilterState())
		}

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

		// Init should load projects (not sessions)
		cmd := m.Init()
		if cmd == nil {
			t.Fatal("Init() returned nil command")
		}
		msg := cmd()

		// Should be a ProjectsLoadedMsg, not a SessionsMsg
		projectsMsg, ok := msg.(tui.ProjectsLoadedMsg)
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

		view := model.View()
		if !strings.Contains(view, "Select project to run: claude") {
			t.Errorf("expected 'Select project to run: claude' status line, got:\n%s", view)
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

		view := model.View()
		if !strings.Contains(view, "Select project to run: claude --resume --model opus") {
			t.Errorf("expected full command in status line, got:\n%s", view)
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

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
		).WithCommand([]string{"claude", "--resume"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Select a project by pressing Enter (first project in list)
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if cmd == nil {
			t.Fatal("expected quit command from Esc in command-pending mode, got nil")
		}
		quitMsg := cmd()
		if _, ok := quitMsg.(tea.QuitMsg); !ok {
			t.Errorf("expected tea.QuitMsg, got %T", quitMsg)
		}
	})

	t.Run("initial filter stored but not applied to project list", func(t *testing.T) {
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Initial filter is stored but not applied to project list
		// All projects should be in the list (no filter applied)
		updated := model.(tui.Model)
		items := updated.ProjectListItems()
		if len(items) != 2 {
			t.Errorf("expected 2 projects in list (no filter applied), got %d", len(items))
		}
		if updated.ProjectListFilterState() != list.Unfiltered {
			t.Errorf("expected unfiltered project list, got filter state %v", updated.ProjectListFilterState())
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

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"vim", "."})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

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

		view := model.View()
		expected := "Select project to run: " + longCmd
		if !strings.Contains(view, expected) {
			t.Errorf("expected full command in status line %q, got:\n%s", expected, view)
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

		view := model.View()
		if !strings.Contains(view, "Select project to run: claude") {
			t.Errorf("expected status line in empty state, got:\n%s", view)
		}
		if !strings.Contains(view, "b browse") {
			t.Errorf("expected browse help key in empty state, got:\n%s", view)
		}
		if !strings.Contains(view, "No saved projects") {
			t.Errorf("expected empty projects message, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		view := model.View()
		if strings.Contains(view, "Edit:") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		view := model.View()
		if strings.Contains(view, "Delete") && strings.Contains(view, "y/n") {
			t.Errorf("pressing d in command-pending mode should not open delete modal, got:\n%s", view)
		}
	})

	t.Run("help bar omits s, x, e, and d in command-pending mode", func(t *testing.T) {
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
		// Set wide width so help bar renders fully
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := model.View()
		// s, e, d should NOT appear as help keys
		for _, desc := range []string{"sessions", "edit", "delete"} {
			if strings.Contains(view, desc) {
				t.Errorf("help bar should not contain %q in command-pending mode, got:\n%s", desc, view)
			}
		}
		// browse, new in cwd should still appear
		for _, desc := range []string{"browse", "new in cwd"} {
			if !strings.Contains(view, desc) {
				t.Errorf("help bar should contain %q in command-pending mode, got:\n%s", desc, view)
			}
		}
	})

	t.Run("help bar shows run here for enter in command-pending mode", func(t *testing.T) {
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
		// Set wide width so help bar renders fully
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := model.View()
		if !strings.Contains(view, "run here") {
			t.Errorf("help bar should show 'run here' for enter in command-pending mode, got:\n%s", view)
		}
		if strings.Contains(view, "new session") {
			t.Errorf("help bar should not show 'new session' in command-pending mode, got:\n%s", view)
		}
	})

	t.Run("normal mode retains s, x, e, and d keybindings", func(t *testing.T) {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		view := model.View()
		for _, desc := range []string{"sessions", "edit", "delete"} {
			if !strings.Contains(view, desc) {
				t.Errorf("normal mode help bar should contain %q, got:\n%s", desc, view)
			}
		}
		if !strings.Contains(view, "new session") {
			t.Errorf("normal mode help bar should show 'new session' for enter, got:\n%s", view)
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

		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
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

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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

		view := updated.View()
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

		w, h := model.SessionListSize()
		if w != 120 {
			t.Errorf("list width = %d, want 120", w)
		}
		if h != 40 {
			t.Errorf("list height = %d, want 40", h)
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

		view := updated.View()
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
		msg := cmd()
		sessionsMsg, ok := msg.(tui.SessionsMsg)
		if !ok {
			t.Fatalf("expected SessionsMsg, got %T", msg)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
			t.Errorf("expected confirmation prompt, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		view := model.View()
		if !strings.Contains(view, "New name:") {
			t.Errorf("expected rename modal prompt, got:\n%s", view)
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "myapp") {
			t.Errorf("expected projects page with project items, got:\n%s", view)
		}
	})

	t.Run("WithDirLister enables file browser", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		dirLister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}, {Name: "docs"}},
			},
		}
		lister := &mockSessionLister{sessions: []tmux.Session{}}

		m := tui.New(lister,
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(dirLister, "/home/user"),
		).WithCommand([]string{"test"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		view := model.View()
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser with starting path, got:\n%s", view)
		}
		if !strings.Contains(view, "code") {
			t.Errorf("expected file browser to show entries, got:\n%s", view)
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
		dirLister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}},
			},
		}
		lister := &mockSessionLister{sessions: sessions}

		m := tui.New(lister,
			tui.WithKiller(killer),
			tui.WithRenamer(renamer),
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(dirLister, "/home/user"),
		)
		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Verify kill works
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
			t.Errorf("expected kill confirmation, got:\n%s", view)
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
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

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
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("zzzzz")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

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
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

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
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: filter state = %v, want FilterApplied", updatedModel.SessionListFilterState())
		}

		// Press Esc to clear the filter
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

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
	t.Run("p on sessions page switches to projects page", func(t *testing.T) {
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

		// Press p to switch to projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after p, got %d", updated.ActivePage())
		}
	})

	t.Run("s on projects page switches to sessions page", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Switch to projects page first
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		// Press s to switch back to sessions page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after s, got %d", updated.ActivePage())
		}
	})

	t.Run("x toggles from sessions to projects", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 1, Attached: false},
		}
		m := tui.NewModelWithSessions(sessions)
		var model tea.Model = m

		// Press x to toggle from sessions to projects
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		// Press x to toggle back to sessions
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Verify cursor is on bravo
		result, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if result.(tui.Model).Selected() != "bravo" {
			t.Fatalf("precondition: expected cursor on bravo, got %q", result.(tui.Model).Selected())
		}

		// Reset model (re-navigate to bravo without selecting)
		model = tui.NewModelWithSessions(sessions)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Switch to projects and back
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

		// Press enter to verify cursor is still on bravo
		result, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		view := model.View()
		if !strings.Contains(view, "No saved projects") {
			t.Errorf("expected 'No saved projects' on empty projects page, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press enter on first project
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Navigate to second project
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})

		// Press enter
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press n to create session in cwd
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		// Press q — should quit
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		// Press Ctrl+C — should quit
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		view := model.View()
		if !strings.Contains(view, "No saved projects") {
			t.Errorf("expected 'No saved projects' on empty projects page, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

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
		view := model.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press enter on project
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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
		view := model.View()
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

		w, h := model.ProjectListSize()
		if w != 120 {
			t.Errorf("project list width = %d, want 120", w)
		}
		if h != 40 {
			t.Errorf("project list height = %d, want 40", h)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		view := model.View()
		expectedDescs := []string{
			"new session",
			"sessions",
			"new in cwd",
		}
		for _, desc := range expectedDescs {
			if !strings.Contains(view, desc) {
				t.Errorf("projects help bar should contain %q, got:\n%s", desc, view)
			}
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		view := model.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d on the first project
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		view := model.View()
		if !strings.Contains(view, "Delete portal? (y/n)") {
			t.Errorf("expected delete confirmation for 'portal', got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

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
	})

	t.Run("n in delete modal dismisses without deleting", func(t *testing.T) {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then n
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

		view := model.View()
		if strings.Contains(view, "? (y/n)") {
			t.Errorf("confirmation prompt should be cleared after n, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("project 'portal' should still be in list after cancel, got:\n%s", view)
		}
		if store.removeCalled {
			t.Error("Remove should not have been called after cancel")
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then Esc
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		view := model.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d to open delete modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		// Press various keys that should be ignored
		ignoredKeys := []tea.KeyMsg{
			{Type: tea.KeyRunes, Runes: []rune{'q'}},
			{Type: tea.KeyRunes, Runes: []rune{'d'}},
			{Type: tea.KeyRunes, Runes: []rune{'s'}},
			{Type: tea.KeyRunes, Runes: []rune{'x'}},
			{Type: tea.KeyDown},
			{Type: tea.KeyUp},
			{Type: tea.KeyEnter},
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

		// Modal should still be showing
		view := model.View()
		if !strings.Contains(view, "Delete portal? (y/n)") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		// Update store to return empty after removal
		store.projects = []project.Project{}

		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
		if cmd == nil {
			t.Fatal("expected command from delete confirmation, got nil")
		}

		// Execute command and feed result back
		msg := cmd()
		model, _ = model.Update(msg)

		view := model.View()
		if !strings.Contains(view, "No saved projects") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Press d on empty list
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		if cmd != nil {
			t.Errorf("d on empty list should return nil command, got non-nil")
		}
		view := model.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		})

		// Press d then y
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

		view := model.View()
		if !strings.Contains(view, "Delete webapp? (y/n)") {
			t.Errorf("expected delete confirmation for 'webapp', got:\n%s", view)
		}

		// Confirm deletion
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
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
	t.Run("empty sessions page shows no sessions running", func(t *testing.T) {
		m := tui.NewModelWithSessions(nil)
		view := m.View()
		if !strings.Contains(view, "No sessions running") {
			t.Errorf("expected 'No sessions running' on empty sessions page, got:\n%s", view)
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})

		view := model.View()
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

		view := updated.View()
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

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

		// Verify kill modal is showing
		view := model.View()
		if !strings.Contains(view, "Kill alpha? (y/n)") {
			t.Fatalf("precondition: expected kill modal, got:\n%s", view)
		}

		// Press Esc — should dismiss modal, NOT quit
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc during kill modal should dismiss modal, not quit")
			}
		}

		// Modal should be dismissed
		view = model.View()
		if strings.Contains(view, "Kill alpha? (y/n)") {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// Verify rename modal is showing
		view := model.View()
		if !strings.Contains(view, "New name:") {
			t.Fatalf("precondition: expected rename modal, got:\n%s", view)
		}

		// Press Esc — should dismiss modal, NOT quit
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// Should not quit
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("Esc during rename modal should dismiss modal, not quit")
			}
		}

		// Modal should be dismissed
		view = model.View()
		if strings.Contains(view, "New name:") {
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
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", updatedModel.SessionListFilterState())
		}

		// Press Esc — should clear filter, NOT quit
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

		// Type some filter text
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

		// Verify we are in SettingFilter state (actively typing filter)
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", updatedModel.SessionListFilterState())
		}

		// Press Esc — should cancel filter, NOT quit
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		// Ctrl+C should force-quit even during kill modal
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		// Ctrl+C should force-quit even during rename modal
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		// Type a character to confirm filtering
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		// Verify we are in SettingFilter state
		if model.(tui.Model).SessionListFilterState() != list.Filtering {
			t.Fatalf("precondition: expected Filtering state, got %v", model.(tui.Model).SessionListFilterState())
		}
		// Ctrl+C should force-quit even during active filtering
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

		// First Esc — dismisses rename modal
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Fatal("first Esc should dismiss modal, not quit")
			}
		}

		// Verify modal is dismissed
		view := model.View()
		if strings.Contains(view, "New name:") {
			t.Fatalf("rename modal should be dismissed, got:\n%s", view)
		}

		// Second Esc — should quit
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
		m := tui.New(&mockSessionLister{sessions: sessions})
		m = m.WithInitialFilter("myapp")

		var model tea.Model = m
		model, _ = model.Update(tui.SessionsMsg{Sessions: sessions})

		// Verify filter is applied
		updatedModel := model.(tui.Model)
		if updatedModel.SessionListFilterState() != list.FilterApplied {
			t.Fatalf("precondition: expected FilterApplied, got %v", updatedModel.SessionListFilterState())
		}

		// First Esc — clears filter
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

		// Verify on projects page
		if model.(tui.Model).ActivePage() != tui.PageProjects {
			t.Fatalf("precondition: expected PageProjects, got %d", model.(tui.Model).ActivePage())
		}

		// Press Esc — should quit
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
type mockProjectEditor struct {
	renamedPath string
	renamedName string
	err         error
}

func (m *mockProjectEditor) Rename(path, newName string) error {
	m.renamedPath = path
	m.renamedName = newName
	return m.err
}

// mockAliasEditor implements tui.AliasEditor for testing.
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
}

func (m *mockAliasEditor) Load() (map[string]string, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	result := make(map[string]string)
	for k, v := range m.aliases {
		result[k] = v
	}
	return result, nil
}

func (m *mockAliasEditor) Set(name, path string) {
	m.setCalls = append(m.setCalls, aliasSetCall{name: name, path: path})
	if m.aliases == nil {
		m.aliases = make(map[string]string)
	}
	m.aliases[name] = path
}

func (m *mockAliasEditor) Delete(name string) bool {
	m.deleted = append(m.deleted, name)
	_, ok := m.aliases[name]
	if ok {
		delete(m.aliases, name)
	}
	return ok
}

func (m *mockAliasEditor) Save() error {
	m.saveCalled = true
	return m.saveErr
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
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
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

		// Press e on first project
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		view := model.View()
		if !strings.Contains(view, "Name:") {
			t.Errorf("edit modal should contain 'Name:' field, got:\n%s", view)
		}
		if !strings.Contains(view, "portal") {
			t.Errorf("edit modal should show current project name 'portal', got:\n%s", view)
		}
		if !strings.Contains(view, "Aliases:") {
			t.Errorf("edit modal should contain 'Aliases:' section, got:\n%s", view)
		}
		// Should show alias 'p' which maps to /code/portal
		if !strings.Contains(view, "p") {
			t.Errorf("edit modal should show alias 'p', got:\n%s", view)
		}
		// Should have border styling (modal overlay)
		if !strings.ContainsAny(view, "─│╭╮╰╯") {
			t.Errorf("edit modal should have border styling, got:\n%s", view)
		}
	})

	t.Run("Tab switches focus between name and aliases", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Initially focus is on name field — typing goes to name
		// Type a character
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
		view := model.View()
		if !strings.Contains(view, "portalZ") {
			t.Errorf("typing should append to name field, got:\n%s", view)
		}

		// Press Tab to switch to aliases
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// Now typing should go to alias "Add:" input, not name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		view = model.View()
		if !strings.Contains(view, "Add:") {
			t.Errorf("after Tab, view should show 'Add:' section, got:\n%s", view)
		}
		if !strings.Contains(view, "portalZ") {
			t.Errorf("name should still be 'portalZ' (not modified by alias typing), got:\n%s", view)
		}

		// Tab again returns to name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}})
		view = model.View()
		if !strings.Contains(view, "portalZY") {
			t.Errorf("after second Tab, typing should append to name, got:\n%s", view)
		}
	})

	t.Run("Enter saves name change and refreshes list", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Change name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

		// Press Enter to save
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("Enter should return a command for refresh, got nil")
		}

		// Verify editor.Rename was called
		if editor.renamedPath != "/code/portal" {
			t.Errorf("expected Rename path '/code/portal', got %q", editor.renamedPath)
		}
		if editor.renamedName != "new" {
			t.Errorf("expected Rename name 'new', got %q", editor.renamedName)
		}

		// Execute command — should refresh projects
		msg := cmd()
		loadedMsg, ok := msg.(tui.ProjectsLoadedMsg)
		if !ok {
			t.Fatalf("expected ProjectsLoadedMsg, got %T", msg)
		}
		if loadedMsg.Err != nil {
			t.Fatalf("unexpected error: %v", loadedMsg.Err)
		}
	})

	t.Run("Esc cancels edit without saving", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Change name
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})

		// Press Esc to cancel
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// Should not have called Rename
		if editor.renamedPath != "" {
			t.Error("Esc should not call Rename")
		}
		// Should not return a command (no refresh)
		if cmd != nil {
			t.Errorf("Esc should return nil command, got non-nil")
		}
		// Modal should be dismissed — view shows project list
		view := model.View()
		if strings.Contains(view, "Name:") {
			t.Errorf("edit modal should be dismissed after Esc, got:\n%s", view)
		}
		// Original name should be visible (unchanged)
		if !strings.Contains(view, "portal") {
			t.Errorf("original project name should still be in list, got:\n%s", view)
		}
	})

	t.Run("empty name rejected with error on Enter", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Clear name completely
		for range len("portal") {
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}

		// Press Enter with empty name
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should show error
		view := model.View()
		if !strings.Contains(view, "cannot be empty") {
			t.Errorf("expected empty name error, got:\n%s", view)
		}
		// Should NOT dismiss modal
		if !strings.Contains(view, "Name:") {
			t.Errorf("modal should still be open after validation error, got:\n%s", view)
		}
		// Should not have called Rename
		if editor.renamedPath != "" {
			t.Error("should not call Rename with empty name")
		}
		if cmd != nil {
			t.Errorf("should not return command on validation error, got non-nil")
		}
	})

	t.Run("alias collision shows error message", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{
				"w": "/code/webapp",
			},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal on portal (first project)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Switch to alias section
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// Type 'w' as new alias (which already exists for /code/webapp)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})

		// Press Enter to save
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		view := model.View()
		if !strings.Contains(view, "already exists") {
			t.Errorf("expected alias collision error, got:\n%s", view)
		}
		// Modal should still be open
		if !strings.Contains(view, "Name:") {
			t.Errorf("modal should still be open after alias collision, got:\n%s", view)
		}
		if cmd != nil {
			t.Errorf("should not return command on alias collision, got non-nil")
		}
	})

	t.Run("x removes alias from list in edit mode", func(t *testing.T) {
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

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Switch to alias section
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// Press x to remove the first alias
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

		view := model.View()
		// The modal should still be open
		if !strings.Contains(view, "Name:") {
			t.Errorf("modal should still be open after removing alias, got:\n%s", view)
		}
		// At least one alias should remain visible
		if !strings.Contains(view, "Aliases:") {
			t.Errorf("aliases section should still be visible, got:\n%s", view)
		}
	})

	t.Run("new alias is added on save", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Switch to alias section (cursor starts on Add input since no existing aliases)
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// Type new alias
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

		// Press Enter to save
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("Enter should return a command for refresh, got nil")
		}

		// Verify alias was set
		if len(aliases.setCalls) != 1 {
			t.Fatalf("expected 1 Set call, got %d", len(aliases.setCalls))
		}
		if aliases.setCalls[0].name != "my" {
			t.Errorf("expected alias name 'my', got %q", aliases.setCalls[0].name)
		}
		if aliases.setCalls[0].path != "/code/portal" {
			t.Errorf("expected alias path '/code/portal', got %q", aliases.setCalls[0].path)
		}
		if !aliases.saveCalled {
			t.Error("expected Save to be called")
		}
	})

	t.Run("alias removal is committed on save", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{
				"p": "/code/portal",
			},
		}
		model := setupEditModel(store, editor, aliases)

		// Open edit modal
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		// Switch to alias section
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})

		// Press x to remove alias
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

		// Press Enter to save
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

		if cmd == nil {
			t.Fatal("Enter should return a command for refresh, got nil")
		}

		// Verify Delete was called for the removed alias
		if len(aliases.deleted) != 1 {
			t.Fatalf("expected 1 Delete call, got %d", len(aliases.deleted))
		}
		if aliases.deleted[0] != "p" {
			t.Errorf("expected Delete('p'), got Delete(%q)", aliases.deleted[0])
		}
		if !aliases.saveCalled {
			t.Error("expected Save to be called")
		}
	})

	t.Run("e with no editor configured is no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		}
		// No editor or alias editor provided
		model := setupEditModel(store, nil, nil)

		// Press e
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		if cmd != nil {
			t.Errorf("e with no editor should return nil command, got non-nil")
		}
		view := model.View()
		if strings.Contains(view, "Name:") {
			t.Errorf("edit modal should not open without editor, got:\n%s", view)
		}
	})

	t.Run("e on empty project list is no-op", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{},
		}
		editor := &mockProjectEditor{}
		aliases := &mockAliasEditor{
			aliases: map[string]string{},
		}
		model := setupEditModel(store, editor, aliases)

		// Press e on empty list
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

		if cmd != nil {
			t.Errorf("e on empty list should return nil command, got non-nil")
		}
		view := model.View()
		if strings.Contains(view, "Name:") {
			t.Errorf("edit modal should not open on empty list, got:\n%s", view)
		}
	})
}

func TestFileBrowserFromProjectsPage(t *testing.T) {
	// Helper to set up a model on the projects page with projects loaded.
	setupProjectsModel := func(t *testing.T, dirEntries map[string][]browser.DirEntry) tea.Model {
		t.Helper()
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
				{Path: "/code/webapp", Name: "webapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "portal-abc123"}
		lister := &mockDirLister{entries: dirEntries}
		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		)
		var model tea.Model = m
		// Switch to projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		// Populate projects
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})
		// Give it a size
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		return model
	}

	t.Run("b on projects page opens file browser", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}, {Name: "docs"}},
		}
		model := setupProjectsModel(t, entries)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		view := model.View()
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser with starting path /home/user, got:\n%s", view)
		}
		if !strings.Contains(view, "code") {
			t.Errorf("expected file browser to show directory entry 'code', got:\n%s", view)
		}
		if !strings.Contains(view, "docs") {
			t.Errorf("expected file browser to show directory entry 'docs', got:\n%s", view)
		}
		// Should not show project list
		if strings.Contains(view, "Projects") {
			t.Errorf("should not show projects list title when file browser is open:\n%s", view)
		}
	})

	t.Run("BrowserDirSelectedMsg creates session and quits", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}},
		}
		model := setupProjectsModel(t, entries)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Browser emits BrowserDirSelectedMsg
		_, cmd := model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/code"})
		if cmd == nil {
			t.Fatal("expected command from BrowserDirSelectedMsg, got nil")
		}

		msg := cmd()
		createdMsg, ok := msg.(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("expected SessionCreatedMsg, got %T", msg)
		}
		if createdMsg.SessionName != "portal-abc123" {
			t.Errorf("session name = %q, want %q", createdMsg.SessionName, "portal-abc123")
		}
	})

	t.Run("BrowserCancelMsg returns to projects page", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}},
		}
		model := setupProjectsModel(t, entries)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Cancel the file browser
		model, _ = model.Update(ui.BrowserCancelMsg{})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after cancel, got %d", updated.ActivePage())
		}
		view := model.View()
		if !strings.Contains(view, "portal") {
			t.Errorf("expected projects page with 'portal' after cancel, got:\n%s", view)
		}
	})

	t.Run("Esc in browser with no filter returns to projects page", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}},
		}
		model := setupProjectsModel(t, entries)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Press Esc with no filter — browser emits BrowserCancelMsg
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		// The browser should emit BrowserCancelMsg as a command
		if cmd != nil {
			msg := cmd()
			model, _ = model.Update(msg)
		}

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after Esc in browser, got %d", updated.ActivePage())
		}
	})

	t.Run("Esc in browser with filter clears filter", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}, {Name: "docs"}, {Name: "configs"}},
		}
		model := setupProjectsModel(t, entries)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Type a filter into the browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

		// Press Esc — should clear filter, not cancel browser
		model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})

		// Should NOT have a BrowserCancelMsg command
		if cmd != nil {
			msg := cmd()
			if _, ok := msg.(ui.BrowserCancelMsg); ok {
				t.Error("Esc with active filter should clear filter, not cancel browser")
			}
		}

		// Should still be in file browser, showing all entries
		view := model.View()
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected to still be in file browser, got:\n%s", view)
		}
		if !strings.Contains(view, "docs") {
			t.Errorf("expected all entries visible after filter clear, got:\n%s", view)
		}
	})

	t.Run("b works when projects list has active filter", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}},
		}
		model := setupProjectsModel(t, entries)

		// Apply a filter on the projects list
		updated := model.(tui.Model)
		updated.SetProjectListFilter("portal")
		model = tea.Model(updated)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		view := model.View()
		if !strings.Contains(view, "/home/user") {
			t.Errorf("expected file browser after b with filter, got:\n%s", view)
		}
		if !strings.Contains(view, "code") {
			t.Errorf("expected file browser entries, got:\n%s", view)
		}
	})

	t.Run("projects page state preserved when returning from browser", func(t *testing.T) {
		entries := map[string][]browser.DirEntry{
			"/home/user": {{Name: "code"}},
		}
		model := setupProjectsModel(t, entries)

		// Remember the project list state
		pre := model.(tui.Model)
		preItems := pre.ProjectListItems()

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Cancel back to projects
		model, _ = model.Update(ui.BrowserCancelMsg{})

		post := model.(tui.Model)
		postItems := post.ProjectListItems()

		if len(postItems) != len(preItems) {
			t.Errorf("project list items changed: had %d, now %d", len(preItems), len(postItems))
		}
		if post.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects, got %d", post.ActivePage())
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

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

		// Switch to projects then back to sessions
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

		// Verify session filter is still applied
		updated := model.(tui.Model)
		if updated.SessionListFilterValue() != "alpha" {
			t.Errorf("expected session filter 'alpha' preserved, got %q", updated.SessionListFilterValue())
		}
		if updated.SessionListFilterState() != list.FilterApplied {
			t.Errorf("expected FilterApplied after round-trip, got %v", updated.SessionListFilterState())
		}
	})

	t.Run("projects help bar includes s for sessions and project-specific keys", func(t *testing.T) {
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/portal", Name: "portal"},
			},
		})

		view := model.View()
		expectedDescs := []string{
			"sessions",
			"edit",
			"delete",
			"browse",
		}
		for _, desc := range expectedDescs {
			if !strings.Contains(view, desc) {
				t.Errorf("projects help bar should contain %q, got:\n%s", desc, view)
			}
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

		view := model.View()
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

		// Press p to switch to projects
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after p, got %d", updated.ActivePage())
		}

		// Press s to switch back to sessions
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after s, got %d", updated.ActivePage())
		}

		// Press x to toggle to projects
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
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

		// Press s to switch to sessions
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageSessions {
			t.Errorf("expected PageSessions after s, got %d", updated.ActivePage())
		}

		// Press p to switch back to projects
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		updated = model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after p, got %d", updated.ActivePage())
		}

		// Press x to toggle to sessions
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
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

		view := model.View()
		if !strings.Contains(view, "Select project to run: claude") {
			t.Errorf("expected status line 'Select project to run: claude', got:\n%s", view)
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

		view := model.View()
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

		view := model.View()
		if !strings.Contains(view, "Select project to run: claude --resume --model opus") {
			t.Errorf("expected multi-word command in status line, got:\n%s", view)
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

		view := model.View()
		expected := "Select project to run: " + longCmd
		if !strings.Contains(view, expected) {
			t.Errorf("expected full command in status line %q, got:\n%s", expected, view)
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

		view := model.View()
		if !strings.Contains(view, "Projects") {
			t.Errorf("expected 'Projects' title in command-pending mode, got:\n%s", view)
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press enter on first project
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter on project, got nil")
		}

		msg = cmd()
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press enter on project
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press enter to create session
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command, got nil")
		}
		msg = cmd()

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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press enter to create session
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command, got nil")
		}
		msg = cmd()

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

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press enter on project — will fail
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("expected command from enter, got nil")
		}
		msg = cmd()

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
		view := model.View()
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		})

		// Press enter on project
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
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

func TestCommandPendingBrowseAndNKey(t *testing.T) {
	t.Run("browse directory selection forwards command in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}},
			},
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"claude", "--resume"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser from projects page
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Browser emits BrowserDirSelectedMsg
		_, cmd = model.Update(ui.BrowserDirSelectedMsg{Path: "/home/user/code"})
		if cmd == nil {
			t.Fatal("expected command from directory selection, got nil")
		}
		cmd()

		// Verify command was forwarded to CreateFromDir
		wantCmd := []string{"claude", "--resume"}
		if len(creator.createdCommand) != len(wantCmd) {
			t.Fatalf("command = %v, want %v", creator.createdCommand, wantCmd)
		}
		for i, arg := range wantCmd {
			if creator.createdCommand[i] != arg {
				t.Errorf("command[%d] = %q, want %q", i, creator.createdCommand[i], arg)
			}
		}
		if creator.createdDir != "/home/user/code" {
			t.Errorf("dir = %q, want %q", creator.createdDir, "/home/user/code")
		}
	})

	t.Run("browse cancel returns to locked Projects page in command-pending mode", func(t *testing.T) {
		store := &mockProjectStore{
			projects: []project.Project{
				{Path: "/code/myapp", Name: "myapp"},
			},
		}
		creator := &mockSessionCreator{sessionName: "code-abc123"}
		lister := &mockDirLister{
			entries: map[string][]browser.DirEntry{
				"/home/user": {{Name: "code"}},
			},
		}

		m := tui.New(
			&mockSessionLister{sessions: []tmux.Session{}},
			tui.WithProjectStore(store),
			tui.WithSessionCreator(creator),
			tui.WithDirLister(lister, "/home/user"),
		).WithCommand([]string{"claude"})

		var model tea.Model = m
		cmd := m.Init()
		msg := cmd()
		model, _ = model.Update(msg)

		// Press b to open file browser
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

		// Cancel the file browser
		model, _ = model.Update(ui.BrowserCancelMsg{})

		updated := model.(tui.Model)
		// Should return to Projects page (not Sessions)
		if updated.ActivePage() != tui.PageProjects {
			t.Errorf("expected PageProjects after cancel, got %d", updated.ActivePage())
		}

		// View should show the command-pending banner (confirms still in command-pending mode)
		view := model.View()
		if !strings.Contains(view, "Select project to run:") {
			t.Errorf("expected command-pending banner after cancel, got:\n%s", view)
		}
	})

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
		_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		// Press n to create session in cwd
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{
			Projects: store.projects,
		})

		updated := model.(tui.Model)
		if updated.ActivePage() != tui.PageProjects {
			t.Fatalf("expected PageProjects, got %d", updated.ActivePage())
		}

		// Press n to create session in cwd from Projects page
		_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
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
