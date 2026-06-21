package tui

// keymapEntry is one declarative binding in a per-page keymap descriptor: a key
// glyph, its action label, a Core flag marking footer membership (the §3.4
// condensed core keys) vs help-only keys (surfaced only in the ? help modal,
// §8.5), and a RightAligned flag for the trailing entry the footer pins to the
// right (the ? help hint).
//
// The descriptor is the single source of truth driving BOTH the condensed
// footer (task 2-4) and the per-page ? help modal (Phase 3, §8.5/§14.4): the
// help modal lists every entry, the footer renders only the Core entries. It is
// pure data — no rendering lives here; consumers map the glyphs to display
// forms and lay them out.
type keymapEntry struct {
	// Key is the key glyph as it appears to the user (e.g. "↑/↓", "enter",
	// "Ctrl+↑/↓"). It is a display token, not a tea key code — the live dispatch
	// in updateSessionList owns the actual key matching.
	Key string
	// Action is the action label shown beside the glyph (e.g. "attach",
	// "switch view").
	Action string
	// Core marks an entry as a §3.4 footer-core key. Core entries appear in the
	// condensed footer; non-Core entries are help-only (the ? help modal lists
	// both).
	Core bool
	// RightAligned marks the single trailing entry the footer pins to the right
	// (the "? help" hint). At most one entry sets this.
	RightAligned bool
}

// sessionsKeymap returns the ordered Sessions keymap descriptor (§12.1, post
// §12.2 revision). It enumerates every Sessions binding exactly once and
// classifies each as a §3.4 core-footer key or a help-only key.
//
// Core (footer) keys, in footer order: ↑/↓ navigate · enter attach · / filter ·
// space preview · s switch view · x projects, then a right-aligned ? help.
// Help-only keys (Core=false): n new in cwd · r rename · k kill · q quit ·
// Ctrl+↑/↓ page.
//
// Per §12.2 the descriptor carries no vim aliases (h/j/k-nav/l/g/G), no
// PgUp/PgDn/Home/End, and no uppercase bindings; nav is ↑/↓ and paging is
// Ctrl+↑/↓ only. ? help is modelled for the footer hint only — the descriptor
// does not bind it (Phase 3 binds the key).
func sessionsKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "↑/↓", Action: "navigate", Core: true},
		{Key: "enter", Action: "attach", Core: true},
		{Key: "/", Action: "filter", Core: true},
		{Key: "space", Action: "preview", Core: true},
		{Key: "s", Action: "switch view", Core: true},
		{Key: "x", Action: "projects", Core: true},
		{Key: "?", Action: "help", Core: true, RightAligned: true},
		{Key: "n", Action: "new in cwd"},
		{Key: "r", Action: "rename"},
		{Key: "k", Action: "kill"},
		{Key: "q", Action: "quit"},
		{Key: "Ctrl+↑/↓", Action: "page"},
	}
}

// projectsKeymap returns the ordered Projects keymap descriptor for the §6.3
// condensed Projects footer. It carries EXACTLY the §6.3 condensed copy —
// `enter new session` · `x sessions` · `e edit` · `/ filter`, then a
// right-aligned `? help` — every entry Core (the §6.3 condensed string is the
// footer's full left cluster). It is the same shape as sessionsKeymap so the
// shared condensed-footer renderer (renderCondensedFooter) drives both pages
// from one descriptor.
//
// This is a SCOPED descriptor for the footer only: the formal §12 Projects
// keymap descriptor (with the help-only keys n / d / q surfaced in the ? help
// modal) is task 3-3. 3-3 refactors these entries into that fuller descriptor;
// until then this carries just the footer-core copy the §6.3 reference shows.
func projectsKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "⏎", Action: "new session", Core: true},
		{Key: "x", Action: "sessions", Core: true},
		{Key: "e", Action: "edit", Core: true},
		{Key: "/", Action: "filter", Core: true},
		{Key: "?", Action: "help", Core: true, RightAligned: true},
	}
}
