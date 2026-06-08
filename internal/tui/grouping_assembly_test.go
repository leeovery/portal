package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/leeovery/portal/internal/tmux"
)

func TestSessionItemsToList(t *testing.T) {
	t.Run("nil input yields a non-nil empty slice", func(t *testing.T) {
		got := sessionItemsToList(nil)
		if got == nil {
			t.Fatalf("got nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Fatalf("len(got) = %d, want 0", len(got))
		}
	})

	t.Run("boxes each SessionItem into list.Item preserving order and identity", func(t *testing.T) {
		in := []SessionItem{
			{Session: tmux.Session{Name: "a"}, GroupKey: "k1"},
			{Session: tmux.Session{Name: "b"}, GroupKey: "k2"},
			{Session: tmux.Session{Name: "c"}, GroupKey: "k3"},
		}

		got := sessionItemsToList(in)

		if len(got) != len(in) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(in))
		}
		for i := range in {
			si, ok := got[i].(SessionItem)
			if !ok {
				t.Fatalf("got[%d] is not a SessionItem", i)
			}
			if si != in[i] {
				t.Errorf("got[%d] = %+v, want %+v", i, si, in[i])
			}
		}
	})
}

func TestAssembleGroups(t *testing.T) {
	t.Run("empty resolved and empty catch-all yields empty slice", func(t *testing.T) {
		got := assembleGroups(nil, nil, "Heading")
		if len(got) != 0 {
			t.Fatalf("len(got) = %d, want 0", len(got))
		}
	})

	t.Run("sorts resolved by group key then session name", func(t *testing.T) {
		resolved := []SessionItem{
			{Session: tmux.Session{Name: "bravo-1"}, GroupKey: "b"},
			{Session: tmux.Session{Name: "alpha-z"}, GroupKey: "a"},
			{Session: tmux.Session{Name: "alpha-a"}, GroupKey: "a"},
		}

		got := assembleGroups(resolved, nil, "Heading")

		want := []struct {
			key  string
			name string
		}{
			{"a", "alpha-a"},
			{"a", "alpha-z"},
			{"b", "bravo-1"},
		}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			si := asSessionItem(t, got[i])
			if si.GroupKey != w.key || si.Session.Name != w.name {
				t.Errorf("got[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, w.key, w.name)
			}
		}
	})

	t.Run("empty catch-all yields just the sorted resolved (empty-suppression preserved)", func(t *testing.T) {
		resolved := []SessionItem{
			{Session: tmux.Session{Name: "s1"}, GroupKey: "a", GroupHeading: "A"},
		}

		got := assembleGroups(resolved, nil, "Heading")

		if len(got) != 1 {
			t.Fatalf("len(got) = %d, want 1", len(got))
		}
		si := asSessionItem(t, got[0])
		if si.CatchAll {
			t.Errorf("unexpected catch-all item; heading should be suppressed when no catch-all items")
		}
		if si.GroupHeading == "Heading" {
			t.Errorf("unexpected catch-all heading %q; should be suppressed", "Heading")
		}
	})

	t.Run("pins catch-all last after sorted resolved, stamping heading and ordering by name", func(t *testing.T) {
		resolved := []SessionItem{
			{Session: tmux.Session{Name: "zulu-1"}, GroupKey: "Zulu", GroupHeading: "Zulu"},
		}
		catchAll := []list.Item{
			SessionItem{Session: tmux.Session{Name: "charlie"}, GroupHeading: "Heading", CatchAll: true},
			SessionItem{Session: tmux.Session{Name: "alpha"}, GroupHeading: "Heading", CatchAll: true},
		}

		got := assembleGroups(resolved, catchAll, "Heading")

		if len(got) != 3 {
			t.Fatalf("len(got) = %d, want 3", len(got))
		}
		first := asSessionItem(t, got[0])
		if first.CatchAll || first.GroupHeading != "Zulu" {
			t.Errorf("got[0] = %+v, want resolved Zulu item first", first)
		}
		second := asSessionItem(t, got[1])
		third := asSessionItem(t, got[2])
		if !second.CatchAll || second.Session.Name != "alpha" || second.GroupKey != "Heading" {
			t.Errorf("got[1] = %+v, want catch-all alpha stamped with Heading", second)
		}
		if !third.CatchAll || third.Session.Name != "charlie" || third.GroupKey != "Heading" {
			t.Errorf("got[2] = %+v, want catch-all charlie stamped with Heading", third)
		}
	})
}
