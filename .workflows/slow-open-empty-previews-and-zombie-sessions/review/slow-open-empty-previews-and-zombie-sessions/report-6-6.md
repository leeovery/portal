TASK: 6-6 — Assert Component D self-eject in live context after external saver-pane kill

STATUS: Complete

SPEC CONTEXT: Spec § Composite step 8 — Component D behaviour in full A+B+C+D+E+F live state. Per-tick probe; hysteresis N ≥ 1; eject via `os.Exit(0)` (no final flush); stale daemon.pid retained.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/composition_e2e_self_eject_integration_test.go` (433 lines)
- `//go:build integration`; consumes `setupCompositeHarness`
- Drives production bootstrap slice (SweepOrphanDaemons + BootstrapPortalSaver)
- Mismatch mechanism: `tmux new-window` — file-header (19-53) compares against `break-pane`, `move-pane`, `respawn-pane -k`, `kill-session`, explaining `new-window` is structurally equivalent (placeholder becomes active pane → `SaverPanePIDOrAbsent` returns placeholder PID → mismatch) with single tmux call. Confirmed via `internal/tmux/saver_pane_pid.go:91` (`list-panes -t =_portal-saver` enumerates active-window panes only). Deviation from plan's `break-pane` rational and explicit
- Constants mirror production
- Pre-eject structural sanity checks (267-303)
- Three end-state assertions: scrollback bytes-identical (349-361); daemon.pid retained + value matches survivor (370-387); portal.log contains marker (394-401)

TESTS:
- Status: Adequate
- All five plan-listed tests covered by single `TestCompositeBootstrap_ExternalSaverKillTriggersSelfEject`
- NOTE: plan's `ProcessState.Exited() == true AND ExitCode() == 0` NOT directly performed — test polls `kill(pid, 0) == ESRCH`; process owned by tmux, acknowledged in `pollForPIDExit` comment (410-414). Compensated by three-assertion triad (SIGHUP would trigger defaultShutdownFlush perturbing scrollback; INFO marker only on `os.Exit(0)` path)
- Rich failure diagnostics

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good
- Complexity: Low; narrative test, each step gated on prior
- Modern idioms: `t.Setenv`, structured diagnostics
- Readability: Excellent; spec citations per assertion
- Performance: ~6-8s runtime

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan's `ProcessState.Exited()` criterion structurally untestable when daemon owned by tmux; codify the surrogate triad in spec § Component D for future re-tests
- [quickfix] Compile-time guards `var _ = syscall.SIGKILL` (431) and `var _ = errors.Is` (432) anchor imports no longer directly referenced; can drop
- [idea] Constants duplicated across files; exporting `cmd.SelfSupervisionHysteresisTicks` would eliminate mirror-drift risk
