package tui

import "testing"

// TestProjectsKeymap locks the Projects keymap descriptor as the declarative
// source for the condensed Projects footer (this task, 3-2) ahead of the formal
// §12 Projects descriptor refactor (task 3-3). It enumerates the §6.3 condensed
// Projects footer copy EXACTLY — `⏎ new session` / `x sessions` / `e edit` /
// `/ filter` / `? help` — classifies them as Core footer keys, and marks `? help`
// as the sole right-aligned entry. Pure data, no rendering (mirrors
// sessionsKeymap / TestSessionsKeymap). The `⏎` return glyph matches the §6.3
// reference frame (testdata/vhs/reference/projects-mv.png).
func TestProjectsKeymap(t *testing.T) {
	entries := projectsKeymap()

	t.Run("it enumerates exactly the §6.3 condensed Projects footer copy in order", func(t *testing.T) {
		want := []keymapEntry{
			{Key: "⏎", Action: "new session", Core: true},
			{Key: "x", Action: "sessions", Core: true},
			{Key: "e", Action: "edit", Core: true},
			{Key: "/", Action: "filter", Core: true},
			{Key: "?", Action: "help", Core: true, RightAligned: true},
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

	t.Run("it marks every footer entry as Core", func(t *testing.T) {
		for _, e := range entries {
			if !e.Core {
				t.Errorf("Projects footer entry %q should be Core, got Core=false", e.Key)
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
}
