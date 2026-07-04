package cmd

// No-regression / no-migration upgrade coverage (spec Testing Requirements →
// Legacy / no-regression; Acceptance Criteria 5).
//
// The fix switches both hook registration and stale-cleanup to HookKeyFormat
// (prefer @portal-id, else session_name). The load-bearing upgrade property is
// that this is a NO-MIGRATION change: an existing hooks.json entry written for
// an un-stamped, never-renamed session (created before @portal-id shipped) is
// keyed by the session NAME. When the switched cleanup enumerates live keys via
// ListAllPaneHookKeys, an un-stamped session's key falls back through
// HookKeyFormat's #{session_name} branch to that same name — so the pre-existing
// on-disk key coincides with the live key and the entry keeps resolving. It must
// NOT be mass-orphaned by the switched cleanup.
//
// This test proves that end-to-end against a REAL tmux server: it seeds a
// hooks.json carrying a name-keyed entry for a live un-stamped session plus a
// truly-stale name-keyed entry (no matching live pane), runs the shared
// runHookStaleCleanup path fed by ListAllPaneHookKeys against the live server,
// and asserts the un-stamped entry survives while the truly-stale one is swept.
//
// Why real tmux and not a stub lister: the coincidence being proved is that
// tmux's HookKeyFormat conditional resolves an un-stamped session to its NAME.
// A stub could just hand back the name and prove nothing. Driving a genuinely
// un-stamped live session through ListAllPaneHookKeys exercises the actual tmux
// #{?@portal-id,...,#{session_name}} fallback — the mechanism under test.
//
// This deliberately does NOT build the portal binary or spawn subprocesses: it
// drives runHookStaleCleanup directly against a *tmux.Client wired to an
// isolated tmuxtest socket, so it runs under `go test ./cmd/...` (SkipIfNoTmux-
// gated) with no //go:build integration tag. Rename (Phase 3 headline) and
// persistence/capture/restore are out of scope here — this proves ONLY that the
// pre-existing entry survives the switched cleanup, the precondition for firing.

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestHookKeyNoRegressionUpgrade_UnstampedNameKeyedHookSurvives asserts that an
// un-stamped, never-renamed session's pre-existing name-keyed hooks.json entry
// survives the upgraded (HookKeyFormat-based) stale cleanup, while a truly-stale
// name-keyed entry with no live pane is still swept.
func TestHookKeyNoRegressionUpgrade_UnstampedNameKeyedHookSurvives(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Live un-stamped tmux server: ts.Run("new-session") creates a session
	// WITHOUT any set-option @portal-id, exactly as a pre-@portal-id (legacy)
	// or manually-created session would exist on disk. HookKeyFormat's tmux
	// conditional therefore takes the #{session_name} branch for it.
	ts := tmuxtest.New(t, "ptl-upgrade-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	const liveName = "legacy-proj"
	ts.Run(t, "new-session", "-d", "-s", liveName)
	ts.WaitForSession(t, liveName, 2*time.Second)

	// Sanity-guard the name-fallback coincidence: the live hook key for the
	// un-stamped session MUST resolve to the NAME-based key. If a future change
	// stamped @portal-id on new-session, this key would flip to an id and the
	// test would no longer exercise the name-fallback path — fail loudly rather
	// than pass for the wrong reason.
	const liveKey = liveName + ":0.0"
	assertLiveHookKeyPresent(t, client, liveKey)

	// Keys used across the assertions.
	const staleKey = "gone-session:0.0" // no matching live pane → truly stale

	// Seed a pre-upgrade hooks.json via the store-based temp-file helper (no
	// dependency on the PORTAL_HOOKS_FILE isolation sentinel). Two entries:
	//   - legacy-proj:0.0  → matches the live un-stamped session, must SURVIVE
	//   - gone-session:0.0 → no live pane, truly stale, must be SWEPT
	seed := `{
  "` + liveKey + `": {"on-resume": "echo legacy"},
  "` + staleKey + `": {"on-resume": "echo gone"}
}`
	store, path := newTempHooksStore(t, seed)

	// Non-vacuous guard: the pre-cleanup seed must actually contain BOTH
	// entries, so a "survives" assertion cannot pass because the entry was
	// never written and a "swept" assertion cannot pass because it was never
	// present.
	preRun, err := store.Load()
	if err != nil {
		t.Fatalf("pre-cleanup store.Load: %v", err)
	}
	if _, ok := preRun[liveKey]; !ok {
		t.Fatalf("pre-cleanup seed missing %q; keys=%v", liveKey, keysOf(preRun))
	}
	if _, ok := preRun[staleKey]; !ok {
		t.Fatalf("pre-cleanup seed missing %q; keys=%v", staleKey, keysOf(preRun))
	}

	// Drive the switched cleanup. *tmux.Client satisfies AllPaneLister directly,
	// so ListAllPaneHookKeys enumerates the live un-stamped session as its
	// name-based hook key. Because that live key equals the on-disk name-based
	// key, CleanStale keeps legacy-proj:0.0 and removes gone-session:0.0.
	// swallowListError=false and onRemoved=nil match the bootstrap step-11 call
	// shape; a nil logger is tolerated by the helper.
	if err := runHookStaleCleanup(client, store, nil, nil); err != nil {
		t.Fatalf("runHookStaleCleanup: %v", err)
	}

	// Post-cleanup assertions read the file back through the store.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("post-cleanup store.Load: %v", err)
	}
	if _, ok := postRun[liveKey]; !ok {
		t.Errorf("un-stamped session's name-keyed hook %q was swept; want preserved "+
			"(name-based live key coincides with the on-disk key — no-migration upgrade). "+
			"post-cleanup keys=%v (path=%s)", liveKey, keysOf(postRun), path)
	}
	if _, ok := postRun[staleKey]; ok {
		t.Errorf("truly-stale name-keyed hook %q survived; want swept "+
			"(no matching live pane — cleanup correctness must not be weakened). "+
			"post-cleanup keys=%v (path=%s)", staleKey, keysOf(postRun), path)
	}
}

// assertLiveHookKeyPresent fails the test unless the given hook key appears in
// the live ListAllPaneHookKeys enumeration — the same enumeration the cleanup
// path consumes. It pins the name-fallback coincidence at the source so a
// surviving-entry assertion cannot pass merely because the entry was left
// untouched by an empty/erroring live set.
func assertLiveHookKeyPresent(t *testing.T, lister AllPaneLister, want string) {
	t.Helper()
	live, err := lister.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	if slices.Contains(live, want) {
		return
	}
	t.Fatalf("live hook key %q not enumerated; got %v (an un-stamped session must "+
		"resolve to its name-based key via HookKeyFormat's #{session_name} fallback)", want, live)
}
