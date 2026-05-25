TASK: 4-2 — Add no-final-flush snapshot test for escalation-killed orphans

STATUS: Complete

SPEC CONTEXT: Component A — orphan must not run final captureAndCommit/gcOrphanScrollback on its way out. Direct SIGKILL bypasses `defaultShutdownFlush`. Snapshots byte-equal across 200 ms post-exit settle window.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go` (1-346) — integration test
  - `internal/portaltest/fingerprint.go` (42-244) — `Fingerprint`, `SnapshotStateDir`, `DiffFingerprints`, `FormatDelta`
  - `internal/portaltest/spawn_daemon.go` (88-100) — `RegisterSubprocessCleanup`
  - `internal/portaltest/isolated_env.go:56` — `IsolateStateForTest`
- File name `kill_barrier_escalation_no_final_flush_integration_test.go` is more descriptive than spec's `portal_saver_escalation_integration_test.go`
- Integration "tag" is `tmuxtest.SkipIfNoTmux(t)` rather than `//go:build` tag (canonical mechanism in `internal/tmux/`)
- Choreography matches spec: isolated env + tmux fixture + `work` capture-target session; orphan spawned as `portal state daemon` against shared PORTAL_STATE_DIR (divergent-view by construction); `state.TouchSaveRequested` forces tick; poll until ≥1 `.bin`; direct `syscall.Kill(SIGKILL)` tolerating ESRCH; reap channel; exact `time.Sleep(200 * time.Millisecond)`; post-snapshot + `DiffFingerprints` equality
- Snapshots target `state.ScrollbackDir(stateDir)` per spec

TESTS:
- Status: Adequate
- Coverage:
  - `TestKillBarrierEscalation_NoScrollbackDeltaIn200msPostExit` — primary spec acceptance E2E
  - Meta-test "snapshot diff fails on deliberately-modified `.bin`" satisfied by `internal/portaltest/fingerprint_diff_test.go` (covers every Field channel)
  - Pre-snapshot double-guarded: `countBinFiles >= 1` poll AND `hasAnyBin(pre)` post-snapshot check; fails loudly with directory listing on precondition violation

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `IsolateStateForTest` with subprocess env propagation
- SOLID: Good; single linear choreography with small helpers
- Complexity: Low
- Modern idioms: `maps.Keys` + `slices.Sorted` (Go 1.23+), `errors.Is(err, syscall.ESRCH)`
- Readability: Excellent; extensive file-header explains rationale, divergent-view, ESRCH race window, empirical timing margins

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Spec/plan reference `NewIsolatedStateEnv` and filename `portal_saver_escalation_integration_test.go`; shipped names diverge — one-line plan/CLAUDE.md note on rename would prevent future grep miss
- [idea] "FAILS against hypothetical SIGTERM-with-marker variant" criterion not directly exercised by counterfactual; acceptable per spec framing
