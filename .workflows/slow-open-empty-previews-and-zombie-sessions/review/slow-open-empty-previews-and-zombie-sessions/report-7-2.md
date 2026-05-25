TASK: 7-2 — Collapse spawnOrphanDaemonIsolated and spawnOrphanDaemonIsolatedNamed

STATUS: Complete (exceeded — promoted to internal/portaltest)

SPEC CONTEXT: analysis-duplication-c1.md F2 flagged the two helpers as line-for-line identical in package bootstrap_test, differing only in return signature. Planning task 7-2 called for local Named-superset collapse; later analysis cycle 4 (c4 F1) upgraded scope.

IMPLEMENTATION:
- Status: Implemented (broader than originally planned, fully subsumes 7-2)
- Location:
  - `internal/portaltest/spawn_daemon.go:55, 88` — canonical `SpawnIsolatedDaemon` + `RegisterSubprocessCleanup`
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:197, 224`
  - `cmd/bootstrap/composition_abc_integration_test.go:147-148`
  - `cmd/bootstrap/orphan_sweep_integration_test.go:135-136`
  - `cmd/bootstrap/upgrade_path_integration_test.go:160, 292` (RegisterSubprocessCleanup direct, shared-stateDir)
  - `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:184-192` (shared-stateDir inline + RegisterSubprocessCleanup; rationale comment points back to spawn_daemon.go)
  - `internal/tmux/portal_saver_endstate_integration_test.go:323-330` (same pattern)
- Grep for `spawnOrphanDaemon` in code returns no matches outside historical analysis docs
- Two inline-spawn sites in internal/tmux have load-bearing reasons (shared stateDir required by lock-loss / scrollback-snapshot semantics), documented in 10+ line rationale comments

TESTS:
- Status: Adequate (test-helper task — no direct test, exercised by 6+ integration test files)
- Per c5 closeout and prior reviews (4-2, 4-5, 4-9, 6-1) consumers pass

CODE QUALITY:
- Project conventions: Followed; `*testing.T` first arg enforces test-only structurally
- SOLID: Good; `SpawnIsolatedDaemon` composes `RegisterSubprocessCleanup`; shared-stateDir bypass uses Register directly — clean orthogonality
- Complexity: Low; each function <15 LOC
- Modern idioms: `t.Helper()`, `t.TempDir()`, `t.Cleanup`, channel-based reap, `append([]string{}, ...)` for env copy
- Readability: Excellent; godoc exemplary documenting every load-bearing detail (unqualified `portal` argv[0] for darwin comm match, PORTAL_STATE_DIR-per-call, when NOT to use, reap-vs-Wait deadlock avoidance, SIGKILL belt-and-braces)

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] "Shared stateDir" inline-spawn pattern now in 4 sites (upgrade_path×2, kill_barrier, portal_saver_endstate) with verbatim 4-line body; could fold into `SpawnDaemonInStateDir(t, envSlice, stateDir)`; c5 considered and rejected further consolidation
- [idea] internal/tmux test files now reach into internal/portaltest — fine but renames in portaltest must propagate
