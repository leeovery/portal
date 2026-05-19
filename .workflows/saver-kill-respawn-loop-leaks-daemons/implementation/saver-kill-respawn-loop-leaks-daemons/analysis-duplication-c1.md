STATUS: findings
FINDINGS_COUNT: 6

AGENT: duplication

FINDINGS:

- FINDING: Parallel version-decision predicates in portal_saver.go
  SEVERITY: high
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:334-358 (shouldKillSaverOnVersionDecision), /Users/leeovery/Code/portal/internal/tmux/portal_saver.go:390-401 (portalSaverVersionMismatch)
  DESCRIPTION: Two predicates encode the same version-comparison rules independently. `portalSaverVersionMismatch` is the legacy "any-error or dev-or-mismatch → true" predicate; `shouldKillSaverOnVersionDecision` is the new caller-layer predicate that reimplements all three dev/empty short-circuits and the readErr handling inline. Comments call out the duplication: "The dev short-circuit and 'read-error-is-mismatch' behaviours of the predicate are reproduced inline here, byte-equivalent in semantics" (portal_saver.go:271-275). Parallel-computation anti-pattern — two implementations of the same concept must be kept in sync by hand. The `TestPortalSaverVersionMismatch_PredicateMatrix` test pins the legacy predicate so divergence is not auto-detected.
  RECOMMENDATION: Derive `shouldKillSaverOnVersionDecision` from `portalSaverVersionMismatch`. Concrete shape: keep `portalSaverVersionMismatch` as the single predicate; make the new function a thin wrapper carving out only the new exception — `if errors.Is(readErr, state.ErrVersionFileAbsent) { return false }; return portalSaverVersionMismatch(stored, currentVersion, readErr)`. Drops ~20 LOC and eliminates the byte-equivalence contract.

- FINDING: Six near-identical polling-with-timeout helpers in integration tests
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon_integration_test.go:576-594 (waitForDaemonAlive), /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:547-559 (waitForDaemonNotAlive), /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:566-578 (waitForSessionAbsent), /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:584-594 (waitForVersionFile), /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:672-685 (waitForLiveDaemon), /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:694-707 (waitForNewLiveDaemon)
  DESCRIPTION: Six helpers share the same `deadline := time.Now().Add(timeout); for time.Now().Before(deadline) { if <cond> { return ... }; time.Sleep(tick) }; t.Fatalf(...)` skeleton. Variation is in the predicate, return shape, and fatal message. Loop wiring, deadline arithmetic, and t.Fatal-on-timeout idiom are duplicated six times.
  RECOMMENDATION: Extract a single generic poll helper, e.g. `func pollUntil(t *testing.T, timeout, tick time.Duration, cond func() bool) bool`. Each call site collapses to a one-liner that handles its return/fatal shape locally. Can live in `internal/tmuxtest` (existing test-only helper package).

- FINDING: Repeated portal-binary build + PATH-prepend prelude in integration tests
  SEVERITY: medium
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon_integration_test.go:181-189, /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:134-151, /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:287-294, /Users/leeovery/Code/portal/internal/tmux/portal_saver_integration_test.go:427-434
  DESCRIPTION: Four real-tmux integration tests open with a near-identical ~8-line preamble: `t.TempDir()` for binDir, `restoretest.BuildPortalBinary` skip-on-failure, `t.Setenv("PATH", ...)`, `exec.LookPath("portal")` skip-on-failure. The pattern is structural and the cost of accidental divergence is silent test fragility.
  RECOMMENDATION: Extract `stagePortalBinary(t *testing.T) string` (returns binDir, handles skip-on-build-failure and skip-on-LookPath-failure, calls `t.Setenv("PATH", ...)`). Either in `restoretest` (existing test-only helper package) or as a local helper.

- FINDING: Repeated sentinel-PrevIndex fixture + unchanged-pointer assertion in captureAndCommit tests
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:880-908, /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:967-1010, /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:1056-1117, /Users/leeovery/Code/portal/cmd/state_daemon_run_test.go:1162-1209
  DESCRIPTION: Four tests construct an identical sentinelPrev fixture and apply the same post-call assertions ("PrevIndex pointer unchanged from sentinel", "sessions.json must not exist on disk"). Fixture build-up consumes 6-8 lines per test; assertion block another 4-6.
  RECOMMENDATION: Extract `sentinelIndex(name string) *state.Index` and `assertNoCommit(t, deps, sentinel)` helpers. The "replaced" variant flips to an `assertCommitReplacedPrev` helper.

- FINDING: Repeated "kill-session must precede new-session" order-check block
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:200-215, /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:293-309, /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:668-683, /Users/leeovery/Code/portal/internal/tmux/portal_saver_test.go:1521-1535
  DESCRIPTION: Four tests contain an identical ~15-line scan of `mock.Calls` to capture the first kill-session index and the first new-session index, then assert kill precedes new. Block is mechanical and identical across copies.
  RECOMMENDATION: Extract `assertKillBeforeNew(t *testing.T, calls [][]string)` and replace four copies with a single call each.

- FINDING: Repeated ctx.Done() default-select pattern inside captureAndCommit
  SEVERITY: low
  FILES: /Users/leeovery/Code/portal/cmd/state_daemon.go:139-143, /Users/leeovery/Code/portal/cmd/state_daemon.go:157-161, /Users/leeovery/Code/portal/cmd/state_daemon.go:174-178
  DESCRIPTION: The same four-line `select { case <-ctx.Done(): return nil; default: }` block appears verbatim at three observation points. Borderline on Rule of Three; spec structurally guarantees three checks.
  RECOMMENDATION: Optional. Extract `func ctxCancelled(ctx context.Context) bool`. Three 4-line blocks → three one-line guards while keeping per-site comments. Low priority.

SUMMARY: One high-severity finding in production code: `shouldKillSaverOnVersionDecision` and `portalSaverVersionMismatch` independently encode the same dev/empty/error rules — the comment explicitly admits "byte-equivalent in semantics" reimplementation. Five test-level duplication patterns each present extraction opportunities.
