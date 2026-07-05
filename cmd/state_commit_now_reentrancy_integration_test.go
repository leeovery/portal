//go:build integration

package cmd_test

// Real-tmux integration gate for the killed-session-resurrects-within-
// tick-window spec. This file pins the spec's "Hook Re-entrancy" gate
// (§ Hook Re-entrancy and § Testing Requirements → Integration tests:
// "Hook re-entrancy validation"). It is the single end-to-end check
// that `portal state commit-now`, when invoked from inside the
// `session-closed` hook subprocess against the very same tmux server
// that fired the hook, does not hang or deadlock and emits a current
// `sessions.json` within a bounded window.
//
// Why this test is the gate (per spec § Hook Re-entrancy):
//
//   `commit-now` makes synchronous tmux client calls back into the
//   same server from within `session-closed`. `pane-focus-out` and
//   `session-renamed` have historical re-entrancy quirks; `session-
//   closed` is less suspect but not pre-validated for this specific
//   call pattern. The spec mandates this real-tmux fixture pass
//   BEFORE the rest of the implementation is taken as complete. A
//   failure here is a spec-level pivot signal — not an implementation
//   bug — and the work unit returns to the specification phase (e.g.
//   to consider deferring the tmux work via `tmux run-shell`, or
//   moving the synchronous write off the hook seam). See spec § Hook
//   Re-entrancy "On re-entrancy test failure".
//
// Skip behaviour mirrors the sibling real-tmux integration tests in
// this package (state_daemon_integration_test.go) and in
// internal/tmux/portal_saver_integration_test.go:
//
//   - tmuxtest.SkipIfNoTmux skips when tmux is not on PATH.
//   - portalbintest.StagePortalBinary skips cleanly if `go build`
//     fails — a dev machine without `go` or a broken build is not a
//     re-entrancy gate failure.
//
// No t.Parallel: the cmd-package convention (mock injection via
// package-level mutable state cleaned up by t.Cleanup) applies here
// even though the test exercises a subprocess rather than in-process
// seams. t.Parallel is forbidden across the portal test suite (see
// CLAUDE.md).
//
// Default-lane integration test (no `//go:build integration` tag),
// matching the convention of cmd/state_daemon_integration_test.go and
// cmd/root_integration_test.go. The gate must run on the same lane as
// the rest of the cmd-package tests so a re-entrancy regression cannot
// hide behind an opt-in tag.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// reentrancyHookBudget is the upper bound on hook subprocess completion
// after `tmux kill-session -t B` returns. The spec calls out "completion
// within a reasonable bound (e.g. 1s)"; 1.5s is the budget specified by
// the task as the gate, leaving headroom over the 1s spec example for
// CI scheduling jitter, `go build` cold-cache cost amortised across
// other tests, and tmux's own session-closed dispatch latency. A
// healthy run completes well inside this window; a re-entrancy
// deadlock will hit the budget cleanly.
const reentrancyHookBudget = 1500 * time.Millisecond

// reentrancyPollInterval is the cadence at which the test re-reads
// sessions.json while waiting for the hook subprocess to commit the
// post-kill index. 25ms is short enough that a subprocess completing
// in ~100-300ms (the expected happy-path range — tmux IPC + JSON
// encode + atomic rename) is observed within a handful of polls.
const reentrancyPollInterval = 25 * time.Millisecond

// reentrancyConsecutiveReads is the number of consecutive consistent
// reads of sessions.json the test requires before concluding "B is
// absent". Two consecutive reads guards against the race between the
// hook subprocess's atomic rename and the poll loop: a single read
// could land on the pre-rename file and observe stale content, but
// two consecutive reads of the post-rename file (each ≥ 25ms apart)
// prove the rename has settled. The spec calls this out as an edge
// case under § Concurrent commit-now invocations.
const reentrancyConsecutiveReads = 2

// TestCommitNowFromSessionClosedHook_NoDeadlockUnderRealTmux is the
// re-entrancy gate from spec § Hook Re-entrancy and § Testing
// Requirements → Integration tests → Hook re-entrancy validation.
//
// Failure mode: spec-level pivot signal. The test failing means the
// chosen mechanism (synchronous tmux calls from within the
// session-closed hook subprocess) is not viable under real tmux, and
// the work unit must return to specification to redesign the fix
// shape. See spec § Hook Re-entrancy → "On re-entrancy test failure".
//
// Flow:
//
//  1. Skip if tmux is not on PATH (CI without tmux).
//  2. Skip if `go build` cannot stage a portal binary (dev machine
//     without `go`, or broken build).
//  3. Stand up an isolated tmux server via tmuxtest.New.
//  4. Set PORTAL_STATE_DIR to t.TempDir so the hook subprocess writes
//     to the test's isolated state dir. The tmux server inherits env
//     from this process, and run-shell propagates env to the hook
//     subprocess; PATH-prepend from StagePortalBinary ensures `portal`
//     resolves to the freshly built binary.
//  5. Bring the server up with an anchor session so set-hook -g lands
//     on a live server; this also keeps the server alive after we
//     kill session B (tmux exits when the last session closes — the
//     anchor prevents server teardown from racing the hook
//     subprocess).
//  6. Invoke production `tmux.RegisterPortalHooks(client, nil)` to
//     install the full hook table including `commitNowCommand` on
//     `session-closed`. This is the load-bearing assertion that the
//     migration from task 1-4 is wired correctly: the test does NOT
//     hand-roll the hook string; it runs the same code path
//     bootstrap step 2 uses.
//  7. Create user sessions A and B.
//  8. Issue `tmux kill-session -t B` against the test socket. This
//     synchronously fires `session-closed`, which `run-shell`'s the
//     `portal state commit-now` subprocess against the same server.
//  9. Poll sessions.json on a 25ms cadence under a 1.5s
//     context.WithTimeout. Require two consecutive consistent reads
//     where B is absent and A is present before concluding success.
//  10. On timeout: dump (a) state directory listing, (b) live tmux
//     sessions/panes via the test client, (c) elapsed wall time,
//     via t.Fatalf so the diagnostic surfaces in the failure log.
//
// On success: assert (a) elapsed wall time strictly under
// reentrancyHookBudget, (b) sessions.json contains A, (c) sessions.json
// omits B (the killed session).
func TestCommitNowFromSessionClosedHook_NoDeadlockUnderRealTmux(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build the portal binary and PATH-prepend so the hook subprocess
	// resolves `portal state commit-now` to the freshly built binary.
	// The tmux server inherits PATH from this process (it is fork-
	// exec'd by tmuxtest.New which uses exec.Command("tmux", ...) and
	// inherits env), and run-shell propagates the server's env to the
	// hook subprocess. The `command -v portal` defensive guard inside
	// commitNowCommand short-circuits to a no-op if the PATH prepend
	// did not take effect — that would manifest as the test timing out
	// because sessions.json never gets written, distinguishable in
	// the diagnostic dump from a genuine re-entrancy deadlock by the
	// absence of any portal-authored files under the state dir.
	_ = portalbintest.StagePortalBinary(t)

	// Full isolation posture (HOME/XDG scrub + fingerprint backstop +
	// sandbox registry for the hook-spawned portal subprocesses) — this
	// fixture previously set only PORTAL_STATE_DIR, the weakest posture of
	// the real-binary-spawning tests.
	_, stateDir := portaltest.IsolateStateForTest(t)
	// commit-now resolves its state dir via state.EnsureDir which
	// honours PORTAL_STATE_DIR. Setting it on the test process
	// propagates to the tmux server (forked from this process by
	// tmuxtest.New) and onward to every hook subprocess.
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// Teardown-race guard (see RegisterStateDirTeardownGuard): straggler
	// hook-spawned commit-now subprocesses must not race the stateDir
	// TempDir RemoveAll. LIFO position: after IsolateStateForTest,
	// before tmuxtest.New.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-reentr-")
	client := sock.Client()

	// Bring the server up with an anchor session BEFORE registering
	// hooks. tmux's set-hook -g requires a live server (no implicit
	// start), and the anchor session prevents tmux from exiting after
	// session B is killed below (the server exits when the last
	// session closes, which would race the hook subprocess and leave
	// the test reading sessions.json against a torn-down server's
	// post-mortem state). The anchor pane runs `sleep infinity` so it
	// produces no scrollback and is cheap to enumerate during
	// commit-now's CaptureStructure.
	//
	// Naming the anchor `_anchor` (leading underscore) means it is
	// filtered out by keepSessionNames the same way `_portal-saver`
	// would be in production, so the assertion-side scan of
	// sessions.json need only consider real user sessions A and B.
	sock.Run(t, "new-session", "-d", "-s", "_anchor", "sh", "-c", "sleep infinity")

	// Install the production hook table. This is the load-bearing
	// step that proves the migration from task 1-4 is wired: the test
	// does not hand-author the hook command string — it runs the same
	// code path bootstrap step 2 invokes. A regression that left
	// session-closed on the cheap notifyCommand path would surface
	// here as "test never observes sessions.json reflecting the kill"
	// (notify just touches save.requested; it does not commit).
	if err := tmux.RegisterPortalHooks(client, nil); err != nil {
		t.Fatalf("RegisterPortalHooks: %v", err)
	}

	// Create the two user sessions. -d is required so new-session
	// does not try to attach (no controlling terminal in `go test`).
	// `sleep infinity` keeps the pane alive so commit-now's
	// CaptureStructure sees a stable per-pane shape during the pre-
	// kill snapshot and so session A's pane survives across the kill
	// of B.
	sock.Run(t, "new-session", "-d", "-s", "A", "sh", "-c", "sleep infinity")
	sock.Run(t, "new-session", "-d", "-s", "B", "sh", "-c", "sleep infinity")

	// The kill itself is synchronous from tmux's POV: kill-session
	// removes the session and dispatches the session-closed hook
	// before returning. The hook's run-shell, however, fork-execs the
	// portal subprocess asynchronously — so sessions.json is not
	// guaranteed to be written by the time sock.Run returns. The
	// post-kill poll below is the load-bearing wait.
	killStart := time.Now()
	sock.Run(t, "kill-session", "-t", "B")

	// Bounded wait: ctx.WithTimeout enforces the 1.5s gate. On a
	// healthy run sessions.json appears in ~50-300ms and the poll
	// returns long before the deadline; on a re-entrancy deadlock
	// the deadline fires and the diagnostic dump runs.
	ctx, cancel := context.WithTimeout(context.Background(), reentrancyHookBudget)
	defer cancel()

	if err := pollSessionsJSON(ctx, stateDir, []string{"A"}, []string{"B"}); err != nil {
		// Failure path: dump (a) state directory contents, (b) live
		// tmux session/pane list, (c) elapsed wall time. These three
		// signals together distinguish (i) re-entrancy deadlock (no
		// sessions.json written, hook subprocess still in flight) from
		// (ii) PATH-resolution failure (no sessions.json, no hook
		// subprocess in flight) from (iii) slow-but-progressing
		// (sessions.json written but stale; would have succeeded with
		// a larger budget). The exact failure mode informs whether the
		// spec-level pivot is needed (case i) or a test-fixture fix
		// suffices (case ii or iii).
		elapsed := time.Since(killStart)
		t.Fatalf(
			"commit-now hook did not produce sessions.json reflecting kill within %s "+
				"(elapsed=%s): %v\n"+
				"--- state directory contents ---\n%s\n"+
				"--- live tmux sessions ---\n%s\n"+
				"--- live tmux panes ---\n%s\n",
			reentrancyHookBudget, elapsed, err,
			dumpStateDir(stateDir),
			dumpTmuxSessions(sock),
			dumpTmuxPanes(sock),
		)
	}

	// Success: assert the spec's three observable contracts.
	elapsed := time.Since(killStart)

	// Assertion 1: the gate's primary contract — completion strictly
	// under the 1.5s budget. pollSessionsJSON returning nil means we
	// observed two consecutive consistent reads inside the context
	// deadline, so this is a belt-and-braces check on the elapsed
	// wall time (the context already enforces the ceiling).
	if elapsed >= reentrancyHookBudget {
		t.Errorf("hook subprocess elapsed %s exceeds budget %s "+
			"(test would have timed out — pollSessionsJSON returned nil but the elapsed "+
			"measurement disagrees, indicating a clock race or budget-edge condition)",
			elapsed, reentrancyHookBudget)
	}

	// Assertions 2 and 3: re-read the final sessions.json and assert
	// shape directly (rather than relying on the poll loop's
	// transitional view). This pins the steady-state observable that
	// downstream consumers (bootstrap, TUI) would see.
	idx, skip, err := state.ReadIndex(stateDir)
	if err != nil || skip {
		t.Fatalf("post-poll ReadIndex: skip=%v err=%v", skip, err)
	}
	names := sessionNames(idx)
	if _, ok := names["A"]; !ok {
		t.Errorf("sessions.json missing surviving session %q; sessions=%v", "A", keysOf(names))
	}
	if _, ok := names["B"]; ok {
		t.Errorf("sessions.json still contains killed session %q; sessions=%v", "B", keysOf(names))
	}

	t.Logf("commit-now hook completed in %s (budget=%s, sessions=%v)",
		elapsed, reentrancyHookBudget, keysOf(names))
}

// sessionNames builds a presence set from idx.Sessions so callers can
// assert "B is absent and A is present" without iterating the slice
// at every call site. Underscore-prefixed sessions (e.g. `_anchor`,
// `_portal-saver`) are filtered out at commit-now write time via
// keepSessionNames, so they will not appear in the result. The
// `map[string]struct{}` shape is the canonical name-set carrier
// shared across the cmd_test integration files (symptom, daemon-
// merge, reentrancy) — membership checks use the two-value comma-ok
// idiom (`_, ok := names[n]`) and diagnostic prints route through
// keysOf for a slice view.
func sessionNames(idx state.Index) map[string]struct{} {
	out := make(map[string]struct{}, len(idx.Sessions))
	for _, s := range idx.Sessions {
		out[s.Name] = struct{}{}
	}
	return out
}

// dumpStateDir returns a newline-joined listing of every entry under
// stateDir (recursive one level) for the failure diagnostic. The shape
// is "<name> (size=<bytes>)" per entry so a missing sessions.json (no
// hook subprocess output) is visually distinct from a present-but-
// empty sessions.json (hook subprocess exited mid-write).
func dumpStateDir(stateDir string) string {
	var lines []string
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return fmt.Sprintf("(readdir %s: %v)", stateDir, err)
	}
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			lines = append(lines, fmt.Sprintf("%s (stat err: %v)", e.Name(), ierr))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (size=%d, mode=%s)", e.Name(), info.Size(), info.Mode()))
		// One level of recursion is enough to surface scrollback/
		// contents and portal.log without flooding the failure log.
		if e.IsDir() {
			sub, serr := os.ReadDir(filepath.Join(stateDir, e.Name()))
			if serr != nil {
				lines = append(lines, fmt.Sprintf("  (readdir %s: %v)", e.Name(), serr))
				continue
			}
			for _, se := range sub {
				si, sierr := se.Info()
				if sierr != nil {
					lines = append(lines, fmt.Sprintf("  %s (stat err: %v)", se.Name(), sierr))
					continue
				}
				lines = append(lines, fmt.Sprintf("  %s (size=%d)", se.Name(), si.Size()))
			}
		}
	}
	// Inline the sessions.json bytes too (if present) so a stale-but-
	// progressing write is visible in the diagnostic. Capped to keep
	// the failure log readable on a corruption case.
	if data, rerr := os.ReadFile(state.SessionsJSON(stateDir)); rerr == nil {
		const cap = 2048
		blob := string(data)
		if len(blob) > cap {
			blob = blob[:cap] + "...(truncated)"
		}
		lines = append(lines, "--- sessions.json contents ---", blob)
	}
	return strings.Join(lines, "\n")
}

// dumpTmuxSessions returns `tmux list-sessions -F "#{session_name}"`
// output against the test socket. Used in the failure diagnostic to
// distinguish (a) tmux server unreachable / dead from (b) tmux alive
// but the kill never happened / was undone. A healthy failure
// diagnostic shows _anchor and A (B successfully killed).
func dumpTmuxSessions(sock *tmuxtest.Socket) string {
	out, err := sock.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return fmt.Sprintf("(list-sessions error: %v)\n%s", err, out)
	}
	return out
}

// dumpTmuxPanes returns `tmux list-panes -a -F "#{session_name}:#{window_index}.#{pane_index} #{pane_current_command}"`
// output against the test socket. Used in the failure diagnostic to
// surface every live pane in case a pane-level anomaly (e.g. the hook
// subprocess wedged a tmux client somewhere) is contributing to the
// re-entrancy stall.
func dumpTmuxPanes(sock *tmuxtest.Socket) string {
	out, err := sock.TryRun("list-panes", "-a", "-F",
		"#{session_name}:#{window_index}.#{pane_index} #{pane_current_command}")
	if err != nil {
		return fmt.Sprintf("(list-panes error: %v)\n%s", err, out)
	}
	return out
}
