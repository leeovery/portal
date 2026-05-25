# Duplication Analysis — Cycle 5 (Phase 11 re-scan)

AGENT: duplication
STATUS: clean
FINDINGS_COUNT: 0

## Phase 11 diff coverage

The Phase 11 remediation touched only two code files; spec edits (T11-3) are excluded by definition.

### T11-1 — `cmd/state_daemon_test.go`: ERROR→WARN flip + test rename

The flip makes `TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure` and `TestStateDaemon_EmitsWarnOnLockContention` more visibly parallel — both now assert "exactly one WARN line containing <needle>" against the same `portal.log` file in the same `dir := t.TempDir()` setup. Shared assertion envelope is ~10 LOC across both tests.

Considered and rejected as a finding:

- The two tests' "exactly one" mechanics diverge in code: the non-contention test iterates split lines counting matches where the line contains BOTH "WARN" AND "acquire daemon lock" (conjunctive per-line); the contention test uses `strings.Count(got, "another daemon holds the lock")` against the whole buffer (single-substring whole-buffer). Difference is load-bearing — extracting `assertExactlyOneLogLine(t, got, level, needle)` would erase that distinction.
- Total duplicated envelope is ~10 LOC in the same file. Below proportionality threshold ("three similar 20-line blocks").

### T11-4 — symmetric `os.Stat(daemon.version)` assertion

Adds one `os.Stat` + `!os.IsNotExist` check four lines below the existing `os.Stat(daemon.pid)` check. The two checks are intentionally adjacent, asserting the same invariant against two filenames. Not duplication — a documented symmetric pair pinning the spec amendment. Extracting `assertFileAbsent(t, dir, name)` would obscure spec-pinning intent.

### T11-2 — `snapshotScrollbackPaths` signature change

The `(paths, dirExists)` return consolidates ENOENT-at-root handling into the helper's prologue (single `os.Stat`) and removes the `os.IsNotExist(walkErr) && path == dir` branch from the WalkDir callback. Net is reduced duplication, not introduced.

## Convergence statement

Phase 11 did not introduce duplication crossing the proportionality threshold. The codebase remains converged against the c1-c4 findings list (all closed per the prior c5 sweep summary, retained as context above). No new cross-file patterns surfaced.

## Prior c5 convergence context (carried forward)

Pre-Phase-11 closure status retained for reference:
- c1 F1 / c2 F1 unified via `portaltest.DiffFingerprints` + `portaltest.FormatDelta`.
- c1 F2 collapsed via T10-1 promotion.
- c1 F3 folded into `portaltest.IsolateStateForTest` (T9-5).
- c2 F2 / c3 F1 promoted to `state.PgrepPortalDaemons` (T9-1).
- c2 F3 — `SaverPanePIDOrAbsent` is the sole exported entry point (T9-2 / T10-2 / T10-3).
- c2 F4 — `captureLogger` deleted; single exported `bootstrap.RecordingLogger`.
- c2 F5 — `slices.Sorted(maps.Keys(m))` used everywhere (T8-5).
- c3 F2 — `waitForAnyDaemonPID` deleted (T9-3).
- c3 F3 — `ReadPortalLogSafe` promoted (T9-4).
- c4 F1 — `SpawnIsolatedDaemon` + `RegisterSubprocessCleanup` promoted (T10-1).
