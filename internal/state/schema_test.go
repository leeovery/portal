package state_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

func fullyPopulatedIndex() state.Index {
	return state.Index{
		Version: state.SchemaVersion,
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 123456789, time.UTC),
		Sessions: []state.Session{
			{
				Name:     "work",
				PortalID: "aB3xY9kZ",
				Environment: map[string]string{
					"LANG": "en_US.UTF-8",
					"TERM": "xterm-256color",
				},
				Windows: []state.Window{
					{
						Index:  0,
						Name:   "main",
						Layout: "b25f,200x50,0,0",
						Zoomed: false,
						Active: true,
						Panes: []state.Pane{
							{
								Index:          0,
								CWD:            "/Users/leeovery/Code/portal",
								Active:         true,
								CurrentCommand: "zsh",
								ScrollbackFile: "scrollback/work__0.0.bin",
							},
							{
								Index:          1,
								CWD:            "/tmp",
								Active:         false,
								CurrentCommand: "vim",
								ScrollbackFile: "scrollback/work__0.1.bin",
							},
						},
					},
				},
			},
			{
				Name:        "play",
				Environment: map[string]string{"FOO": "bar"},
				Windows: []state.Window{
					{
						Index:  1,
						Name:   "shell",
						Layout: "abcd,80x24,0,0",
						Zoomed: true,
						Active: true,
						Panes: []state.Pane{
							{
								Index:          0,
								CWD:            "/home/leeovery",
								Active:         true,
								CurrentCommand: "bash",
								ScrollbackFile: "scrollback/play__1.0.bin",
							},
						},
					},
				},
			},
		},
	}
}

func TestEncodeDecodeIndex_RoundTripsFullyPopulatedIndex(t *testing.T) {
	original := fullyPopulatedIndex()

	data, err := state.EncodeIndex(original)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, original) {
		t.Errorf("round-trip mismatch:\n got:  %#v\n want: %#v", got, original)
	}
}

func TestEncodeDecodeIndex_RoundTripsNonEmptyPortalID(t *testing.T) {
	original := state.Index{
		Version: state.SchemaVersion,
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name:        "work",
				PortalID:    "aB3xY9kZ",
				Environment: map[string]string{},
				Windows:     []state.Window{},
			},
		},
	}

	data, err := state.EncodeIndex(original)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}
	if !bytes.Contains(data, []byte(`"portal_id": "aB3xY9kZ"`)) {
		t.Errorf("expected portal_id to serialise under its JSON tag; got:\n%s", data)
	}

	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("expected 1 session; got %d", len(got.Sessions))
	}
	if got.Sessions[0].PortalID != "aB3xY9kZ" {
		t.Errorf("PortalID not preserved: got %q; want %q", got.Sessions[0].PortalID, "aB3xY9kZ")
	}
}

func TestDecodeIndex_DecodesSessionsWithoutPortalIDToEmptyString(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [
    {
      "name": "work",
      "windows": [
        {
          "index": 0,
          "name": "main",
          "panes": [{"index": 0}]
        }
      ]
    }
  ]
}`)

	idx, err := state.DecodeIndex(raw)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if len(idx.Sessions) != 1 {
		t.Fatalf("expected 1 session; got %d", len(idx.Sessions))
	}
	if idx.Sessions[0].PortalID != "" {
		t.Errorf("expected empty PortalID for legacy payload; got %q", idx.Sessions[0].PortalID)
	}
}

func TestSchemaVersion_NotBumpedForAdditivePortalIDField(t *testing.T) {
	if state.SchemaVersion != 1 {
		t.Errorf("SchemaVersion must stay 1 for the additive portal_id field; got %d", state.SchemaVersion)
	}
}

func TestEncodeIndex_SerialisesEmptySessionsAsArrayNotNull(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	if !bytes.Contains(data, []byte(`"sessions": []`)) {
		t.Errorf("expected sessions to serialise as []; got:\n%s", data)
	}
	if bytes.Contains(data, []byte(`"sessions": null`)) {
		t.Errorf("sessions must not serialise as null; got:\n%s", data)
	}
}

func TestEncodeIndex_SerialisesEmptyEnvironmentAsObjectNotNull(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name: "work",
				// Environment intentionally nil
				Windows: []state.Window{
					{
						Index: 0,
						Name:  "main",
						Panes: []state.Pane{{Index: 0}},
					},
				},
			},
		},
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	if !bytes.Contains(data, []byte(`"environment": {}`)) {
		t.Errorf("expected environment to serialise as {}; got:\n%s", data)
	}
	if bytes.Contains(data, []byte(`"environment": null`)) {
		t.Errorf("environment must not serialise as null; got:\n%s", data)
	}
}

func TestEncodeIndex_SerialisesEmptyPanesAsArrayNotNull(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name: "work",
				Windows: []state.Window{
					{Index: 0, Name: "main"},
				},
			},
		},
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	if !bytes.Contains(data, []byte(`"panes": []`)) {
		t.Errorf("expected panes to serialise as []; got:\n%s", data)
	}
	if bytes.Contains(data, []byte(`"panes": null`)) {
		t.Errorf("panes must not serialise as null; got:\n%s", data)
	}
}

func TestEncodeIndex_AlwaysSetsVersionToOne(t *testing.T) {
	idx := state.Index{
		// Version intentionally zero
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	v, ok := raw["version"]
	if !ok {
		t.Fatalf("version field missing; got:\n%s", data)
	}
	num, ok := v.(float64)
	if !ok {
		t.Fatalf("version is not a number; got %T (%v)", v, v)
	}
	if int(num) != 1 {
		t.Errorf("expected version 1; got %v", num)
	}
}

func TestEncodeIndex_SerialisesSavedAtAsRFC3339UTC(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	if !bytes.Contains(data, []byte(`"saved_at": "2026-04-17T10:30:00Z"`)) {
		t.Errorf("expected RFC 3339 UTC saved_at; got:\n%s", data)
	}
}

func TestEncodeIndex_UsesTwoSpaceIndent(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	// First indented line should start with exactly two spaces.
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected indented output; got:\n%s", data)
	}
	if !strings.HasPrefix(lines[1], "  ") || strings.HasPrefix(lines[1], "   ") {
		t.Errorf("expected 2-space indent on line 2; got %q", lines[1])
	}
}

func TestDecodeIndex_SilentlyIgnoresUnknownFields(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [],
  "future_field": "ignored",
  "another": {"nested": 42}
}`)

	idx, err := state.DecodeIndex(raw)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if idx.Version != 1 {
		t.Errorf("expected version 1; got %d", idx.Version)
	}
}

func TestDecodeIndex_DecodesMissingOptionalFieldsAsZeroValues(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [
    {
      "name": "work",
      "windows": [
        {
          "index": 0,
          "name": "main",
          "panes": [{"index": 0}]
        }
      ]
    }
  ]
}`)

	idx, err := state.DecodeIndex(raw)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if len(idx.Sessions) != 1 {
		t.Fatalf("expected 1 session; got %d", len(idx.Sessions))
	}
	s := idx.Sessions[0]
	if len(s.Environment) != 0 {
		t.Errorf("expected zero-value environment; got %#v", s.Environment)
	}
	w := s.Windows[0]
	if w.Layout != "" {
		t.Errorf("expected zero-value layout; got %q", w.Layout)
	}
	if w.Zoomed {
		t.Errorf("expected zero-value zoomed=false; got true")
	}
	if w.Active {
		t.Errorf("expected zero-value active=false; got true")
	}
	p := w.Panes[0]
	if p.CWD != "" || p.Active || p.CurrentCommand != "" || p.ScrollbackFile != "" {
		t.Errorf("expected zero-value pane fields; got %#v", p)
	}
}

func TestDecodeIndex_ReturnsErrorWhenVersionIsZero(t *testing.T) {
	raw := []byte(`{
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`)

	_, err := state.DecodeIndex(raw)
	if err == nil {
		t.Fatalf("expected error for missing version; got nil")
	}
	if !strings.Contains(err.Error(), "missing version") {
		t.Errorf("expected error to mention missing version; got %v", err)
	}
}

func TestDecodeIndex_ReturnsErrorWhenVersionUnsupported(t *testing.T) {
	raw := []byte(`{
  "version": 99,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": []
}`)

	_, err := state.DecodeIndex(raw)
	if err == nil {
		t.Fatalf("expected error for unsupported version; got nil")
	}
	if !strings.Contains(err.Error(), "unsupported sessions.json version") {
		t.Errorf("expected error to mention unsupported version; got %v", err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("expected error to mention the offending version number; got %v", err)
	}
}

func TestDecodeIndex_ReturnsWrappedErrorForMalformedJSON(t *testing.T) {
	raw := []byte(`{not json`)
	_, err := state.DecodeIndex(raw)
	if err == nil {
		t.Fatalf("expected error for malformed JSON; got nil")
	}
	if !strings.Contains(err.Error(), "decode sessions.json") {
		t.Errorf("expected wrapped decode error; got %v", err)
	}
}

func TestEncodeDecodeIndex_PreservesNanosecondPrecision(t *testing.T) {
	original := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 123456789, time.UTC),
	}

	data, err := state.EncodeIndex(original)
	if err != nil {
		t.Fatalf("EncodeIndex: unexpected error: %v", err)
	}

	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}

	if !got.SavedAt.Equal(original.SavedAt) {
		t.Errorf("saved_at not preserved: got %v; want %v", got.SavedAt, original.SavedAt)
	}
	if got.SavedAt.Nanosecond() != 123456789 {
		t.Errorf("nanosecond precision lost: got %d; want 123456789", got.SavedAt.Nanosecond())
	}
}

func TestCanonicalize_ReplacesNilSlicesAndMaps(t *testing.T) {
	idx := state.Index{
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 0, time.UTC),
		Sessions: []state.Session{
			{
				Name: "work",
				// Environment nil, Windows nil
			},
			{
				Name: "play",
				Windows: []state.Window{
					{Index: 0, Name: "main"}, // Panes nil
				},
				// Environment nil
			},
		},
	}

	idx.Canonicalize()

	if idx.Version != state.SchemaVersion {
		t.Errorf("expected Canonicalize to set Version=%d; got %d", state.SchemaVersion, idx.Version)
	}
	for i, s := range idx.Sessions {
		if s.Environment == nil {
			t.Errorf("session[%d].Environment still nil", i)
		}
		if s.Windows == nil {
			t.Errorf("session[%d].Windows still nil", i)
		}
		for j, w := range s.Windows {
			if w.Panes == nil {
				t.Errorf("session[%d].windows[%d].Panes still nil", i, j)
			}
		}
	}
}

func TestCanonicalize_NilSessionsBecomesEmpty(t *testing.T) {
	idx := state.Index{}
	idx.Canonicalize()
	if idx.Sessions == nil {
		t.Errorf("expected Sessions to be non-nil after Canonicalize")
	}
	if len(idx.Sessions) != 0 {
		t.Errorf("expected Sessions length 0; got %d", len(idx.Sessions))
	}
}
