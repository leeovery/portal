package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
)

// BrowserCancelMsg is emitted when the user cancels the file browser with Esc (no filter active).
type BrowserCancelMsg struct{}

// DirLister abstracts directory listing for testability.
type DirLister interface {
	ListDirectories(path string, showHidden bool) ([]browser.DirEntry, error)
}

// FileBrowserModel is the Bubble Tea model for the file browser view.
type FileBrowserModel struct {
	path       string
	entries    []browser.DirEntry
	cursor     int
	lister     DirLister
	filterText string
}

// NewFileBrowser creates a FileBrowserModel starting at the given path.
func NewFileBrowser(startPath string, lister DirLister) FileBrowserModel {
	m := FileBrowserModel{
		path:   startPath,
		lister: lister,
	}
	m.loadEntries()
	return m
}

// loadEntries refreshes the directory listing for the current path.
func (m *FileBrowserModel) loadEntries() {
	entries, err := m.lister.ListDirectories(m.path, false)
	if err != nil {
		m.entries = []browser.DirEntry{}
		return
	}
	m.entries = entries
}

// filteredEntries returns the directory entries that match the current filter text.
func (m FileBrowserModel) filteredEntries() []browser.DirEntry {
	if m.filterText == "" {
		return m.entries
	}
	lowerFilter := strings.ToLower(m.filterText)
	var filtered []browser.DirEntry
	for _, e := range m.entries {
		if fuzzyMatch(strings.ToLower(e.Name), lowerFilter) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// totalItems returns the count of navigable items (dot entry + filtered directory entries).
func (m FileBrowserModel) totalItems() int {
	return 1 + len(m.filteredEntries()) // 1 for the "." entry
}

// Init satisfies the tea.Model interface.
func (m FileBrowserModel) Init() tea.Cmd {
	return nil
}

// Update handles messages and key input for the file browser.
func (m FileBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m FileBrowserModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyDown:
		if m.cursor < m.totalItems()-1 {
			m.cursor++
		}

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}

	case tea.KeyEnter, tea.KeyRight:
		return m.handleDescend()

	case tea.KeyLeft:
		return m.handleAscend()

	case tea.KeyBackspace:
		if m.filterText != "" {
			m.filterText = m.filterText[:len(m.filterText)-1]
			m.cursor = 0
		} else {
			return m.handleAscend()
		}

	case tea.KeyEsc:
		if m.filterText != "" {
			m.filterText = ""
			m.cursor = 0
		} else {
			return m, func() tea.Msg { return BrowserCancelMsg{} }
		}

	case tea.KeyRunes:
		m.filterText += string(msg.Runes)
		m.cursor = 0
	}

	return m, nil
}

// handleDescend enters the directory at the current cursor position.
func (m FileBrowserModel) handleDescend() (tea.Model, tea.Cmd) {
	// Cursor 0 is the "." entry - no-op for navigation (selection handled elsewhere)
	if m.cursor == 0 {
		return m, nil
	}

	filtered := m.filteredEntries()
	entryIdx := m.cursor - 1 // offset by 1 for the "." entry
	if entryIdx >= len(filtered) {
		return m, nil
	}

	entry := filtered[entryIdx]
	m.path = filepath.Join(m.path, entry.Name)
	m.cursor = 0
	m.filterText = ""
	m.loadEntries()

	return m, nil
}

// handleAscend moves to the parent directory. No-op at filesystem root.
func (m FileBrowserModel) handleAscend() (tea.Model, tea.Cmd) {
	parent := filepath.Dir(m.path)
	if parent == m.path {
		// Already at root
		return m, nil
	}

	m.path = parent
	m.cursor = 0
	m.filterText = ""
	m.loadEntries()

	return m, nil
}

// View renders the file browser.
func (m FileBrowserModel) View() string {
	var b strings.Builder

	// Header: current path
	fmt.Fprintf(&b, "%s\n\n", m.path)

	// "." entry (current directory indicator)
	dotCursor := "  "
	if m.cursor == 0 {
		dotCursor = "> "
	}
	fmt.Fprintf(&b, "%s.\n", dotCursor)

	// Directory entries (filtered)
	filtered := m.filteredEntries()
	for i, entry := range filtered {
		cursor := "  "
		if i+1 == m.cursor { // +1 because index 0 is the "." entry
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, entry.Name)
	}

	return b.String()
}
