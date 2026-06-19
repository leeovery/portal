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

	t.Run("it enumerates exactly the §12.1 Sessions bindings in order", func(t *testing.T) {
		want := []keymapEntry{
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
		wantHelpOnly := []string{"n", "r", "k", "q", "Ctrl+↑/↓"}
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
