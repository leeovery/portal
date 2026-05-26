//go:build integration

// Fingerprint backstop verification test for spec § "Composite End-to-End
// Verification" end-state — task 6-8.
//
// Purpose
// -------
// The portaltest fingerprint-diff backstop is registered automatically
// inside portaltest.IsolateStateForTest (Phase 1 Task 1-3). For every
// Phase 6 test that consumes setupCompositeHarness, the backstop fires
// at test exit via t.Cleanup in LIFO order — AFTER harness teardown.
//
// This test makes the backstop's contract EXPLICIT: it consumes the
// harness, performs no further work, and exits. If the backstop's
// silence holds (no t.Errorf call against the host *testing.T), the
// test passes. The proof is by absence-of-failure: the backstop sees
// the developer's real ~/.config/portal/state/ tree, fingerprints it
// pre-test, snapshots it again at test exit, and would call t.Errorf
// for any divergence. A silent backstop means the env override
// (XDG_CONFIG_HOME=<tempDir>/config) successfully contained every
// subprocess spawned by the harness — i.e. no portal binary spawned
// by setupCompositeHarness's daemons or saver wrote to the real state
// dir.
//
// LIFO cleanup ordering
// ---------------------
// The backstop's correctness depends on cleanup running AFTER harness
// teardown. portaltest.IsolateStateForTest is called FIRST inside
// setupCompositeHarness (step 2), so its t.Cleanup is registered first
// and fires LAST per Go's LIFO Cleanup semantics. Subsequent harness
// setup steps register their own Cleanups (orphan SIGKILL+Wait via
// portaltest.RegisterSubprocessCleanup, tmux kill-server via tmuxtest.New); those
// fire BEFORE the backstop, so by the time the backstop walks the dev
// state dir, all spawned daemons and the isolated tmux server are
// already torn down. This means the backstop observes the post-
// teardown state of the dev install — exactly the steady-state surface
// the spec § End-State Verification section requires to be unchanged.
//
// Host-noise caveat
// -----------------
// On the developer's host where a live portal daemon mutates
// ~/.config/portal/state/ continuously (sessions.json + scrollback
// .bin churn), the backstop WILL report a "developer state dir
// mutated" t.Errorf at test exit. This is host noise documented in the
// 6-3..6-7 task reports — it does NOT indicate a regression in the
// harness's env containment. The test passes cleanly in CI / on any
// host where no live portal daemon is running.
//
// Documented backstop failure causes
// ----------------------------------
// When the backstop reports a divergence, the cause is one of three
// documented categories:
//
//  1. Subprocess inherited the developer's XDG_CONFIG_HOME — a spawned
//     `portal` binary received the developer's env instead of the
//     isolated env returned by portaltest.IsolateStateForTest. The
//     containment helper's reach stops at the env it returns; tests
//     that do not propagate the helper-supplied env to every spawned
//     subprocess will leak.
//  2. A direct file write bypassed the env entirely — a test or helper
//     opened a path under the developer's real state dir directly,
//     without consulting XDG_CONFIG_HOME at all. The env override
//     can't catch this because the path is hard-coded or computed from
//     a non-env source.
//  3. The helper snapshot semantics changed — portaltest.IsolateStateForTest
//     snapshots BEFORE applying its env mutation in normal operation
//     (the slow-open-empty-previews-and-zombie-sessions Task 1-3
//     deviation snapshot-AFTER-mutation is host-noise-mitigation-only
//     and remains documented). A future refactor that moves the
//     snapshot point or alters the fingerprint walk's ignore list can
//     surface as false positives until the snapshot/walk semantics are
//     audited.
//
// No explicit backstop invocation
// -------------------------------
// There is no Go-stdlib API to inspect t.Cleanup registrations, so this
// test cannot programmatically prove the backstop is wired in. The
// proof relies on the harness contract: setupCompositeHarness calls
// portaltest.IsolateStateForTest, which (per its package documentation
// and Phase 1 Task 1-3 acceptance criteria) registers the backstop via
// t.Cleanup. The TestFingerprintBackstop_RegistersAndDoesNotErrorf
// unit test in internal/portaltest covers the registration mechanics
// directly; this integration test exercises the wiring end-to-end
// through the Phase 6 harness call path.
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"testing"
)

// TestCompositeBootstrap_FingerprintBackstopRunsClean consumes
// setupCompositeHarness, performs no further work, and exits. The
// portaltest fingerprint backstop registered by
// portaltest.IsolateStateForTest (called inside the harness) fires
// via t.Cleanup AFTER harness teardown (LIFO ordering) and walks the
// developer's real ~/.config/portal/state/ tree, calling t.Errorf for
// any divergence from the pre-test snapshot.
//
// The test PASSES if the backstop reports clean — i.e. it does NOT
// call t.Errorf against this test's *testing.T. The structural
// no-op-after-harness body is intentional: the value of the test is
// in proving by absence-of-failure that the harness's env containment
// keeps every spawned subprocess from leaking into the dev state dir.
//
// On a host with a live portal daemon mutating the real state dir,
// this test will fail with a "developer state dir mutated" diagnostic
// emitted by the backstop. That is documented host noise (see the
// file header) — the test passes on a clean host / in CI.
func TestCompositeBootstrap_FingerprintBackstopRunsClean(t *testing.T) {
	// Consume the harness so portaltest.IsolateStateForTest runs and
	// registers the backstop. The returned struct is intentionally
	// unused beyond a single liveness reference below — this test
	// makes no behavioural assertions on the harness itself; the
	// dedicated TestCompositeHarness_PreState test (in the harness
	// file) already covers harness preconditions.
	h := setupCompositeHarness(t)

	// Reference one field so the compiler does not flag the harness
	// return value as unused. The field choice is incidental — any
	// non-zero field would suffice. We pick StateDir because it is the
	// most semantically relevant field to a backstop test: the
	// backstop watches the DEV state dir, and h.StateDir is the
	// ISOLATED state dir, so reading it here documents the
	// containment-boundary intent (we touch the isolated dir, not the
	// dev one).
	if h.StateDir == "" {
		t.Fatalf("harness returned empty StateDir; setupCompositeHarness contract broken")
	}

	// Test body intentionally ends here. The backstop registered by
	// portaltest.IsolateStateForTest inside setupCompositeHarness
	// fires at test exit (LIFO Cleanup ordering, AFTER harness
	// subprocess + tmux teardown) and will call t.Errorf on this
	// *testing.T for any divergence in the dev state dir snapshot.
	// Silence == pass.
}
