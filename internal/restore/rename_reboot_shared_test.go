//go:build integration

// Shared harness leaves for the rename→reboot integration cluster
// (rename_reboot_hook / rename_reboot_durability / multipane_legacy). These
// fixture consts and assertion helpers are used by all three files;
// consolidating them here makes ownership explicit rather than leaving it
// implicit in whichever sibling test the analysis cycles happened to settle
// them in. (restoreWithMarker — the other cross-file leaf — lives in the
// general integration_test.go.)

package restore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/state"
)

// renameRebootFireConsts pin the fixture identity used by every subtest.
const (
	// renamePortalID is the immutable @portal-id stamped on the session. The
	// whole point of the fix is that the hook key derives from THIS token,
	// not the session name, so a rename leaves the key untouched.
	renamePortalID = "tok123"
	// renameOldName / renameNewName are the pre- and post-rename session
	// names. Only #{session_name} changes; @portal-id stays renamePortalID.
	renameOldName = "renamesrc"
	renameNewName = "renamedst"
)

// findCapturedSession returns the captured Session with the given name, or
// fatals with the captured names for diagnostics. It is the non-vacuous guard
// that the round-trip actually captured the post-rename session.
func findCapturedSession(t *testing.T, idx state.Index, name string) state.Session {
	t.Helper()
	var names []string
	for _, s := range idx.Sessions {
		if s.Name == name {
			return s
		}
		names = append(names, s.Name)
	}
	t.Fatalf("captured index has no session %q; captured names=%v", name, names)
	return state.Session{}
}

// verifyHookKeyed asserts hooks.json contains an on-resume entry under the
// exact stable id-key — proving registration stored under the rename-immune
// key rather than the mutable name. Reads via the same store the production
// path uses.
func verifyHookKeyed(t *testing.T, hooksPath, wantKey string) {
	t.Helper()
	events, err := hooks.NewStore(hooksPath).Get(wantKey)
	if err != nil {
		t.Fatalf("hooks.Get(%q): %v", wantKey, err)
	}
	if _, ok := events["on-resume"]; !ok {
		t.Fatalf("hooks.json missing on-resume entry under stable key %q; got events=%v", wantKey, events)
	}
}

// persistIndex writes idx to sessions.json via the canonical encoder so the
// on-disk schema matches what CaptureStructure produced.
func persistIndex(t *testing.T, idx state.Index, stateDir string) {
	t.Helper()
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}
}

// seedScrollback writes the pane's on-disk scrollback fixture — the bytes the
// hydrate helper later dumps. Seeded fresh per cycle so each restore replays a
// deterministic buffer regardless of the previous cycle's helper run.
func seedScrollback(t *testing.T, stateDir, name string) {
	t.Helper()
	scrollbackKey := state.SanitizePaneKey(name, 0, 0)
	scrollbackPath := state.ScrollbackFile(stateDir, scrollbackKey)
	if err := os.MkdirAll(filepath.Dir(scrollbackPath), 0o700); err != nil {
		t.Fatalf("mkdir scrollback dir: %v", err)
	}
	if err := os.WriteFile(scrollbackPath, []byte("\x1b[31mred\x1b[0m\nbefore reboot\n"), 0o600); err != nil {
		t.Fatalf("write fixture scrollback: %v", err)
	}
}

// assertHookFireCount asserts the on-resume hook has fired exactly `want` times
// cumulatively — the hook command appends one HOOK_FIRED marker per firing, so
// the marker count is the cumulative firing count across reboot cycles. Zero /
// short of `want` means a bare-shell miss on the most recent cycle (the chain-
// (a) regression); more than `want` means the helper's exec $SHELL branch did
// not replace the helper.
func assertHookFireCount(t *testing.T, hookFireFile string, want int) {
	t.Helper()
	data, err := os.ReadFile(hookFireFile)
	if err != nil {
		t.Fatalf("read hook fire file %s (bare-shell miss leaves it absent): %v", hookFireFile, err)
	}
	got := strings.Count(string(data), "HOOK_FIRED")
	if got != want {
		t.Errorf("hook fired %d times cumulatively; want exactly %d\nfile contents:\n%s", got, want, data)
	}
}
