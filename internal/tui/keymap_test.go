package tui

import "testing"

// TestSessionsKeymap locks the Sessions keymap descriptor (§12.1 / §3.4) as the
// single declarative source for the footer (task 2-4) and the ? help modal
// (Phase 3, §8.5). The descriptor enumerates exactly the §12.1 Sessions
// bindings, classifies the §3.4 core-footer keys (Core=true) against the
// help-only remainder (Core=false), and marks ? help as the sole right-aligned
// entry. No rendering happens here — the descriptor is pure data.
func TestSessionsKeymap(t *testing.T) {
	entries := sessionsKeymap()

	t.Run("it enumerates exactly the §12.1 Sessions bindings in the reference help order", func(t *testing.T) {
		// Reference help order (testdata/vhs/reference/sessions-help-modal-mv.png):
		// ↑/↓ → ^↑/↓ (page) → ⏎ → / → ␣ → s → n → r → k → q → x, then ? last.
		// Per the "all symbols, caret for ctrl" decision the help body reads Key
		// directly for nav and page; the HelpKey overrides are enter→"⏎" and
		// space→"␣". The footer always reads Key, so its Core relative order
		// (↑/↓ · enter · / · space · s · x · ?) is preserved.
		want := []keymapEntry{
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
		if len(entries) != len(want) {
			t.Fatalf("descriptor has %d entries, want %d: %+v", len(entries), len(want), entries)
		}
		for i, w := range want {
			if entries[i] != w {
				t.Errorf("entry[%d] = %+v, want %+v", i, entries[i], w)
			}
		}
	})

	t.Run("it marks the §3.4 core-footer keys as Core and the rest as help-only", func(t *testing.T) {
		core := map[string]bool{}
		for _, e := range entries {
			core[e.Key] = e.Core
		}
		wantCore := []string{"↑/↓", "enter", "/", "space", "s", "x", "?"}
		for _, k := range wantCore {
			if !core[k] {
				t.Errorf("key %q should be Core (footer), got Core=false", k)
			}
		}
		wantHelpOnly := []string{"n", "r", "k", "q", "^↑/↓"}
		for _, k := range wantHelpOnly {
			if core[k] {
				t.Errorf("key %q should be help-only (Core=false), got Core=true", k)
			}
		}
	})

	t.Run("it marks only ? help as right-aligned", func(t *testing.T) {
		for _, e := range entries {
			wantRight := e.Key == "?"
			if e.RightAligned != wantRight {
				t.Errorf("entry %q RightAligned = %v, want %v", e.Key, e.RightAligned, wantRight)
			}
		}
	})

	t.Run("it preserves the Core relative order the footer reads (footer unchanged)", func(t *testing.T) {
		// FIX 1 invariant: the help reorder MUST NOT change the footer. The footer
		// renders only Core entries in descriptor order, so the relative order of the
		// Core keys must stay exactly ↑/↓ · enter · / · space · s · x · ?.
		var coreKeys []string
		for _, e := range entries {
			if e.Core {
				coreKeys = append(coreKeys, e.Key)
			}
		}
		wantCoreOrder := []string{"↑/↓", "enter", "/", "space", "s", "x", "?"}
		if len(coreKeys) != len(wantCoreOrder) {
			t.Fatalf("Core entries = %v, want %v", coreKeys, wantCoreOrder)
		}
		for i, k := range wantCoreOrder {
			if coreKeys[i] != k {
				t.Errorf("Core entry %d = %q, want %q (footer order must not change)", i, coreKeys[i], k)
			}
		}
	})

	t.Run("the HelpKey overrides are enter→⏎ and space→␣; every other entry falls back to Key", func(t *testing.T) {
		// Post the "all symbols, caret for ctrl" decision the help body reads Key
		// directly for nav ("↑/↓") and page ("^↑/↓"). The HelpKey overrides are the
		// Sessions enter→"⏎" (footer keeps Key "enter") and space→"␣" (footer keeps
		// Key "space"). Every other entry must have an empty HelpKey so the help
		// modal falls back to its Key form.
		wantOverride := map[string]string{"enter": "⏎", "space": "␣"}
		for _, e := range entries {
			if want, ok := wantOverride[e.Key]; ok {
				if e.HelpKey != want {
					t.Errorf("%s HelpKey = %q, want %q", e.Key, e.HelpKey, want)
				}
				continue
			}
			if e.HelpKey != "" {
				t.Errorf("key %q must have NO HelpKey override (got %q); only enter and space override", e.Key, e.HelpKey)
			}
		}
	})

	t.Run("it has no uppercase or vim-alias key in the descriptor", func(t *testing.T) {
		banned := map[string]bool{
			"h": true, "j": true, "l": true, "g": true, "G": true,
			"K": true, "N": true, "R": true, "Q": true, "S": true, "X": true,
			"pgup": true, "pgdown": true, "home": true, "end": true,
		}
		for _, e := range entries {
			if banned[e.Key] {
				t.Errorf("descriptor contains banned key %q (§12.2: no vim/uppercase/page-jump aliases)", e.Key)
			}
		}
	})
}
