//go:build integration

// Phase 1 task 1-6 — multi-session cold-start integration test for the
// eager-signal-hydrate step (spec § Acceptance Criteria → AC1).
//
// AC1: After a tmux server cold-start with N>=2 saved sessions, all
// `@portal-skeleton-<paneKey>` markers are unset within 2 seconds
// post-bootstrap, no client attach required to drive the unset.
//
// What this test exercises end-to-end:
//
//   - Builds the portal binary so each restored pane's `portal state hydrate`
//     helper resolves on PATH (the helper is spawned via respawn-pane -k by
//     restore step 5 and must execute to dump scrollback + unset its marker).
//   - Seeds sessions.json with N>=2 saved sessions, each with a single
//     window + single pane.
//   - Runs the bootstrap.Orchestrator wired with the production
//     RestoreAdapter (skeleton-creates each session, sets each pane's
//     @portal-skeleton-* marker, arms each FIFO) AND the production
//     EagerHydrateSignaler adapter (writes the FIFO byte to every armed
//     pane during step 6 of bootstrap).
//   - Polls state.ListSkeletonMarkers every 50ms with a 2-second deadline.
//     Pass: empty marker set within the window.
//
// Why this gates AC1 and existing tests don't:
//
//   - reboot_roundtrip_test.go's TestPhase5RebootRoundTripEndToEnd waits
//     up to 10s for marker clearance and drives signal-hydrate via either
//     a binary or direct FIFO write — that test exercises the full
//     reboot pipeline but does NOT pin the 2-second eager-signal contract,
//     and explicitly drives signal-hydrate as a post-bootstrap step.
//   - This test asserts that the orchestrator's own step 6 drives every
//     marker's helper to clear within 2 seconds, with NO client attach,
//     NO post-bootstrap signal-hydrate invocation, and NO switch-client.
//     A regression that wires NoOpEagerHydrateSignaler{} (or removes step 6)
//     surfaces here as a 2s timeout failure.
//
// Build & run:
//
//	go test -tags=integration ./cmd/bootstrap/...
//
// Tests in this file are NOT included in the default `go test ./...` run
// because the `//go:build integration` tag gates them off. They also call
// `testing.Short()` so `go test -short -tags=integration ./...` skips them.
//
// CI parallelism note:
//
// AC1 is a no-load contract. The production EagerSignalHydrate step relies
// on internal/state.WriteFIFOSignal, which retries ENXIO opens for a total
// of 500ms (see SignalHydrateRetryDelays). Under heavy parallel CI load —
// e.g. `go test -tags=integration ./...` running multiple package test
// binaries that each invoke `go build` for the portal helper concurrently —
// the in-pane hydrate helper's fork+exec can exceed 500ms before reaching
// open(O_RDONLY). When that happens the eager-signal write fails, the
// helper stays blocked on its blocking O_RDONLY (no one re-signals it
// inside this test's single-bootstrap scope), and the marker stays stuck.
// portal.log captures the canonical "retries exhausted opening fifo …
// device not configured" WARN line in that scenario; dumpPortalLogOnFailure
// surfaces it on test failure so CI-load flakes are distinguishable from
// genuine AC1 regressions.
//
// Empirically the test passes deterministically under:
//
//   - `go test -tags=integration ./cmd/bootstrap/...` (single-package)
//   - `go test -tags=integration -p 1 ./...` (sequential package execution)
//
// And can fail under:
//
//   - `go test -tags=integration ./...` (default parallelism on
//     CPU-constrained runners where multiple packages build the portal
//     binary concurrently)
//
// CI lanes that gate AC1 should use `-p 1` (or run cmd/bootstrap in
// isolation) to avoid the parallel-build CPU saturation. The test itself
// remains correct — it gates the spec's published 2-second contract.

package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s
// is the AC1 gate for the eager-signal-hydrate step.
//
// Each sub-test stands up a fresh isolated tmux server, seeds an
// N-session sessions.json fixture, runs bootstrap.Orchestrator with the
// production RestoreAdapter + EagerHydrateSignaler adapter, and asserts
// that every @portal-skeleton-* marker is cleared within 2 seconds via
// poll on state.ListSkeletonMarkers. No client attach or switch-client
// is issued — the eager-signal step alone must drive the unset.
//
// Sub-tests parameterise N to demonstrate the bug's deterministic shape:
// pre-fix, only the user's first attached session's helpers received
// their FIFO byte; the N-1 remaining sessions timed out at 3s and
// leaked their markers. Both N=2 and N=3 reproduce the leak shape; N=3
// is the optional larger-set sub-test mandated by the task plan.
func TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Build & PATH-prepend portal once at the parent level so each restored
	// pane's hydrate helper (spawned by restore via `respawn-pane -k 'portal
	// state hydrate --fifo X --file Y --hook-key Z'`) can resolve the binary. Hoisted
	// out of the per-sub-test path because `go build` is the heaviest single
	// operation in the test (~1-2s under load) and rebuilding for each sub-test
	// would amplify CPU pressure that already squeezes the eager-signal
	// retry budget. Both sub-tests share the same binary directory; PATH is
	// re-prepended inside each sub-test's t.Setenv scope so cleanup is
	// per-sub-test (PrependPATH uses t.Setenv which restores the prior PATH
	// when the sub-test exits).
	binDir := restoretest.BuildPortalBinaryDir(t)

	cases := []struct {
		name     string
		sessions []string
	}{
		{"N=2_DefaultIndices", []string{"alpha", "beta"}},
		{"N=3_LargerSet", []string{"alpha", "beta", "gamma"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runEagerSignalMultiSessionAC1(t, binDir, tc.sessions)
		})
	}
}

// runEagerSignalMultiSessionAC1 is the shared body driving each sub-test.
// It PATH-prepends the pre-built portal binary so the in-pane hydrate
// helper resolves on PATH, sets up an isolated state dir, seeds
// sessions.json describing the supplied saved sessions (each
// single-window/single-pane), wires the bootstrap orchestrator with
// production RestoreAdapter + EagerHydrateSignaler, runs it, then polls
// state.ListSkeletonMarkers every 50ms for up to 2 seconds. Pass: empty
// marker set within the window.
//
// binDir is supplied by the parent test (which built the binary once,
// see the comment in the parent for why the build is hoisted).
func runEagerSignalMultiSessionAC1(t *testing.T, binDir string, sessions []string) {
	t.Helper()

	// PATH-prepend the pre-built portal binary directory so each restored
	// pane's hydrate helper (spawned by restore via `respawn-pane -k 'portal
	// state hydrate --fifo X --file Y --hook-key Z'`) can resolve the binary.
	// Without this the helper exits before open(O_RDONLY), no marker is
	// unset, and the 2-second window expires for reasons unrelated to AC1.
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	// Seed sessions.json with N saved sessions, each with one window and
	// one pane at default base-index 0 / pane-base-index 0. The
	// ScrollbackFile path is recorded in the encoded hydrate command but
	// never opened by Restore itself — only by the in-pane hydrate helper,
	// whose bytes-on-disk we don't seed here (the helper handles a
	// missing file gracefully via its file-missing recovery path, still
	// unsetting its marker).
	restoretest.SeedSessionsJSON(t, stateDir, sessions...)

	ts := tmuxtest.New(t, "ptl-eager-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Pre-condition: none of the saved sessions are live yet — Restore
	// will skeleton-create them as part of step 5. If a session were
	// already live, Restore's idempotent path would short-circuit and the
	// marker would never be set, masking the AC1 contract.
	for _, name := range sessions {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("session %q unexpectedly live before Run", name)
		}
	}

	logger := openTestLogger(t, stateDir)

	// The production *bootstrap.EagerSignalCore — which iterates the
	// post-Restore @portal-skeleton-* marker set and writes the FIFO byte
	// to each pane's hydration FIFO via state.DefaultFIFOSignaler{} (the
	// production no-seam wrapper around state.SendHydrateSignal) — is the
	// step under test. We rely on buildIntegrationOrchestrator's
	// "Restore real → EagerSignaler defaults to real EagerSignalCore"
	// auto-default (see orchestrator_builder_test.go and defaults.go) to
	// produce identical wiring without restating the literal here. A
	// regression that swaps in NoOpEagerHydrateSignaler{} would leave
	// markers stuck and the 2-second poll would expire.
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
		Logger:  logger,
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Sanity: every saved session must now be live (Restore step 5
	// skeleton-created them). If Restore silently no-op'd, the marker set
	// would be empty for an unrelated reason — the AC1 poll below would
	// be vacuously true. Surfacing this as a separate fatal distinguishes
	// "Restore broken" from "eager-signal broken."
	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q "+
				"(non-vacuity guard cannot be evaluated — Restore did "+
				"not skeleton-create)", name, liveOut)
		}
	}

	// AC1 contract: poll state.ListSkeletonMarkers every 50ms for up to
	// 2 seconds. Pass condition is empty marker set within the window.
	// NO client attach, NO switch-client, NO signal-hydrate invocation —
	// the orchestrator's step 6 must have driven every helper to clear
	// its marker on its own.
	//
	// On failure, dump portal.log so the diagnostic includes the
	// WriteFIFOSignal retry path's WARN lines (e.g. ENXIO retries
	// exhausted). Failure mode under heavy parallel CI load typically
	// shows "retries exhausted opening fifo ... device not configured"
	// — meaning the in-pane hydrate helper hadn't reached open(O_RDONLY)
	// within the production 500ms retry budget. That is an environmental
	// CPU-pressure failure (helper fork+exec slow under load), not an
	// AC1 regression; the diagnostic dump distinguishes the two.
	defer dumpPortalLogOnFailure(t, stateDir)
	pollUntilMarkersCleared(t, client, 2*time.Second, 50*time.Millisecond)
}

// dumpPortalLogOnFailure prints <stateDir>/portal.log via t.Logf when the
// caller test has failed. Used on the AC1 path to surface the
// WriteFIFOSignal retry-exhaustion WARN line (when present) so a flake
// caused by helper-startup latency under heavy parallel CI load is
// distinguishable from a genuine AC1 regression.
func dumpPortalLogOnFailure(t *testing.T, stateDir string) {
	t.Helper()
	if !t.Failed() {
		return
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "portal.log"))
	if err != nil {
		return
	}
	t.Logf("portal.log contents on failure:\n%s", data)
}

// TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4 gates AC4:
// "Scrollback save resumes for previously-stuck-marker panes — daemon
// captureAndCommit no longer indefinitely skips any live pane." See
// specification.md § Acceptance Criteria → AC4 and § AC ↔ Fix Traceability
// (AC4 satisfied by Fix 1: eager signaling unsets markers, daemon resumes
// capturing those panes).
//
// Pre-fix shape: only the user's first attached session's helper received
// its FIFO byte; the second session's helper timed out, leaving its
// @portal-skeleton-* marker set. The daemon's per-pane skip-save guard
// (cmd/state_daemon.go:131-133) then suppressed scrollback save for that
// pane indefinitely — until something else attached to (or otherwise
// signaled) the helper, which never happens for sessions the user does not
// attach to. This test reproduces that bug-scope at the AC4 level: the
// non-attached "beta" session is the deterministic-bug-scope item.
//
// Post-fix shape: the eager-signal step in bootstrap drives every helper's
// FIFO byte, every marker is cleared, and the next daemon tick captures
// scrollback for beta — including beta, which under the pre-fix shape would
// have stayed in the skip-save set forever.
//
// Why this is distinct from the AC1 sub-test in this file:
//
//   - AC1 asserts marker clearance within 2s (fast eager-signal contract).
//   - AC4 asserts the *downstream daemon-side consequence* of that
//     clearance: a daemon tick after marker clearance writes a non-empty
//     scrollback file for the previously-stuck pane.
//
// A regression that wires NoOpEagerHydrateSignaler{} would surface here as
// a stuck beta marker → daemon skip-save → empty/missing scrollback file
// for beta. A regression that wires eager signaling but breaks the daemon's
// post-marker-clear capture loop would also surface here, distinguishing
// AC4 from AC1 even though both share the same Fix 1 chain.
func TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Reuse the same N=2 fixture shape as the AC1 sub-test: alpha + beta,
	// each single-window/single-pane. beta is the "non-attached" session
	// — under the pre-fix shape its marker would stay stuck and the
	// daemon would indefinitely skip its scrollback save.
	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	sessions := []string{"alpha", "beta"}
	restoretest.SeedSessionsJSON(t, stateDir, sessions...)

	ts := tmuxtest.New(t, "ptl-eager-ac4-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	for _, name := range sessions {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("session %q unexpectedly live before Run", name)
		}
	}

	logger := openTestLogger(t, stateDir)

	// EagerSignaler is left to buildIntegrationOrchestrator's auto-default
	// (real EagerSignalCore when Restore is real) — see the AC1 sub-test
	// above for the rationale.
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
		Logger:  logger,
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Sanity: every saved session must now be live (Restore step 5
	// skeleton-created them). If beta were missing, the daemon-tick
	// capture below would be vacuously skip-save-free for beta — a
	// false-positive AC4 pass.
	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q",
				name, liveOut)
		}
	}

	// Wait for marker clearance — the daemon-tick capture below requires
	// beta's marker to be unset, otherwise the per-pane skip-save guard
	// in runDaemonTick (mirroring cmd/state_daemon.go:131-133) would
	// suppress the very write we are asserting on. The 2s budget is the
	// AC1 contract; AC4 strictly requires AC1 to have passed first.
	defer dumpPortalLogOnFailure(t, stateDir)
	pollUntilMarkersCleared(t, client, 2*time.Second, 50*time.Millisecond)

	// Force a deterministic terminated record into beta's pane buffer so
	// state.TailScrollback has at least one '\n'-terminated line to
	// return. capture-pane on a freshly-exec'd shell can otherwise yield
	// content with no terminating newline (only a partial prompt line),
	// in which case TailScrollback would return (nil, nil) per its
	// no-content-available contract — masking AC4 with a tooling-level
	// flake. send-keys + a brief settle window guarantees the record
	// lands in the pane buffer before the daemon tick captures.
	betaPaneKey := state.SanitizePaneKey("beta", 0, 0)
	betaTarget := tmux.PaneTarget("beta", 0, 0)
	if err := client.SendKeys(betaTarget, "echo ac4-marker"); err != nil {
		t.Fatalf("SendKeys to %s: %v", betaTarget, err)
	}
	waitForPaneText(t, client, betaTarget, "ac4-marker", 2*time.Second, 50*time.Millisecond)

	// Drive one daemon-equivalent capture-and-commit pass. runDaemonTick
	// mirrors cmd/state_daemon.go captureAndCommit: it lists skeleton
	// markers, captures structure, and per-pane invokes
	// CaptureAndHashPane → WriteScrollbackIfChanged → Commit. Because
	// beta's marker was cleared by the eager-signal step, the per-pane
	// skip-save guard does NOT fire for beta, and its scrollback bytes
	// land at state.ScrollbackFile(stateDir, "beta__0.0").
	runDaemonTick(t, client, stateDir)

	// AC4 assertion: the previously-non-attached pane's scrollback file
	// holds at least one terminated record. TailScrollback returns
	// (nil, nil) for missing/empty/unterminated content; under the
	// pre-fix shape beta's marker would still be set, the daemon's
	// skip-save guard would have fired, and the .bin file would not
	// exist on disk at all — converging on (nil, nil) here. Non-nil
	// bytes are the post-fix outcome.
	scrollbackPath := state.ScrollbackFile(stateDir, betaPaneKey)
	tail, err := state.TailScrollback(scrollbackPath, 10)
	if err != nil {
		t.Fatalf("TailScrollback %s: %v", scrollbackPath, err)
	}
	if tail == nil {
		t.Fatalf("AC4 violation: scrollback for non-attached pane %s "+
			"holds no terminated records after daemon tick; want "+
			"non-empty (Fix 1: eager-signal unset beta's marker → "+
			"daemon resumed capturing beta). scrollback path=%s",
			betaPaneKey, scrollbackPath)
	}
}

// waitForPaneText polls capture-pane on target until needle appears in the
// captured bytes or the deadline elapses. Used by AC4 to confirm that a
// send-keys command has been processed by the in-pane shell — without this
// the subsequent daemon tick may capture the pane buffer before the keys
// have been echoed to the screen, producing a flaky empty-content path.
func waitForPaneText(t *testing.T, client *tmux.Client, target, needle string, budget, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		out, err := client.CapturePane(target)
		if err == nil && strings.Contains(out, needle) {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("pane %s did not echo %q within %s", target, needle, budget)
}

// pollUntilMarkersCleared is the AC1-specific 2-second / 50ms poll loop.
//
// Distinct from restoretest.WaitForSkeletonMarkersCleared (which uses a
// 10-second budget for the full reboot round-trip): AC1 mandates a
// strict 2-second bound, so a longer budget would silently mask a
// regression where the eager-signal step ran but took N seconds. The
// 2-second value is the spec's published contract — see
// specification.md § Acceptance Criteria → AC1.
//
// On expiry the test fails with a sorted list of stuck paneKeys for
// stable diagnostics. A stuck marker after the eager-signal step
// indicates either: (a) the helper crashed before unsetting it, (b) the
// FIFO write never reached the helper (signal byte lost), or (c) the
// eager-signal step itself was skipped or no-op'd.
func pollUntilMarkersCleared(t *testing.T, client *tmux.Client, budget, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		markers, err := state.ListSkeletonMarkers(client)
		if err != nil {
			t.Fatalf("ListSkeletonMarkers: %v", err)
		}
		if len(markers) == 0 {
			return
		}
		time.Sleep(tick)
	}
	markers, _ := state.ListSkeletonMarkers(client)
	t.Fatalf("AC1 violation: skeleton markers still set after %s; "+
		"want empty set within budget. stuck markers=%v",
		budget, restoretest.SortedKeySet(markers))
}
