package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
	"github.com/leeovery/portal/internal/ui"
)

// mockDirLister implements ui.DirLister for testing.
type mockDirLister struct {
	entries map[string][]browser.DirEntry
}

func (m *mockDirLister) ListDirectories(path string, showHidden bool) ([]browser.DirEntry, error) {
	if entries, ok := m.entries[path]; ok {
		return entries, nil
	}
	return []browser.DirEntry{}, nil
}

// newTestBrowser creates a FileBrowserModel with the given start path and mock entries.
func newTestBrowser(startPath string, entries map[string][]browser.DirEntry) ui.FileBrowserModel {
	return ui.NewFileBrowser(startPath, &mockDirLister{entries: entries})
}

// standardEntries returns a mock directory structure for testing.
// /home/user/code contains: alpha, beta, gamma
// /home/user/code/alpha contains: sub1
// /home/user contains: code, docs
// /home contains: user
// / contains: home, tmp
func standardEntries() map[string][]browser.DirEntry {
	return map[string][]browser.DirEntry{
		"/home/user/code": {
			{Name: "alpha"},
			{Name: "beta"},
			{Name: "gamma"},
		},
		"/home/user/code/alpha": {
			{Name: "sub1"},
		},
		"/home/user": {
			{Name: "code"},
			{Name: "docs"},
		},
		"/home": {
			{Name: "user"},
		},
		"/": {
			{Name: "home"},
			{Name: "tmp"},
		},
	}
}

func keyUp() tea.Msg    { return tea.KeyMsg{Type: tea.KeyUp} }
func keyRight() tea.Msg { return tea.KeyMsg{Type: tea.KeyRight} }
func keyLeft() tea.Msg  { return tea.KeyMsg{Type: tea.KeyLeft} }

func sendBrowserKeys(m tea.Model, keys ...tea.Msg) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func TestFileBrowser_StartsAtCWD(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())
	view := m.View()

	if !strings.Contains(view, "/home/user/code") {
		t.Errorf("view should display starting path /home/user/code:\n%s", view)
	}
}

func TestFileBrowser_DisplaysCurrentPath(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())
	view := m.View()

	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("view is empty")
	}
	if !strings.Contains(lines[0], "/home/user/code") {
		t.Errorf("first line should contain path header, got: %q", lines[0])
	}
}

func TestFileBrowser_ShowsDotEntryAtTop(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())
	view := m.View()

	lines := strings.Split(view, "\n")
	// Find the first line with "." that is a listing entry (has cursor indicator or spaces)
	foundDot := false
	foundDir := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "." || strings.HasSuffix(trimmed, " .") || strings.HasPrefix(trimmed, "> .") {
			foundDot = true
		}
		if strings.Contains(line, "alpha") {
			foundDir = true
		}
		// . must come before any directory entry
		if foundDir && !foundDot {
			t.Error(". entry should appear before directory entries")
			break
		}
	}
	if !foundDot {
		t.Errorf("view should show . entry:\n%s", view)
	}
}

func TestFileBrowser_EnterDescendsIntoDirectory(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Cursor starts at 0 (the . entry). Move down to first dir (alpha).
	var model tea.Model = m
	model = sendBrowserKeys(model, keyDown())

	// Press Enter to descend into alpha
	model, _ = model.Update(keyEnter())

	view := model.View()
	if !strings.Contains(view, "/home/user/code/alpha") {
		t.Errorf("should have descended into alpha, view:\n%s", view)
	}
	if !strings.Contains(view, "sub1") {
		t.Errorf("should show contents of alpha (sub1), view:\n%s", view)
	}
}

func TestFileBrowser_RightArrowDescendsIntoDirectory(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Move down to first dir (alpha), then press right arrow
	var model tea.Model = m
	model = sendBrowserKeys(model, keyDown(), keyRight())

	view := model.View()
	if !strings.Contains(view, "/home/user/code/alpha") {
		t.Errorf("right arrow should descend into alpha, view:\n%s", view)
	}
}

func TestFileBrowser_BackspaceGoesToParent(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	var model tea.Model = m
	model = sendBrowserKeys(model, keyBackspace())

	view := model.View()
	if !strings.Contains(view, "/home/user") {
		t.Errorf("backspace should go to parent, view:\n%s", view)
	}
	// Should not still show /home/user/code as path header
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[0], "/home/user/code") {
		t.Errorf("header should be parent path, not original, got: %q", lines[0])
	}
}

func TestFileBrowser_LeftArrowGoesToParent(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	var model tea.Model = m
	model = sendBrowserKeys(model, keyLeft())

	view := model.View()
	if !strings.Contains(view, "/home/user") {
		t.Errorf("left arrow should go to parent, view:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[0], "/home/user/code") {
		t.Errorf("header should be parent path, got: %q", lines[0])
	}
}

func TestFileBrowser_NoOpAtRootDirectory(t *testing.T) {
	m := newTestBrowser("/", standardEntries())

	var model tea.Model = m
	// Try backspace and left arrow at root
	model = sendBrowserKeys(model, keyBackspace(), keyLeft())

	view := model.View()
	lines := strings.Split(view, "\n")
	// Path should still be /
	if !strings.Contains(lines[0], "/") {
		t.Errorf("should still be at root, got: %q", lines[0])
	}
	// Should still show root contents
	if !strings.Contains(view, "home") {
		t.Errorf("should still show root contents, view:\n%s", view)
	}
}

func TestFileBrowser_CursorNavigationWorks(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Listing: . alpha beta gamma (4 items, indices 0-3)
	// Start at cursor 0 (.)
	var model tea.Model = m

	// Move down twice with arrow keys
	model = sendBrowserKeys(model, keyDown(), keyDown())
	// Should be on "beta" (index 2)
	// Press Enter: should not descend since beta has no entries in our mock
	// Instead verify position by pressing Enter and seeing we're in beta path
	model, _ = model.Update(keyEnter())
	view := model.View()
	if !strings.Contains(view, "/home/user/code/beta") {
		t.Errorf("cursor should have been on beta, view:\n%s", view)
	}

	// Reset and test j/k
	model = newTestBrowser("/home/user/code", standardEntries())
	model = sendBrowserKeys(model, keyRune('j'), keyRune('j'), keyRune('j'))
	// Should be on "gamma" (index 3)
	model, _ = model.Update(keyEnter())
	view = model.View()
	if !strings.Contains(view, "/home/user/code/gamma") {
		t.Errorf("j navigation should reach gamma, view:\n%s", view)
	}

	// Test k (up)
	model = newTestBrowser("/home/user/code", standardEntries())
	model = sendBrowserKeys(model, keyRune('j'), keyRune('j'), keyRune('k'))
	// Should be on "alpha" (index 1)
	model, _ = model.Update(keyEnter())
	view = model.View()
	if !strings.Contains(view, "/home/user/code/alpha") {
		t.Errorf("k navigation should go back to alpha, view:\n%s", view)
	}
}

func TestFileBrowser_CursorResetsOnDirectoryChange(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Move cursor down to beta (index 2)
	var model tea.Model = m
	model = sendBrowserKeys(model, keyDown(), keyDown())

	// Now go to parent to change directory
	model = sendBrowserKeys(model, keyBackspace())

	// Cursor should be reset to 0 in the new directory
	// Pressing Enter on cursor 0 should be on "." (current dir indicator)
	// Pressing down once then Enter should select first dir entry
	model = sendBrowserKeys(model, keyDown())
	model, _ = model.Update(keyEnter())
	view := model.View()
	// First dir in /home/user is "code"
	if !strings.Contains(view, "/home/user/code") {
		t.Errorf("cursor should have reset; first entry in /home/user is code, view:\n%s", view)
	}
}

func TestFileBrowser_DeeplyNestedPathShowsFullPath(t *testing.T) {
	entries := map[string][]browser.DirEntry{
		"/a/b/c/d/e/f": {
			{Name: "g"},
		},
	}
	m := newTestBrowser("/a/b/c/d/e/f", entries)
	view := m.View()

	if !strings.Contains(view, "/a/b/c/d/e/f") {
		t.Errorf("view should show full deeply nested path:\n%s", view)
	}
}

func TestFileBrowser_SingleSubdirectoryCursorStaysAtZero(t *testing.T) {
	entries := map[string][]browser.DirEntry{
		"/solo": {
			{Name: "only"},
		},
	}
	m := newTestBrowser("/solo", entries)

	// Items: . only (2 items)
	// Cursor starts at 0, try moving up - should stay at 0
	var model tea.Model = m
	model = sendBrowserKeys(model, keyUp(), keyUp())

	// Cursor should still be at 0 (on .)
	// Move down to "only" (index 1) and verify
	model = sendBrowserKeys(model, keyDown())
	model, _ = model.Update(keyEnter())
	view := model.View()
	if !strings.Contains(view, "/solo/only") {
		t.Errorf("should descend into only, view:\n%s", view)
	}
}
