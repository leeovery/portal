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
