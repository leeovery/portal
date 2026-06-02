AGENT: architecture
CYCLE: 5
STATUS: findings

FINDINGS:

- FINDING: Test logger (restoretest.OpenTestLogger) parallel-implements the production sink with an incompatible on-disk shape
  - SEVERITY: low
  - FILES: internal/restoretest/logger.go:27-38, internal/restore/exit_closes_pane_integration_test.go:293,375, internal/log/sink.go:304-344
  - DESCRIPTION: `OpenTestLogger` writes a REGULAR FILE at `<stateDir>/portal.log` via a vanilla `slog.NewTextHandler`, but production's `rotatingSink` now owns `portal.log` as a SYMLINK pointing at `portal.log.<date>`, and its `reopen()` runs `migrationGuard` which `os.Remove()`s any regular-file `portal.log` on first write. `exit_closes_pane_integration_test.go` builds and runs the real portal binary (line 293) against the SAME `stateDir` for which it opened a regular-file test logger (line 375). The two writers contend for the same path: the test's restorer appends to the regular file while the spawned helper/daemon's sink deletes that regular file and writes to `portal.log.<date>` through the symlink. This test happens to assert on tmux state (not log content) so it passes, but the scaffold is a hand-rolled second implementation of the production sink that no longer matches its file contract — a latent trap for any future test that asserts on `portal.log` content while also spawning the real binary (it would read the regular file the production sink has already unlinked). The production sink is the real owner of the `portal.log` shape.
  - RECOMMENDATION: Route `OpenTestLogger` through the production sink shape rather than a bare regular file — e.g. drive it through `internal/log`'s own handler/sink, or have it write `portal.log.<date>` + the symlink the way the sink does, so test infra and production agree on the `portal.log` contract. At minimum, document on `OpenTestLogger` that it must not share a `stateDir` with a real-binary subprocess, since the migration guard will delete its file.

SUMMARY: Implementation architecture is strong — strict leaf-package import-cycle discipline (tmuxerr/tmuxout/storelog), a single-owner slog wrapper with a clean swappable-handler indirection, a small well-scoped public API (Init/For/Close/SetTestHandler), and consistently-applied, spec-backed component-split patterns (per-item WARN under the driver component, cycle summary under the owning component — identical between SweepOrphanFIFOs/clean and capture/daemon). The only actionable seam is the test-infra logger above.

NON-FINDINGS examined and deliberately NOT flagged:
- The daemon vs capture component split inside captureAndCommit is an intentional, documented, spec-backed pattern (cmd/state_common.go:37-43) mirrored by the caller/clean split in internal/state/fifo_sweep.go:38-64. Consistent and defensible.
- Per-mutation audit emission (INFO-success / WARN-failure-with-error_class) repeated across hooks/alias/project stores while only CleanStale was factored into storelog — a duplication concern, out of architecture scope.
- resolveLevel/levelString are logical inverses but not cleanly derivable from each other — not worth a composition flag.
