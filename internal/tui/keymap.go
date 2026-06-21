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
	// "ctrl+↑/↓"). It is a display token, not a tea key code — the live dispatch
	// in updateSessionList owns the actual key matching. This is the TERSE footer
	// form: the condensed §3.4 footer ALWAYS reads Key (never HelpKey).
	Key string
	// HelpKey is the longer, glyph-rich key form the ? help modal (§8.5) renders
	// where it diverges from the footer Key form. Post the "all symbols, caret for
	// ctrl" decision the help body reads Key directly for nav ("↑/↓") and page
	// ("^↑/↓"); the HelpKey overrides are Sessions enter→"⏎" (footer keeps "enter")
	// and Sessions space→"␣" (footer keeps the word "space"). When empty the help
	// modal falls back to Key. The footer NEVER reads this field. It mirrors
	// HelpAction's footer-vs-help split, keeping both forms on the ONE descriptor so
	// the single-source-of-truth contract holds: a binding change updates the footer
	// and the help together.
	HelpKey string
	// Action is the action label shown beside the glyph (e.g. "attach",
	// "switch view"). It is the TERSE footer form — the condensed §3.4 footer is
	// space-constrained, so the labels are short.
	Action string
	// HelpAction is the longer, friendlier action label the ? help modal (§8.5)
	// shows — the help panel has room for full descriptions ("Open / attach
	// session" vs the footer's "attach"). When empty the help modal falls back to
	// Action, so an entry that needs no longer form sets it once. The footer NEVER
	// reads this field. Keeping both forms on the ONE descriptor preserves the
	// single-source-of-truth contract (§8.5): a binding change updates the footer
	// and the help together, neither hand-authored as a second list.
	HelpAction string
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
// Descriptor order follows the §8.5 help reference
// (testdata/vhs/reference/sessions-help-modal-mv.png), which lists the rows
// nav-first: ↑/↓ → ^↑/↓ (page) → ⏎ → / → ␣ → s → n → r → k → q → x, then
// a right-aligned ? help last. The help modal renders every entry in this order.
//
// The footer is UNAFFECTED by this order: it renders only the Core entries in
// descriptor order, and the Core relative order is preserved here as
// ↑/↓ · enter · / · space · s · x · ?, so the condensed footer left cluster
// (↑/↓ navigate · enter attach · / filter · space preview · s switch view ·
// x projects) plus the right-aligned ? help is byte-identical to before.
//
// Per the "all symbols, caret for ctrl" key-glyph decision the help body reads
// Key directly for nav ("↑/↓") and page ("^↑/↓"); the HelpKey overrides are
// enter→"⏎" and space→"␣" (the footer keeps "enter" / "space"). The footer
// always reads Key.
//
// Per §12.2 the descriptor carries no vim aliases (h/j/k-nav/l/g/G), no
// PgUp/PgDn/Home/End, and no uppercase bindings; nav is ↑/↓ and paging is
// ^↑/↓ only. ? help is modelled for the footer hint only — the descriptor
// does not bind it (Phase 3 binds the key).
func sessionsKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "↑/↓", Action: "navigate", HelpAction: "Move selection", Core: true},
		{Key: "^↑/↓", Action: "page", HelpAction: "Next / prev page"},
		{Key: "enter", HelpKey: "⏎", Action: "attach", HelpAction: "Open / attach session", Core: true},
		{Key: "/", Action: "filter", HelpAction: "Filter sessions", Core: true},
		{Key: "space", HelpKey: "␣", Action: "preview", HelpAction: "Preview scrollback", Core: true},
		{Key: "s", Action: "switch view", HelpAction: "Switch view — flat / project / tag", Core: true},
		{Key: "n", Action: "new in cwd", HelpAction: "New session in cwd"},
		{Key: "r", Action: "rename", HelpAction: "Rename session"},
		{Key: "k", Action: "kill", HelpAction: "Kill session"},
		{Key: "q", Action: "quit", HelpAction: "Quit"},
		{Key: "x", Action: "projects", HelpAction: "Switch to Projects", Core: true},
		{Key: "?", Action: "help", HelpAction: "Show this help", Core: true, RightAligned: true},
	}
}

// projectsKeymap returns the ordered Projects keymap descriptor (§12.1, post
// §12.2 revision). It enumerates every Projects binding exactly once and
// classifies each as a §6.3 core-footer key or a help-only key, so the SAME
// descriptor drives BOTH the condensed Projects footer (task 3-2) and the
// per-page ? help modal (Phase 3, §8.5) — neither hand-authored. It is the same
// shape as sessionsKeymap, so the shared condensed-footer renderer
// (renderCondensedFooter) drives both pages from one descriptor; this retired
// the legacy projectHelpKeys footer/help source.
//
// Descriptor order follows the SAME nav-first principle as sessionsKeymap (FIX 2
// internal consistency; there is no Projects help reference frame): the
// navigation/paging entries lead, then the page actions, then the right-aligned
// ? help last. The Core relative order the footer reads (⏎ · x · e · / · ?) is
// preserved, so the Projects footer is byte-identical to before. Per the "all
// symbols, caret for ctrl" key-glyph decision the help body reads Key directly
// for nav ("↑/↓") and page ("^↑/↓"); the ⏎ Key is already a glyph. Projects
// carries NO HelpKey override.
//
// Per §12.2 the descriptor carries NO Projects-side s→Sessions alias (x is the
// sole both-directions page toggle), no vim aliases, no PgUp/PgDn/Home/End, and
// no uppercase bindings; nav is ↑/↓ and paging is ^↑/↓ only. ? help is
// modelled for the footer hint only — the descriptor does not bind the key
// (Phase 3 binds it).
func projectsKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "↑/↓", Action: "navigate", HelpAction: "Move selection"},
		{Key: "^↑/↓", Action: "page", HelpAction: "Next / prev page"},
		{Key: "⏎", Action: "new session", HelpAction: "New session from project", Core: true},
		{Key: "x", Action: "sessions", HelpAction: "Switch to Sessions", Core: true},
		{Key: "e", Action: "edit", HelpAction: "Edit project", Core: true},
		{Key: "/", Action: "filter", HelpAction: "Filter projects", Core: true},
		{Key: "d", Action: "delete", HelpAction: "Delete project"},
		{Key: "n", Action: "new in cwd", HelpAction: "New session in cwd"},
		{Key: "q", Action: "quit", HelpAction: "Quit"},
		{Key: "esc", Action: "back", HelpAction: "Back / quit"},
		{Key: "?", Action: "help", HelpAction: "Show this help", Core: true, RightAligned: true},
	}
}
