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
		{"a future version", `{"version":3,"sessions":[]}`},
		{"version 1", `{"version":1,"sessions":[]}`},
	} {
		t.Run(tc.name+" is an error", func(t *testing.T) {
			if _, err := sessionindex.Decode([]byte(tc.data)); err == nil {
				t.Errorf("Decode(%s) succeeded", tc.data)
			}
		})
	}

	t.Run("version 1 wraps ErrLegacyIndex and carries the version found", func(t *testing.T) {
		_, err := sessionindex.Decode([]byte(`{"version":1,"saved_at":"2026-01-01T00:00:00Z","sessions":[{"name":"a","windows":[]}]}`))
		if !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("error = %v, want ErrLegacyIndex", err)
		}
		var verr *sessionindex.VersionError
		if !errors.As(err, &verr) || verr.Found != 1 {
			t.Errorf("error = %#v, want a VersionError with Found 1", err)
		}
	})

	t.Run("another unsupported version is not ErrLegacyIndex", func(t *testing.T) {
		_, err := sessionindex.Decode([]byte(`{"version":3,"sessions":[]}`))
		if errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("version 3 error %v reads as ErrLegacyIndex", err)
		}
		var verr *sessionindex.VersionError
		if !errors.As(err, &verr) || verr.Found != 3 {
			t.Errorf("error = %#v, want a VersionError with Found 3", err)
		}
	})
}
