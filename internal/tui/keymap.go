package tui

// keymapEntry is one declarative binding in a per-page keymap descriptor: a key
// glyph, its action label, a Core flag marking footer membership (the §3.4
// condensed core keys) vs help-only keys (surfaced only in the ? help modal,
// §8.5), and a RightAligned flag for the trailing entry the footer pins to the
// right (the ? help hint).
//
// The descriptor is the single source of truth for the footer + help DISPLAY —
// it drives BOTH the condensed footer (task 2-4) and the per-page ? help modal
// (Phase 3, §8.5/§14.4): the help modal lists every entry, the footer renders
// only the Core entries. It is pure data — no rendering lives here; consumers
// map the glyphs to display forms and lay them out.
//
// SCOPE (important): the descriptor governs the two DISPLAY surfaces ONLY. It
// does NOT govern key DISPATCH — the live per-page Update switch
// (updateSessionList / updateProjectsPage / handlePreviewKey) is a separate
// hand-coded match over tea key codes with no compiler-enforced link back here.
// So a binding change must be made in BOTH places; the descriptor↔dispatch
// correspondence is held instead by the guard tests in
// keymap_dispatch_guard_test.go, which fail if the two silently diverge.
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
	// the single-source-of-truth-for-DISPLAY contract holds: a binding change updates
	// the footer and the help together. (Dispatch is out of scope — see the type doc.)
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
	// single-source-of-truth-for-DISPLAY contract (§8.5): a binding change updates
	// the footer and the help together, neither hand-authored as a second list.
	// (Dispatch is out of scope — see the type doc.)
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

// commandPendingKeymap returns the §11.4 command-pending footer descriptor:
// `enter run here · n run in cwd · esc cancel`. It is the single binding source for
// the swapped Projects footer (renderCommandPendingFooter renders these in MV
// chrome). Authoring it as a keymapEntry slice folds the command-pending footer into
// the SAME descriptor/entry vocabulary as every other footer — the `enter` glyph is
// encoded declaratively via HelpKey ("⏎", resolved by helpKeyGlyph), retiring the
// former inline enter→⏎ string rewrite that was the one footer source outside the
// vocabulary. The footer Key stays the terse "enter" word (the descriptor convention,
// mirroring sessionsKeymap's enter binding). q quit is deferred to the ? help modal;
// s/x/e/d are suppressed in command-pending, so the descriptor lists only these three
// (it is a footer-copy source, not a help reference, so no Core/RightAligned flags).
func commandPendingKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "enter", HelpKey: "⏎", Action: "run here"},
		{Key: "n", Action: "run in cwd"},
		{Key: "esc", Action: "cancel"},
	}
}

// previewKeymap returns the ordered §9.3 Preview keymap descriptor — the single
// source of truth that drives the §9.1 full-screen overlay footer (the four nav
// hints) and the per-page ? help reference (§8.5). The descriptor lists the
// COMPLETE Preview keymap (§12.1): the help shows EVERY entry, the footer filters
// to the Core entries. The bindings follow the §9 spatial model (post the §9
// chrome restructure, task 4-6/4-7): window is `←`/`→`, pane is `Tab` (REPLACING
// the former `]`/`[` window + `Ctrl+←`/`Ctrl+→` pane — `Ctrl+←/→` is hijacked by
// macOS Mission Control Spaces switching, so pane reverts to the pre-rebuild `Tab`
// forward-cycle). The footer renders only the Core entries (the four nav hints,
// space-separated); the scroll/page/top/bottom keys are help-only (the footer has
// no room and scrolling is the obvious arrow default).
//
// The top/bottom jumps (`Home`/`End`) are preview-owned (Update's handlePreviewKey
// binds them) so they appear in the help reference even though the §12.2 nav
// revision dropped Home/End from the LIST pages — the preview is a scrollback
// viewport, not a list, and keeps the explicit top/bottom jumps.
//
// Glyphs follow the project key-glyph convention: `←→` (left+right arrows) for the
// window pair, `⇥` (U+21E5 rightwards-arrow-to-bar / Tab) for the pane forward
// cycle, `⏎` (U+23CE) for attach, `␣` (U+2423) for the space-back, and the literal
// `Home`/`End` words for the top/bottom jumps. The footer reads Key directly; no
// HelpKey override is needed since the Core Key forms are already glyphs.
func previewKeymap() []keymapEntry {
	return []keymapEntry{
		{Key: "↑/↓", Action: "scroll", HelpAction: "Scroll up / down"},
		{Key: "^↑/↓", Action: "page", HelpAction: "Page up / down"},
		{Key: "Home/End", Action: "top/bottom", HelpAction: "Jump to top / bottom"},
		{Key: "←→", Action: "window", HelpAction: "Prev / next window", Core: true},
		{Key: "⇥", Action: "pane", HelpAction: "Next pane", Core: true},
		{Key: "⏎", Action: "attach", HelpAction: "Attach this pane", Core: true},
		{Key: "␣", Action: "back", HelpAction: "Back to sessions", Core: true},
	}
}
