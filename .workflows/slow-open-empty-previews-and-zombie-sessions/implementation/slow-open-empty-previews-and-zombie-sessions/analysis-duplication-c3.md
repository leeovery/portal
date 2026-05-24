# Duplication Analysis — Cycle 3 (independent re-scan)

STATUS: findings
FINDINGS_COUNT: 4

## Finding 1: pgrepPortalDaemons production adapter and PgrepPortalDaemons test helper are byte-identical bodies

SEVERITY: high

FILES:
- `internal/bootstrapadapter/orphan_sweep.go:75-109` (`pgrepPortalDaemons`)
- `internal/portaltest/pgrep.go:37-68` (`PgrepPortalDaemons`)

DESCRIPTION: Both functions execute `exec.Command("pgrep", "-fx", state.PortalDaemonArgvPattern).Output()`, switch on `*exec.ExitError` exit code 1 + empty stdout → `(nil, nil)`, then split/trim/Atoi each line. Control flow + parse loop are line-for-line equivalent. The test helper's docstring (added by T8-3) explicitly calls out the duplication. T8-3 added the portaltest helper but did NOT collapse the adapter to use it.

RECOMMENDATION: Promote the implementation to `internal/state` (canonical home — already owns `PortalDaemonArgvPattern`) as `state.PgrepPortalDaemons`. Both call sites become one-line forwarders.

EFFORT: small

## Finding 2: waitForDaemonPID and waitForAnyDaemonPID are functionally identical poll loops

SEVERITY: medium

FILES:
- `cmd/bootstrap/upgrade_path_integration_test.go:383-402` (`waitForDaemonPID`)
- `cmd/bootstrap/composition_e2e_harness_integration_test.go:421-440` (`waitForAnyDaemonPID`)

DESCRIPTION: Both helpers poll `state.ReadPIDFile(stateDir)` via `tmuxtest.PollUntil` until it equals an expected PID. Differences limited to constant pairs and diagnostic prose. `waitForAnyDaemonPID`'s docstring claims it is "distinct from waitForDaemonPID (which enforces an expected PID)" but its body absolutely does enforce `pid == orphanPID` — docstring lies.

RECOMMENDATION: Delete `waitForAnyDaemonPID`; have its caller use `waitForDaemonPID(t, stateDir, legitimateDaemonPID)` directly.

EFFORT: small

## Finding 3: readPortalLogSafe and readPortalLogSafeBootstrap are byte-identical across test packages

SEVERITY: medium

FILES:
- `cmd/state_daemon_self_supervision_integration_test.go:637-648` (`readPortalLogSafe`)
- `cmd/bootstrap/composition_e2e_self_eject_integration_test.go:422-439` (`readPortalLogSafeBootstrap`)

DESCRIPTION: Byte-identical 5-line `os.ReadFile(state.PortalLog(stateDir))` wrappers. The `Bootstrap` suffix exists solely because the two helpers live in different test packages. The duplication is explicitly documented with a "should consolidate" comment that was never actioned.

RECOMMENDATION: Promote `ReadPortalLogSafe(stateDir string) string` to `internal/portaltest`. Both call sites drop their local defs.

EFFORT: small

## Finding 4: Recurring `with{X}Fake` seam-swap scaffold across 9+ test helpers

SEVERITY: low

FILES (representative):
- `cmd/state_daemon_self_supervision_test.go:21,33,43`
- `cmd/state_daemon_test.go:417`
- `internal/state/daemon_lock_test.go:17,27,37,432,442`
- `internal/state/daemon_identity_test.go:12`

DESCRIPTION: Every helper is the identical 4-line scaffold. Each closes over a different package-private `var`, so cross-package extraction is structurally blocked.

RECOMMENDATION: No change for this work unit. Vigilance item only.

EFFORT: none
