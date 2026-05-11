AGENT: architecture
STATUS: findings
FINDINGS_COUNT: 4
SUMMARY: Four low-severity architectural refinements found; no high or medium issues. Both kill-barrier production call sites correctly route through `killSaverAndWaitForDaemonFn` (portal_saver.go:211 and :256); Step 2 HookRegistrar reliably installs `SetBarrierLogger` before Step 4 reaches the barrier; lock-fd retention is correctly wired via the `daemonLockFile` package var on the success path.

FINDINGS:

- FINDING: SetBarrierLogger nil-guard does not catch typed-nil *state.Logger boxed in BarrierLogger interface
  SEVERITY: low
  FILES: internal/tmux/portal_saver.go:108-113, internal/bootstrapadapter/adapters.go:75-78, cmd/state_common.go:18-28
  DESCRIPTION: `openNoRotateLogger()` returns a nil `*state.Logger` on its error path. That value flows into `HookRegistrar{Logger: logger}` and into `tmux.SetBarrierLogger(r.Logger)` at adapters.go:76. The guard `if l == nil { return }` in SetBarrierLogger only catches the untyped-nil case — a typed-nil `*state.Logger` boxed in BarrierLogger is a non-nil interface and bypasses the guard. `killBarrierLogger` then holds an interface backed by a nil receiver. No bug today because `(*state.Logger).Warn` calls `write` which has a `if l == nil { return }` guard — but that defense lives one method-dispatch away from the seam and is not load-bearing in the SetBarrierLogger contract. A future BarrierLogger implementer lacking nil-receiver safety would silently panic. The guard misleads readers into thinking SetBarrierLogger is bulletproof against nil inputs.
  RECOMMENDATION: Change the call site in `HookRegistrar.RegisterPortalHooks` to skip `SetBarrierLogger` when `r.Logger == nil` (concrete-type nil check DOES work for the typed pointer). Or document explicitly that the SetBarrierLogger guard only catches untyped-nil and that BarrierLogger implementations must be nil-receiver-safe.

- FINDING: BarrierLogger WARN uses literal "bootstrap" rather than state.ComponentBootstrap
  SEVERITY: low
  FILES: internal/tmux/portal_saver.go:177, internal/tmux/hooks_register.go:221,229
  DESCRIPTION: Already covered in standards analysis. Cross-referenced here for architecture: the literal would be marginally fragile if the constant value drifted; production logs would then diverge across call sites. The fix is mechanical.
  RECOMMENDATION: Reference `state.ComponentBootstrap` directly at the WARN site; optionally clean up the two pre-existing hooks_register.go occurrences.

- FINDING: cmd.daemonLockFile package var leaks open fds across tests that exercise the real AcquireDaemonLock
  SEVERITY: low
  FILES: cmd/state_daemon.go:61,264, cmd/state_daemon_test.go:412-417,47,65,88,112,151,174,196,355
  DESCRIPTION: `daemonLockFile` is a package-level `*os.File` retained for the daemon's process lifetime — load-bearing for production. In tests, every successful RunE path that goes through the real `state.AcquireDaemonLock` (tests that don't stub `acquireDaemonLock`) sets the var and leaves it pointing at an open fd in a now-deleted t.TempDir. Only `withDaemonLockFileReset` clears the var. The lock/retain tests at lines 419, 451, 601 correctly call it, but tests at 47, 65, 88, 112, 151, 174, 196, 355 do not — each leaves the var holding the most recently acquired lock fd. No production-correctness risk (test binary is short-lived; lockfile is in a deleted TempDir), but the asymmetry creates a maintenance trap: a future test asserting something about `daemonLockFile` without its own reset call would see leaked state.
  RECOMMENDATION: Call `withDaemonLockFileReset` in every cmd-package test that runs the daemon's RunE through the real lock path. Consistent with seam-reset discipline elsewhere.

- FINDING: Kill-barrier first probe inside ticker loop is delayed by one pollInterval after KillSession
  SEVERITY: low
  FILES: internal/tmux/portal_saver.go:164-185
  DESCRIPTION: `killSaverAndWaitForDaemon` issues `KillSession` at line 165, then `time.NewTicker(killBarrierPollInterval)` at 167, then `for range ticker.C` at 171. `time.NewTicker` does not fire immediately — the first tick arrives after `pollInterval`. So after the kill, the helper sleeps a full poll interval before checking whether the prior daemon has exited, even when SIGHUP propagation is sub-millisecond. The pre-loop `killBarrierIsAlive(priorPID)` check at line 158 fires BEFORE the kill, not after. Production defaults (50ms / 5s) make it a 50ms tax on every recycle — small but consistent on the bootstrap critical path.
  RECOMMENDATION: Probe `killBarrierIsAlive` once immediately after KillSession before entering the ticker loop, OR restructure the loop to `for { check; sleep }` instead of relying on a ticker that swallows the first interval. Defensible to leave as-is given the production cost is small — but if so, document the deliberate one-interval delay at the call site.
