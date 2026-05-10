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
	"github.com/leeovery/portal/internal/restore"
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
	// pane's hydrate helper (spawned by restore via `respawn-pane -k 'sh -c
	// portal state hydrate ...; exec $SHELL'`) can resolve the binary. Hoisted
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
	// pane's hydrate helper (spawned by restore via `respawn-pane -k 'sh -c
	// portal state hydrate ...; exec $SHELL'`) can resolve the binary.
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
	idx := state.Index{
		Sessions: make([]state.Session, 0, len(sessions)),
	}
	for _, name := range sessions {
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
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

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

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	// Wire the production EagerHydrateSignaler adapter: it iterates the
	// post-Restore @portal-skeleton-* marker set and writes the FIFO byte
	// to each pane's hydration FIFO. This is the step under test — a
	// regression that swaps in NoOpEagerHydrateSignaler{} would leave
	// markers stuck and the 2-second poll would expire.
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
		EagerSignaler: &bootstrapadapter.EagerHydrateSignaler{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		Logger: logger,
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
