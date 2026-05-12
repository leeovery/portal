# Review Report — Task 2.3

TASK: Real-tmux integration test asserts singleton invariant after recycle

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- New file `internal/tmux/portal_saver_integration_test.go` with `TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle`.
- `tmuxtest.SkipIfNoTmux(t)` and `t.Skip` for missing portal binary.
- `t.TempDir()` per-test isolation.
- `tmuxtest.New(t, "ptl-saver-")` isolated tmux server.
- Two `EnsurePortalSaverVersion` calls — `"v-test-1"`, then `state.WriteVersionFile(dir, "v-test-0-old")`, then `"v-test-1"` again.
- No new seam for `portalSaverVersionMismatch`.
- `pgrep -P <server_pid> -f 'portal state daemon'` returns exactly one line after both calls return.
- Diagnostic dump on failure (pgrep output, daemon.pid, daemon.version, server PID).
- No `t.Parallel()`.

SPEC CONTEXT:
Spec §"Test Strategy → Integration test — singleton invariant under real tmux" and §"Acceptance Criteria → Singleton invariant" require a real-tmux fixture exercising both back-to-back `EnsurePortalSaverVersion` calls (one must trigger a recycle via real on-disk version mismatch with no new seam). Load-bearing test composing Phase 1 (flock) + Phase 2 (kill barrier).

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tmux/portal_saver_integration_test.go`
- Notes:
  - Test function at line 124; `tmuxtest.SkipIfNoTmux(t)` at line 125.
  - `restoretest.BuildPortalBinary(binDir)` builds portal into `t.TempDir`; `t.Setenv("PATH", ...)`; `exec.LookPath("portal")` verifies resolvability.
  - `t.Setenv("PORTAL_STATE_DIR", dir)` ensures the tmux-spawned daemon resolves the same stateDir.
  - First `EnsurePortalSaverVersion("v-test-1")` at line 164; `sock.WaitForSession` at 167.
  - `captureTmuxServerPID` (257-265) uses `display-message -p '#{pid}'`.
  - `waitForLiveDaemon` polls `daemon.pid` until live, bounded at 5s.
  - `state.WriteVersionFile(dir, "v-test-0-old")` forces real version mismatch directly on disk. No new test seam.
  - Second `EnsurePortalSaverVersion("v-test-1")` routes through real `portalSaverVersionMismatch` + real kill barrier.
  - Spec-mandated pgrep assertion: `postRecycleServerPID := captureTmuxServerPID(t, sock)` (line 242). `countDaemonChildren` (273-292) runs `pgrep -P <serverPID> -f 'portal state daemon'` and asserts count == 1.
  - No `t.Parallel()`.

TESTS:
- Status: Adequate
- Coverage: The test is the deliverable — exercises full Phase 1 + Phase 2 composition end-to-end.
- Notes:
  - Defense-in-depth assertion shape (alive-pair + pgrep) slightly exceeds spec's strict "pgrep count == 1" requirement, but justified — alive-pair catches "recycle never happened" while pgrep catches "recycle happened but left an orphan".
  - Optional `BarrierTimeoutPath` sub-test correctly omitted per plan's "may be omitted" allowance.

CODE QUALITY:
- Project conventions: Followed. Naming convention matches `hooks_migration_test.go`. Package is `tmux_test` (external).
- SOLID: Helpers each single responsibility.
- Complexity: Low. Linear flow with bounded polls.
- Modern idioms: `t.Setenv`, `t.TempDir`, `errors.As`, `strings.Builder`.
- Readability: Excellent. Extensive doc comments explain rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] `waitForNewLiveDaemon` short-circuits on `pid != prior && IsProcessAlive(pid)`. Subsequent `state.IsProcessAlive(priorPID)` check at line 210 is the actual singleton gate, so early return is benign — a one-line comment would aid future maintainers.
- [idea] Test relies on env inheritance through `tmux → daemon`. If `tmuxtest.New` were ever made explicit-env, this test would silently lose its PORTAL_STATE_DIR.
- [idea] `countDaemonChildren` hardcodes argv string `"portal state daemon"`. A future rename of `portalSaverCommand` would silently make this test pass with count=0.
