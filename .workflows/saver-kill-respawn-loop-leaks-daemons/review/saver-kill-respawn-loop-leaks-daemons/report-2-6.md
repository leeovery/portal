TASK: Fault-injection integration test — lock-loser daemon's pane exit destroys _portal-saver (cascade regression guard)

STATUS: Complete

SPEC CONTEXT: Phase 1 eliminates the *natural* trigger for the lock-contention cascade. The chain (lock-loser daemon exits → pane process exits → tmux destroys _portal-saver → next SetSessionOption returns `no such session`) remains reachable only via forced contention. Spec §Testing Requirements > Integration tests #3 mandates this as a permanent regression guard using a sentinel goroutine holding daemon.lock for the test duration.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/portal_saver_integration_test.go:406-514 — TestBootstrapPortalSaver_LockContention_CascadeChainReachable
- Notes:
  - Sentinel goroutine pattern (lines 423-435) uses closed-on-ready chan as the synchronisation primitive (no time.Sleep), matching the plan's edge case.
  - t.Cleanup to close the sentinel fd registered immediately after acquisition (line 443) — before any code path that could t.Fatal.
  - Skip patterns (tmuxtest.SkipIfNoTmux + portalbintest.StagePortalBinary) mirror the two existing integration tests in the same file.
  - PORTAL_STATE_DIR env var propagation ensures the daemon contends against the same daemon.lock path the sentinel holds.
  - Defensive holder session _cascade-holder (lines 461-463) keeps the tmux server alive after _portal-saver destruction so the final SetSessionOption hits "no such session" rather than "no server running" — an implementation refinement not explicitly in the plan but materially correct.
  - Race handling at lines 472-477: `_ = tmux.BootstrapPortalSaver(...)` documents that BootstrapPortalSaver may itself return the cascade-chain error on a fast race; both outcomes still exercise the chain.
  - Signature observation: plan text mentions BootstrapPortalSaver(client, stateDir, currentVersion) but the actual production signature is BootstrapPortalSaver(c, stateDir) (no currentVersion). The test correctly uses the real signature.

TESTS:
- Status: Adequate
- Coverage: All four assertions from spec §Integration tests #3 are pinned in order:
  1. Daemon exit within 1s (50ms tick) — waitForDaemonNotAlive at line 484
  2. has-session failure within 2s (100ms tick) — waitForSessionAbsent at line 493
  3. SetSessionOption error contains `exit status 1` — line 508
  4. Same error contains `no such session` — line 511 (asserted independently from #3, ruling out unrelated exit-1 false positives)
- Notes: Regression-watch suites are implicitly covered by go test ./... rather than directly invoked.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(), uses package-level state.* helpers, mirrors existing integration tests.
- SOLID: Good. New helpers waitForDaemonNotAlive and waitForSessionAbsent are single-purpose extensions of the existing tmuxtest.PollUntil pattern.
- Complexity: Low.
- Modern idioms: Yes. t.Cleanup, t.TempDir, closed-on-ready chan idiom.
- Readability: Very good. Top-of-function doc comment clearly explains why fault injection is needed post-fix.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] _cascade-holder uses `sleep 60` (line 461) as its pane command. Comment at line 458 mentions `sleep infinity`. `sleep infinity` would be marginally more defensive.
- [idea] The sentinel goroutine writes sentinelErr / sentinelFile without a mutex, relying on the close(ready) happens-before edge. Technically correct under Go's memory model, but a one-line comment noting "memory ordering established by close(ready)" would help.
- [idea] Regression-watch suites listed in the plan are not enumerated in a comment near the new test. Adding a brief reference comment naming the three packages would make the linkage discoverable.
