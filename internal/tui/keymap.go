package tui

import "slices"

// Descriptors drive display only, not dispatch — a binding change must also be
// made in the per-page Update switch.
type keymapEntry struct {
	Key string
	// HelpKey is the help-modal key form; empty falls back to Key.
	HelpKey string
	Action  string
	// HelpAction is the help-modal label; empty falls back to Action.
	HelpAction string
	Core       bool
	// RightAligned pins the entry to the footer's right edge; at most one
	// entry sets it.
	RightAligned bool
	Destructive  bool
}

// Shared so the arrow glyphs and help labels cannot diverge between descriptors.
func navKeymapEntries() []keymapEntry {
	return []keymapEntry{
		{Key: "↑↓", HelpKey: "↑/↓", Action: "navigate", HelpAction: "Move selection"},
		{Key: "^↑/↓", Action: "page", HelpAction: "Next / prev page"},
	}
}

// Stays a pure static function: the `t`/`m` blocked-key filters are applied at
// the call site, so the descriptor stays a complete statement of the bindings.
func sessionsKeymap() []keymapEntry {
	return slices.Concat(navKeymapEntries(), []keymapEntry{
		{Key: "⏎", HelpKey: "⏎", Action: "attach", HelpAction: "Open / attach session", Core: true},
		{Key: "/", Action: "filter", HelpAction: "Filter sessions", Core: true},
		{Key: "␣", HelpKey: "␣", Action: "preview", HelpAction: "Preview scrollback", Core: true},
		{Key: "s", Action: "switch view", HelpAction: "Switch view — flat / project / tag", Core: true},
		{Key: "n", Action: "new in cwd", HelpAction: "New session in cwd"},
		{Key: "r", Action: "rename", HelpAction: "Rename session"},
		{Key: "k", Action: "kill", HelpAction: "Kill session", Destructive: true},
		{Key: "q", Action: "quit", HelpAction: "Quit"},
		{Key: "x", Action: "projects", HelpAction: "Switch to Projects", Core: true},
		{Key: "t", Action: "theme", HelpAction: "Theme picker", Core: true},
		{Key: "m", Action: "multi", HelpAction: "Multi-select mode", Core: true},
		{Key: "?", Action: "help", HelpAction: "Show this help", Core: true, RightAligned: true},
	})
}

// Stays a pure static function: the `t` blocked-key filter is applied at the call site.
func projectsKeymap() []keymapEntry {
	return slices.Concat(navKeymapEntries(), []keymapEntry{
		{Key: "⏎", Action: "new session", HelpAction: "New session from project", Core: true},
		{Key: "x", Action: "sessions", HelpAction: "Switch to Sessions", Core: true},
		{Key: "e", Action: "edit", HelpAction: "Edit project", Core: true},
		{Key: "/", Action: "filter", HelpAction: "Filter projects", Core: true},
		{Key: "t", Action: "theme", HelpAction: "Theme picker", Core: true},
		{Key: "d", Action: "delete", HelpAction: "Delete project", Destructive: true},
		{Key: "n", Action: "new in cwd", HelpAction: "New session in cwd"},
		{Key: "q", Action: "quit", HelpAction: "Quit"},
		{Key: "esc", Action: "back", HelpAction: "Back / quit"},
		{Key: "?", Action: "help", HelpAction: "Show this help", Core: true, RightAligned: true},
	})
}

// Complete scope — every key the panel dispatches, arrows and paging included.
// Do not trim it to the Core keys the footer shows.
func themePanelKeymap() []keymapEntry {
	return slices.Concat(navKeymapEntries(), []keymapEntry{
		{Key: "⏎", Action: "set theme", HelpAction: "Set as the theme", Core: true},
		{Key: "d", Action: "set as dark", HelpAction: "Assign to the dark slot", Core: true},
		{Key: "l", Action: "set as light", HelpAction: "Assign to the light slot", Core: true},
		{Key: "esc", Action: "close", HelpAction: "Close the theme picker", Core: true},
	})
}

// helpScrollKey is the help modal's scroll binding, which its title names
// only while the body is taller than the room.
const helpScrollKey = "↑↓"

// The help modal's own keys, which its title names. `?` closes it too, as the
// key that opened it.
func helpModalKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: helpScrollKey, Action: "scroll", HelpAction: "Scroll the keys"},
		{Key: "esc", Action: "close", HelpAction: "Close this help"},
	}
}

// Key-exclusive within the panel while the confirm is live, so its footer
// replaces the standing keys. Uppercase Y/N dispatches but is not restated.
func themePanelConfirmKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "y", Action: "confirm", HelpAction: "Clear the constant and set the slot", Core: true},
		{Key: "n", Action: "cancel", HelpAction: "Keep the constant", Core: true},
	}
}

// A footer-copy source, not a help reference — no Core/RightAligned flags.
func commandPendingKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "enter", HelpKey: "⏎", Action: "run here"},
		{Key: "n", Action: "run in cwd"},
		{Key: "esc", Action: "cancel"},
	}
}

// Line scroll is j/k, not ⌘↑/↓: the terminal keeps ⌘↑/↓ for itself, so it
// would never reach Portal.
func previewKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "j/k", Action: "scroll", HelpAction: "Scroll down / up"},
		{Key: "^↑/↓", Action: "page", HelpAction: "Page up / down"},
		{Key: "Home/End", Action: "top/bottom", HelpAction: "Jump to top / bottom"},
		{Key: "↑↓", HelpKey: "↑/↓", Action: "session", HelpAction: "Prev / next session", Core: true},
		{Key: "⏎", Action: "attach", HelpAction: "Attach this session", Core: true},
		{Key: "␣", Action: "back", HelpAction: "Back to sessions", Core: true},
	}
}
