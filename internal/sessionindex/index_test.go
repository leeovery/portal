package sessionindex_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestLookup(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Dir: "/a"}, {Name: "b"}}}

	e, ok := idx.Lookup("a")
	if !ok || e != (sessionindex.Entry{Name: "a", Dir: "/a"}) {
		t.Fatalf("Lookup(a) = %+v, %v; want the a entry", e, ok)
	}
	e.Dir = "/moved"
	if idx.Sessions[0].Dir != "/a" {
		t.Errorf("an edit to the looked-up copy reached the index")
	}
	if _, ok := idx.Lookup("missing"); ok {
		t.Errorf("Lookup(missing) found an entry")
	}
}

func TestLabel(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}, {Name: "b"}}}
	for name, want := range map[string]string{"a": "Alpha", "b": "b", "missing": "missing"} {
		if got := idx.Label(name); got != want {
			t.Errorf("Label(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestSetLabel(t *testing.T) {
	t.Run("it sets a trimmed label", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Dir: "/a"}}}
		if err := idx.SetLabel("a", "  my work \n"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if want := (sessionindex.Entry{Name: "a", Label: "my work", Dir: "/a"}); idx.Sessions[0] != want {
			t.Errorf("entry = %+v, want %+v", idx.Sessions[0], want)
		}
	})

	t.Run("it creates a bare entry for a session with none", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.SetLabel("a", "Alpha"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0] != (sessionindex.Entry{Name: "a", Label: "Alpha"}) {
			t.Errorf("sessions = %+v, want one bare entry labelled Alpha", idx.Sessions)
		}
	})

	for _, reset := range []string{"", "   ", "a", " a "} {
		t.Run("the label "+`"`+reset+`"`+" resets to the default", func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}}}
			if err := idx.SetLabel("a", reset); err != nil {
				t.Fatalf("SetLabel: %v", err)
			}
			if idx.Sessions[0].Label != "" {
				t.Errorf("label = %q, want cleared", idx.Sessions[0].Label)
			}
			if got := idx.Label("a"); got != "a" {
				t.Errorf("Label = %q, want the name", got)
			}
		})

		t.Run("the label "+`"`+reset+`"`+" on a session with no entry creates none", func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b"}}}
			if err := idx.SetLabel("a", reset); err != nil {
				t.Fatalf("SetLabel: %v", err)
			}
			if len(idx.Sessions) != 1 {
				t.Errorf("sessions = %+v, want only b", idx.Sessions)
			}
		})
	}

	for name, label := range map[string]string{
		"a newline":             "two\nlines",
		"a tab":                 "tab\there",
		"an escape":             "red\x1b[31m",
		"a DEL":                 "del\x7f",
		"a bidi override":       "abc\u202edef",
		"a zero-width space":    "zero\u200bwidth",
		"a line separator":      "line\u2028sep",
		"a paragraph separator": "para\u2029sep",
		"invalid UTF-8":         "bad\xffbyte",
	} {
		t.Run("a label holding "+name+" is refused and changes nothing", func(t *testing.T) {
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "Alpha"}}}
			if err := idx.SetLabel("a", label); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
				t.Errorf("SetLabel(%q) = %v, want ErrLabelUnprintable", label, err)
			}
			if err := idx.SetLabel("b", label); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
				t.Errorf("SetLabel on a new session = %v, want ErrLabelUnprintable", err)
			}
			if len(idx.Sessions) != 1 || idx.Sessions[0].Label != "Alpha" {
				t.Errorf("sessions = %+v, want untouched", idx.Sessions)
			}
		})
	}

	t.Run("printable non-ASCII is kept", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.SetLabel("a", "café — 日本"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if got := idx.Label("a"); got != "café — 日本" {
			t.Errorf("Label = %q", got)
		}
	})

	t.Run("a joined emoji is kept", func(t *testing.T) {
		var idx sessionindex.Index
		family := "team \U0001F468\u200d\U0001F469\u200d\U0001F467"
		if err := idx.SetLabel("a", family); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if got := idx.Label("a"); got != family {
			t.Errorf("Label = %q, want %q", got, family)
		}
	})
}

func TestClaim(t *testing.T) {
	created := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
	earlier := created.Add(-time.Hour)
	labelled := func(at time.Time, dir string) sessionindex.Entry {
		return sessionindex.Entry{Name: "a", Label: "Old", Dir: dir, CreatedAt: at, ScrollbackFile: "scrollback/a.bin"}
	}

	for _, tc := range []struct {
		name    string
		held    []sessionindex.Entry
		dir     string
		created time.Time
		want    sessionindex.Entry
	}{
		{
			name:    "a session with no entry gets one",
			held:    []sessionindex.Entry{{Name: "b"}},
			dir:     "/a",
			created: created,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created},
		},
		{
			name:    "an entry from the same creation keeps its label, scrollback and dir",
			held:    []sessionindex.Entry{labelled(created, "/kept")},
			dir:     "/a",
			created: created,
			want:    labelled(created, "/kept"),
		},
		{
			name:    "an entry from the same creation with no dir takes the one given",
			held:    []sessionindex.Entry{labelled(created, "")},
			dir:     "/a",
			created: created,
			want:    labelled(created, "/a"),
		},
		{
			name:    "an entry from another creation is replaced whole",
			held:    []sessionindex.Entry{labelled(earlier, "/old")},
			dir:     "/a",
			created: created,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created},
		},
		{
			name:    "an entry with no creation time is replaced whole",
			held:    []sessionindex.Entry{labelled(time.Time{}, "/old")},
			dir:     "/a",
			created: created,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created},
		},
		{
			name:    "an entry claimed with no creation time is replaced whole",
			held:    []sessionindex.Entry{labelled(time.Time{}, "/old")},
			dir:     "/a",
			created: time.Time{},
			want:    sessionindex.Entry{Name: "a", Dir: "/a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := sessionindex.Index{Sessions: tc.held}
			idx.Claim("a", tc.dir, tc.created)
			got, ok := idx.Lookup("a")
			if !ok || got.Name != tc.want.Name || got.Label != tc.want.Label || got.Dir != tc.want.Dir ||
				!got.CreatedAt.Equal(tc.want.CreatedAt) || got.ScrollbackFile != tc.want.ScrollbackFile {
				t.Errorf("entry = %+v, %v; want %+v", got, ok, tc.want)
			}
			wantLen := len(tc.held)
			if !hasEntry(tc.held, "a") {
				wantLen++
			}
			if len(idx.Sessions) != wantLen {
				t.Errorf("sessions = %+v, want one entry for a beside the others", idx.Sessions)
			}
		})
	}

	t.Run("a creation time recorded in another zone is the same creation", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{labelled(created, "/kept")}}
		idx.Claim("a", "/a", created.In(time.FixedZone("x", 3600)))
		if got := idx.Label("a"); got != "Old" {
			t.Errorf("Label(a) = %q, want the label kept", got)
		}
	})
}

func hasEntry(entries []sessionindex.Entry, name string) bool {
	for _, e := range entries {
		if e.Name == name {
			return true
		}
	}
	return false
}

func TestRemove(t *testing.T) {
	idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	if !idx.Remove("b") {
		t.Errorf("Remove(b) = false, want true")
	}
	if len(idx.Sessions) != 2 || idx.Sessions[0].Name != "a" || idx.Sessions[1].Name != "c" {
		t.Errorf("sessions = %+v, want a and c", idx.Sessions)
	}
	if idx.Remove("b") {
		t.Errorf("Remove of an absent entry = true, want false")
	}
}
