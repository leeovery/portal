package sessionindex_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestLookup(t *testing.T) {
	idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a", Dir: "/a"}, {Name: "b"}})

	e, ok := idx.Lookup("a")
	if !ok || e != (sessionindex.Entry{Name: "a", Dir: "/a"}) {
		t.Fatalf("Lookup(a) = %+v, %v; want the a entry", e, ok)
	}
	e.Dir = "/moved"
	if idx.Entries()[0].Dir != "/a" {
		t.Errorf("an edit to the looked-up copy reached the index")
	}
	if _, ok := idx.Lookup("missing"); ok {
		t.Errorf("Lookup(missing) found an entry")
	}
}
func TestSetLabel(t *testing.T) {
	t.Run("it sets a trimmed label", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a", Dir: "/a"}})
		if err := idx.SetLabel("a", "  my work \n"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if want := (sessionindex.Entry{Name: "a", Label: "my work", Dir: "/a"}); idx.Entries()[0] != want {
			t.Errorf("entry = %+v, want %+v", idx.Entries()[0], want)
		}
	})

	t.Run("it creates a bare entry for a session with none", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.SetLabel("a", "Alpha"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if idx.Len() != 1 || idx.Entries()[0] != (sessionindex.Entry{Name: "a", Label: "Alpha"}) {
			t.Errorf("sessions = %+v, want one bare entry labelled Alpha", idx.Entries())
		}
	})

	for _, reset := range []string{"", "   ", "a", " a "} {
		t.Run("the label "+`"`+reset+`"`+" resets to the default", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a", Label: "Alpha"}})
			if err := idx.SetLabel("a", reset); err != nil {
				t.Fatalf("SetLabel: %v", err)
			}
			if idx.Entries()[0].Label != "" {
				t.Errorf("label = %q, want cleared", idx.Entries()[0].Label)
			}
		})

		t.Run("the label "+`"`+reset+`"`+" on a session with no entry creates none", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b"}})
			if err := idx.SetLabel("a", reset); err != nil {
				t.Fatalf("SetLabel: %v", err)
			}
			if idx.Len() != 1 {
				t.Errorf("sessions = %+v, want only b", idx.Entries())
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
			idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a", Label: "Alpha"}})
			if err := idx.SetLabel("a", label); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
				t.Errorf("SetLabel(%q) = %v, want ErrLabelUnprintable", label, err)
			}
			if err := idx.SetLabel("b", label); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
				t.Errorf("SetLabel on a new session = %v, want ErrLabelUnprintable", err)
			}
			if idx.Len() != 1 || idx.Entries()[0].Label != "Alpha" {
				t.Errorf("sessions = %+v, want untouched", idx.Entries())
			}
		})
	}

	t.Run("printable non-ASCII is kept", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.SetLabel("a", "café — 日本"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if got := labelOf(idx, "a"); got != "café — 日本" {
			t.Errorf("Label = %q", got)
		}
	})

	t.Run("a joined emoji is kept", func(t *testing.T) {
		var idx sessionindex.Index
		family := "team \U0001F468\u200d\U0001F469\u200d\U0001F467"
		if err := idx.SetLabel("a", family); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}
		if got := labelOf(idx, "a"); got != family {
			t.Errorf("Label = %q, want %q", got, family)
		}
	})
}
func TestSetScrollbackAndRecordCwd(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(*sessionindex.Index, string, string)
		field func(sessionindex.Entry) string
	}{
		{"SetScrollback", (*sessionindex.Index).SetScrollback, func(e sessionindex.Entry) string { return e.ScrollbackFile }},
		{"RecordCwd", (*sessionindex.Index).RecordCwd, func(e sessionindex.Entry) string { return e.Cwd }},
	} {
		t.Run(tc.name+" sets the field of an existing entry and nothing else", func(t *testing.T) {
			kept := sessionindex.Entry{Name: "a", Label: "Alpha", Dir: "/a", CreatedAt: time.Unix(1793000000, 0).UTC(), PID: 7}
			idx := sessionindex.IndexOf([]sessionindex.Entry{kept})

			tc.set(&idx, "a", "value")

			got, _ := idx.Lookup("a")
			if tc.field(got) != "value" {
				t.Errorf("entry = %+v, want the field set", got)
			}
			got.ScrollbackFile, got.Cwd = "", ""
			if !sameEntry(got, kept) {
				t.Errorf("entry = %+v, want the rest of %+v kept", got, kept)
			}
		})

		t.Run(tc.name+" creates no entry for a session with none", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "b"}})

			tc.set(&idx, "a", "value")

			if idx.Len() != 1 || idx.Entries()[0] != (sessionindex.Entry{Name: "b"}) {
				t.Errorf("sessions = %+v, want only b, untouched", idx.Entries())
			}
		})
	}
}

func TestRemove(t *testing.T) {
	idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	if !idx.Remove("b") {
		t.Errorf("Remove(b) = false, want true")
	}
	if idx.Len() != 2 || idx.Entries()[0].Name != "a" || idx.Entries()[1].Name != "c" {
		t.Errorf("sessions = %+v, want a and c", idx.Entries())
	}
	if idx.Remove("b") {
		t.Errorf("Remove of an absent entry = true, want false")
	}
}

func TestOwe(t *testing.T) {
	idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a"}, {Name: "b"}})

	if !idx.Owe("a") {
		t.Error("Owe(a) = false, want true for the mark it made")
	}
	if idx.Owe("a") {
		t.Error("Owe(a) again = true, want false: it was owed already")
	}
	if idx.Owe("c") {
		t.Error("Owe(c) = true, want false for a session with no entry")
	}
	if got := idx.Entries(); len(got) != 2 || !got[0].Owed || got[1].Owed {
		t.Errorf("sessions = %+v, want a owed and b not, and no entry made for c", got)
	}
}

func labelOf(idx sessionindex.Index, name string) string {
	e, _ := idx.Lookup(name)
	return e.Label
}
