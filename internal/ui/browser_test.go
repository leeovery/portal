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

	// Note: j/k are no longer navigation keys in the file browser.
	// With inline filtering, all rune keypresses become filter input.
	// Navigation is arrow keys only.
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

func TestFileBrowser_TypingFiltersDirectoryListing(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "al" to filter - should match "alpha" only
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('a'), keyRune('l'))

	view := model.View()
	if !strings.Contains(view, "alpha") {
		t.Errorf("filtered view should contain 'alpha':\n%s", view)
	}
	if strings.Contains(view, "beta") {
		t.Errorf("filtered view should not contain 'beta':\n%s", view)
	}
	if strings.Contains(view, "gamma") {
		t.Errorf("filtered view should not contain 'gamma':\n%s", view)
	}
}

func TestFileBrowser_BackspaceRemovesFilterChar(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "al" to filter to alpha only, then backspace to "a"
	// "a" should match "alpha" and "gamma" (fuzzy: 'a' appears in both)
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('a'), keyRune('l'), keyBackspace())

	view := model.View()
	if !strings.Contains(view, "alpha") {
		t.Errorf("view should contain 'alpha' after backspace:\n%s", view)
	}
	if !strings.Contains(view, "gamma") {
		t.Errorf("view should contain 'gamma' (fuzzy match 'a') after backspace:\n%s", view)
	}
}

func TestFileBrowser_BackspaceOnEmptyFilterGoesToParent(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// No filter active, backspace should go to parent
	var model tea.Model = m
	model = sendBrowserKeys(model, keyBackspace())

	view := model.View()
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[0], "/home/user/code") {
		t.Errorf("should have navigated to parent, got: %q", lines[0])
	}
	if !strings.Contains(view, "/home/user") {
		t.Errorf("should show parent directory /home/user:\n%s", view)
	}
}

func TestFileBrowser_EscClearsActiveFilter(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "al" to filter, then Esc to clear
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('a'), keyRune('l'), keyEsc())

	view := model.View()
	// All entries should be visible again
	if !strings.Contains(view, "alpha") {
		t.Errorf("all entries should be visible after Esc clears filter:\n%s", view)
	}
	if !strings.Contains(view, "beta") {
		t.Errorf("all entries should be visible after Esc clears filter:\n%s", view)
	}
	if !strings.Contains(view, "gamma") {
		t.Errorf("all entries should be visible after Esc clears filter:\n%s", view)
	}
	// Should still be in same directory
	if !strings.Contains(view, "/home/user/code") {
		t.Errorf("should still be in same directory:\n%s", view)
	}
}

func TestFileBrowser_EscCancelsBrowserWhenNoFilter(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Esc with no filter active should emit BrowserCancelMsg
	var model tea.Model = m
	_, cmd := model.Update(keyEsc())

	if cmd == nil {
		t.Fatal("expected command from Esc, got nil")
	}

	msg := cmd()
	if _, ok := msg.(ui.BrowserCancelMsg); !ok {
		t.Fatalf("expected BrowserCancelMsg, got %T", msg)
	}
}

func TestFileBrowser_FilterResetsOnDirectoryChange(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "al" to filter to alpha, then move down to alpha and enter it
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('a'), keyRune('l'))

	// Verify filter is active (only alpha visible)
	view := model.View()
	if strings.Contains(view, "beta") {
		t.Fatalf("filter should hide beta before descending:\n%s", view)
	}

	// Move down to alpha (cursor 1 in filtered list) and descend
	model = sendBrowserKeys(model, keyDown())
	model, _ = model.Update(keyEnter())

	// After descending, filter should be reset and all entries in alpha visible
	view = model.View()
	if !strings.Contains(view, "/home/user/code/alpha") {
		t.Errorf("should have descended into alpha:\n%s", view)
	}
	if !strings.Contains(view, "sub1") {
		t.Errorf("filter should have reset, showing sub1:\n%s", view)
	}
}

func TestFileBrowser_CursorResetsOnFilterChange(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Move cursor down to gamma (index 3)
	var model tea.Model = m
	model = sendBrowserKeys(model, keyDown(), keyDown(), keyDown())

	// Now type a filter character - cursor should reset to 0
	model = sendBrowserKeys(model, keyRune('a'))

	// Move down once - should be on first filtered entry
	model = sendBrowserKeys(model, keyDown())
	model, _ = model.Update(keyEnter())

	view := model.View()
	// "a" matches "alpha" and "gamma" (fuzzy). First match is "alpha".
	if !strings.Contains(view, "/home/user/code/alpha") {
		t.Errorf("cursor should have reset to 0 on filter change, first filtered entry is alpha:\n%s", view)
	}
}

func TestFileBrowser_FilterMatchesNothingShowsEmptyListing(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "zzz" which matches nothing
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('z'), keyRune('z'), keyRune('z'))

	view := model.View()
	if strings.Contains(view, "alpha") {
		t.Errorf("no entries should match 'zzz':\n%s", view)
	}
	if strings.Contains(view, "beta") {
		t.Errorf("no entries should match 'zzz':\n%s", view)
	}
	if strings.Contains(view, "gamma") {
		t.Errorf("no entries should match 'zzz':\n%s", view)
	}
	// The "." entry should still be visible (current directory indicator)
	if !strings.Contains(view, ".") {
		t.Errorf("dot entry should still be visible:\n%s", view)
	}
}

func TestFileBrowser_AllFilterCharsDeletedRestoresFullListing(t *testing.T) {
	m := newTestBrowser("/home/user/code", standardEntries())

	// Type "al" then backspace twice to empty the filter
	var model tea.Model = m
	model = sendBrowserKeys(model, keyRune('a'), keyRune('l'), keyBackspace(), keyBackspace())

	view := model.View()
	// Full listing should be restored
	if !strings.Contains(view, "alpha") {
		t.Errorf("full listing should be restored after clearing filter:\n%s", view)
	}
	if !strings.Contains(view, "beta") {
		t.Errorf("full listing should be restored after clearing filter:\n%s", view)
	}
	if !strings.Contains(view, "gamma") {
		t.Errorf("full listing should be restored after clearing filter:\n%s", view)
	}
	// Should still be in same directory (not navigated to parent)
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "/home/user/code") {
		t.Errorf("should still be in /home/user/code, not parent:\n%s", view)
	}
}
