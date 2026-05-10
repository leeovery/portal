//go:build integration

// Phase 2 task 2-6 — end-to-end real-tmux integration test gating AC2.
//
// AC2 (spec § Acceptance Criteria → AC2): On-resume hooks registered via
// `portal hooks set --on-resume "<cmd>"` fire end-to-end on cold-start for
// **every** restored pane that has a hook registered, regardless of which
// session the user attached to. The attached-session case is already
// covered by existing happy-path resurrection integration tests; this test
// closes the previously-broken non-attached case.
//
// What this test exercises end-to-end:
//
//   - Builds the portal binary so each restored pane's `portal state hydrate`
//     helper resolves on PATH (the helper is spawned via respawn-pane -k by
//     restore step 5 and must execute to fire its on-resume hook).
//   - Seeds sessions.json with TWO saved sessions (alpha, beta), each with
//     a single window/single pane. The on-resume hook is registered against
//     beta's pane — beta is the deterministic Symptom B repro (the
//     non-attached session under the pre-fix shape).
//   - Seeds hooks.json via the production hooks store, keyed by beta's
//     saved structural identifier (`beta:0.0` — the same shape
//     internal/restore/session.go's collectArmInfos passes via --hook-key).
//   - Runs the bootstrap.Orchestrator wired with the production
//     RestoreAdapter + EagerHydrateSignaler + HookRegistrar so the full
//     bootstrap chain (skeleton restore → eager signal → in-pane helper →
//     hook fire) is exercised.
//   - Polls a sentinel side-effect file (the on-resume hook command is
//     `touch <sentinelFile>`) every 50ms with a 2-second budget. Pass:
//     sentinel exists within the window.
//   - Asserts both alpha and beta are live via `tmux list-sessions` so the
//     test cannot vacuously pass if Restore silently no-op'd.
//
// Why this test is path-agnostic:
//
// The AC2 contract holds whether the eager-signal step delivered the FIFO
// byte to beta's helper inside its 3s OpenFIFO window (success path) OR
// the helper's 3s timeout fall-through fired (defense-in-depth path). Both
// paths exec through `execShellOrHookAndExit` per spec § Fix 2 → Specific
// Changes → 2, so the on-resume hook fires on either branch. The 2-second
// poll budget is intentionally below the 3s helper timeout so the test
// mostly observes the eager-signal success path; if eager-signal-write
// fails (e.g. CPU pressure under heavy parallelism), the hook still fires
// via the timeout fall-through but later than 2s — which would surface as
// a failure here. Per the task's important guidance, that legitimate
// >2s observation should be reported as an ISSUE rather than silently
// extending the budget.
//
// CI parallelism note (mirrors the AC1 sibling test):
//
// The production EagerSignalHydrate step relies on
// `internal/state.WriteFIFOSignal`, which retries ENXIO opens for ~500ms.
// Under heavy parallel CI load the in-pane hydrate helper's fork+exec can
// exceed 500ms before reaching open(O_RDONLY), causing the eager-signal
// write to fail. Run with `-p 1` (or in single-package isolation) to avoid
// the parallel-build CPU saturation.
//
// Build & run:
//
//	go test -tags=integration ./cmd/bootstrap/...
//
// Tests in this file are NOT included in the default `go test ./...` run
// because the `//go:build integration` tag gates them off. They also call
// `testing.Short()` so `go test -short -tags=integration ./...` skips them.

package bootstrap_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestPhase2_HookFiresOnNonAttachedSession_AC2 gates AC2 on cold-start.
//
// Pre-fix shape: beta's helper would not receive its FIFO byte (only the
// attached session's helper got signaled), the helper would time out at
// 3s, and the timeout fall-through routed through `execShellAndExit`
// (bare shell) — bypassing the hook-firing exec path. Result: the
// sentinel file is never created.
//
// Post-fix shape: either the eager-signal step delivers beta's FIFO byte
// (helper success path) OR the timeout fall-through routes through
// `execShellOrHookAndExit` (defense-in-depth). Either way, the on-resume
// hook fires and the sentinel file lands on disk.
func TestPhase2_HookFiresOnNonAttachedSession_AC2(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Build & PATH-prepend portal so each restored pane's hydrate helper
	// (spawned by restore via `respawn-pane -k portal state hydrate ...`)
	// can resolve the binary. Without this the helper exits before
	// reaching its hook-firing exec, sentinel never lands, test fails.
	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	// Wire the hooks store via PORTAL_HOOKS_FILE so the in-pane hydrate
	// helper's loadHookStore() resolves to the test's isolated path. The
	// helper runs in a child process inside the restored pane, inheriting
	// this env var via the tmux server's environment (which is itself
	// inherited from this test process).
	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Sentinel side-effect file. The on-resume hook command is
	// `touch <sentinelFile>` — its only observable effect is creating the
	// file at the named path. Sentinel observability without a real PTY
	// attach is intentional (per task edge-case note: `touch <path>` is
	// sufficient — no PTY required).
	sentinelDir := t.TempDir()
	sentinelFile := filepath.Join(sentinelDir, "hook-fired")

	// Saved sessions: alpha (no hook — control) and beta (hook registered
	// — the deterministic Symptom B pane).
	sessions := []string{"alpha", "beta"}
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

	// Register the on-resume hook against beta's saved structural key.
	// Hook-store keys are in tmux's "session:window.pane" form (see
	// internal/restore/session.go collectArmInfos — `tmux.PaneTarget(...)`
	// is what gets passed via --hook-key, and the in-pane helper looks up
	// hooks.json by the same key). For beta at default base-index 0 /
	// pane-base-index 0 the key is "beta:0.0".
	betaHookKey := tmux.PaneTarget("beta", 0, 0)
	hookCmd := fmt.Sprintf("touch %s", sentinelFile)
	store := hooks.NewStore(hooksPath)
	if err := store.Set(betaHookKey, "on-resume", hookCmd); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-p2-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Pre-condition: neither saved session is live yet — Restore will
	// skeleton-create them. If a session were already live, Restore's
	// idempotent path would short-circuit and the helper would never run.
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

	// Wire production adapters for the steps under test:
	//   - RestoreAdapter: skeleton-creates sessions, arms FIFOs, sets
	//     @portal-skeleton-* markers, spawns the in-pane helper.
	//   - EagerHydrateSignaler: writes the FIFO byte to every armed pane
	//     during step 6 — the success path that AC2 mostly observes.
	// HookRegistrar is left as the default NoOp since the test drives
	// hydration through the orchestrator's eager-signal step (and
	// optionally the helper's own 3s timeout fall-through), not through
	// tmux's `client-attached` hook — no client ever attaches.
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

	// Non-vacuity guard: both alpha and beta MUST be live post-bootstrap.
	// If beta were missing, the hook lookup would never fire (no helper
	// to look it up) and the sentinel would never land for an unrelated
	// reason — AC2 would be vacuously false. Surfacing this as a separate
	// fatal distinguishes "Restore broken" from "AC2 broken".
	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q "+
				"(non-vacuity guard cannot be evaluated — Restore did "+
				"not skeleton-create)", name, liveOut)
		}
	}

	// AC2 contract: poll for the sentinel file every 50ms for up to
	// 2 seconds. Pass condition is sentinel present within the window.
	// On failure, dump portal.log so the diagnostic includes any helper
	// WARN lines (e.g. hook lookup error, FIFO retry exhaustion).
	defer dumpPortalLogOnFailure(t, stateDir)
	pollUntilSentinelPresent(t, sentinelFile, 2*time.Second, 50*time.Millisecond)
}

// pollUntilSentinelPresent polls os.Stat(path) every tick until the
// sentinel file exists or the deadline elapses. The 2-second budget
// matches the AC2 contract's expected fast-path timing (eager-signal +
// helper exec + touch should complete well inside this window). A
// failure here means: (a) the eager-signal step did not deliver beta's
// FIFO byte AND the 3s timeout fall-through did not fire within the
// window, OR (b) the hook lookup failed inside the helper, OR (c) the
// helper crashed before reaching its exec.
//
// Distinct from pollUntilMarkersCleared (the AC1 helper) — that polls
// tmux server-option state, this polls a filesystem side-effect.
func pollUntilSentinelPresent(t *testing.T, path string, budget, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("AC2 violation: sentinel file %s not created within %s; "+
		"on-resume hook for non-attached session did not fire end-to-end",
		path, budget)
}
