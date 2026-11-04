package sessionindex_test

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestCodec(t *testing.T) {
	t.Run("an index round-trips through Encode and Decode", func(t *testing.T) {
		in := sessionindex.IndexOf([]sessionindex.Entry{
			{Name: "dotfiles-PpaTER", Label: "dots", Dir: "/code/dotfiles", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/dotfiles-PpaTER.bin"},
			{Name: "bare"},
		})
		in.Version, in.SavedAt = sessionindex.SchemaVersion, time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
		data, err := sessionindex.Encode(in)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		out, err := sessionindex.Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if !reflect.DeepEqual(out, in) {
			t.Errorf("round trip = %+v, want %+v", out, in)
		}
	})

	t.Run("it writes the version 2 shape with RFC3339 timestamps", func(t *testing.T) {
		idx := sessionindex.IndexOf([]sessionindex.Entry{{Name: "a", Label: "A", Dir: "/d", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/a.bin"}})
		idx.SavedAt = time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
		data, err := sessionindex.Encode(idx)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if raw["version"] != float64(2) {
			t.Errorf("version = %v, want 2", raw["version"])
		}
		if raw["saved_at"] != "2026-11-02T09:30:00Z" {
			t.Errorf("saved_at = %v, want RFC3339", raw["saved_at"])
		}
		entry := raw["sessions"].([]any)[0].(map[string]any)
		want := map[string]any{"name": "a", "label": "A", "dir": "/d", "created_at": "2026-10-26T07:33:20Z", "scrollback_file": "scrollback/a.bin"}
		if !reflect.DeepEqual(entry, want) {
			t.Errorf("entry = %v, want %v", entry, want)
		}
	})

	t.Run("empty fields are omitted and no sessions encode as an empty list", func(t *testing.T) {
		data, err := sessionindex.Encode(sessionindex.IndexOf([]sessionindex.Entry{{Name: "a"}}))
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		var raw struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if want := map[string]any{"name": "a"}; !reflect.DeepEqual(raw.Sessions[0], want) {
			t.Errorf("bare entry = %v, want %v", raw.Sessions[0], want)
		}

		data, err = sessionindex.Encode(sessionindex.Index{})
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if !strings.Contains(string(data), `"sessions": []`) {
			t.Errorf("empty index = %s, want sessions []", data)
		}
	})

	t.Run("an entry's cwd and pid encode under their own keys and round-trip", func(t *testing.T) {
		in := sessionindex.Entry{Name: "a", Dir: "/d", Cwd: "/d/src", CreatedAt: time.Unix(1793000000, 0).UTC(), PID: 4242}
		data, err := sessionindex.Encode(sessionindex.IndexOf([]sessionindex.Entry{in}))
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		var raw struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if raw.Sessions[0]["cwd"] != "/d/src" || raw.Sessions[0]["pid"] != float64(4242) {
			t.Errorf("entry = %v, want cwd and pid keys", raw.Sessions[0])
		}
		out, err := sessionindex.Decode(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if !reflect.DeepEqual(out.Entries(), []sessionindex.Entry{in}) {
			t.Errorf("round trip = %+v, want %+v", out.Entries(), in)
		}
	})

	t.Run("an entry written before cwd and pid decodes with neither", func(t *testing.T) {
		idx, err := sessionindex.Decode([]byte(`{"version":2,"sessions":[` +
			`{"name":"a","label":"A","dir":"/d","created_at":"2026-10-26T07:33:20Z","scrollback_file":"scrollback/a.bin"}]}`))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		want := sessionindex.Entry{Name: "a", Label: "A", Dir: "/d", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/a.bin"}
		if !reflect.DeepEqual(idx.Entries(), []sessionindex.Entry{want}) {
			t.Errorf("sessions = %+v, want %+v", idx.Entries(), want)
		}
	})

	t.Run("unknown fields decode beside the known ones and encode again as read", func(t *testing.T) {
		in := `{"version":2,"saved_at":"2026-11-02T09:30:00Z","future":{"x":[1,2]},"sessions":[{"name":"a","label":"A","extra":true,"more":"m"},{"name":"b"}]}`
		idx, err := sessionindex.Decode([]byte(in))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if idx.Len() != 2 || idx.Entries()[0].Name != "a" || idx.Entries()[0].Label != "A" {
			t.Errorf("sessions = %+v, want a labelled A and b", idx.Entries())
		}

		data, err := sessionindex.Encode(idx)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		assertSameJSON(t, data, in)
	})

	t.Run("a member spelling a known key in another case is dropped, not written back to shadow the field", func(t *testing.T) {
		idx, err := sessionindex.Decode([]byte(`{"version":2,"saved_at":"2026-11-02T09:30:00Z","Saved_At":"2020-01-01T00:00:00Z",` +
			`"sessions":[{"name":"a","LABEL":"stale","label":"A","future":1}]}`))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}

		data, err := sessionindex.Encode(idx)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		var raw struct {
			Root     map[string]json.RawMessage
			Sessions []map[string]json.RawMessage `json:"sessions"`
		}
		if err := json.Unmarshal(data, &raw.Root); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		if got := slices.Sorted(maps.Keys(raw.Root)); !slices.Equal(got, []string{"saved_at", "sessions", "version"}) {
			t.Errorf("root keys = %v, want the known three alone", got)
		}
		if got := slices.Sorted(maps.Keys(raw.Sessions[0])); !slices.Equal(got, []string{"future", "label", "name"}) {
			t.Errorf("entry keys = %v, want name, label and the unknown future", got)
		}
		again, err := sessionindex.Decode(data)
		if err != nil {
			t.Fatalf("Decode again: %v", err)
		}
		if got := again.Entries()[0].Label; got != idx.Entries()[0].Label {
			t.Errorf("label read back = %q, want %q as first read", got, idx.Entries()[0].Label)
		}
	})
}

func TestUpdate_KeepsUnknownFields(t *testing.T) {
	const seed = `{"version":2,"saved_at":"2026-11-01T00:00:00Z","future":{"x":[1,2]},"sessions":[` +
		`{"name":"a","label":"A","created_at":"2026-11-03T06:00:00Z","extra":true},` +
		`{"name":"b","more":{"deep":"value"}}]}`

	for name, fn := range map[string]func(*sessionindex.Index) error{
		"another entry changes": func(idx *sessionindex.Index) error { return idx.SetLabel("b", "B") },
		"the entry itself changes": func(idx *sessionindex.Index) error {
			idx.RecordCwd("a", "/work/a")
			return nil
		},
		"the entry is adopted by a claim": func(idx *sessionindex.Index) error {
			idx.Claim("a", "", liveCreation, 4242, ruleRestoring())
			return nil
		},
	} {
		t.Run("a rewrite keeps the root's and each entry's unknown members when "+name, func(t *testing.T) {
			dir := t.TempDir()
			writeIndex(t, dir, seed)

			if _, err := sessionindex.Update(dir, "cli", fn); err != nil {
				t.Fatalf("Update: %v", err)
			}

			var raw struct {
				Future   json.RawMessage              `json:"future"`
				Sessions []map[string]json.RawMessage `json:"sessions"`
			}
			if err := json.Unmarshal(readFile(t, dir), &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			assertSameJSON(t, raw.Future, `{"x":[1,2]}`)
			assertSameJSON(t, raw.Sessions[0]["extra"], `true`)
			assertSameJSON(t, raw.Sessions[1]["more"], `{"deep":"value"}`)
		})
	}

	t.Run("an entry replaced whole drops the members the earlier creation's entry carried", func(t *testing.T) {
		dir := t.TempDir()
		writeIndex(t, dir, `{"version":2,"sessions":[{"name":"a","created_at":"2026-11-04T09:00:00Z","extra":true}]}`)

		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.Claim("a", "/a", liveCreation, 4242, ruleRestoring())
			return nil
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}

		var raw struct {
			Sessions []map[string]json.RawMessage `json:"sessions"`
		}
		if err := json.Unmarshal(readFile(t, dir), &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := raw.Sessions[0]["extra"]; ok {
			t.Errorf("entry = %v, want the earlier creation's extra gone with it", raw.Sessions[0])
		}
	})
}

// assertSameJSON compares JSON documents by value, ignoring layout and key
// order.
func assertSameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal %s: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("unmarshal %s: %v", want, err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestDecode_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name, data string
	}{
		{"malformed JSON", `{"version":2,`},
		{"a missing version", `{"sessions":[]}`},
		{"version 0", `{"version":0,"sessions":[]}`},
		{"a future version", `{"version":3,"sessions":[]}`},
		{"version 1", `{"version":1,"sessions":[]}`},
		{"an entry with no name", `{"version":2,"sessions":[{"name":"a"},{"label":"x"}]}`},
		{"an entry with an empty name", `{"version":2,"sessions":[{"name":""}]}`},
		{"a name given twice", `{"version":2,"sessions":[{"name":"a"},{"name":"b"},{"name":"a","label":"x"}]}`},
	} {
		t.Run(tc.name+" is an error", func(t *testing.T) {
			if _, err := sessionindex.Decode([]byte(tc.data)); err == nil {
				t.Errorf("Decode(%s) succeeded", tc.data)
			}
		})
	}

	// The version is judged before the body: another version's body need not
	// fit version 2's types.
	for _, tc := range []struct {
		name, data string
		found      int
		legacy     bool
	}{
		{"version 1", `{"version":1,"saved_at":"2026-01-01T00:00:00Z","sessions":[{"name":"a","windows":[]}]}`, 1, true},
		{"version 1 with a body that does not fit version 2", `{"version":1,"saved_at":7,"sessions":{"a":{}}}`, 1, true},
		{"version 3", `{"version":3,"sessions":[]}`, 3, false},
		{"version 3 with a body that does not fit version 2", `{"version":3,"sessions":[{"name":"a","created_at":1793000000}]}`, 3, false},
	} {
		t.Run(tc.name+" is a VersionError carrying the version found", func(t *testing.T) {
			_, err := sessionindex.Decode([]byte(tc.data))
			var verr *sessionindex.VersionError
			if !errors.As(err, &verr) || verr.Found != tc.found {
				t.Fatalf("error = %#v, want a VersionError with Found %d", err, tc.found)
			}
			if got := errors.Is(err, sessionindex.ErrLegacyIndex); got != tc.legacy {
				t.Errorf("errors.Is(ErrLegacyIndex) = %v, want %v", got, tc.legacy)
			}
		})
	}
}
