//go:build integration

// Phase 3 task 3-6 — Durable across repeated reboots (chain (a)).
//
// The rename+restore fire proven by task 3-5
// (rename_reboot_hook_integration_test.go) is necessary but NOT sufficient.
// If restore did not RE-STAMP the live @portal-id onto the recreated session
// (task 3-3, internal/restore/session.go createSkeleton), the id would survive
// only the FIRST resume:
//
//   - The next capture (~1s later in production) rewrites sessions.json from
//     live tmux state; a session with no live @portal-id is captured as
//     PortalID == "", erasing the id from the snapshot → the SECOND reboot
//     resurrects a bare shell (spec Cross-Reboot Persistence (a), line 107).
//
// This file is the durability half of task 3-6. It reuses the task-3-5 harness
// (the renamePortalID/renameOldName/renameNewName fixture consts, the leaf
// helpers restoreWithMarker / findCapturedSession / verifyHookKeyed, and the
// build/PATH/state-dir/hooks-file scaffolding) and adds the two-cycle
// orchestration the single-fire 3-5 test does not exercise: a first
// rename+restore+fire, then a SIMULATED next capture (which must read the
// re-stamped live @portal-id back into PortalID), then a SECOND restore+
// signal-hydrate cycle whose hook must fire AGAIN.
//
// The cleanup half of task 3-6 (chain (b) — post-restore stale-cleanup keeps
// the restored id-keyed hook) lives in
// cmd/rename_restore_cleanup_survival_integration_test.go, because it must call
// the UNEXPORTED cmd.runHookStaleCleanup directly.
//
// Build & run:
//
//	go test -tags=integration ./internal/restore/... -run RenameRebootDurable
//	go test -short -tags=integration ./internal/restore/...   # skips this

package restore_test

import (
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

// TestRenameRebootHook_DurableAcrossRepeatedReboots proves the two durability
// guarantees of chain (a): after a rename+restore, (1) a SIMULATED next
// CaptureStructure against the live re-stamped server RE-PERSISTS
// PortalID == "tok123" (fails without task 3-3's re-stamp), and (2) a SECOND
// restore+signal-hydrate cycle fires the on-resume hook AGAIN.
//
// The two named subtests map to the task's test list:
//   - "it re-persists the @portal-id on the next capture after restore"
//   - "it fires the resume hook again on a second reboot cycle"
//
// They share one setup+two-cycle body (both facts are observed on the same
// run) but each asserts its own guarantee, so a failure names the broken leg.
//
// Spec: Acceptance Criteria 3 ("Durable across repeated reboots"); Testing
// Requirements → "Durable across repeated reboots"; Cross-Reboot Persistence
// (a).
func TestRenameRebootHook_DurableAcrossRepeatedReboots(t *testing.T) {
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

	// Hook side-effect file. The on-resume hook appends a line on each firing,
	// so its line count is the cumulative firing count across both reboot
	// cycles: 1 after cycle one, 2 after cycle two. A second-cycle bare-shell
	// miss (the pre-fix chain-(a) regression) would leave it stuck at 1.
	hookFireFile := filepath.Join(t.TempDir(), "hook-fired.txt")
	hookCmd := "echo HOOK_FIRED >> " + hookFireFile

	// Register the on-resume hook under the STABLE id-key (rename-immune).
	// tok123:0.0 — independent of the session name.
	stableKey := tmux.HookKey(renamePortalID, renameOldName, 0, 0)
	if stableKey != renamePortalID+":0.0" {
		t.Fatalf("stable hook key = %q; want %q (id-key must not embed the name)",
			stableKey, renamePortalID+":0.0")
	}
	store := hooks.NewStore(hooksPath)
	if err := store.Set(stableKey, "on-resume", hookCmd, "cli"); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-3-6-dur-")
	client := ts.Client()

	// ---- Cycle 1: rename → capture → restore → FIRE (the task-3-5 shape). ----

	cwd := t.TempDir()
	ts.Run(t, "new-session", "-d", "-s", renameOldName, "-c", cwd, "sleep", "infinity")
	ts.WaitForSession(t, renameOldName, 2*time.Second)

	// Stamp the immutable @portal-id — mirrors creation-time stamping.
	if err := client.SetSessionOption(renameOldName, session.PortalIDOption, renamePortalID); err != nil {
		t.Fatalf("SetSessionOption %s=%s: %v", session.PortalIDOption, renamePortalID, err)
	}

	// TRIGGER: external rename while the pane process keeps running. @portal-id
	// is unchanged; #{session_name} becomes renameNewName.
	ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)
	if _, err := ts.TryRun("has-session", "-t", "="+renameNewName); err != nil {
		t.Fatalf("session %q not live after rename: %v", renameNewName, err)
	}

	captureAndPersist(t, client, stateDir, renameNewName, renamePortalID)

	if err := rebootAndHydrate(t, ts, client, stateDir); err != nil {
		t.Fatalf("cycle 1 rebootAndHydrate: %v", err)
	}
	assertHookFireCount(t, hookFireFile, 1)

	// ---- Chain (a): the next capture must RE-PERSIST the re-stamped id. ----

	// SIMULATE THE NEXT CAPTURE: CaptureStructure reads the live re-stamped
	// @portal-id back into Session.PortalID. Without task 3-3's re-stamp the
	// restored session carries no live @portal-id, so this records
	// PortalID == "" (erasing the id → second-reboot bare shell). The capture
	// read (not a direct show-options read) is the exact production mechanism
	// that re-persists the id, so this is where the chain-(a) tripwire fires.
	// Captured ONCE here and reused for the second restore below — nothing
	// mutates the live server between this read and cycle 2.
	nextIdx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("next CaptureStructure: %v", err)
	}

	t.Run("it re-persists the @portal-id on the next capture after restore", func(t *testing.T) {
		sess := findCapturedSession(t, nextIdx, renameNewName)
		if sess.PortalID != renamePortalID {
			t.Fatalf("next capture PortalID = %q; want %q (id must be RE-PERSISTED by the re-stamp — "+
				"an empty id here is the chain-(a) regression: the next reboot resurrects a bare shell)",
				sess.PortalID, renamePortalID)
		}
	})

	// NON-VACUOUS guard on the PARENT t — a subtest's Fatalf halts only the
	// subtest, not this parent, so the pre-second-restore precondition is
	// re-asserted here on the same captured snapshot: the id MUST be non-empty
	// BEFORE the second restore, or cycle 2 would restore a bare shell and the
	// second-fire assertion below could never be reached honestly.
	secondSess := findCapturedSession(t, nextIdx, renameNewName)
	if secondSess.PortalID == "" {
		t.Fatalf("pre-second-restore captured PortalID is empty; the second restore would resurrect a bare shell (chain (a))")
	}
	// And the on-disk hook is still keyed by the stable id-key.
	verifyHookKeyed(t, hooksPath, stableKey)

	// ---- Cycle 2: SECOND reboot from the re-persisted sessions.json. ----

	persistIndex(t, nextIdx, stateDir)
	seedScrollback(t, stateDir, renameNewName)

	if err := rebootAndHydrate(t, ts, client, stateDir); err != nil {
		t.Fatalf("cycle 2 rebootAndHydrate: %v", err)
	}

	// The restored-again session carries the re-persisted id — durability holds
	// across the SECOND reboot, not just the first.
	liveIDAgain := strings.TrimSpace(ts.Run(t, "show-options", "-t", renameNewName,
		"-v", session.PortalIDOption))
	if liveIDAgain != renamePortalID {
		t.Errorf("after second reboot live @portal-id = %q; want %q (must survive repeated reboots)",
			liveIDAgain, renamePortalID)
	}

	t.Run("it fires the resume hook again on a second reboot cycle", func(t *testing.T) {
		// Cumulative count is now 2: one firing per reboot cycle. A stuck-at-1
		// count is the chain-(a) bare-shell miss on the second reboot.
		assertHookFireCount(t, hookFireFile, 2)
	})
}

// captureAndPersist captures the live server, guards (non-vacuously) that the
// named session was recorded with the expected PortalID, seeds the pane's
// scrollback fixture, and writes sessions.json — the save half of one reboot
// cycle.
func captureAndPersist(t *testing.T, client *tmux.Client, stateDir, name, wantPortalID string) {
	t.Helper()

	idx, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	// NON-VACUOUS guard: the snapshot must record the id under the named
	// session, or the round-trip would degrade to the name-fallback path and
	// still "pass" for the wrong reason.
	sess := findCapturedSession(t, idx, name)
	if sess.PortalID != wantPortalID {
		t.Fatalf("captured session %q PortalID = %q; want %q (id must persist under the post-rename name)",
			name, sess.PortalID, wantPortalID)
	}

	seedScrollback(t, stateDir, name)
	persistIndex(t, idx, stateDir)
}

// persistIndex and seedScrollback live in rename_reboot_shared_test.go.

// rebootAndHydrate performs the reboot half of one cycle: kill the server,
// bring up a fresh one, restore from sessions.json (via restoreWithMarker so
// skeleton markers are applied), confirm the session came back under
// renameNewName, then drive signal-hydrate and wait for every helper to reach
// its hook-or-shell exec step (marker cleared). Returns the restore error so
// the caller fatals with cycle context.
func rebootAndHydrate(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, stateDir string) error {
	t.Helper()

	// Kill so Restore runs against a fresh server. The list-sessions error
	// confirms the kill took effect.
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
		return err
	}

	// The restored session comes back under its POST-rename name with a live
	// pane 0:0.
	restoredPanes := ts.Run(t, "list-panes", "-s", "-t", renameNewName,
		"-F", "#{window_index}:#{pane_index}")
	if !strings.Contains(restoredPanes, "0:0") {
		t.Fatalf("restored session %q missing live pane 0:0; got %q", renameNewName, restoredPanes)
	}

	// Drive signal-hydrate on the restored pane, then wait for the helper to
	// finish (marker cleared = it reached the hook-or-shell exec step).
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{renameNewName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)
	return nil
}

// assertHookFireCount lives in rename_reboot_shared_test.go.
