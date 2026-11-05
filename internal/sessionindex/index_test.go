package sessionindex_test

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
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

func TestLabelFor(t *testing.T) {
	for label, want := range map[string]string{
		"  my work \n": "my work",
		"":             "",
		"   ":          "",
		"a":            "",
		" a ":          "",
		"café — 日本":    "café — 日本",
	} {
		got, err := sessionindex.LabelFor("a", label)
		if err != nil || got != want {
			t.Errorf("LabelFor(a, %q) = %q, %v; want %q", label, got, err, want)
		}
	}
	if _, err := sessionindex.LabelFor("a", "two\nlines inside"); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
		t.Errorf("LabelFor of an unprintable label = %v, want ErrLabelUnprintable", err)
	}
}

func TestMigrated(t *testing.T) {
	migrating := sessionindex.Entry{Name: "api", Label: " api · user search ", Dir: "/code/api", Cwd: "/code/api/web", ScrollbackFile: "scrollback/api.bin"}
	made := sessionindex.Entry{Name: "api", Label: "api · user search", Dir: "/code/api", Cwd: "/code/api/web", ScrollbackFile: "scrollback/api.bin", Owed: true, ResumeLazy: true}

	t.Run("it makes an entry owed a restore, recording no creation and resuming lazily, its label as LabelFor gives it", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "other"}})
		if err := idx.Migrated(migrating); err != nil {
			t.Fatalf("Migrated: %v", err)
		}
		if got, _ := idx.Lookup("api"); got != made {
			t.Errorf("entry = %+v, want %+v", got, made)
		}
	})

	t.Run("it takes only the name, label, dirs and scrollback of what it is handed", func(t *testing.T) {
		var idx sessionindex.Index
		handed := migrating
		handed.CreatedAt, handed.PID, handed.Owed, handed.ResumeLazy = liveCreation, 4242, false, false
		if err := idx.Migrated(handed); err != nil {
			t.Fatalf("Migrated: %v", err)
		}
		if got, _ := idx.Lookup("api"); got != made {
			t.Errorf("entry = %+v, want %+v", got, made)
		}
	})

	t.Run("a label equal to the name is omitted", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.Migrated(sessionindex.Entry{Name: "api", Label: "api"}); err != nil {
			t.Fatalf("Migrated: %v", err)
		}
		if got := labelOf(idx, "api"); got != "" {
			t.Errorf("label = %q, want none", got)
		}
	})

	t.Run("an unprintable label is refused and changes nothing", func(t *testing.T) {
		var idx sessionindex.Index
		if err := idx.Migrated(sessionindex.Entry{Name: "api", Label: "red\x1b[31m"}); !errors.Is(err, sessionindex.ErrLabelUnprintable) {
			t.Errorf("Migrated = %v, want ErrLabelUnprintable", err)
		}
		if idx.Len() != 0 {
			t.Errorf("sessions = %+v, want none", idx.Entries())
		}
	})

	t.Run("an entry it made already is left as it is", func(t *testing.T) {
		dir := t.TempDir()
		seedIndex(t, dir, made)
		before := readFile(t, dir)

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error { return idx.Migrated(migrating) }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if after := readFile(t, dir); string(after) != string(before) {
			t.Errorf("sessions.json rewritten:\n%s\nwant untouched:\n%s", after, before)
		}
	})

	for name, held := range map[string]sessionindex.Entry{
		"a session's live entry":      {Name: "api", Dir: "/code/api", CreatedAt: liveCreation, PID: 4242},
		"an entry with another label": {Name: "api", Label: "api · other", Dir: "/code/api", Cwd: "/code/api/web", ScrollbackFile: "scrollback/api.bin", Owed: true, ResumeLazy: true},
	} {
		t.Run("a name "+name+" holds is refused and changes nothing", func(t *testing.T) {
			idx := sessionindex.IndexOf([]sessionindex.Entry{held})
			if err := idx.Migrated(migrating); !errors.Is(err, sessionindex.ErrNameHeld) {
				t.Errorf("Migrated = %v, want ErrNameHeld", err)
			}
			if got := idx.Entries(); len(got) != 1 || got[0] != held {
				t.Errorf("sessions = %+v, want only %+v", got, held)
			}
		})
	}

	t.Run("a name this update removed is refused", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "api"}})
		idx.Remove("api")
		if err := idx.Migrated(migrating); !errors.Is(err, sessionindex.ErrRemoved) {
			t.Errorf("Migrated = %v, want ErrRemoved", err)
		}
	})

	t.Run("it logs the entry it made at INFO, with its dir and what it carries", func(t *testing.T) {
		dir := t.TempDir()
		sink := logtest.Install(t)

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error { return idx.Migrated(migrating) }); err != nil {
			t.Fatalf("Update: %v", err)
		}
		rec := sink.Records().Matching("sessions", "migrated").Only(t, "migrated")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "migrated", Component: "sessions", Op: "migrated", Via: "cli"})
		if got := rec.AttrString(t, "value"); got != "/code/api" {
			t.Errorf("value = %q, want the dir", got)
		}
		want := `owed a restore, recording no creation, resuming lazily until it is restored; its label "api · user search" and its scrollback "scrollback/api.bin"`
		if got := rec.AttrString(t, "detail"); got != want {
			t.Errorf("detail = %q, want %q", got, want)
		}
	})
}

func TestCreate(t *testing.T) {
	create := func(idx *sessionindex.Index) error {
		idx.Create()
		return nil
	}

	t.Run("an index read from no file is written, holding no entry", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := sessionindex.Update(dir, "cli", create); err != nil {
			t.Fatalf("Update: %v", err)
		}

		idx, err := sessionindex.Decode(readFile(t, dir))
		if err != nil || idx.Version != sessionindex.SchemaVersion || idx.Len() != 0 {
			t.Errorf("sessions.json = %+v, %v; want a version 2 index of no entries", idx, err)
		}
	})

	t.Run("it writes the entries the same update makes", func(t *testing.T) {
		dir := t.TempDir()
		sink := logtest.Install(t)
		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.Create()
			return idx.Migrated(sessionindex.Entry{Name: "api", Dir: "/code/api"})
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		idx, err := sessionindex.Decode(readFile(t, dir))
		if got := idx.Entries(); err != nil || len(got) != 1 || got[0].Name != "api" {
			t.Errorf("entries = %+v, %v; want api's", got, err)
		}
		sink.Records().Matching("sessions", "migrated").Only(t, "migrated")
	})

	t.Run("an index read from a file is left as it is", func(t *testing.T) {
		dir := t.TempDir()
		seedIndex(t, dir)
		before := readFile(t, dir)

		if _, err := sessionindex.Update(dir, "cli", create); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if after := readFile(t, dir); string(after) != string(before) {
			t.Errorf("sessions.json rewritten:\n%s\nwant untouched:\n%s", after, before)
		}
	})

	t.Run("it logs one INFO naming no session", func(t *testing.T) {
		sink := logtest.Install(t)
		if _, err := sessionindex.Update(t.TempDir(), "cli", create); err != nil {
			t.Fatalf("Update: %v", err)
		}

		rec := sink.Records().Matching("sessions", "create").Only(t, "create")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "create", Component: "sessions", Op: "create", Via: "cli"})
		if rec.HasAttr("session") {
			t.Errorf("create carries a session: %+v", rec.Attrs)
		}
	})
}

func labelOf(idx sessionindex.Index, name string) string {
	e, _ := idx.Lookup(name)
	return e.Label
}
