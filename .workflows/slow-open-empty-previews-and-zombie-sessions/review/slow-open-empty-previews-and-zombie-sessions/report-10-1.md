TASK: 10-1 — Promote orphan-daemon spawn + reap-cleanup helpers to internal/portaltest

STATUS: Complete

SPEC CONTEXT: c4 duplication finding — orphan-daemon spawn + reap-cleanup pattern correctly factored in `bootstrap_test` but reimplemented verbatim in two `internal/tmux/*_integration_test.go` files. Promotion to `internal/portaltest` collapses ~50 LOC and centralises triplicated rationale.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/portaltest/spawn_daemon.go` (1-101)
  - `SpawnIsolatedDaemon(t, envSlice) (*exec.Cmd, string)` at 55-67 — pins fresh `t.TempDir()` per call, unqualified `"portal"` argv[0], wires RegisterSubprocessCleanup
  - `RegisterSubprocessCleanup(t, cmd) <-chan struct{}` at 88-100 — reaper goroutine + `t.Cleanup{Kill; <-reaped}`, returns reaped channel
- Five migration sites confirmed:
  - `cmd/bootstrap/composition_e2e_harness_integration_test.go:197,224`
  - `cmd/bootstrap/composition_abc_integration_test.go:147-148`
  - `cmd/bootstrap/orphan_sweep_integration_test.go:135-136,301`
  - `cmd/bootstrap/upgrade_path_integration_test.go:160,292` (RegisterSubprocessCleanup only)
  - `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:192` (shared stateDir — cannot use SpawnIsolatedDaemon; comment 177-183 documents)
  - `internal/tmux/portal_saver_endstate_integration_test.go:330` (same shared-stateDir rationale 313-322)
- Legacy local definitions fully removed

TESTS:
- Status: Adequate (per task spec — helpers exercised by migrated tests)
- All five migrated tests are integration-tagged
- No `spawn_daemon_test.go` exists (matches acceptance criterion)

CODE QUALITY:
- Project conventions: Followed; `*testing.T` first arg
- SOLID: Good; SpawnIsolatedDaemon composes RegisterSubprocessCleanup
- Complexity: Low
- Modern idioms: goroutine + channel coordination to avoid double-Wait race; `t.Helper()`/`t.Cleanup`
- Readability: Good; load-bearing rationale (darwin comm argv[0], SIGKILL belt-and-braces, reap-vs-Wait) exhaustively documented in godoc

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Two `internal/tmux` integration tests duplicate 6-line spawn envelope; if third shared-stateDir spawn site appears, consider `SpawnDaemonInSharedStateDir(t, envSlice, stateDir)` variant
