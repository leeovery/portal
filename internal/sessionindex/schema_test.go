package sessionindex_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/sessionindex"
)

func TestCodec(t *testing.T) {
	t.Run("an index round-trips through Encode and Decode", func(t *testing.T) {
		in := sessionindex.Index{
			Version: sessionindex.SchemaVersion,
			SavedAt: time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC),
			Sessions: []sessionindex.Entry{
				{Name: "dotfiles-PpaTER", Label: "dots", Dir: "/code/dotfiles", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/dotfiles-PpaTER.bin"},
				{Name: "bare"},
			},
		}
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
		data, err := sessionindex.Encode(sessionindex.Index{
			SavedAt:  time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC),
			Sessions: []sessionindex.Entry{{Name: "a", Label: "A", Dir: "/d", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/a.bin"}},
		})
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
		data, err := sessionindex.Encode(sessionindex.Index{Sessions: []sessionindex.Entry{{Name: "a"}}})
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
		data, err := sessionindex.Encode(sessionindex.Index{Sessions: []sessionindex.Entry{in}})
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
		if !reflect.DeepEqual(out.Sessions, []sessionindex.Entry{in}) {
			t.Errorf("round trip = %+v, want %+v", out.Sessions, in)
		}
	})

	t.Run("an entry written before cwd and pid decodes with neither", func(t *testing.T) {
		idx, err := sessionindex.Decode([]byte(`{"version":2,"sessions":[` +
			`{"name":"a","label":"A","dir":"/d","created_at":"2026-10-26T07:33:20Z","scrollback_file":"scrollback/a.bin"}]}`))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		want := sessionindex.Entry{Name: "a", Label: "A", Dir: "/d", CreatedAt: time.Unix(1793000000, 0).UTC(), ScrollbackFile: "scrollback/a.bin"}
		if !reflect.DeepEqual(idx.Sessions, []sessionindex.Entry{want}) {
			t.Errorf("sessions = %+v, want %+v", idx.Sessions, want)
		}
	})

	t.Run("unknown fields are ignored", func(t *testing.T) {
		idx, err := sessionindex.Decode([]byte(`{"version":2,"future":1,"sessions":[{"name":"a","extra":true}]}`))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "a" {
			t.Errorf("sessions = %+v, want one named a", idx.Sessions)
		}
	})
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
