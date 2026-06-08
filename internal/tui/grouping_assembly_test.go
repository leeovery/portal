package tui

import (
	"testing"

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
		rows := sessionRows(got)
		if len(rows) != len(want) {
			t.Fatalf("len(rows) = %d, want %d", len(rows), len(want))
		}
		for i, w := range want {
			si := rows[i]
			if si.GroupKey != w.key || si.Session.Name != w.name {
				t.Errorf("row[%d] = (%q, %q), want (%q, %q)", i, si.GroupKey, si.Session.Name, w.key, w.name)
			}
		}

		// assembleGroups now interleaves a header before each group: "a" (2 rows)
		// then "b" (1 row).
		headers := headerRows(got)
		wantHeaders := []struct {
			key   string
			count int
		}{
			{"a", 2},
			{"b", 1},
		}
		if len(headers) != len(wantHeaders) {
			t.Fatalf("len(headers) = %d, want %d", len(headers), len(wantHeaders))
		}
		for i, w := range wantHeaders {
			if headers[i].Key != w.key || headers[i].Count != w.count {
				t.Errorf("header[%d] = (key %q, count %d), want (%q, %d)", i, headers[i].Key, headers[i].Count, w.key, w.count)
			}
		}
	})

	t.Run("empty catch-all yields just the sorted resolved (empty-suppression preserved)", func(t *testing.T) {
		resolved := []SessionItem{
			{Session: tmux.Session{Name: "s1"}, GroupKey: "a", GroupHeading: "A"},
		}

		got := assembleGroups(resolved, nil, "Heading")

		rows := sessionRows(got)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.CatchAll {
			t.Errorf("unexpected catch-all item; heading should be suppressed when no catch-all items")
		}
		if si.GroupHeading == "Heading" {
			t.Errorf("unexpected catch-all heading %q; should be suppressed", "Heading")
		}
		// No catch-all header either — empty-suppression holds at the header layer.
		for _, h := range headerRows(got) {
			if h.Heading == "Heading" {
				t.Errorf("unexpected catch-all header %q; should be suppressed", "Heading")
			}
		}
	})

	t.Run("pins catch-all last after sorted resolved, stamping heading and ordering by name", func(t *testing.T) {
		resolved := []SessionItem{
			{Session: tmux.Session{Name: "zulu-1"}, GroupKey: "Zulu", GroupHeading: "Zulu"},
		}
		catchAll := []SessionItem{
			{Session: tmux.Session{Name: "charlie"}, GroupHeading: "Heading", CatchAll: true},
			{Session: tmux.Session{Name: "alpha"}, GroupHeading: "Heading", CatchAll: true},
		}

		got := assembleGroups(resolved, catchAll, "Heading")

		rows := sessionRows(got)
		if len(rows) != 3 {
			t.Fatalf("len(rows) = %d, want 3", len(rows))
		}
		first := rows[0]
		if first.CatchAll || first.GroupHeading != "Zulu" {
			t.Errorf("row[0] = %+v, want resolved Zulu item first", first)
		}
		second := rows[1]
		third := rows[2]
		if !second.CatchAll || second.Session.Name != "alpha" || second.GroupKey != "Heading" {
			t.Errorf("row[1] = %+v, want catch-all alpha stamped with Heading", second)
		}
		if !third.CatchAll || third.Session.Name != "charlie" || third.GroupKey != "Heading" {
			t.Errorf("row[2] = %+v, want catch-all charlie stamped with Heading", third)
		}

		// Two headers: the resolved Zulu group (1 row) then the catch-all
		// "Heading" group (2 rows) pinned last.
		headers := headerRows(got)
		if len(headers) != 2 {
			t.Fatalf("len(headers) = %d, want 2", len(headers))
		}
		if headers[0].Heading != "Zulu" || headers[0].Count != 1 {
			t.Errorf("header[0] = (%q, %d), want (Zulu, 1)", headers[0].Heading, headers[0].Count)
		}
		if headers[1].Heading != "Heading" || headers[1].Count != 2 {
			t.Errorf("header[1] = (%q, %d), want (Heading, 2) pinned last", headers[1].Heading, headers[1].Count)
		}
	})
}
