package legacyindex_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/legacyindex"
)

// fullyPopulatedJSON is a sessions.json as the tmux version wrote it, each
// window's layout stored as Portal's JSON-in-a-string geometry.
const fullyPopulatedJSON = `{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00.123456789Z",
  "sessions": [
    {
      "name": "work",
      "environment": {
        "LANG": "en_US.UTF-8",
        "TERM": "xterm-256color"
      },
      "windows": [
        {
          "index": 0,
          "name": "main",
          "layout": "{\"V\":2,\"L\":{\"t\":\"h\",\"w\":200,\"h\":50,\"x\":0,\"y\":0,\"c\":[{\"t\":\"p\",\"w\":100,\"h\":50,\"x\":0,\"y\":0,\"l\":0,\"i\":0,\"I\":\"%1\"},{\"t\":\"p\",\"w\":99,\"h\":50,\"x\":101,\"y\":0,\"l\":0,\"i\":1,\"I\":\"%2\"}]}}",
          "zoomed": false,
          "active": true,
          "panes": [
            {
              "index": 0,
              "cwd": "/Users/leeovery/Code/portal",
              "active": true,
              "current_command": "zsh",
              "scrollback_file": "scrollback/work__0.0.bin",
              "portal_pane_id": "aB3xY9kZ"
            },
            {
              "index": 1,
              "cwd": "/tmp",
              "active": false,
              "current_command": "vim",
              "scrollback_file": "scrollback/work__0.1.bin",
              "portal_pane_id": ""
            }
          ]
        }
      ]
    },
    {
      "name": "play",
      "environment": {
        "FOO": "bar"
      },
      "windows": [
        {
          "index": 1,
          "name": "shell",
          "layout": "{\"V\":2,\"L\":{\"t\":\"p\",\"w\":80,\"h\":24,\"x\":0,\"y\":0,\"l\":0,\"i\":0,\"I\":\"%3\"}}",
          "zoomed": true,
          "active": true,
          "panes": [
            {
              "index": 0,
              "cwd": "/home/leeovery",
              "active": true,
              "current_command": "bash",
              "scrollback_file": "scrollback/play__1.0.bin",
              "portal_pane_id": ""
            }
          ]
        }
      ]
    }
  ]
}`

func fullyPopulatedIndex() legacyindex.Index {
	return legacyindex.Index{
		Version: legacyindex.SchemaVersion,
		SavedAt: time.Date(2026, 4, 17, 10, 30, 0, 123456789, time.UTC),
		Sessions: []legacyindex.Session{
			{
				Name: "work",
				Environment: map[string]string{
					"LANG": "en_US.UTF-8",
					"TERM": "xterm-256color",
				},
				Windows: []legacyindex.Window{
					{
						Index:  0,
						Name:   "main",
						Layout: `{"V":2,"L":{"t":"h","w":200,"h":50,"x":0,"y":0,"c":[{"t":"p","w":100,"h":50,"x":0,"y":0,"l":0,"i":0,"I":"%1"},{"t":"p","w":99,"h":50,"x":101,"y":0,"l":0,"i":1,"I":"%2"}]}}`,
						Zoomed: false,
						Active: true,
						Panes: []legacyindex.Pane{
							{
								Index:          0,
								CWD:            "/Users/leeovery/Code/portal",
								Active:         true,
								CurrentCommand: "zsh",
								ScrollbackFile: "scrollback/work__0.0.bin",
								PortalPaneID:   "aB3xY9kZ",
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
				Windows: []legacyindex.Window{
					{
						Index:  1,
						Name:   "shell",
						Layout: `{"V":2,"L":{"t":"p","w":80,"h":24,"x":0,"y":0,"l":0,"i":0,"I":"%3"}}`,
						Zoomed: true,
						Active: true,
						Panes: []legacyindex.Pane{
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

func TestDecodeIndex_DecodesEveryFieldTheTmuxVersionWrote(t *testing.T) {
	got, err := legacyindex.DecodeIndex([]byte(fullyPopulatedJSON))
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}

	if want := fullyPopulatedIndex(); !reflect.DeepEqual(got, want) {
		t.Errorf("decode mismatch:\n got:  %#v\n want: %#v", got, want)
	}
}

func TestDecodeIndex_PreservesNanosecondPrecision(t *testing.T) {
	got, err := legacyindex.DecodeIndex([]byte(fullyPopulatedJSON))
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if got.SavedAt.Nanosecond() != 123456789 {
		t.Errorf("nanosecond precision lost: got %d; want 123456789", got.SavedAt.Nanosecond())
	}
}

func TestDecodeIndex_DecodesPreUpgradePayloadToEmptyPortalPaneID(t *testing.T) {
	raw := []byte(`{
  "version": 1,
  "saved_at": "2026-04-17T10:30:00Z",
  "sessions": [
    {
      "name": "work",
      "portal_id": "aB3xY9kZ",
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

	idx, err := legacyindex.DecodeIndex(raw)
	if err != nil {
		t.Fatalf("DecodeIndex: unexpected error: %v", err)
	}
	if idx.Version != legacyindex.SchemaVersion {
		t.Errorf("Version = %d; want %d", idx.Version, legacyindex.SchemaVersion)
	}
	if len(idx.Sessions) != 1 {
		t.Fatalf("expected 1 session; got %d", len(idx.Sessions))
	}
	if got := idx.Sessions[0].Windows[0].Panes[0].PortalPaneID; got != "" {
		t.Errorf("expected empty PortalPaneID for pre-upgrade payload; got %q", got)
	}
}

func TestSchemaVersion_IsOne(t *testing.T) {
	if legacyindex.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d; the tmux version's sessions.json is version 1", legacyindex.SchemaVersion)
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

	idx, err := legacyindex.DecodeIndex(raw)
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

	idx, err := legacyindex.DecodeIndex(raw)
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

	_, err := legacyindex.DecodeIndex(raw)
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

	_, err := legacyindex.DecodeIndex(raw)
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
	_, err := legacyindex.DecodeIndex(raw)
	if err == nil {
		t.Fatalf("expected error for malformed JSON; got nil")
	}
	if !strings.Contains(err.Error(), "decode sessions.json") {
		t.Errorf("expected wrapped decode error; got %v", err)
	}
}
