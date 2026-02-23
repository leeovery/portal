package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/browser"
)

// DirLister abstracts directory listing for testability.
type DirLister interface {
	ListDirectories(path string, showHidden bool) ([]browser.DirEntry, error)
}

// FileBrowserModel is the Bubble Tea model for the file browser view.
type FileBrowserModel struct {
	path    string
	entries []browser.DirEntry
	cursor  int
	lister  DirLister
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

// totalItems returns the count of navigable items (dot entry + directory entries).
func (m FileBrowserModel) totalItems() int {
	return 1 + len(m.entries) // 1 for the "." entry
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
	switch {
	case msg.Type == tea.KeyDown || (msg.Type == tea.KeyRunes && string(msg.Runes) == "j"):
		if m.cursor < m.totalItems()-1 {
			m.cursor++
		}

	case msg.Type == tea.KeyUp || (msg.Type == tea.KeyRunes && string(msg.Runes) == "k"):
		if m.cursor > 0 {
			m.cursor--
		}

	case msg.Type == tea.KeyEnter || msg.Type == tea.KeyRight:
		return m.handleDescend()

	case msg.Type == tea.KeyBackspace || msg.Type == tea.KeyLeft:
		return m.handleAscend()
	}

	return m, nil
}

// handleDescend enters the directory at the current cursor position.
func (m FileBrowserModel) handleDescend() (tea.Model, tea.Cmd) {
	// Cursor 0 is the "." entry - no-op for navigation (selection handled elsewhere)
	if m.cursor == 0 {
		return m, nil
	}

	entryIdx := m.cursor - 1 // offset by 1 for the "." entry
	if entryIdx >= len(m.entries) {
		return m, nil
	}

	entry := m.entries[entryIdx]
	m.path = filepath.Join(m.path, entry.Name)
	m.cursor = 0
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

	// Directory entries
	for i, entry := range m.entries {
		cursor := "  "
		if i+1 == m.cursor { // +1 because index 0 is the "." entry
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, entry.Name)
	}

	return b.String()
}
