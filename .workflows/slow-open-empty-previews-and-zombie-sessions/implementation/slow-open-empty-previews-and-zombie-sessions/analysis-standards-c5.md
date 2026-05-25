# Standards Analysis — Cycle 5 (Phase 11 re-scan)

AGENT: standards
STATUS: clean
FINDINGS_COUNT: 0
FINDINGS: none

## Phase 11 spec/convention verification

**T11-1** (`cmd/state_daemon_test.go`): Test renamed to `TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure`. Sets `PORTAL_LOG_LEVEL=warn`, asserts exactly one log line matching both `"WARN"` and `"acquire daemon lock"`. Matches Component C's WARN emission contract and is symmetric with `TestStateDaemon_EmitsWarnOnLockContention`.

**T11-2** (`cmd/bootstrap/composition_e2e_scrollback_stability_integration_test.go`): `snapshotScrollbackPaths` now returns `(map[string]struct{}, bool dirExists)`. Caller fatals on `!dirExists` with `"scrollback dir does not exist: ..."` and on `len(baseline) == 0` with the plan-specified diagnostic. File-header bullets explicitly mark empty baseline and missing dir as regression signals; no residual "empty set is a valid baseline" claim survives.

**T11-3** (spec amendment): Component F bullet 3 at `specification.md:391` rewritten to "Lock-loser cascade is quiet — no `no such session` log noise" with rationale and the new opt-in note at line 394. Implementation assertion shape (`assertNoNoSuchSessionEntries` in `internal/tmux/portal_saver_endstate_integration_test.go`) matches the log-noise-absence contract.

The line-365 "session persists for the next bootstrap to evaluate" residual the reviewer flagged is unchanged by Phase 11; covered by the architecture finding for this cycle. Not raised here to avoid duplication.

**T11-4** (`cmd/state_daemon_test.go`): Comment rewritten to describe the post-T7-5 `acquireDaemonLock → WritePIDFile → WriteVersionFile` ordering and short-circuit semantics. Symmetric `os.Stat` assertion on `daemon.version` added, paralleling the `daemon.pid` invariant.

## Conventions (carried forward from prior c5)

- No `t.Parallel()` in cmd-package tests
- `*Deps` mutable-seam pattern with `t.Cleanup`
- `%w` wrapping; integration tests carry `//go:build integration`
- Leaf-package discipline preserved
- 11-step bootstrap sequence documented consistently

## Components A–G

All spec contracts verified intact in prior c5 (carried forward): identity-check→SIGKILL adjacency / 50ms cadence / 1s budget (A); canonical pgrep + literal INFO + nil-return contract (B); pre-check + post-flock fstat/stat cross-check + 3×10ms retry + WARN log level (C); probe-before-tick + N=3 hysteresis + literal INFO + osExit(0) bypassing daemonShutdownFunc (D); typed `tmuxerr.ErrNoSuchSession` sentinel (E); placeholder command + ordering + readiness barrier (F); leaf `internal/portaltest` with `*testing.T` parameter (G).
