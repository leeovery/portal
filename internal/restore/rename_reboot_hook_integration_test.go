//go:build integration

// Phase 3 task 3-5 — the headline rename-gap coverage.
//
// This file proves the fix end-to-end for the bug in the spec's Problem
// Statement: a session with a registered resume hook, renamed WHILE its pane
// process keeps running, must still fire that hook after the next reboot —
// instead of silently coming back as a bare $SHELL.
//
// It is the ONLY test that exercises the full rename → capture → restore →
// FIRE chain. The existing reboot round-trips
// (cmd/bootstrap/reboot_roundtrip_test.go, internal/restore/
// integration_full_test.go) assert hook firing WITHOUT a rename, so a
// regression in the rename-immune @portal-id keying would slip past them.
//
// Both rename triggers the spec calls out are covered as sibling subtests
// sharing one setup+assert body (renameRebootFireCase):
//
//   - External `tmux rename-session` (not interceptable by Portal) — driven
//     via a raw `ts.Run(t, "rename-session", ...)`.
//   - Portal's in-TUI rename (renameAndRefresh) — driven via the exact
//     byte-equivalent production call the in-TUI path issues,
//     client.RenameSession(old, new). renameAndRefresh
//     (internal/tui/model.go) is an UNEXPORTED Model method and driving it
//     via tui.New/Update is out of scope for a restore-package integration
//     test; it reduces to m.sessionRenamer.RenameSession(old, new) + a list
//     refresh with ZERO hook re-keying (proven by the pure-Go tui_test
//     subtest "it reduces the in-TUI rename path to a single RenameSession
//     with no hook re-keying" in internal/tui/model_test.go), so exercising
//     RenameSession here is the byte-identical restore-side coverage.
//
// The fix's central invariant is that the hook key is derived from the
// immutable @portal-id, not the mutable session name. So the pane process is
// deliberately KEPT RUNNING across the rename (never killed/respawned) — the
// bug bites ONLY here, because a restarting inner process would let the
// out-of-repo start-hook re-register under the new name and self-heal.
//
// Build & run:
//
//	go test -tags=integration ./internal/restore/... -run TestRenameRebootHook
//	go test -short -tags=integration ./internal/restore/...   # skips this

package restore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
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

// TestRenameRebootHook_ExternalRename covers the external `tmux
// rename-session` trigger: Portal cannot intercept it, so the fix must rest
// entirely on @portal-id being name-independent. The rename runs as a raw
// tmux command against the live pane while its `sleep infinity` process keeps
// running.
//
// Spec: Acceptance Criteria 2 ("Rename survives reboot" — external
// rename-session); Testing Requirements → "The rename gap".
func TestRenameRebootHook_ExternalRename(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRenameRebootFire(t, func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client) {
		t.Helper()
		// External trigger: bare tmux rename-session against the live pane.
		// Portal never sees this call — the fix must hold purely because
		// @portal-id is unchanged by the rename.
		ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)
	})
}

// TestRenameRebootHook_RenameSessionEquivalent covers Portal's in-TUI rename
// trigger via the byte-equivalent production call the in-TUI path issues:
// client.RenameSession(old, new). renameAndRefresh (internal/tui/model.go)
// reduces to exactly m.sessionRenamer.RenameSession(old, new) + a list
// refresh with ZERO hook re-keying — the tui model wires no hook seam at all
// (see the pure-Go tui_test subtest cited in the file header). Driving the
// unexported renameAndRefresh via tui.New/Update is out of scope for a
// restore-package integration test, so this exercises the identical
// client.RenameSession leg the in-TUI path bottoms out in.
//
// Spec: Acceptance Criteria 2 ("Rename survives reboot" — in-TUI rename
// modal) and 6 ("No external/UI change" — the in-TUI path needs no hook
// re-keying); Scope & Non-Goals → "Both rename triggers fixed at the root".
func TestRenameRebootHook_RenameSessionEquivalent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRenameRebootFire(t, func(t *testing.T, _ *tmuxtest.Socket, client *tmux.Client) {
		t.Helper()
		// In-TUI-equivalent trigger. renameAndRefresh's ONLY tmux effect is
		// this single RenameSession(old, new) (model.go: `if err :=
		// m.sessionRenamer.RenameSession(oldName, newName); err != nil` then
		// a ListSessions refresh) — no hook lookup, no hook re-keying.
		if err := client.RenameSession(renameOldName, renameNewName); err != nil {
			t.Fatalf("RenameSession(%q, %q): %v", renameOldName, renameNewName, err)
		}
	})
}

// TestRenameRebootHook_PaneProcessKeptRunning is the explicit guard for the
// edge case that makes this bug real: the inner pane process is NOT restarted
// across the rename. If it restarted, the out-of-repo start-hook would
// re-register the hook under the new name and mask the defect (the spec's
// self-heal path). This subtest asserts the pre-rename pane_pid equals the
// post-rename pane_pid, then runs the full fire assertion — proving the hook
// fires despite no re-registration.
//
// Spec: Problem Statement ("bites only when the inner pane process does not
// restart"); Edge Cases.
func TestRenameRebootHook_PaneProcessKeptRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRenameRebootFire(t, func(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client) {
		t.Helper()
		// Record the live pane's PID BEFORE the rename.
		pidBefore := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", tmux.PaneTarget(renameOldName, 0, 0), "#{pane_pid}"))

		// Rename via the same bare RenameSession the in-TUI path issues —
		// no kill, no respawn, no re-registration.
		if err := client.RenameSession(renameOldName, renameNewName); err != nil {
			t.Fatalf("RenameSession(%q, %q): %v", renameOldName, renameNewName, err)
		}

		// The SAME process must still own the pane under the new name. A
		// changed PID would mean the pane restarted (and the bug would
		// self-heal via the external start-hook), invalidating the test.
		pidAfter := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", tmux.PaneTarget(renameNewName, 0, 0), "#{pane_pid}"))
		if pidBefore == "" || pidAfter == "" {
			t.Fatalf("pane_pid read empty (before=%q after=%q); pane addressing broke", pidBefore, pidAfter)
		}
		if pidBefore != pidAfter {
			t.Fatalf("pane process restarted across rename (pid %q → %q); "+
				"the bug only bites when the process keeps running", pidBefore, pidAfter)
		}
	})
}

// runRenameRebootFire is the shared setup+assert body for the three subtests.
// It stamps a session with @portal-id, registers an on-resume hook under the
// STABLE id-key (tok123:0.0), invokes the caller-supplied rename trigger
// while the pane process keeps running, then drives the full save → kill →
// restore → signal-hydrate cycle and asserts the hook fired exactly once
// under the POST-rename name — not a bare $SHELL.
//
// The harness mirrors internal/restore/integration_full_test.go's
// save→kill→restore→DriveSignalHydrate sequence (Orchestrator +
// restoreWithMarker so skeleton markers are applied, which DriveSignalHydrate
// requires). It differs only in the deliberate rename step and the hook-fire
// assertion.
func runRenameRebootFire(t *testing.T, rename func(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client)) {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	// Isolated baseline env. IsolateStateForTest points PORTAL_HOOKS_FILE at
	// a /nonexistent sentinel and scopes XDG_CONFIG_HOME away from the
	// developer's real config; the concrete PORTAL_STATE_DIR /
	// PORTAL_HOOKS_FILE overrides below (t.TempDir paths) shadow the
	// sentinel via last-write-wins so setup does not touch a read-only FS.
	portaltest.IsolateStateForTest(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Concrete hooks.json under a real temp dir — overrides the /nonexistent
	// sentinel IsolateStateForTest set, so both the in-process hooks.NewStore
	// below and the in-pane hydrate helper read the same file.
	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Hook side-effect file. The on-resume hook appends a line on each
	// firing; assertHookFireCount(t, file, 1) asserts exactly one line — zero
	// means the hook orphaned (bare-shell miss), more than one means the
	// helper's exec $SHELL didn't replace the helper.
	hookFireFile := filepath.Join(t.TempDir(), "hook-fired.txt")
	hookCmd := "echo HOOK_FIRED >> " + hookFireFile

	// Register the on-resume hook under the STABLE id-key. HookKey with a
	// non-empty portalID yields "<id>:w.p" (rename-immune), so this is
	// exactly "tok123:0.0" — independent of the session name. Registration
	// mirrors what `portal hooks set` stores (via=cli).
	stableKey := tmux.HookKey(renamePortalID, renameOldName, 0, 0)
	if stableKey != renamePortalID+":0.0" {
		t.Fatalf("stable hook key = %q; want %q (id-key must not embed the name)",
			stableKey, renamePortalID+":0.0")
	}
	store := hooks.NewStore(hooksPath)
	if err := store.Set(stableKey, "on-resume", hookCmd, "cli"); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-3-5-")
	client := ts.Client()

	// Create the session with a single window / single pane. `sleep
	// infinity` keeps the pane process alive across the rename without
	// producing scrollback noise (the scrollback bytes are seeded on disk
	// after capture, so the live pane content is irrelevant).
	cwd := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", renameOldName, "-c", cwd, "sleep", "infinity")
	ts.WaitForSession(t, renameOldName, 2*time.Second)

	// Stamp the immutable @portal-id — mirrors creation-time stamping
	// (internal/session's CreateFromDir / QuickStart). This is the anchor
	// the hook key derives from.
	if err := client.SetSessionOption(renameOldName, session.PortalIDOption, renamePortalID); err != nil {
		t.Fatalf("SetSessionOption %s=%s: %v", session.PortalIDOption, renamePortalID, err)
	}

	// TRIGGER: rename while the pane process keeps running. @portal-id is
	// unchanged; #{session_name} becomes renameNewName. No kill, no respawn,
	// no hook re-registration — the whole point of the fix.
	rename(t, ts, client)

	// Sanity: the rename took effect (old name gone, new name live) and the
	// @portal-id survived the rename unchanged.
	if _, err := ts.TryRun("has-session", "-t", "="+renameNewName); err != nil {
		t.Fatalf("session %q not live after rename: %v", renameNewName, err)
	}
	liveID := strings.TrimSpace(ts.Run(t, "show-options", "-t", renameNewName,
		"-v", session.PortalIDOption))
	if liveID != renamePortalID {
		t.Fatalf("@portal-id after rename = %q; want %q (must be rename-immune)", liveID, renamePortalID)
	}

	// Capture the post-rename state. Per the Task 3-2 change, CaptureStructure
	// reads #{@portal-id} into Session.PortalID, so the snapshot records
	// PortalID == "tok123" under the NEW name.
	idx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	// NON-VACUOUS guard: the captured snapshot MUST record the id under the
	// post-rename name, or the round-trip would silently degrade to the
	// name-fallback (bare shell) path and still "pass" for the wrong reason.
	sess := findCapturedSession(t, idx, renameNewName)
	if sess.PortalID != renamePortalID {
		t.Fatalf("captured session %q PortalID = %q; want %q (id must persist under the post-rename name)",
			renameNewName, sess.PortalID, renamePortalID)
	}
	// And the on-disk hook entry is keyed by the stable id-key, not the name.
	verifyHookKeyed(t, hooksPath, stableKey)

	// Seed the pane's scrollback file AFTER capture but BEFORE persist — the
	// on-disk file the hydrate helper later dumps is what we control here.
	scrollbackKey := state.SanitizePaneKey(renameNewName, 0, 0)
	scrollbackPath := state.ScrollbackFile(stateDir, scrollbackKey)
	if err := os.MkdirAll(filepath.Dir(scrollbackPath), 0o700); err != nil {
		t.Fatalf("mkdir scrollback dir: %v", err)
	}
	if err := os.WriteFile(scrollbackPath, []byte("\x1b[31mred\x1b[0m\nbefore reboot\n"), 0o600); err != nil {
		t.Fatalf("write fixture scrollback: %v", err)
	}

	// Persist sessions.json via the canonical writer so the on-disk schema
	// matches what CaptureStructure produced.
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// Kill the server so Restore runs against a fresh one. The list-sessions
	// error confirms the kill took effect — a still-live server would mask a
	// Restore that did nothing.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	logger := restoretest.OpenTestLogger(t, stateDir)
	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	// The restored session comes back under its POST-rename name.
	restoredPanes := ts.Run(t, "list-panes", "-s", "-t", renameNewName,
		"-F", "#{window_index}:#{pane_index}")
	if !strings.Contains(restoredPanes, "0:0") {
		t.Fatalf("restored session %q missing live pane 0:0; got %q", renameNewName, restoredPanes)
	}

	// Drive signal-hydrate on the restored pane — the byte-equivalent of
	// `portal state signal-hydrate`. The in-pane helper then dumps
	// scrollback, unsets its marker, looks up hooks.json by the baked
	// --hook-key (the stable id-key), and fires the on-resume hook before
	// exec'ing $SHELL.
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{renameNewName})

	// Wait for the helper to finish (marker cleared = it reached the
	// hook-or-shell exec step).
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	// HEADLINE ASSERTION: the hook fired exactly once. A bare-shell miss
	// (the pre-fix bug) leaves the file empty/absent.
	assertHookFireCount(t, hookFireFile, 1)
}

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
