package tui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

func TestSessionItem(t *testing.T) {
	t.Run("FilterValue returns session name", func(t *testing.T) {
		item := tui.SessionItem{Session: tmux.Session{Name: "dev", Windows: 3, Attached: true}}

		got := item.FilterValue()

		if got != "dev" {
			t.Errorf("FilterValue() = %q, want %q", got, "dev")
		}
	})

	t.Run("implements list.Item interface", func(t *testing.T) {
		var _ list.Item = tui.SessionItem{}
	})

	t.Run("Title returns session name", func(t *testing.T) {
		item := tui.SessionItem{Session: tmux.Session{Name: "myproject", Windows: 2, Attached: false}}

		got := item.Title()

		if got != "myproject" {
			t.Errorf("Title() = %q, want %q", got, "myproject")
		}
	})

	t.Run("Description returns window count and attached badge", func(t *testing.T) {
		tests := []struct {
			name    string
			session tmux.Session
			want    string
		}{
			{
				name:    "plural windows without attached",
				session: tmux.Session{Name: "dev", Windows: 3, Attached: false},
				want:    "3 windows",
			},
			{
				name:    "singular window",
				session: tmux.Session{Name: "dev", Windows: 1, Attached: false},
				want:    "1 window",
			},
			{
				name:    "plural windows with attached",
				session: tmux.Session{Name: "dev", Windows: 5, Attached: true},
				want:    "5 windows  ● attached",
			},
			{
				name:    "singular window with attached",
				session: tmux.Session{Name: "dev", Windows: 1, Attached: true},
				want:    "1 window  ● attached",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				item := tui.SessionItem{Session: tt.session}

				got := item.Description()

				if got != tt.want {
					t.Errorf("Description() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}

func TestSessionItemGroupMetadata(t *testing.T) {
	t.Run("leaves Flat items with empty group metadata", func(t *testing.T) {
		item := tui.SessionItem{Session: tmux.Session{Name: "dev", Windows: 3, Attached: true}}

		if item.GroupKey != "" {
			t.Errorf("GroupKey = %q, want empty", item.GroupKey)
		}
		if item.GroupHeading != "" {
			t.Errorf("GroupHeading = %q, want empty", item.GroupHeading)
		}
		if item.CatchAll {
			t.Errorf("CatchAll = %v, want false", item.CatchAll)
		}
	})

	t.Run("returns the session name from FilterValue regardless of group fields", func(t *testing.T) {
		item := tui.SessionItem{
			Session:      tmux.Session{Name: "dev", Windows: 3, Attached: true},
			GroupKey:     "work",
			GroupHeading: "work",
			CatchAll:     false,
		}

		if got := item.FilterValue(); got != "dev" {
			t.Errorf("FilterValue() = %q, want %q", got, "dev")
		}
	})

	t.Run("returns the session name from Title regardless of group fields", func(t *testing.T) {
		item := tui.SessionItem{
			Session:      tmux.Session{Name: "dev", Windows: 3, Attached: true},
			GroupKey:     "/home/me/project",
			GroupHeading: "project",
			CatchAll:     true,
		}

		if got := item.Title(); got != "dev" {
			t.Errorf("Title() = %q, want %q", got, "dev")
		}
	})

	t.Run("builds two instances of one session sharing the same underlying Session.Name", func(t *testing.T) {
		session := tmux.Session{Name: "dev", Windows: 3, Attached: true}
		work := tui.SessionItem{Session: session, GroupKey: "work", GroupHeading: "work"}
		personal := tui.SessionItem{Session: session, GroupKey: "personal", GroupHeading: "personal"}

		if work.GroupKey == personal.GroupKey {
			t.Fatalf("expected distinct group keys, got %q and %q", work.GroupKey, personal.GroupKey)
		}
		if work.Session.Name != personal.Session.Name {
			t.Errorf("instances do not share Session.Name: %q vs %q", work.Session.Name, personal.Session.Name)
		}
		if work.Session.Name != "dev" {
			t.Errorf("Session.Name = %q, want %q", work.Session.Name, "dev")
		}
	})
}

func TestSessionDelegate(t *testing.T) {
	t.Run("implements list.ItemDelegate interface", func(t *testing.T) {
		var _ list.ItemDelegate = tui.SessionDelegate{}
	})

	t.Run("Height returns 1", func(t *testing.T) {
		d := tui.SessionDelegate{}

		if got := d.Height(); got != 1 {
			t.Errorf("Height() = %d, want 1", got)
		}
	})

	t.Run("Spacing returns 0", func(t *testing.T) {
		d := tui.SessionDelegate{}

		if got := d.Spacing(); got != 0 {
			t.Errorf("Spacing() = %d, want 0", got)
		}
	})

	t.Run("Update returns nil", func(t *testing.T) {
		d := tui.SessionDelegate{}

		cmd := d.Update(nil, nil)

		if cmd != nil {
			t.Error("Update() should return nil")
		}
	})

	t.Run("renders session name and window count", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "dev", Windows: 3, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "dev") {
			t.Errorf("render output missing session name 'dev': %q", output)
		}
		if !strings.Contains(output, "3 windows") {
			t.Errorf("render output missing '3 windows': %q", output)
		}
	})

	t.Run("renders singular window for count of 1", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "single", Windows: 1, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "1 window") {
			t.Errorf("render output missing '1 window': %q", output)
		}
		if strings.Contains(output, "1 windows") {
			t.Errorf("render output should not contain '1 windows': %q", output)
		}
	})

	t.Run("renders plural windows for count > 1", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "multi", Windows: 5, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "5 windows") {
			t.Errorf("render output missing '5 windows': %q", output)
		}
	})

	t.Run("renders attached badge for attached session", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "attached-session", Windows: 2, Attached: true}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "● attached") {
			t.Errorf("render output missing '● attached': %q", output)
		}
	})

	t.Run("does not render attached badge for detached session", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "detached-session", Windows: 2, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if strings.Contains(output, "attached") {
			t.Errorf("render output should not contain 'attached' for detached session: %q", output)
		}
	})

	t.Run("highlights selected item", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "first", Windows: 1, Attached: false}},
			tui.SessionItem{Session: tmux.Session{Name: "second", Windows: 2, Attached: false}},
		}
		m := list.New(items, d, 80, 10)
		// m.Index() defaults to 0, so index 0 is selected

		var selectedBuf bytes.Buffer
		d.Render(&selectedBuf, m, 0, items[0])
		selectedOutput := selectedBuf.String()

		var unselectedBuf bytes.Buffer
		d.Render(&unselectedBuf, m, 1, items[1])
		unselectedOutput := unselectedBuf.String()

		// Selected item should have cursor indicator ">"
		if !strings.Contains(selectedOutput, ">") {
			t.Errorf("selected item should contain cursor indicator '>': %q", selectedOutput)
		}
		// Unselected item should not have cursor indicator
		if strings.Contains(unselectedOutput, ">") {
			t.Errorf("unselected item should not contain cursor indicator '>': %q", unselectedOutput)
		}
	})

	t.Run("long session name renders without truncation", func(t *testing.T) {
		longName := "my-very-long-project-name-that-should-not-be-truncated-x7k2m9"
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: longName, Windows: 3, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, longName) {
			t.Errorf("render output should contain full long name %q: %q", longName, output)
		}
	})
}

// groupSeparator is the heading separator glyph (U+00B7 MIDDLE DOT ×3) used by
// the delegate in the "Heading ··· N" form. Duplicated here so the test pins the
// exact rendered glyph independently of the implementation constant.
const groupSeparator = "···"

func TestSessionDelegateGroupHeadings(t *testing.T) {
	t.Run("injects a dimmed heading before the first item of each group", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "a", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "b", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "c", Windows: 1}, GroupKey: "/p/work", GroupHeading: "Work"},
		}
		m := list.New(items, d, 80, 10)

		// The second group's first item (index 2) starts a new group → heading.
		var buf bytes.Buffer
		d.Render(&buf, m, 2, items[2])
		out := buf.String()
		if !strings.Contains(out, "Work") {
			t.Errorf("expected heading 'Work' before group boundary item: %q", out)
		}
		if !strings.Contains(out, groupSeparator) {
			t.Errorf("expected separator glyph in heading: %q", out)
		}

		// An interior item of the first group (index 1) does NOT start a new group.
		var interior bytes.Buffer
		d.Render(&interior, m, 1, items[1])
		if strings.Contains(interior.String(), groupSeparator) {
			t.Errorf("interior item should emit no heading: %q", interior.String())
		}
	})

	t.Run("injects a leading heading before the very first item", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "a", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "b", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])
		out := buf.String()
		if !strings.Contains(out, "Portal") {
			t.Errorf("expected leading heading 'Portal' before first item: %q", out)
		}
		if !strings.Contains(out, groupSeparator) {
			t.Errorf("expected separator glyph in leading heading: %q", out)
		}
	})

	t.Run("renders a per-group count of the rows beneath the heading", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "a", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "b", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "c", Windows: 1}, GroupKey: "/p/work", GroupHeading: "Work"},
			tui.SessionItem{Session: tmux.Session{Name: "d", Windows: 1}, GroupKey: "/p/work", GroupHeading: "Work"},
			tui.SessionItem{Session: tmux.Session{Name: "e", Windows: 1}, GroupKey: "/p/work", GroupHeading: "Work"},
		}
		m := list.New(items, d, 80, 10)

		var portal bytes.Buffer
		d.Render(&portal, m, 0, items[0])
		if !strings.Contains(portal.String(), "Portal "+groupSeparator+" 2") {
			t.Errorf("expected 'Portal %s 2', got: %q", groupSeparator, portal.String())
		}

		var work bytes.Buffer
		d.Render(&work, m, 2, items[2])
		if !strings.Contains(work.String(), "Work "+groupSeparator+" 3") {
			t.Errorf("expected 'Work %s 3', got: %q", groupSeparator, work.String())
		}
	})

	t.Run("counts a multi-tag session under each of its tag headings", func(t *testing.T) {
		// One live session 'dev' materialised under two tags (Pattern B). The
		// By-Tag header counts sum to 2 while there is only 1 live session.
		dev := tmux.Session{Name: "dev", Windows: 1}
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: dev, GroupKey: "personal", GroupHeading: "personal"},
			tui.SessionItem{Session: dev, GroupKey: "work", GroupHeading: "work"},
		}
		m := list.New(items, d, 80, 10)

		var personal bytes.Buffer
		d.Render(&personal, m, 0, items[0])
		if !strings.Contains(personal.String(), "personal "+groupSeparator+" 1") {
			t.Errorf("expected 'personal %s 1', got: %q", groupSeparator, personal.String())
		}

		var work bytes.Buffer
		d.Render(&work, m, 1, items[1])
		if !strings.Contains(work.String(), "work "+groupSeparator+" 1") {
			t.Errorf("expected 'work %s 1', got: %q", groupSeparator, work.String())
		}
	})

	t.Run("injects no heading for flat items (byte-identical to today)", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "first", Windows: 2, Attached: true}},
			tui.SessionItem{Session: tmux.Session{Name: "second", Windows: 1, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		// Expected output is the legacy session line: cursor + name + "  " + detail.
		for index := range items {
			var buf bytes.Buffer
			d.Render(&buf, m, index, items[index])
			out := buf.String()
			if strings.Contains(out, groupSeparator) {
				t.Errorf("flat item %d emitted a heading: %q", index, out)
			}
			if strings.Contains(out, "\n") {
				t.Errorf("flat item %d emitted a multi-line render: %q", index, out)
			}
		}
	})

	t.Run("carries the correct count for the last group", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "a", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "b", Windows: 1}, GroupKey: "Untagged", GroupHeading: "Untagged", CatchAll: true},
			tui.SessionItem{Session: tmux.Session{Name: "c", Windows: 1}, GroupKey: "Untagged", GroupHeading: "Untagged", CatchAll: true},
			tui.SessionItem{Session: tmux.Session{Name: "d", Windows: 1}, GroupKey: "Untagged", GroupHeading: "Untagged", CatchAll: true},
		}
		m := list.New(items, d, 80, 10)

		var last bytes.Buffer
		d.Render(&last, m, 1, items[1])
		if !strings.Contains(last.String(), "Untagged "+groupSeparator+" 3") {
			t.Errorf("expected last group 'Untagged %s 3', got: %q", groupSeparator, last.String())
		}
	})
}

func TestSessionDelegateFlattenOnFilter(t *testing.T) {
	projectItems := func() []list.Item {
		return []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "a", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "b", Windows: 1}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tmux.Session{Name: "c", Windows: 1}, GroupKey: "/p/work", GroupHeading: "Work"},
		}
	}

	tagItems := func() []list.Item {
		return []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "dev", Windows: 1}, GroupKey: "personal", GroupHeading: "personal"},
			tui.SessionItem{Session: tmux.Session{Name: "dev", Windows: 1}, GroupKey: "work", GroupHeading: "work"},
		}
	}

	renderAll := func(d tui.SessionDelegate, m list.Model, items []list.Item) string {
		var buf bytes.Buffer
		for index := range items {
			d.Render(&buf, m, index, items[index])
		}
		return buf.String()
	}

	t.Run("suppresses group headers while a filter is active in By Project mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := projectItems()
		m := list.New(items, d, 80, 10)
		m.SetFilterState(list.FilterApplied)

		out := renderAll(d, m, items)
		if strings.Contains(out, groupSeparator) {
			t.Errorf("expected no group headings while filtering By Project, got: %q", out)
		}
	})

	t.Run("suppresses group headers while a filter is active in By Tag mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := tagItems()
		m := list.New(items, d, 80, 10)
		m.SetFilterState(list.FilterApplied)

		out := renderAll(d, m, items)
		if strings.Contains(out, groupSeparator) {
			t.Errorf("expected no group headings while filtering By Tag, got: %q", out)
		}
	})

	t.Run("restores group headers when the filter is cleared", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := projectItems()
		m := list.New(items, d, 80, 10)

		m.SetFilterState(list.FilterApplied)
		filtered := renderAll(d, m, items)
		if strings.Contains(filtered, groupSeparator) {
			t.Fatalf("precondition failed: headings present while filtering: %q", filtered)
		}

		m.SetFilterState(list.Unfiltered)
		restored := renderAll(d, m, items)
		if !strings.Contains(restored, groupSeparator) {
			t.Errorf("expected group headings to return on Unfiltered, got: %q", restored)
		}
		if !strings.Contains(restored, "Portal") || !strings.Contains(restored, "Work") {
			t.Errorf("expected By Project headings to return on clear, got: %q", restored)
		}
	})

	t.Run("leaves Flat-mode filtering unchanged", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tmux.Session{Name: "first", Windows: 2, Attached: true}},
			tui.SessionItem{Session: tmux.Session{Name: "second", Windows: 1, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var unfiltered bytes.Buffer
		for index := range items {
			d.Render(&unfiltered, m, index, items[index])
		}

		m.SetFilterState(list.FilterApplied)
		var filtered bytes.Buffer
		for index := range items {
			d.Render(&filtered, m, index, items[index])
		}

		if unfiltered.String() != filtered.String() {
			t.Errorf("flat-mode filtering changed output: unfiltered=%q filtered=%q", unfiltered.String(), filtered.String())
		}
		if strings.Contains(filtered.String(), groupSeparator) {
			t.Errorf("flat-mode filtering should never emit headings: %q", filtered.String())
		}
	})

	t.Run("restores the current mode's headings on clear (By Tag tags, not By Project)", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := tagItems()
		m := list.New(items, d, 80, 10)

		m.SetFilterState(list.FilterApplied)
		_ = renderAll(d, m, items)

		m.SetFilterState(list.Unfiltered)
		restored := renderAll(d, m, items)
		if !strings.Contains(restored, "personal "+groupSeparator) {
			t.Errorf("expected By Tag heading 'personal' to return on clear, got: %q", restored)
		}
		if !strings.Contains(restored, "work "+groupSeparator) {
			t.Errorf("expected By Tag heading 'work' to return on clear, got: %q", restored)
		}
	})

	t.Run("suppresses headings during both Filtering and FilterApplied states", func(t *testing.T) {
		states := []list.FilterState{list.Filtering, list.FilterApplied}
		for _, state := range states {
			d := tui.SessionDelegate{}
			items := projectItems()
			m := list.New(items, d, 80, 10)
			m.SetFilterState(state)

			out := renderAll(d, m, items)
			if strings.Contains(out, groupSeparator) {
				t.Errorf("state %v: expected no headings, got: %q", state, out)
			}
		}
	})

	t.Run("may surface duplicate session rows when filtering in By Tag mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := tagItems()
		m := list.New(items, d, 80, 10)
		m.SetFilterState(list.FilterApplied)

		out := renderAll(d, m, items)
		if strings.Count(out, "dev") < 2 {
			t.Errorf("expected duplicate 'dev' rows (one per matching tag instance), got: %q", out)
		}
		if strings.Contains(out, groupSeparator) {
			t.Errorf("expected no headings around the duplicate rows, got: %q", out)
		}
	})
}

func TestToListItems(t *testing.T) {
	t.Run("converts tmux sessions to list items", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 5, Attached: false},
			{Name: "misc", Windows: 1, Attached: false},
		}

		items := tui.ToListItems(sessions)

		if len(items) != 3 {
			t.Fatalf("ToListItems() returned %d items, want 3", len(items))
		}

		for i, s := range sessions {
			si, ok := items[i].(tui.SessionItem)
			if !ok {
				t.Fatalf("items[%d] is not a SessionItem", i)
			}
			if si.Session.Name != s.Name {
				t.Errorf("items[%d].Session.Name = %q, want %q", i, si.Session.Name, s.Name)
			}
			if si.Session.Windows != s.Windows {
				t.Errorf("items[%d].Session.Windows = %d, want %d", i, si.Session.Windows, s.Windows)
			}
			if si.Session.Attached != s.Attached {
				t.Errorf("items[%d].Session.Attached = %v, want %v", i, si.Session.Attached, s.Attached)
			}
		}
	})

	t.Run("keeps producing flat items with no group metadata", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "dev", Windows: 3, Attached: true},
			{Name: "work", Windows: 5, Attached: false},
		}

		items := tui.ToListItems(sessions)

		for i := range items {
			si, ok := items[i].(tui.SessionItem)
			if !ok {
				t.Fatalf("items[%d] is not a SessionItem", i)
			}
			if si.GroupKey != "" {
				t.Errorf("items[%d].GroupKey = %q, want empty", i, si.GroupKey)
			}
			if si.GroupHeading != "" {
				t.Errorf("items[%d].GroupHeading = %q, want empty", i, si.GroupHeading)
			}
			if si.CatchAll {
				t.Errorf("items[%d].CatchAll = %v, want false", i, si.CatchAll)
			}
		}
	})

	t.Run("empty sessions returns empty items", func(t *testing.T) {
		items := tui.ToListItems([]tmux.Session{})

		if len(items) != 0 {
			t.Errorf("ToListItems([]) returned %d items, want 0", len(items))
		}
	})

	t.Run("nil sessions returns empty items", func(t *testing.T) {
		items := tui.ToListItems(nil)

		if len(items) != 0 {
			t.Errorf("ToListItems(nil) returned %d items, want 0", len(items))
		}
	})
}
