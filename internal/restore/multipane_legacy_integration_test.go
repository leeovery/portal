//go:build integration

// Phase 3 task 3-7 — Multi-pane correct-pane routing + graceful legacy
// degradation.
//
// Two acceptance boundaries the task-3-5 / task-3-6 single-pane, stamped-only
// harness leaves unproven end-to-end:
//
//   - MULTI-PANE (Acceptance Criteria 7). Per-pane on-resume hooks under ONE
//     stamped session must stay independently addressable across rename+restore
//     and fire on the CORRECT pane. The hook key is
//     tmux.HookKey(id, name, w, p) → "<id>:w.p", so two panes under one id
//     (tok123:0.0, tok123:0.1) carry DISTINCT w.p suffixes. A bug keying hooks
//     off the session id alone (dropping the w.p suffix) would cross-fire —
//     both panes running the same hook. This file proves the :w.p suffix is
//     load-bearing: pane 0's hook writes ONLY hook-pane0.txt and pane 1's ONLY
//     hook-pane1.txt.
//
//   - GRACEFUL LEGACY (Acceptance Criteria 8). An un-stamped saved session
//     (no @portal-id) must degrade to the name-based key at EVERY stage —
//     capture yields PortalID == "" (Task 3-2), createSkeleton skips the
//     re-stamp on the empty id (Task 3-3), and the baked --hook-key falls back
//     to the name via tmux.HookKey("", name, w, p) → "<name>:w.p" (Task 1-1) —
//     without panicking on the empty id anywhere. The name fallback coincides
//     with the on-disk name key, so the legacy hook still fires with no
//     migration.
//
// It REUSES the task-3-5 / task-3-6 harness leaves (restoreWithMarker,
// findCapturedSession, verifyHookKeyed, persistIndex, seedScrollback,
// assertHookFireCount) and the renamePortalID/renameOldName/renameNewName
// fixture consts; it adds only the multi-pane split, the per-pane hook-file
// isolation assertion, and the un-stamped legacy leg.
//
// Build & run:
//
//	go test -tags=integration ./internal/restore/... -run MultiPaneLegacy
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

// legacyName is the un-stamped (no @portal-id) session name for the legacy
// leg. Its name-fallback hook key is legacyName+":0.0" — which equals the key
// already on disk, so the pre-fix, name-keyed hooks.json entry keeps matching
// with no migration.
const legacyName = "legacy-proj"

// TestMultiPaneLegacy_PerPaneHookRouting proves Acceptance Criteria 7:
// per-pane hooks under ONE stamped session stay independently addressable and
// fire on the CORRECT pane after rename+restore. Two panes under the shared
// @portal-id tok123 carry distinct w.p suffixes (tok123:0.0, tok123:0.1); each
// pane's hook writes to its OWN side-effect file, so a cross-fire (the shared
// id dropping the w.p suffix) would land pane 1's marker in pane 0's file or
// vice versa. The assertion is airtight: each file contains ONLY its own
// pane's marker and NOT the other pane's.
//
// Spec: Acceptance Criteria 7 ("Multi-pane"); Testing Requirements →
// "Multi-pane (integration)".
func TestMultiPaneLegacy_PerPaneHookRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	// Isolated baseline env — see the task-3-5 harness rationale: the concrete
	// PORTAL_STATE_DIR / PORTAL_HOOKS_FILE overrides below shadow the
	// /nonexistent sentinel via last-write-wins.
	portaltest.IsolateStateForTest(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Two DISTINCT side-effect files — one per pane. Each hook writes a marker
	// UNIQUE to its pane so a cross-fire is detectable: pane 0's marker in
	// hook-pane1.txt (or vice versa) means the shared id cross-fired.
	sideEffectDir := t.TempDir()
	pane0File := filepath.Join(sideEffectDir, "hook-pane0.txt")
	pane1File := filepath.Join(sideEffectDir, "hook-pane1.txt")
	const (
		pane0Marker = "PANE0_HOOK_FIRED"
		pane1Marker = "PANE1_HOOK_FIRED"
	)
	pane0Cmd := "echo " + pane0Marker + " >> " + pane0File
	pane1Cmd := "echo " + pane1Marker + " >> " + pane1File

	// Register two on-resume hooks under DISTINCT id-keys — the shared id with
	// distinct w.p suffixes. tok123:0.0 → pane 0, tok123:0.1 → pane 1.
	pane0Key := tmux.HookKey(renamePortalID, renameOldName, 0, 0)
	pane1Key := tmux.HookKey(renamePortalID, renameOldName, 0, 1)
	if pane0Key != renamePortalID+":0.0" {
		t.Fatalf("pane 0 hook key = %q; want %q", pane0Key, renamePortalID+":0.0")
	}
	if pane1Key != renamePortalID+":0.1" {
		t.Fatalf("pane 1 hook key = %q; want %q", pane1Key, renamePortalID+":0.1")
	}
	// NON-VACUOUS guard: the two keys must be DISTINCT before restore, or a
	// cross-fire could not be told apart from correct routing.
	if pane0Key == pane1Key {
		t.Fatalf("multi-pane hook keys are not distinct: both %q", pane0Key)
	}
	store := hooks.NewStore(hooksPath)
	if err := store.Set(pane0Key, "on-resume", pane0Cmd, "cli"); err != nil {
		t.Fatalf("hooks.Set pane 0: %v", err)
	}
	if err := store.Set(pane1Key, "on-resume", pane1Cmd, "cli"); err != nil {
		t.Fatalf("hooks.Set pane 1: %v", err)
	}
	// NON-VACUOUS: both keys are on disk under DISTINCT entries before restore.
	verifyHookKeyed(t, hooksPath, pane0Key)
	verifyHookKeyed(t, hooksPath, pane1Key)

	ts := tmuxtest.New(t, "ptl-3-7-mp-")
	client := ts.Client()

	// Create the session with ONE window and TWO panes. `sleep infinity` keeps
	// both pane processes alive across the rename without scrollback noise (the
	// scrollback bytes are seeded on disk after capture).
	cwd := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", renameOldName, "-c", cwd, "sleep", "infinity")
	ts.WaitForSession(t, renameOldName, 2*time.Second)
	// Second pane at 0.1 via split-window; keep its process alive too.
	ts.Run(t, "split-window", "-t", tmux.PaneTarget(renameOldName, 0, 0), "-c", cwd, "sleep", "infinity")

	// Sanity: exactly two panes, at 0:0 and 0:1.
	panesOut := strings.TrimSpace(ts.Run(t, "list-panes", "-s", "-t", renameOldName,
		"-F", "#{window_index}:#{pane_index}"))
	if !strings.Contains(panesOut, "0:0") || !strings.Contains(panesOut, "0:1") {
		t.Fatalf("expected panes 0:0 and 0:1 pre-rename; got %q", panesOut)
	}

	// Stamp the immutable @portal-id — mirrors creation-time stamping. The
	// option is session-scoped, so BOTH panes inherit tok123.
	if err := client.SetSessionOption(renameOldName, session.PortalIDOption, renamePortalID); err != nil {
		t.Fatalf("SetSessionOption %s=%s: %v", session.PortalIDOption, renamePortalID, err)
	}

	// TRIGGER: external rename while both pane processes keep running.
	ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)
	if _, err := ts.TryRun("has-session", "-t", "="+renameNewName); err != nil {
		t.Fatalf("session %q not live after rename: %v", renameNewName, err)
	}

	// Capture the post-rename state. BOTH pane rows carry @portal-id = tok123
	// (session-scoped), so Session.PortalID == tok123.
	idx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	sess := findCapturedSession(t, idx, renameNewName)
	if sess.PortalID != renamePortalID {
		t.Fatalf("captured session %q PortalID = %q; want %q", renameNewName, sess.PortalID, renamePortalID)
	}
	// NON-VACUOUS: the capture recorded TWO panes under the one window.
	if len(sess.Windows) != 1 || len(sess.Windows[0].Panes) != 2 {
		t.Fatalf("captured session %q topology = %d window(s) / %v panes; want 1 window / 2 panes",
			renameNewName, len(sess.Windows), paneIndices(sess))
	}
	// And BOTH on-disk hook entries are keyed by their distinct id-keys.
	verifyHookKeyed(t, hooksPath, pane0Key)
	verifyHookKeyed(t, hooksPath, pane1Key)

	// Seed BOTH panes' scrollback fixtures AFTER capture, BEFORE persist.
	seedPaneScrollback(t, stateDir, renameNewName, 0, 0)
	seedPaneScrollback(t, stateDir, renameNewName, 0, 1)

	persistIndex(t, idx, stateDir)

	// Reboot: kill → fresh server → restore.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	logger := restoretest.OpenTestLogger(t, stateDir)
	o := &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	// The restored session comes back under its POST-rename name with BOTH panes.
	restoredPanes := strings.TrimSpace(ts.Run(t, "list-panes", "-s", "-t", renameNewName,
		"-F", "#{window_index}:#{pane_index}"))
	if !strings.Contains(restoredPanes, "0:0") || !strings.Contains(restoredPanes, "0:1") {
		t.Fatalf("restored session %q missing panes 0:0/0:1; got %q", renameNewName, restoredPanes)
	}

	// Drive signal-hydrate for the session — this drives EVERY marked live pane
	// in the session (both 0:0 and 0:1), so each pane's helper fires its own
	// baked --hook-key.
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{renameNewName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	// HEADLINE ASSERTION — airtight per-pane routing, no cross-fire:
	//   pane 0's hook fired ONLY into hook-pane0.txt (its marker present there,
	//     pane 1's marker ABSENT).
	//   pane 1's hook fired ONLY into hook-pane1.txt (its marker present there,
	//     pane 0's marker ABSENT).
	assertMarkerFiredOnce(t, pane0File, pane0Marker)
	assertMarkerFiredOnce(t, pane1File, pane1Marker)
	assertMarkerAbsent(t, pane0File, pane1Marker)
	assertMarkerAbsent(t, pane1File, pane0Marker)
}

// TestMultiPaneLegacy_GracefulLegacyDegradation proves Acceptance Criteria 8:
// an UN-STAMPED saved session degrades to the name-based key at every stage
// without panicking on the empty PortalID. It captures PortalID == "" (Task
// 3-2), createSkeleton skips the re-stamp on the empty id (Task 3-3), the baked
// --hook-key is the name-based legacy-proj:0.0 (Task 1-1), and the name hook
// fires (the fallback coincides with the on-disk name key).
//
// Two named subtests share the one legacy round-trip run so a failure names
// the broken leg:
//   - "it degrades an un-stamped session to the name-based key end-to-end"
//   - "it does not panic on an empty PortalID anywhere in the chain"
//
// Spec: Acceptance Criteria 8 ("Graceful legacy"); Testing Requirements →
// "Legacy / no-regression (integration)".
func TestMultiPaneLegacy_GracefulLegacyDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	portaltest.IsolateStateForTest(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	hookFireFile := filepath.Join(t.TempDir(), "hook-fired.txt")
	const legacyMarker = "LEGACY_HOOK_FIRED"
	hookCmd := "echo " + legacyMarker + " >> " + hookFireFile

	// Register the on-resume hook under the NAME-based key — the fallback key an
	// un-stamped session yields. tmux.HookKey("", name, 0, 0) == "<name>:0.0",
	// which equals the key already on disk (no migration).
	legacyKey := tmux.HookKey("", legacyName, 0, 0)
	if legacyKey != legacyName+":0.0" {
		t.Fatalf("legacy hook key = %q; want %q (empty id must fall back to the name)",
			legacyKey, legacyName+":0.0")
	}
	store := hooks.NewStore(hooksPath)
	if err := store.Set(legacyKey, "on-resume", hookCmd, "cli"); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}
	verifyHookKeyed(t, hooksPath, legacyKey)

	ts := tmuxtest.New(t, "ptl-3-7-legacy-")
	client := ts.Client()

	// Stand up the un-stamped session — deliberately NO SetSessionOption for
	// @portal-id.
	cwd := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", legacyName, "-c", cwd, "sleep", "infinity")
	ts.WaitForSession(t, legacyName, 2*time.Second)

	// Guard: the live session carries NO @portal-id. An unset user-option makes
	// `show-options -v` exit non-zero with "invalid option" — so absent is
	// error-or-empty; only a non-empty value means the session was stamped.
	if liveID := unsetOptionValue(t, ts, legacyName); liveID != "" {
		t.Fatalf("un-stamped session %q unexpectedly has @portal-id = %q", legacyName, liveID)
	}

	// Capture — no panic on the empty id, and PortalID == "".
	idx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	sess := findCapturedSession(t, idx, legacyName)

	// NON-VACUOUS: the captured PortalID is EXACTLY "" before restore, or the
	// legacy path is not being exercised.
	t.Run("it degrades an un-stamped session to the name-based key end-to-end", func(t *testing.T) {
		if sess.PortalID != "" {
			t.Fatalf("captured un-stamped session %q PortalID = %q; want \"\" (name-fallback path)",
				legacyName, sess.PortalID)
		}
		// The baked hook key derives from the empty PortalID, so it MUST be the
		// name-based legacy-proj:0.0 — the same rule collectArmInfos applies
		// (tmux.HookKey(sess.PortalID, sess.Name, w, p)).
		bakedKey := tmux.HookKey(sess.PortalID, sess.Name, 0, 0)
		if bakedKey != legacyKey {
			t.Fatalf("baked --hook-key = %q; want name-based %q", bakedKey, legacyKey)
		}
	})

	seedPaneScrollback(t, stateDir, legacyName, 0, 0)
	persistIndex(t, idx, stateDir)

	// Reboot: kill → fresh server → restore. No panic anywhere in
	// capture → re-stamp(skip) → bake → hydrate on the empty id.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	logger := restoretest.OpenTestLogger(t, stateDir)
	o := &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}

	t.Run("it does not panic on an empty PortalID anywhere in the chain", func(t *testing.T) {
		// A panic in createSkeleton's re-stamp branch or the arm/bake path on
		// the empty id would surface as a test-crashing panic; restoreWithMarker
		// returning cleanly is the no-panic proof for the restore half, and the
		// capture above proved the capture half.
		if err := restoreWithMarker(t, client, o); err != nil {
			t.Fatalf("restoreWithMarker on un-stamped session: %v", err)
		}
	})

	// The restored session is back and — the re-stamp was SKIPPED on the empty
	// id — it still carries NO @portal-id.
	if _, err := ts.TryRun("has-session", "-t", "="+legacyName); err != nil {
		t.Fatalf("un-stamped session %q not restored: %v", legacyName, err)
	}
	// Re-stamp SKIPPED on the empty id: the restored session still carries no
	// @portal-id (absent = the `show-options -v` read errors "invalid option").
	if restampedID := unsetOptionValue(t, ts, legacyName); restampedID != "" {
		t.Errorf("re-stamp was NOT skipped: restored un-stamped session %q now has @portal-id = %q",
			legacyName, restampedID)
	}

	// Drive signal-hydrate — the name-based hook fires (fallback coincides with
	// the on-disk name key).
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{legacyName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	assertHookFireCount(t, hookFireFile, 1)
}

// TestMultiPaneLegacy_UnstampedNoHookLandsOnBareShell is the optional
// clean-miss leg: an un-stamped session with NO registered hook must land on a
// bare $SHELL cleanly — no panic, and the (absent) hook side-effect file is
// never written.
//
// Spec: Acceptance Criteria 8 ("Graceful legacy") — the clean-miss corollary.
func TestMultiPaneLegacy_UnstampedNoHookLandsOnBareShell(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	portaltest.IsolateStateForTest(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Empty hooks.json (no entries) — every lookup is a clean miss.
	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	ts := tmuxtest.New(t, "ptl-3-7-bare-")
	client := ts.Client()

	cwd := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", legacyName, "-c", cwd, "sleep", "infinity")
	ts.WaitForSession(t, legacyName, 2*time.Second)

	idx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	sess := findCapturedSession(t, idx, legacyName)
	if sess.PortalID != "" {
		t.Fatalf("captured un-stamped session %q PortalID = %q; want \"\"", legacyName, sess.PortalID)
	}

	seedPaneScrollback(t, stateDir, legacyName, 0, 0)
	persistIndex(t, idx, stateDir)

	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	logger := restoretest.OpenTestLogger(t, stateDir)
	o := &restore.Orchestrator{Client: client, StateDir: stateDir, Logger: logger}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker (no-hook clean miss): %v", err)
	}

	// Drive signal-hydrate — the helper looks up the baked name-key, misses
	// cleanly (empty hooks.json), and exec's a bare $SHELL. No panic, and no
	// side-effect file is ever created.
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{legacyName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	// The session is back with its live pane and the helper reached the bare-
	// shell exec (marker cleared). Nothing else to assert: the absence of a
	// panic + cleared marker IS the clean-miss proof.
	if _, err := ts.TryRun("has-session", "-t", "="+legacyName); err != nil {
		t.Fatalf("un-stamped no-hook session %q not restored: %v", legacyName, err)
	}
}

// unsetOptionValue reads @portal-id off the named session and returns "" when
// the option is absent. tmux's `show-options -v <user-option>` exits non-zero
// with "invalid option" when the option was never set, so an un-stamped session
// yields an error here — collapsed to "" (absent). A non-empty return means the
// session IS stamped. Used only to assert the un-stamped / re-stamp-skipped
// state, where the sibling harness's ts.Run (fatal-on-error) cannot be used.
func unsetOptionValue(t *testing.T, ts *tmuxtest.Socket, name string) string {
	t.Helper()
	out, err := ts.TryRun("show-options", "-t", name, "-v", session.PortalIDOption)
	if err != nil {
		return "" // unset user-option → "invalid option" exit → absent
	}
	return strings.TrimSpace(out)
}

// paneIndices renders a captured session's per-window pane indices for
// diagnostics — used only in a fatal message when the multi-pane topology guard
// trips.
func paneIndices(sess state.Session) [][]int {
	var out [][]int
	for _, w := range sess.Windows {
		var panes []int
		for _, p := range w.Panes {
			panes = append(panes, p.Index)
		}
		out = append(out, panes)
	}
	return out
}

// seedPaneScrollback writes a pane's on-disk scrollback fixture at the given
// (window, pane) — the bytes the hydrate helper later dumps. The sibling
// seedScrollback helper only seeds pane 0.0; this variant lets the multi-pane
// leg seed both panes and the legacy leg seed its single pane symmetrically.
func seedPaneScrollback(t *testing.T, stateDir, name string, window, pane int) {
	t.Helper()
	scrollbackKey := state.SanitizePaneKey(name, window, pane)
	scrollbackPath := state.ScrollbackFile(stateDir, scrollbackKey)
	if err := os.MkdirAll(filepath.Dir(scrollbackPath), 0o700); err != nil {
		t.Fatalf("mkdir scrollback dir: %v", err)
	}
	if err := os.WriteFile(scrollbackPath, []byte("\x1b[31mred\x1b[0m\nbefore reboot\n"), 0o600); err != nil {
		t.Fatalf("write fixture scrollback: %v", err)
	}
}

// assertMarkerFiredOnce asserts the given marker appears exactly once in the
// file — the per-pane hook fired exactly once into ITS own side-effect file.
func assertMarkerFiredOnce(t *testing.T, path, marker string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hook fire file %s (bare-shell miss leaves it absent): %v", path, err)
	}
	if got := strings.Count(string(data), marker); got != 1 {
		t.Errorf("marker %q fired %d times in %s; want exactly 1\ncontents:\n%s", marker, got, path, data)
	}
}

// assertMarkerAbsent asserts the given marker does NOT appear in the file — the
// load-bearing anti-cross-fire assertion: pane 1's marker must NOT be in pane
// 0's file (and vice versa). A missing file counts as absent (the other pane's
// hook never wrote here).
func assertMarkerAbsent(t *testing.T, path, marker string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read hook fire file %s: %v", path, err)
	}
	if strings.Contains(string(data), marker) {
		t.Errorf("CROSS-FIRE: marker %q leaked into %s (the :w.p suffix did not route hooks per-pane)\ncontents:\n%s",
			marker, path, data)
	}
}
