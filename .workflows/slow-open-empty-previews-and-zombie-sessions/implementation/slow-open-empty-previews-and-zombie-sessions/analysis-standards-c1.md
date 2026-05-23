# Standards Analysis — Cycle 1

STATUS: clean
FINDINGS_COUNT: 0

SUMMARY: Implementation conforms to the specification and project conventions across Components A–G.

## Component A — Kill-Barrier Escalation

`escalateKillToSIGKILL` in `internal/tmux/portal_saver.go` enforces the load-bearing identity-check → SIGKILL adjacency invariant. Uses `killBarrierIdentifyDaemon` (= `state.IdentifyDaemon`) and emits WARN under `state.ComponentBootstrap` on non-IdentifyIsPortalDaemon outcomes. Post-SIGKILL poll at 50ms cadence for up to 1s budget.

## Component B — Bootstrap-Time Orphan Sweep

Step 4 inserted between `Set @portal-restoring` and `EnsureSaver`. Canonical pgrep form `pgrep -fx '^portal state daemon( |$)'`. Spec-mandated INFO log `"sweep: killed orphan daemon pid=%d"`. Best-effort: always returns nil; errors logged WARN and swallowed. Defensive self-PID skip. pgrep exit-1 + empty stdout treated as "no matches".

## Component C — Daemon Lock

Pre-acquire daemon.pid liveness check + post-flock fstat/stat inode cross-check + 3-attempt retry with 10ms sleep. `defaultDaemonRun` places `state.WritePIDFile` as the next statement after a successful `acquireDaemonLock` return; `TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire` AST test enforces.

## Component D — Per-tick Self-Supervision

`defaultDaemonTickLoop` extracted so probe runs before `tick`, outside `IsRestoringSet` short-circuit. `selfSupervisionHysteresisTicks = 3` with in-source measurement comment block. `osExit(0)` bypasses `daemonShutdownFunc`. INFO log format matches spec template. Both `>= 1` and `3 ≤ N ≤ 9` assertions present.

## Component E — Per-session Log-and-Continue

Typed sentinel `tmuxerr.ErrNoSuchSession` in dependency-free leaf package; re-exported from `internal/tmux` as identity-equal symbol. Substring matching confined to `wrapNoSuchSession`. Daemon-layer uses `errors.Is`. Per-session loop logs WARN + continues; pre-loop calls remain fail-fatal. Natural-churn vs anomalous discriminator correct.

## Component F — BootstrapPortalSaver reorder

`portalSaverPlaceholderCommand = "sh -c 'exec tail -f /dev/null'"`. Order: create-with-placeholder → SetSessionOption(destroy-unattached, off) → RespawnPane(daemon) → waitForSaverDaemonReady. Readiness barrier: 2s budget, 50ms cadence.

## Component G — Test Isolation Contract

New leaf `internal/portaltest` with `NewIsolatedStateEnv(t *testing.T) (env []string, stateDir string)`. `*testing.T` parameter structurally prevents production import. Build-tag-split for Stat_t. `Fingerprint` captures size + mtime + ctime + SHA-256 + symlink target. Audit deliverable enumerates every `exec.Command*` site.

## CLAUDE.md updates

11-step bootstrap sequence documented including SweepOrphanDaemons step 4. Test-isolation contract documented under "DI / testing pattern".

## Conventions

No t.Parallel in cmd-package tests. *Deps mutable seam pattern with t.Cleanup. errors.Is used for sentinel discrimination. Multi-%w error wrapping. Integration tests tagged //go:build integration.

## Note (not a finding)

Spec Component C wording "log WARN under ComponentDaemon and exit with status 1" is internally inconsistent (WARN vs "matching existing Error treatment"). `defaultDaemonRun` preserves existing Error-level treatment for non-`ErrDaemonLockHeld` wrapped errors. Defensible.
