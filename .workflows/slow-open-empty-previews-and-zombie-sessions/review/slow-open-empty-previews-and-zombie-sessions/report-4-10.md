TASK: 4-10 — Composite integration test: A+B+C converge to pgrep -fxc == 1 within 6 s

STATUS: Complete

SPEC CONTEXT: Spec § Composite End-to-End Verification (Phase-4 subset) — 3-daemon reporter scenario converges to 1 within 6s, scrollback stable across 10×1s, C pre-check verifiable from fresh process.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/composition_abc_integration_test.go` (1-308); `//go:build integration` (line 1)
- `portaltest.IsolateStateForTest` (123) with backstop fingerprint-diff; `portalbintest.StagePortalBinary` (121)
- Pre-state barrier `waitForPgrepCount(t, 3, 3s)` (155) with per-orphan PID/alive/pgrep-snapshot diagnostic — prevents silent N=2→1 passes
- Bootstrap shape: direct adapter form (B via `NewOrphanSweeper.SweepOrphanDaemons` then idempotent `BootstrapPortalSaver`); file-header (36-42) defends choice
- 6s budget measured from `start := time.Now()` (181) with remaining-budget arithmetic (195)
- Survivor identity (220-236) re-reads pane_pid post-bootstrap with diagnostic naming both regression hypotheses
- Scrollback stability (254-285) via `SnapshotStateDir`/`DiffFingerprints` — reuses Task 4-2 infrastructure; empty-stays-empty explicit
- Fresh-process `AcquireDaemonLock` (295-307) uses `errors.Is(acquireErr, state.ErrDaemonLockHeld)`; defensive close of returned fd
- Spec deviation documented (44-53): NOT writing orphan PID to daemon.pid (would defeat C pre-check)
- Per-orphan PORTAL_STATE_DIR tempdirs to keep 3 daemons alive long enough for pgrep N=3 before C convergence

TESTS:
- Status: Adequate
- Coverage: pre-state N=3, post-state N=1 within 6s, survivor==saver_pid, 10×1s scrollback stability, ErrDaemonLockHeld
- Single top-level test running 4 assertions sequentially (defensible: shared expensive setup, first-failure short-circuit)
- `skipIfNoPgrep` + `tmuxtest.SkipIfNoTmux`

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel` (documented line 73)
- Complexity: Low; linear; no goroutines
- Modern idioms: `errors.Is`, `time.Since`, `t.Setenv`
- Readability: Excellent; file-header exemplary

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Split 4 spec-named assertions into `t.Run` subtests for better failure reporting
- [idea] `compositionPreStateTimeout` mirrors sibling const — risk of silent drift; extract to shared
- [idea] "no orphan-PID written to daemon.pid" deviation not cross-linked from spec; optional footnote
