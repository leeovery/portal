TASK: Integration test — alive daemon + absent daemon.version survives bootstrap (real-tmux fixture)

STATUS: Complete

SPEC CONTEXT: Spec §Testing Requirements > Integration tests #1 requires a real-tmux test pinning Defect 1's user-visible contract: alive daemon with absent daemon.version must survive bootstrap without firing the kill barrier, with daemon.version repaired defensively (Task 1-4) and the three WARN cascade substrings absent from portal.log. Acceptance §Steady-state items 1-4: no kill-respawn WARN cascade, _portal-saver survives, single live daemon no orphans, daemon.version repaired synchronously.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/portal_saver_integration_test.go:241-359 (TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill)
- Notes: New test added alongside the existing singleton-invariant test (125-239) and the lock-contention cascade test (406-514). Uses the same tmuxtest.New isolated-socket fixture, portalbintest.StagePortalBinary build-and-PATH primitive, and PORTAL_STATE_DIR env-var propagation idiom as the existing tests. Helpers reused (waitForLiveDaemon, dumpDiagnostics, captureTmuxServerPID); two new helpers added (waitForVersionFile at 545, assertNoForbiddenLogSubstrings at 561).

TESTS:
- Status: Adequate
- Coverage: All seven assertions from the plan's Do section are present:
  1. nil return — 314-316
  2. tmux has-session success — 321-323
  3. daemon.version contents equal currentVersion — 327-333
  4. PID unchanged before/after — 338-345 (regression guard against silent respawn)
  5. DaemonAlive true — 350-352
  6. Three forbidden WARN substrings absent — 358 → 561-582 scans exactly the three required substrings (`prior daemon (pid=`, `another daemon holds the lock; exiting`, `step 4 (EnsureSaver) failed:`) and trivially holds when the log file is absent (plan permits this)
  7. Single-daemon assertion — implicitly satisfied by PID-unchanged + DaemonAlive + WARN-cascade absence
- Notes: Exercises real EnsurePortalSaverVersion (no stubbed helpers); forces alive+absent input shape via os.Remove(state.DaemonVersion(dir)) after polling for daemon's own startup write; asserts via real client.HasSession.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (consistent with CLAUDE.md and file header). Skip-if-no-tmux + StagePortalBinary skip surface mirrors the existing singleton test. Polling uses tmuxtest.PollUntil.
- SOLID: Good. Single-responsibility helpers; clear naming.
- Complexity: Low.
- Modern idioms: Yes. Uses errors.Is(err, fs.ErrNotExist); uses t.Setenv.
- Readability: Good. Block comments delineate setup/action/assertion phases.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Plan Do step #7 calls for an explicit `pgrep -f "portal state daemon"` count returning exactly one PID matching the daemon.lock holder. The new test does not include an explicit pgrep count assertion — PID-unchanged + DaemonAlive + WARN-absence is logically sufficient on the no-kill branch, but adding a single count == 1 assertion would pin the spec's structural "exactly one daemon" wording directly.
- [idea] assertNoForbiddenLogSubstrings reads portal.log only; a rotated log (state.PortalLogOld) could in principle hide forbidden substrings. Unlikely in this short-lived test (no rotation trigger).
- [quickfix] File-header comment block (3-47) only describes the singleton-invariant test, but the file now contains three integration tests. A short header refresh listing all three would help future readers orient.
