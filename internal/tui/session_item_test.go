package tui_test

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tui"
)

func TestSessionItem(t *testing.T) {
	t.Run("it returns the session name alone when the session carries no recorded directory", func(t *testing.T) {
		item := tui.SessionItem{Session: tui.Session{Name: "dev", Answered: true, Attached: true}}

		got := item.FilterValue()

		if got != "dev" {
			t.Errorf("FilterValue() = %q, want %q (name alone, no trailing separator)", got, "dev")
		}
	})

	t.Run("it returns the name, one space and the abbreviated directory", func(t *testing.T) {
		home := t.TempDir()
		item := tui.SessionItem{Session: tui.Session{Name: "api-work", Answered: true, Dir: home + "/Code/portal"}, Home: home}

		got := item.FilterValue()

		if got != "api-work ~/Code/portal" {
			t.Errorf("FilterValue() = %q, want %q", got, "api-work ~/Code/portal")
		}
	})

	t.Run("FilterValue is the SearchFields slice joined by a single space", func(t *testing.T) {
		home := t.TempDir()
		session := tui.Session{Name: "api-work", Answered: true, Dir: home + "/Code/portal"}
		item := tui.SessionItem{Session: session, Home: home}

		got := item.FilterValue()

		want := strings.Join(resolver.SearchFields(home, session.Name, session.Dir), " ")
		if got != want {
			t.Errorf("FilterValue() = %q, want %q (the search fields joined)", got, want)
		}
	})

	t.Run("FilterValue for a session with no recorded directory has no trailing separator", func(t *testing.T) {
		home := t.TempDir()
		session := tui.Session{Name: "dev", Answered: true}
		item := tui.SessionItem{Session: session, Home: home}

		got := item.FilterValue()

		want := strings.Join(resolver.SearchFields(home, session.Name, session.Dir), " ")
		if got != want {
			t.Errorf("FilterValue() = %q, want %q (the one-element slice joins to the bare name)", got, want)
		}
		if strings.HasSuffix(got, " ") {
			t.Errorf("FilterValue() = %q, want no trailing separator", got)
		}
	})

	t.Run("implements list.Item interface", func(t *testing.T) {
		var _ list.Item = tui.SessionItem{}
	})
}

func TestSessionItemGroupMetadata(t *testing.T) {
	t.Run("leaves Flat items with empty group metadata", func(t *testing.T) {
		item := tui.SessionItem{Session: tui.Session{Name: "dev", Answered: true, Attached: true}}

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
			Session:      tui.Session{Name: "dev", Answered: true, Attached: true},
			GroupKey:     "work",
			GroupHeading: "work",
			CatchAll:     false,
		}

		if got := item.FilterValue(); got != "dev" {
			t.Errorf("FilterValue() = %q, want %q", got, "dev")
		}
	})

	t.Run("returns the name and abbreviated directory from FilterValue regardless of group fields", func(t *testing.T) {
		home := t.TempDir()
		item := tui.SessionItem{
			Session:      tui.Session{Name: "dev", Answered: true, Attached: true, Dir: home + "/Code/portal"},
			Home:         home,
			GroupKey:     "work",
			GroupHeading: "work",
			CatchAll:     false,
		}

		if got := item.FilterValue(); got != "dev ~/Code/portal" {
			t.Errorf("FilterValue() = %q, want %q", got, "dev ~/Code/portal")
		}
	})

	t.Run("the live render emits the session name regardless of group fields", func(t *testing.T) {
		item := tui.SessionItem{
			Session:      tui.Session{Name: "dev", Answered: true, Attached: true},
			GroupKey:     "/home/me/project",
			GroupHeading: "project",
			CatchAll:     true,
		}
		d := tui.SessionDelegate{}
		m := list.New([]list.Item{item}, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, item)

		if got := ansi.Strip(buf.String()); !strings.Contains(got, "dev") {
			t.Errorf("render output missing session name 'dev': %q", got)
		}
	})

	t.Run("builds two instances of one session sharing the same underlying Session.Name", func(t *testing.T) {
		session := tui.Session{Name: "dev", Answered: true, Attached: true}
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
			t.Errorf("Spacing() = %d, want 0 (tight rows; no half-row exists in a terminal)", got)
		}
	})

	t.Run("Update returns nil", func(t *testing.T) {
		d := tui.SessionDelegate{}

		cmd := d.Update(nil, nil)

		if cmd != nil {
			t.Error("Update() should return nil")
		}
	})

	t.Run("renders the session name", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "dev", Answered: true, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, "dev") {
			t.Errorf("render output missing session name 'dev': %q", output)
		}
	})

	t.Run("renders attached badge for attached session", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "attached-session", Answered: true, Attached: true}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.HasSuffix(output, " ●  ") {
			t.Errorf("render output must end with the bare '●' indicator: %q", output)
		}
	})

	t.Run("does not render attached badge for detached session", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "detached-session", Answered: true, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if strings.Contains(output, "●") {
			t.Errorf("render output should not contain the '●' indicator for detached session: %q", output)
		}
	})

	t.Run("highlights selected item", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "first", Answered: true, Attached: false}},
			tui.SessionItem{Session: tui.Session{Name: "second", Answered: true, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var selectedBuf bytes.Buffer
		d.Render(&selectedBuf, m, 0, items[0])
		selectedOutput := selectedBuf.String()

		var unselectedBuf bytes.Buffer
		d.Render(&unselectedBuf, m, 1, items[1])
		unselectedOutput := unselectedBuf.String()

		if !strings.Contains(selectedOutput, "▌") {
			t.Errorf("selected item should contain selector bar glyph '▌': %q", selectedOutput)
		}
		if strings.Contains(unselectedOutput, "▌") {
			t.Errorf("unselected item should not contain selector bar glyph '▌': %q", unselectedOutput)
		}
	})

	t.Run("short session name renders in full", func(t *testing.T) {
		name := "my-project"
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: name, Answered: true, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, items[0])

		output := buf.String()
		if !strings.Contains(output, name) {
			t.Errorf("render output should contain the short name %q: %q", name, output)
		}
		if strings.Contains(output, "…") {
			t.Errorf("a short name should not be truncated: %q", output)
		}
	})
}

const groupSeparator = "···"

func TestSessionDelegateGroupHeadings(t *testing.T) {
	t.Run("renders a HeaderItem as the dimmed 'Heading ··· N' separator", func(t *testing.T) {
		d := tui.SessionDelegate{}
		header := tui.HeaderItem{Heading: "Work", Count: 1, Key: "/p/work"}
		m := list.New([]list.Item{header}, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, header)
		out := buf.String()
		if !strings.Contains(out, "Work") {
			t.Errorf("expected heading label 'Work': %q", out)
		}
		if !strings.Contains(out, groupSeparator) {
			t.Errorf("expected separator glyph in heading: %q", out)
		}
	})

	t.Run("a SessionItem renders only its own row, never a heading line", func(t *testing.T) {
		d := tui.SessionDelegate{}
		item := tui.SessionItem{Session: tui.Session{Name: "a", Answered: true}, GroupKey: "/p/portal", GroupHeading: "Portal"}
		m := list.New([]list.Item{item}, d, 80, 10)

		var buf bytes.Buffer
		d.Render(&buf, m, 0, item)
		out := buf.String()
		if strings.Contains(out, groupSeparator) {
			t.Errorf("SessionItem render emitted a heading separator: %q", out)
		}
		if strings.Contains(out, "Portal") {
			t.Errorf("SessionItem render leaked its GroupHeading 'Portal': %q", out)
		}
		if !strings.Contains(out, "a") {
			t.Errorf("SessionItem render missing its own name 'a': %q", out)
		}
		if strings.Contains(out, "\n") {
			t.Errorf("SessionItem emitted a multi-line render: %q", out)
		}
	})

	t.Run("renders the HeaderItem Count as the per-group total", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(nil, d, 80, 10)

		var portal bytes.Buffer
		d.Render(&portal, m, 0, tui.HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"})
		if vis := ansi.Strip(portal.String()); !strings.Contains(vis, "Portal "+groupSeparator+" 2") {
			t.Errorf("expected 'Portal %s 2', got: %q", groupSeparator, vis)
		}

		var work bytes.Buffer
		d.Render(&work, m, 1, tui.HeaderItem{Heading: "Work", Count: 3, Key: "/p/work"})
		if vis := ansi.Strip(work.String()); !strings.Contains(vis, "Work "+groupSeparator+" 3") {
			t.Errorf("expected 'Work %s 3', got: %q", groupSeparator, vis)
		}
	})

	t.Run("renders a per-tag count of 1 for each of a multi-tag session's headers", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(nil, d, 80, 10)

		var personal bytes.Buffer
		d.Render(&personal, m, 0, tui.HeaderItem{Heading: "personal", Count: 1, Key: "personal"})
		if vis := ansi.Strip(personal.String()); !strings.Contains(vis, "personal "+groupSeparator+" 1") {
			t.Errorf("expected 'personal %s 1', got: %q", groupSeparator, vis)
		}

		var work bytes.Buffer
		d.Render(&work, m, 1, tui.HeaderItem{Heading: "work", Count: 1, Key: "work"})
		if vis := ansi.Strip(work.String()); !strings.Contains(vis, "work "+groupSeparator+" 1") {
			t.Errorf("expected 'work %s 1', got: %q", groupSeparator, vis)
		}
	})

	t.Run("injects no heading for flat items (byte-identical to today)", func(t *testing.T) {
		d := tui.SessionDelegate{}
		items := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "first", Answered: true, Attached: true}},
			tui.SessionItem{Session: tui.Session{Name: "second", Answered: true, Attached: false}},
		}
		m := list.New(items, d, 80, 10)

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

	t.Run("renders the catch-all (Untagged) HeaderItem with its group count", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(nil, d, 80, 10)

		var last bytes.Buffer
		d.Render(&last, m, 0, tui.HeaderItem{Heading: "Untagged", Count: 3, Key: "Untagged"})
		if vis := ansi.Strip(last.String()); !strings.Contains(vis, "Untagged "+groupSeparator+" 3") {
			t.Errorf("expected 'Untagged %s 3', got: %q", groupSeparator, vis)
		}
	})

	t.Run("FilterValue is empty so the built-in filter drops headers", func(t *testing.T) {
		var _ list.Item = tui.HeaderItem{}
		if got := (tui.HeaderItem{Heading: "Portal", Count: 2}).FilterValue(); got != "" {
			t.Errorf("HeaderItem.FilterValue() = %q, want empty", got)
		}
	})
}

func TestSessionDelegateFlattenOnFilter(t *testing.T) {
	projectItems := func() []list.Item {
		return []list.Item{
			tui.HeaderItem{Heading: "Portal", Count: 2, Key: "/p/portal"},
			tui.SessionItem{Session: tui.Session{Name: "a", Answered: true}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.SessionItem{Session: tui.Session{Name: "b", Answered: true}, GroupKey: "/p/portal", GroupHeading: "Portal"},
			tui.HeaderItem{Heading: "Work", Count: 1, Key: "/p/work"},
			tui.SessionItem{Session: tui.Session{Name: "c", Answered: true}, GroupKey: "/p/work", GroupHeading: "Work"},
		}
	}

	tagItems := func() []list.Item {
		return []list.Item{
			tui.HeaderItem{Heading: "personal", Count: 1, Key: "personal"},
			tui.SessionItem{Session: tui.Session{Name: "dev", Answered: true}, GroupKey: "personal", GroupHeading: "personal"},
			tui.HeaderItem{Heading: "work", Count: 1, Key: "work"},
			tui.SessionItem{Session: tui.Session{Name: "dev", Answered: true}, GroupKey: "work", GroupHeading: "work"},
		}
	}

	countHeaders := func(items []list.Item) int {
		n := 0
		for _, it := range items {
			if _, ok := it.(tui.HeaderItem); ok {
				n++
			}
		}
		return n
	}
	countSessions := func(items []list.Item) int {
		n := 0
		for _, it := range items {
			if _, ok := it.(tui.SessionItem); ok {
				n++
			}
		}
		return n
	}

	t.Run("the built-in filter drops all headers in By Project mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(projectItems(), d, 80, 10)
		m.SetFilterText("a")

		visible := m.VisibleItems()
		if countHeaders(visible) != 0 {
			t.Errorf("expected no HeaderItems while filtering By Project, got %d in %v", countHeaders(visible), visible)
		}
		if countSessions(visible) == 0 {
			t.Errorf("expected at least one matching session row to survive the filter, got none")
		}
	})

	t.Run("the built-in filter drops all headers in By Tag mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(tagItems(), d, 80, 10)
		m.SetFilterText("dev")

		visible := m.VisibleItems()
		if countHeaders(visible) != 0 {
			t.Errorf("expected no HeaderItems while filtering By Tag, got %d in %v", countHeaders(visible), visible)
		}
	})

	t.Run("headers return to the visible set when the filter is cleared", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(projectItems(), d, 80, 10)

		m.SetFilterText("a")
		if countHeaders(m.VisibleItems()) != 0 {
			t.Fatalf("precondition failed: headers present while filtering")
		}

		m.ResetFilter()
		restored := m.VisibleItems()
		if countHeaders(restored) != 2 {
			t.Errorf("expected the 2 group headers to return on clear, got %d", countHeaders(restored))
		}
		var buf bytes.Buffer
		for i := range restored {
			d.Render(&buf, m, i, restored[i])
		}
		out := ansi.Strip(buf.String())
		if !strings.Contains(out, "Portal "+groupSeparator) || !strings.Contains(out, "Work "+groupSeparator) {
			t.Errorf("expected By Project headings to render again on clear, got: %q", out)
		}
	})

	t.Run("leaves Flat-mode filtering unchanged (no headers ever)", func(t *testing.T) {
		d := tui.SessionDelegate{}
		flat := []list.Item{
			tui.SessionItem{Session: tui.Session{Name: "first", Answered: true, Attached: true}},
			tui.SessionItem{Session: tui.Session{Name: "second", Answered: true, Attached: false}},
		}
		m := list.New(flat, d, 80, 10)
		m.SetFilterText("first")

		visible := m.VisibleItems()
		if countHeaders(visible) != 0 {
			t.Errorf("flat-mode filtering should never surface headers, got %d", countHeaders(visible))
		}
		if countSessions(visible) == 0 {
			t.Errorf("expected the matching flat session row to survive the filter")
		}
	})

	t.Run("the current mode's headers return on clear (By Tag tags, not By Project)", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(tagItems(), d, 80, 10)

		m.SetFilterText("dev")
		_ = m.VisibleItems()

		m.ResetFilter()
		restored := m.VisibleItems()
		var buf bytes.Buffer
		for i := range restored {
			d.Render(&buf, m, i, restored[i])
		}
		out := ansi.Strip(buf.String())
		if !strings.Contains(out, "personal "+groupSeparator) {
			t.Errorf("expected By Tag heading 'personal' to return on clear, got: %q", out)
		}
		if !strings.Contains(out, "work "+groupSeparator) {
			t.Errorf("expected By Tag heading 'work' to return on clear, got: %q", out)
		}
	})

	t.Run("may surface duplicate session rows when filtering in By Tag mode", func(t *testing.T) {
		d := tui.SessionDelegate{}
		m := list.New(tagItems(), d, 80, 10)
		m.SetFilterText("dev")

		visible := m.VisibleItems()
		if countSessions(visible) < 2 {
			t.Errorf("expected duplicate 'dev' rows (one per matching tag instance), got %d", countSessions(visible))
		}
		if countHeaders(visible) != 0 {
			t.Errorf("expected no headers around the duplicate rows, got %d", countHeaders(visible))
		}
	})
}

func TestToListItems(t *testing.T) {
	t.Run("converts sessions to list items", func(t *testing.T) {
		sessions := []tui.Session{
			{Name: "dev", Answered: true, Attached: true},
			{Name: "work", Answered: true, Attached: false},
			{Name: "misc", Answered: true, Attached: false},
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
			if si.Session != s {
				t.Errorf("items[%d].Session = %+v, want %+v", i, si.Session, s)
			}
		}
	})

	t.Run("keeps producing flat items with no group metadata", func(t *testing.T) {
		sessions := []tui.Session{
			{Name: "dev", Answered: true, Attached: true},
			{Name: "work", Answered: true, Attached: false},
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
		items := tui.ToListItems([]tui.Session{})

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
