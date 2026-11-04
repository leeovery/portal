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

func TestLookupLive(t *testing.T) {
	created := time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)
	other := created.Add(time.Second)
	entry := sessionindex.Entry{Name: "api-x7Kd9a", Label: "api work", Dir: "/work/api", CreatedAt: created}
	withPID := sessionindex.Entry{Name: "api-x7Kd9a", Label: "api work", CreatedAt: created, PID: 4242}

	tests := []struct {
		name    string
		entries []sessionindex.Entry
		live    time.Time
		livePID int
		want    sessionindex.Entry
		wantOK  bool
	}{
		{"the same creation finds the entry", []sessionindex.Entry{entry}, created, 0, entry, true},
		{"the same creation in another zone finds the entry", []sessionindex.Entry{entry}, created.In(time.FixedZone("BST", 3600)), 0, entry, true},
		{"another creation finds none", []sessionindex.Entry{entry}, other, 0, sessionindex.Entry{}, false},
		{"an entry recording no creation finds none", []sessionindex.Entry{{Name: "api-x7Kd9a", Label: "api work"}}, created, 0, sessionindex.Entry{}, false},
		{"a live session reporting no creation finds none", []sessionindex.Entry{entry}, time.Time{}, 0, sessionindex.Entry{}, false},
		{"no entry finds none", []sessionindex.Entry{{Name: "web-Qm3pLz", CreatedAt: created}}, created, 0, sessionindex.Entry{}, false},
		{"the same time and pid find the entry", []sessionindex.Entry{withPID}, created, 4242, withPID, true},
		{"the same time with another pid is another creation, and finds none", []sessionindex.Entry{withPID}, created, 4243, sessionindex.Entry{}, false},
		{"the same time from a live session reporting no pid finds the entry", []sessionindex.Entry{withPID}, created, 0, withPID, true},
		{"the same time on an entry recording no pid finds it", []sessionindex.Entry{entry}, created, 4242, entry, true},
		{
			"an unprintable label is dropped, the rest kept",
			[]sessionindex.Entry{{Name: "api-x7Kd9a", Label: "api\nwork", Dir: "/work/api", CreatedAt: created}},
			created,
			0,
			sessionindex.Entry{Name: "api-x7Kd9a", Dir: "/work/api", CreatedAt: created},
			true,
		},
		{
			"a label of only whitespace is dropped, the rest kept",
			[]sessionindex.Entry{{Name: "api-x7Kd9a", Label: " \u00a0 ", Dir: "/work/api", CreatedAt: created}},
			created,
			0,
			sessionindex.Entry{Name: "api-x7Kd9a", Dir: "/work/api", CreatedAt: created},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := sessionindex.Index{Sessions: tt.entries}

			got, ok := idx.LookupLive("api-x7Kd9a", tt.live, tt.livePID)

			if ok != tt.wantOK || got != tt.want {
				t.Errorf("LookupLive = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}

	t.Run("it leaves an unprintable label in the index", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a", Label: "x\ty", CreatedAt: created}}}

		_, _ = idx.LookupLive("a", created, 0)

		if idx.Sessions[0].Label != "x\ty" {
			t.Errorf("Label = %q, want the file's label untouched", idx.Sessions[0].Label)
		}
	})
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

func TestClaim(t *testing.T) {
	created := time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC)
	earlier := created.Add(-time.Hour)
	labelled := func(at time.Time, dir string) sessionindex.Entry {
		return sessionindex.Entry{Name: "a", Label: "Old", Dir: dir, Cwd: "/was/here", CreatedAt: at, ScrollbackFile: "scrollback/a.bin"}
	}
	labelledPID := func(at time.Time, pid int) sessionindex.Entry {
		e := labelled(at, "/kept")
		e.PID = pid
		return e
	}

	for _, tc := range []struct {
		name    string
		held    []sessionindex.Entry
		dir     string
		created time.Time
		pid     int
		want    sessionindex.Entry
	}{
		{
			name:    "a session with no entry gets one, recording its creation",
			held:    []sessionindex.Entry{{Name: "b"}},
			dir:     "/a",
			created: created,
			pid:     4242,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created, PID: 4242},
		},
		{
			name:    "an entry from the same creation keeps its label, scrollback, dir and cwd",
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
			name:    "an entry from the same time recording no pid is the same creation, and takes the pid",
			held:    []sessionindex.Entry{labelled(created, "/kept")},
			dir:     "/a",
			created: created,
			pid:     4242,
			want:    labelledPID(created, 4242),
		},
		{
			name:    "an entry from the same time and pid is the same creation",
			held:    []sessionindex.Entry{labelledPID(created, 4242)},
			dir:     "/a",
			created: created,
			pid:     4242,
			want:    labelledPID(created, 4242),
		},
		{
			name:    "an entry from the same time claimed with no pid is the same creation, keeping its pid",
			held:    []sessionindex.Entry{labelledPID(created, 4242)},
			dir:     "/a",
			created: created,
			want:    labelledPID(created, 4242),
		},
		{
			name:    "an entry from the same time with another pid is another creation, replaced whole",
			held:    []sessionindex.Entry{labelledPID(created, 4242)},
			dir:     "/a",
			created: created,
			pid:     4243,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created, PID: 4243},
		},
		{
			name:    "an entry from another creation is replaced whole",
			held:    []sessionindex.Entry{labelled(earlier, "/old")},
			dir:     "/a",
			created: created,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created},
		},
		{
			name:    "an entry from another time with the same pid is another creation, replaced whole",
			held:    []sessionindex.Entry{labelledPID(earlier, 4242)},
			dir:     "/a",
			created: created,
			pid:     4242,
			want:    sessionindex.Entry{Name: "a", Dir: "/a", CreatedAt: created, PID: 4242},
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
			idx.Claim("a", tc.dir, tc.created, tc.pid)
			got, ok := idx.Lookup("a")
			if !ok || !sameEntry(got, tc.want) {
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
		idx.Claim("a", "/a", created.In(time.FixedZone("x", 3600)), 0)
		if got := labelOf(idx, "a"); got != "Old" {
			t.Errorf("Label(a) = %q, want the label kept", got)
		}
	})
}

// sameEntry compares entries field by field, creation times as instants.
func sameEntry(got, want sessionindex.Entry) bool {
	if !got.CreatedAt.Equal(want.CreatedAt) {
		return false
	}
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	return got == want
}

func TestRestored(t *testing.T) {
	saved := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	restored := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	entry := sessionindex.Entry{
		Name: "a", Label: "api work", Dir: "/work/api", Cwd: "/work/api/src",
		CreatedAt: saved, PID: 1717, ScrollbackFile: "scrollback/a.bin",
	}

	t.Run("it records the new creation and keeps the label, dirs and scrollback", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{entry, {Name: "b", Label: "Beta"}}}

		if !idx.Restored("a", restored, 4242) {
			t.Fatal("Restored = false, want the entry found")
		}

		want := entry
		want.CreatedAt, want.PID = restored, 4242
		if got, _ := idx.Lookup("a"); !sameEntry(got, want) {
			t.Errorf("entry = %+v, want %+v", got, want)
		}
		if got, _ := idx.Lookup("b"); got != (sessionindex.Entry{Name: "b", Label: "Beta"}) {
			t.Errorf("b = %+v, want it untouched", got)
		}
		if got, ok := idx.LookupLive("a", restored, 4242); !ok || got.Label != "api work" {
			t.Errorf("LookupLive of the restored creation = %+v, %v; want the label shown", got, ok)
		}
	})

	t.Run("a zero creation records nothing", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{entry}}

		if !idx.Restored("a", time.Time{}, 4242) {
			t.Fatal("Restored = false, want the entry found")
		}
		if got, _ := idx.Lookup("a"); !sameEntry(got, entry) {
			t.Errorf("entry = %+v, want it untouched", got)
		}
	})

	t.Run("a session with no entry gets none", func(t *testing.T) {
		idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b"}}}

		if idx.Restored("a", restored, 4242) {
			t.Error("Restored = true, want no entry found")
		}
		if len(idx.Sessions) != 1 {
			t.Errorf("sessions = %+v, want only b", idx.Sessions)
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
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{kept}}

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
			idx := sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "b"}}}

			tc.set(&idx, "a", "value")

			if len(idx.Sessions) != 1 || idx.Sessions[0] != (sessionindex.Entry{Name: "b"}) {
				t.Errorf("sessions = %+v, want only b, untouched", idx.Sessions)
			}
		})
	}
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

func labelOf(idx sessionindex.Index, name string) string {
	e, _ := idx.Lookup(name)
	return e.Label
}
