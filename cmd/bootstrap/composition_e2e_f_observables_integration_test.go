//go:build integration

// Composite end-to-end Component F end-state observables test for spec
// § "Composite End-to-End Verification" bullet 9 — task 6-7.
//
// Consumes the shared compositeHarness (3-daemon pre-state: legitimate
// saver-pane daemon + 2 orphans; legitimate stateDir's daemon.pid
// references orphan1). Invokes the production bootstrap slice —
// `SweepOrphanDaemons` (Component B) then `BootstrapPortalSaver`
// (Component F + A's escalation path) in the same direct-adapter order
// the orchestrator runs them at steps 4–5 — waits for pgrep convergence
// to 1, and then asserts Component F's three end-state observables on
// the `_portal-saver` session:
//
//  1. The `_portal-saver` pane has a single integer `pane_pid` > 0,
//     read via `tmux list-panes -t _portal-saver -F '#{pane_pid}'`.
//  2. That pane process's args (via `ps -o args= -p <pid>`) contain
//     "portal state daemon" AND do NOT contain "tail -f /dev/null"
//     (the placeholder command F installs pre-respawn). This is the
//     load-bearing proof that F's create-with-placeholder → set-option →
//     respawn-with-daemon ordering shipped correctly — pre-F the
//     placeholder would never appear at all, but a regressed-mid-F
//     ordering (e.g., placeholder swap never fired) would surface as
//     the placeholder argv leaking past bootstrap.
//  3. `tmux show-options -t _portal-saver destroy-unattached` parses to
//     "off". The raw tmux output is `destroy-unattached off` (or with
//     quoted value on some tmux versions); we strip the leading key +
//     whitespace and apply `tmuxout.StripMatchedOuterQuotes` to the
//     remaining value so the assertion is robust across tmux quoting /
//     padding variations.
//
// This file's assertions complement the Phase 3 task 3-5 non-composite
// end-state coverage in
// `internal/tmux/portal_saver_endstate_integration_test.go` (specifically
// `TestBootstrapPortalSaver_CleanBootstrap_EndState`): same three F
// observables, but exercised here AFTER the composite A+B+F bootstrap
// slice converges from the 3-daemon dysfunction pre-state, proving F
// holds in the composite end-state and not just in the clean-bootstrap
// scenario.
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxout"
)

// fObservablesConvergenceTimeout is the 6 s post-bootstrap convergence
// budget from spec § "Composite End-to-End Verification" bullet 5.
// Matches convergencePGrepTimeout (6-3) and freshAcquireConvergenceTimeout
// (6-5) verbatim — same spec citation, same budget. The F-observables
// assertions only fire AFTER pgrep convergence so the assertions target
// the converged-healthy saver, not a mid-bootstrap intermediate state.
const fObservablesConvergenceTimeout = 6 * time.Second

// TestCompositeBootstrap_FObservables exercises the composite end-to-end
// Component F end-state observables against the 3-daemon harness
// pre-state. See the file-header comment for the assertion shape and
// the parsing-robustness rationale for the destroy-unattached parse.
func TestCompositeBootstrap_FObservables(t *testing.T) {
	h := setupCompositeHarness(t)

	// Bootstrap slice: same direct-adapter order as the orchestrator
	// runs at steps 4–5 (and as 6-3 / 6-4 / 6-5 invoke). Logger arg is
	// nil — this test does not assert on logger emissions (6-3 covers
	// the forbidden-strings check).
	sweeper := bootstrapadapter.NewOrphanSweeper(h.Client, nil)
	start := time.Now()
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error "+
			"(best-effort step must return nil): %v", err)
	}
	if err := tmux.BootstrapPortalSaver(h.Client, h.StateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (post-sweep idempotent re-run): %v", err)
	}

	// Convergence: pgrep -fx must reach 1 within the 6 s budget measured
	// from `start`. Compute REMAINING budget at the poll site so the
	// assertion enforces "within 6 s of bootstrap entry" rather than
	// restarting a fresh 6 s window after the bootstrap slice returns.
	remaining := fObservablesConvergenceTimeout - time.Since(start)
	if remaining <= 0 {
		t.Fatalf("post-bootstrap: 6 s budget already exhausted by the bootstrap "+
			"slice itself (elapsed=%s) — cannot assert convergence",
			time.Since(start))
	}
	if !waitForPgrepCount(t, 1, remaining) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("post-bootstrap: pgrep -fx did not converge to 1 within %s of "+
			"bootstrap-slice entry (elapsed=%s)\n"+
			"  harness saver PID (setup-time): %d (alive=%v)\n"+
			"  harness orphan1 PID: %d (alive=%v)\n"+
			"  harness orphan2 PID: %d (alive=%v)\n"+
			"  current pgrep snapshot: %v",
			fObservablesConvergenceTimeout, time.Since(start),
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID),
			pids)
	}

	// --- Observable 1: pane_pid is a single integer > 0. ---
	//
	// Uses sock.TryRun (not sock.Run) so we surface a rich diagnostic on
	// failure rather than the test runner's default "tmux args: error"
	// message. The raw list-panes output is included in any failure for
	// inspection.
	panePIDRaw, err := h.Sock.TryRun("list-panes", "-t", tmux.PortalSaverName, "-F", "#{pane_pid}")
	if err != nil {
		t.Fatalf("list-panes -t %s -F #{pane_pid}: %v\n%s",
			tmux.PortalSaverName, err, panePIDRaw)
	}
	panePIDStr := strings.TrimSpace(panePIDRaw)
	if panePIDStr == "" {
		t.Fatalf("list-panes returned empty pane_pid output for %s\n--- raw ---\n%q",
			tmux.PortalSaverName, panePIDRaw)
	}
	// Multi-pane saver session would be a Component F regression — F
	// guarantees a single pane (the daemon). Reject any newline-separated
	// extra lines explicitly so the integer parse below doesn't fail with
	// a misleading "strconv" message.
	if strings.Contains(panePIDStr, "\n") {
		t.Fatalf("list-panes returned multiple pane_pid lines for %s "+
			"(want exactly 1):\n--- raw ---\n%q",
			tmux.PortalSaverName, panePIDRaw)
	}
	panePID, err := strconv.Atoi(panePIDStr)
	if err != nil {
		t.Fatalf("parse pane_pid %q as int: %v\n--- raw ---\n%q",
			panePIDStr, err, panePIDRaw)
	}
	if panePID <= 0 {
		t.Fatalf("pane_pid = %d; want > 0\n--- raw ---\n%q",
			panePID, panePIDRaw)
	}

	// --- Observable 2: pane process args contain "portal state daemon"
	// and NOT "tail -f /dev/null" (the placeholder). ---
	//
	// psArgsForPID lives in internal/tmux/portal_saver_endstate_integration_test.go
	// (test-package internal/tmux_test, not importable from this package).
	// Re-implementing the same ~3-line `ps -o args= -p <pid>` shell-out
	// inline avoids a cross-package test-helper extraction for a single
	// caller — and matches the file-header note that the assertion
	// "mirrors the simpler assertions inline" from the 3-5 endstate test.
	args, err := psArgsForPIDInline(panePID)
	if err != nil {
		t.Fatalf("ps -o args= -p %d: %v", panePID, err)
	}
	const wantDaemonArgs = "portal state daemon"
	if !strings.Contains(args, wantDaemonArgs) {
		t.Fatalf("pane process args do not contain %q\n"+
			"  pane_pid: %d\n"+
			"  ps args: %q\n"+
			"  hint: Component F's respawn-pane swap from placeholder to "+
			"`portal state daemon` did not fire, or the daemon process exited "+
			"and tmux respawned the placeholder",
			wantDaemonArgs, panePID, args)
	}
	const forbiddenPlaceholder = "tail -f /dev/null"
	if strings.Contains(args, forbiddenPlaceholder) {
		t.Fatalf("pane process args still contain placeholder %q\n"+
			"  pane_pid: %d\n"+
			"  ps args: %q\n"+
			"  hint: Component F's respawn-pane swap appears to have NOT replaced "+
			"the placeholder command — F's ordering regression",
			forbiddenPlaceholder, panePID, args)
	}

	// --- Observable 3: show-options destroy-unattached == "off". ---
	//
	// tmux's show-options output shape is typically
	//   destroy-unattached off
	// but some tmux versions wrap values in matched outer quotes
	//   destroy-unattached "off"
	// and may include trailing whitespace. Parse robustly:
	//   1. Trim leading/trailing whitespace from the raw line.
	//   2. Strip the leading "destroy-unattached" key prefix.
	//   3. Trim whitespace from the remaining value.
	//   4. Strip matched outer quotes via tmuxout.StripMatchedOuterQuotes
	//      (the canonical helper for un-quoting tmux show-* values).
	//   5. Assert the resulting value equals "off" exactly.
	const optKey = "destroy-unattached"
	optRaw, err := h.Sock.TryRun("show-options", "-t", tmux.PortalSaverName, optKey)
	if err != nil {
		t.Fatalf("show-options -t %s %s: %v\n--- raw ---\n%q",
			tmux.PortalSaverName, optKey, err, optRaw)
	}
	optLine := strings.TrimSpace(optRaw)
	if !strings.HasPrefix(optLine, optKey) {
		t.Fatalf("show-options output missing %q key prefix\n--- raw ---\n%q\n--- trimmed ---\n%q",
			optKey, optRaw, optLine)
	}
	value := strings.TrimSpace(strings.TrimPrefix(optLine, optKey))
	value = tmuxout.StripMatchedOuterQuotes(value)
	if value != "off" {
		t.Fatalf("destroy-unattached parsed value = %q; want %q\n--- raw ---\n%q",
			value, "off", optRaw)
	}
}

// psArgsForPIDInline returns the `args` field for pid via
// `ps -o args= -p <pid>`. Inlined here (rather than importing from
// internal/tmux_test) because the helper there lives in a different
// _test package and is not cross-importable; replicating the ~3-line
// shell-out keeps this file self-contained.
//
// The function is robust across macOS and Linux: `ps -o args= -p <pid>`
// is POSIX-portable and produces the full argv on both platforms.
// Trailing whitespace is stripped so substring comparisons are clean.
func psArgsForPIDInline(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps -p %d: %w", pid, err)
	}
	return strings.TrimSpace(string(out)), nil
}
