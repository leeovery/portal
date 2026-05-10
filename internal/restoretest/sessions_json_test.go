package restoretest_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
)

// TestSeedSessionsJSON_BytesEquivalentToInlineBlock asserts that the
// shared seed helper produces sessions.json bytes byte-identical to the
// inline blocks it replaces across cmd/reattach_integration_test.go and
// the six cmd/bootstrap/*_integration_test.go sites. Drift here would
// silently change the structural fixture every replaced test depends on.
func TestSeedSessionsJSON_BytesEquivalentToInlineBlock(t *testing.T) {
	stateDir := t.TempDir()

	names := []string{"alpha", "beta"}
	restoretest.SeedSessionsJSON(t, stateDir, names...)

	got, err := os.ReadFile(state.SessionsJSON(stateDir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}

	// Reproduce the inline shape verbatim from
	// cmd/bootstrap/eager_signal_hydrate_integration_test.go:174-191
	// (the canonical zero-savedAt site).
	idx := state.Index{
		Sessions: make([]state.Session, 0, len(names)),
	}
	for _, name := range names {
		idx.Sessions = append(idx.Sessions, state.Session{
			Name: name,
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: "scrollback/" + name + "-w0-p0.bin",
				}},
			}},
		})
	}
	want, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("sessions.json bytes diverged from inline shape\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestSeedSessionsJSONWithSavedAt_PreservesSavedAt asserts the
// savedAt-aware variant encodes the supplied timestamp verbatim. This
// is the contract phase5_marker_suppression_integration_test.go relies
// on for its non-vacuous suppression check.
func TestSeedSessionsJSONWithSavedAt_PreservesSavedAt(t *testing.T) {
	stateDir := t.TempDir()
	savedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	restoretest.SeedSessionsJSONWithSavedAt(t, stateDir, savedAt, "probe-target")

	data, err := os.ReadFile(state.SessionsJSON(stateDir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	got, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("DecodeIndex: %v", err)
	}
	if !got.SavedAt.Equal(savedAt) {
		t.Errorf("SavedAt = %v; want %v", got.SavedAt, savedAt)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Name != "probe-target" {
		t.Errorf("sessions = %+v; want single probe-target", got.Sessions)
	}
}

// TestSeedSessionsJSON_WritesAtCanonicalPath asserts the helper writes
// at state.SessionsJSON(stateDir) — the same path Restore reads from.
// A drift here would make the helper appear to succeed while leaving
// every consumer test reading an empty index from the canonical path.
func TestSeedSessionsJSON_WritesAtCanonicalPath(t *testing.T) {
	stateDir := t.TempDir()
	restoretest.SeedSessionsJSON(t, stateDir, "solo")

	if _, err := os.Stat(state.SessionsJSON(stateDir)); err != nil {
		t.Fatalf("expected sessions.json at %s: %v", state.SessionsJSON(stateDir), err)
	}
	// Helper must NOT create an unrelated path under stateDir.
	stray := filepath.Join(stateDir, "scrollback", "solo-w0-p0.bin")
	if _, err := os.Stat(stray); err == nil {
		t.Errorf("helper unexpectedly created scrollback file at %s; should only write sessions.json", stray)
	}
}
