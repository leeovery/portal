# Duplication Analysis — Cycle 5

AGENT: duplication
STATUS: clean
FINDINGS_COUNT: 0

SUMMARY: No substantive new duplication remains. All findings from c1-c4 have been actioned:

- **c1 F1 / c2 F1** — Fingerprint diff/format suites unified via `portaltest.DiffFingerprints` + `portaltest.FormatDelta`. All 4 call sites (`composition_abc`, `composition_e2e_self_eject`, `state_daemon_self_supervision`, `kill_barrier_escalation_no_final_flush`) now use the single helper.
- **c1 F2** — `spawnOrphanDaemonIsolated{,Named}` collapsed via T10-1 promotion.
- **c1 F3** — `applyHostNoiseMitigation` folded into `portaltest.IsolateStateForTest` (T9-5).
- **c2 F2 / c3 F1** — `pgrepPortalDaemons` promoted to `state.PgrepPortalDaemons`; bootstrapadapter + portaltest both forward (T9-1).
- **c2 F3** — `FirstPanePIDInSession` removed; `SaverPanePIDOrAbsent` is the sole exported entry point (T9-2 / T10-2 / T10-3).
- **c2 F4** — `captureLogger` deleted; single exported `bootstrap.RecordingLogger` consumed everywhere.
- **c2 F5** — `sortedSnapKeys` / `sortedKeys` / `sortedSnapshotKeys` replaced with `slices.Sorted(maps.Keys(m))` (T8-5).
- **c3 F2** — `waitForAnyDaemonPID` deleted (T9-3).
- **c3 F3** — `ReadPortalLogSafe` promoted to `internal/portaltest` (T9-4).
- **c4 F1** — `SpawnIsolatedDaemon` + `RegisterSubprocessCleanup` promoted to `internal/portaltest` (T10-1); `internal/tmux` integration tests now reach the canonical reap pattern.

Net-new patterns checked this cycle:
- 14 remaining `os.ReadFile(state.PortalLog(...))` sites are NOT duplicates of `ReadPortalLogSafe` — each has distinct error-handling semantics (ErrNotExist branching, fatal-on-err, informational t.Logf, substring-grep). The diagnostic-fallback shape `ReadPortalLogSafe` was extracted from no longer occurs elsewhere.
- `escalateKillToSIGKILL` (Component A, `internal/tmux/portal_saver.go`) identify+kill+wait is structurally analogous to Component B `OrphanSweepCore.SweepOrphanDaemons` loop but routes through different seam struct (`SaverSeams` vs `OrphanSweepCore`), single-target with wait-for-exit barrier, different log sink — not a consolidation candidate.
- All `waitFor*` helpers in `bootstrap_test` (`waitForDaemonPID`, `waitForSaverPanePID`, `waitForPgrepCount`, `waitForIdentifyDaemon`, `waitForSessionMarkerCleared`, `waitForPaneText`) observe distinct conditions; sibling `cmd/state_daemon_integration_test.go:waitForDaemonAlive` uses `state.DaemonAlive` (signal-0 probe), not `IdentifyDaemon` (ps comm/argv match) — distinct semantics.

Returning clean — codebase has converged.
