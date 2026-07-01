//go:build integration

package cmd

// Phase 3 task 3-6 — Post-restore cleanup keeps the restored hook (chain (b)).
//
// This is the cmd-package half of task 3-6. The durability half (chain (a) —
// re-persistence + a second reboot fire) lives in
// internal/restore/rename_reboot_durability_integration_test.go, because it
// drives the full restore Orchestrator. This half must live in package cmd
// because runHookStaleCleanup (cmd/run_hook_stale_cleanup.go) is UNEXPORTED —
// the spec forbids re-implementing the prune algorithm, so the cleanup leg
// calls the real function directly.
//
// The bug being guarded (Cross-Reboot Persistence (b), spec lines 108 / 150):
// post-restore stale-cleanup (bootstrap step 11 / `portal clean`) builds its
// live-key set from the live @portal-id via ListAllPaneHookKeys. Restore's
// re-stamp (internal/restore/session.go createSkeleton) re-seeds the
// recreated live session with its saved @portal-id, so the live key resolves
// to the immutable id-key ("tok123:0.0"), which MATCHES the id-keyed
// hooks.json entry the hook was registered under — and cleanup keeps it.
//
// Without the re-stamp the restored session carries no live @portal-id, so
// ListAllPaneHookKeys falls back through HookKeyFormat's #{session_name}
// branch to the (post-rename) NAME key ("renamedst:0.0"), which does NOT
// match the on-disk id-key ("tok123:0.0") — and cleanup deletes the hook that
// just fired, in the same bootstrap. This test's live session is stamped with
// @portal-id exactly as restore's re-stamp leaves it, so it reproduces the
// post-restore live state cleanup reads; the assertLiveHookKeyPresent guard
// pins that the id-key (not the name) is what cleanup enumerates.
//
// Faithfulness to "after a rename+restore": the session is created under the
// POST-rename name (renamedst) and stamped with @portal-id = tok123 — the
// exact live shape restore leaves after re-stamping a renamed-then-restored
// session. This mirrors cmd/hookkey_no_regression_upgrade_test.go's
// real-tmux drive of the shared runHookStaleCleanup path; it differs only in
// that the live session is STAMPED (id-key path) rather than un-stamped
// (name-fallback path).
//
// Build & run:
//
//	go test -tags=integration ./cmd/... -run RenameRestoreCleanupSurvival
//	go test -short -tags=integration ./cmd/...   # skips this

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// renameRestorePortalID / renameRestoreName pin the post-rename+restore live
// identity: the recreated session's name is renameRestoreName (the post-rename
// name), and restore re-stamps @portal-id = renameRestorePortalID onto it. The
// hook was registered under the immutable id-key, so the on-disk hooks.json
// entry is renameRestorePortalID:0.0 ("tok123:0.0").
const (
	renameRestorePortalID = "tok123"
	renameRestoreName     = "renamedst"
)

// TestRenameRestoreCleanupSurvival_KeepsRestoredIdKeyedHook proves the
// post-restore stale-cleanup pass does NOT delete the freshly-restored,
// id-keyed hook — while a truly-stale entry (no matching live pane) is still
// swept.
//
// The live session models restore's post-re-stamp state: created under the
// post-rename name and stamped with @portal-id = tok123, so
// ListAllPaneHookKeys enumerates the immutable id-key "tok123:0.0" — matching
// the id-keyed hooks.json entry. runHookStaleCleanup (the real bootstrap
// step-11 / portal-clean prune) therefore keeps it.
//
// Spec: Acceptance Criteria 3 & 4; Testing Requirements → "Post-restore
// cleanup keeps the restored hook"; Cross-Reboot Persistence (b); Stage 4
// Post-restore consistency.
func TestRenameRestoreCleanupSurvival_KeepsRestoredIdKeyedHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Live un-stamped tmux server. ts.Run("new-session") creates the recreated
	// session under its POST-rename name; the SetSessionOption below is
	// restore's re-stamp (internal/restore/session.go createSkeleton).
	ts := tmuxtest.New(t, "ptl-3-6-clean-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	ts.Run(t, "new-session", "-d", "-s", renameRestoreName, "sleep", "infinity")
	ts.WaitForSession(t, renameRestoreName, 2*time.Second)

	// RESTORE RE-STAMP: re-seed the recreated live session with its saved
	// @portal-id, exactly as createSkeleton does after NewSessionWithCommand.
	// This is the step under test — omitting it drops the live key back to the
	// name and cleanup would delete the id-keyed hook (chain (b)).
	if err := client.SetSessionOption(renameRestoreName, session.PortalIDOption, renameRestorePortalID); err != nil {
		t.Fatalf("SetSessionOption %s=%s: %v", session.PortalIDOption, renameRestorePortalID, err)
	}

	// The id-key the hook was registered under (rename-immune). HookKey with a
	// non-empty portalID yields "<id>:w.p", independent of the session name.
	liveKey := tmux.HookKey(renameRestorePortalID, renameRestoreName, 0, 0)
	if liveKey != renameRestorePortalID+":0.0" {
		t.Fatalf("id-key = %q; want %q (id-key must not embed the name)", liveKey, renameRestorePortalID+":0.0")
	}

	// NON-VACUOUS guard: the live-key enumeration (the exact one cleanup
	// consumes) MUST contain the id-key — proving the re-stamp took effect. A
	// missing re-stamp resolves this to the NAME key instead, and this guard
	// would fail loudly rather than let the survival assertion pass for the
	// wrong reason. Also assert the NAME key is NOT what cleanup sees.
	assertLiveHookKeyPresent(t, client, liveKey)
	assertLiveHookKeyAbsent(t, client, renameRestoreName+":0.0")

	// Truly-stale entry: no matching live pane, must be SWEPT (cleanup
	// correctness must not be weakened by the survival guarantee).
	const staleKey = "gone-session:0.0"

	// Seed hooks.json: the restored id-keyed entry + a truly-stale entry.
	seed := `{
  "` + liveKey + `": {"on-resume": "echo restored"},
  "` + staleKey + `": {"on-resume": "echo gone"}
}`
	store, path := newTempHooksStore(t, seed)

	// Non-vacuous seed guard: BOTH entries must actually be on disk before
	// cleanup, so "survives" cannot pass because the entry was never written
	// and "swept" cannot pass because it was never present.
	preRun, err := store.Load()
	if err != nil {
		t.Fatalf("pre-cleanup store.Load: %v", err)
	}
	if _, ok := preRun[liveKey]; !ok {
		t.Fatalf("pre-cleanup seed missing id-key %q; keys=%v", liveKey, keysOf(preRun))
	}
	if _, ok := preRun[staleKey]; !ok {
		t.Fatalf("pre-cleanup seed missing stale key %q; keys=%v", staleKey, keysOf(preRun))
	}

	// Drive the REAL bootstrap step-11 / portal-clean prune. *tmux.Client
	// satisfies AllPaneLister directly, so ListAllPaneHookKeys enumerates the
	// re-stamped session's id-key. swallowListError=false and onRemoved=nil
	// match the bootstrap step-11 call shape; a nil logger is tolerated.
	if err := runHookStaleCleanup(client, store, nil, false, nil); err != nil {
		t.Fatalf("runHookStaleCleanup: %v", err)
	}

	// Post-cleanup assertions read the file back through the store.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("post-cleanup store.Load: %v", err)
	}
	if _, ok := postRun[liveKey]; !ok {
		t.Errorf("freshly-restored id-keyed hook %q was swept; want preserved "+
			"(re-stamped live @portal-id makes the live key match the on-disk id-key — chain (b)). "+
			"post-cleanup keys=%v (path=%s)", liveKey, keysOf(postRun), path)
	}
	if _, ok := postRun[staleKey]; ok {
		t.Errorf("truly-stale hook %q survived; want swept "+
			"(no matching live pane — cleanup correctness must not be weakened). "+
			"post-cleanup keys=%v (path=%s)", staleKey, keysOf(postRun), path)
	}
}

// assertLiveHookKeyAbsent fails the test if the given hook key IS enumerated by
// the live ListAllPaneHookKeys read. It is the counterpart to
// assertLiveHookKeyPresent (cmd/hookkey_no_regression_upgrade_test.go): here it
// pins that cleanup does NOT see the post-rename NAME key — the re-stamp made
// the id-key win, so the name key must be absent from the live-key set. If a
// regression dropped the re-stamp, the name key would appear (and the id-key
// would be absent), which assertLiveHookKeyPresent already catches; this
// tightens the pin from the other side.
func assertLiveHookKeyAbsent(t *testing.T, lister AllPaneLister, notWant string) {
	t.Helper()
	live, err := lister.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	if slices.Contains(live, notWant) {
		t.Fatalf("live hook key %q WAS enumerated; want absent (a stamped session must resolve "+
			"to its @portal-id key, not the name — got %v)", notWant, live)
	}
}
