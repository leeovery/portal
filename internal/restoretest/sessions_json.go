package restoretest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// SeedSessionsJSON writes a minimal sessions.json containing one
// single-window/single-pane session per supplied name. The pane's
// ScrollbackFile points at a placeholder path under
// stateDir/scrollback/ — the file does not have to exist for skeleton
// restoration to succeed (Restore only reads the index; the in-pane
// hydrate helper reads the file).
//
// SavedAt is left zero — callers that need to assert SavedAt is
// preserved across a Run window should use SeedSessionsJSONWithSavedAt
// to plant a known-good timestamp.
//
// The encoded shape is byte-identical to the seven inline blocks this
// helper replaced (cmd/reattach_integration_test.go and the six
// cmd/bootstrap/*_integration_test.go sites) — single window at
// Index 0, Layout "tiled", Active true; single pane at Index 0, Active
// true, ScrollbackFile "scrollback/<name>-w0-p0.bin".
func SeedSessionsJSON(t *testing.T, stateDir string, names ...string) {
	t.Helper()
	SeedSessionsJSONWithSavedAt(t, stateDir, time.Time{}, names...)
}

// SeedSessionsJSONWithSavedAt is the savedAt-aware variant of
// SeedSessionsJSON. The supplied savedAt is encoded verbatim into the
// Index so the caller can capture it pre-Run and assert that it is not
// advanced by anything in the orchestrator's pipeline (the suppression
// invariant from spec "Save-Side Architecture → Triggers & Serialization
// → Properties → Restoration guard").
func SeedSessionsJSONWithSavedAt(t *testing.T, stateDir string, savedAt time.Time, names ...string) {
	t.Helper()
	sessions := make([]state.Session, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, state.Session{
			Name: name,
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: filepath.Join("scrollback", name+"-w0-p0.bin"),
				}},
			}},
		})
	}
	idx := state.Index{SavedAt: savedAt, Sessions: sessions}
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}
