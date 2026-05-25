# Duplication Analysis — Cycle 4

AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 1

## Finding 1: Orphan `portal state daemon` spawn + reap-cleanup pattern duplicated across two test packages

SEVERITY: medium

FILES:
- `/Users/leeovery/Code/portal/cmd/bootstrap/composition_e2e_harness_integration_test.go:382-394` (`spawnOrphanDaemonIsolated` — canonical helper inside `bootstrap_test`)
- `/Users/leeovery/Code/portal/cmd/bootstrap/orphan_sweep_integration_test.go:365-381` (`registerSubprocessCleanup` — the cleanup half, also `bootstrap_test`)
- `/Users/leeovery/Code/portal/cmd/bootstrap/upgrade_path_integration_test.go:150-158` (inline v(N) daemon spawn + `registerSubprocessCleanup` call — same package, OK)
- `/Users/leeovery/Code/portal/internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go:176-207` (inline orphan spawn + reap-goroutine + Cleanup Kill, package `tmux_test`)
- `/Users/leeovery/Code/portal/internal/tmux/portal_saver_endstate_integration_test.go:312-334` (inline seeded competing-daemon spawn + Cleanup Kill+Wait, package `tmux_test`)

DESCRIPTION: The "spawn an isolated `portal state daemon` subprocess and arrange a guaranteed reap" pattern is implemented in three structurally identical shapes across two test packages. Inside `bootstrap_test` it is correctly factored: `spawnOrphanDaemonIsolated` covers the `env + exec.Command + Start` triplet and `registerSubprocessCleanup` covers the reap-goroutine + `t.Cleanup{Kill; <-reaped}` pair, used by both `upgrade_path_integration_test.go` and `composition_abc_integration_test.go`.

The two `internal/tmux` integration tests inline the same pattern verbatim because the helpers cannot be cross-imported from `bootstrap_test`. The reap-goroutine block in `kill_barrier_escalation_no_final_flush_integration_test.go:191-207` is line-for-line equivalent to `registerSubprocessCleanup` (same `make(chan struct{})` + `go func(){ Wait; close(reaped) }()` + `t.Cleanup{ Kill; <-reaped }` shape). `portal_saver_endstate_integration_test.go:327-334` uses the simpler `Kill+Wait` shape (no separate reaper goroutine) but is the same envelope. The rationale comments (load-bearing `"portal"` unqualified-name for darwin `comm`-match, SIGKILL belt-and-braces, errors-swallowed-because-typically-already-exited) are triplicated.

This duplication was structurally invisible to prior cycles because all three earlier scans focused on the `bootstrap_test`-internal helpers and treated the `internal/tmux` integration tests as a separate concern. With `internal/portaltest` now established as the canonical cross-package test-helper home (T9-1 / T9-4 / T9-5), the spawn pattern is ripe for the same promotion.

RECOMMENDATION: Promote two helpers to `internal/portaltest` (new file, e.g. `spawn_daemon.go`):

```go
// SpawnIsolatedDaemon spawns `portal state daemon` with PORTAL_STATE_DIR
// appended to envSlice, registers Kill+reap cleanup, and returns the cmd
// and the stateDir.
func SpawnIsolatedDaemon(t *testing.T, envSlice []string) (*exec.Cmd, string)

// RegisterSubprocessCleanup is the reap-goroutine + Cleanup-Kill primitive
// (currently in bootstrap_test). Returned channel closes when the reaper
// observes Wait, so callers needing to time process death can select on it.
func RegisterSubprocessCleanup(t *testing.T, cmd *exec.Cmd) <-chan struct{}
```

Then:
- `bootstrap_test`'s `spawnOrphanDaemonIsolated` + `registerSubprocessCleanup` become one-line forwarders (or be deleted, with call sites switching to `portaltest.SpawnIsolatedDaemon` / `portaltest.RegisterSubprocessCleanup`).
- `internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go` replaces the 30-line spawn+reap block with a 2-line call.
- `internal/tmux/portal_saver_endstate_integration_test.go` replaces its inline spawn with the helper (the "no separate reaper" variant here is gratuitous — the canonical reap-goroutine shape works for both).

Net: ~-50 LOC and a single canonical rationale-comment location for the load-bearing details (unqualified `portal` argv[0], SIGKILL on cleanup, reap-via-goroutine to avoid kill-zero races against unreaped zombies).

EFFORT: small

SUMMARY: One new cross-package finding remains after c1-c3 closed the obvious within-package duplications: the orphan-daemon spawn + reap-cleanup pattern is now correctly factored inside `bootstrap_test` but reimplemented twice inside `internal/tmux/*_integration_test.go`. Promotion to `internal/portaltest` (the now-established cross-package test-helper home) collapses ~50 LOC and centralises the triplicated darwin-`comm` / SIGKILL / reap-goroutine rationale comments.
