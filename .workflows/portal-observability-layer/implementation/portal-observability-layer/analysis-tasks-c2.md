---
topic: portal-observability-layer
cycle: 2
total_proposed: 4
---
# Analysis Tasks: Portal Observability Layer (Cycle 2)

## Task 1: Extract a `log.Took` helper to pin the cycle-summary `took` attr contract
status: pending
severity: medium
sources: duplication

**Problem**: The cycle-summary timing bookend — `start := time.Now()` at entry, local counters, and a terminal `logger.Info("... complete"/"tick complete", <counters...>, "took", time.Since(start))` — is hand-authored across nine production sites: cmd/state_daemon.go:391+491-497, cmd/bootstrap/orphan_sweep.go:151+208, cmd/bootstrap/stale_marker_cleanup.go:125+133, internal/state/fifo_sweep.go:67+100, internal/hooks/store.go:249+294, internal/project/store.go:190+232, internal/restore/restore.go:69+85, internal/restore/session.go:261+292. The `took` attr key is the spec's reserved cycle-summary attr and carries a `time.Duration` (not string) contract, yet it is re-typed by hand at every site with no compiler signal binding them. Cycle 1 flagged this at four sites (low) and deferred with "consolidate if a fifth sweep appears"; five further sites have since landed, so the Rule-of-Three threshold the c1 deferral set is well past. A future change to the `took` attr name or its rendering would have to be applied nine times or the summaries silently diverge.

**Solution**: Add one tiny helper to internal/log (every site already imports it for `log.For`): `func Took(start time.Time) slog.Attr` returning `slog.Duration("took", time.Since(start))`. Each site keeps its own `start` capture and its divergent counters but ends its terminal Info with `..., log.Took(start))` instead of the hand-written `"took", time.Since(start)` pair. Do NOT unify the loop bodies or the counter sets — they legitimately diverge; only the `took` bookend repeats.

**Outcome**: The `took` attr key and its `time.Duration` type are pinned in exactly one place. All nine cycle-summary sites route their `took` attr through `log.Took`, so a change to the key or type is a single-point edit and the summaries cannot silently diverge. Emitted log output is byte-identical to today (still `took=<duration>`).

**Do**:
1. Add `Took(start time.Time) slog.Attr` to internal/log (a small new file e.g. internal/log/took.go, or alongside an existing leaf file in the package). It must return `slog.Duration("took", time.Since(start))`. Add a doc comment stating it is the single source of truth for the reserved `took` cycle-summary attr key and its Duration type.
2. Replace the trailing `"took", time.Since(start)` pair with `log.Took(start)` at each of the nine sites listed in Problem. Keep each site's `start := time.Now()` and counter attrs exactly as they are.
3. Confirm no behavioural change: `log.Took(start)` placed as the final variadic arg produces the same `took=<duration>` key/value the hand-written pairs produced.
4. Do NOT touch the loop bodies, counter names, or the per-site message strings.

**Acceptance Criteria**:
- `internal/log` exports `Took(start time.Time) slog.Attr` returning a `slog.Duration` attr keyed `"took"`.
- All nine cycle-summary sites use `log.Took(start)` for the `took` attr; no production site outside the helper constructs a `"took"` attr by hand.
- Log output for every affected summary is unchanged: the `took` key is present with a duration value, identical attr ordering preserved (helper appended in the same final position the manual pair occupied).
- No loop body, counter, or message string is altered.

**Tests**:
- Unit test in internal/log asserting `Took(start)` returns an attr whose Key is `"took"` and whose Value kind is `slog.KindDuration`.
- A grep/static assertion (or test-handler capture on at least one representative site, e.g. the daemon tick summary) confirming the emitted `took` key and value type are unchanged after the swap.
- `go test ./...` passes; the existing cycle-summary assertions at the affected sites still pass unmodified.

## Task 2: Extract a shared `emitCleanStaleSummary` helper for the hooks and project stores
status: pending
severity: medium
sources: duplication

**Problem**: `Store.CleanStale` in internal/hooks/store.go:248-297 and internal/project/store.go:189-235 are structurally identical ~45-line blocks: `start := time.Now()` → Load (wrapped error) → in-memory kept/removed partition → zero-removal early return (skip Save + summary) → per-entry DEBUG loop → `Save(kept)` → on failure a `logger.Warn("clean-stale", "op", "clean-stale", "entries", len(removed), "via", "internal", "error", err, "error_class", fileutil.ClassifyWriteError(err), "took", time.Since(start))` + wrapped return → on success a `logger.Info("clean-stale", "op", "clean-stale", "entries", len(removed), "via", "internal", "took", time.Since(start))`. The only legitimate differences are the per-entry DEBUG attr key (`hook_key` vs `project`/`path`) and the partition predicate (liveKeys membership vs os.Stat). Both bodies even carry a verbatim-copied comment cross-referencing each other ("Same reasoning as the hooks store's CleanStale"). Cycle 1 identified this exact sub-case as "the worthwhile sub-case" and recommended a shared helper; it remains unaddressed. The two terminal emission shapes (the Warn and Info attr lists, the zero-removal skip semantics, the `took` contract) must currently be kept byte-identical by hand.

**Solution**: Extract the terminal summary emission into a shared helper parameterized by removed-count, start, and the optional save error — e.g. `func emitCleanStaleSummary(logger *slog.Logger, removed int, start time.Time, saveErr error)` in a store-adjacent location (or internal/log). The helper owns the success-Info vs failure-Warn branch and the identical attr list (`op`/`entries`/`via`/`error`/`error_class`/`took`). Each store keeps its own Load, partition predicate, zero-removal early-return, and per-entry DEBUG loop (the per-entry attr key legitimately differs) and routes only the terminal Warn/Info through the helper. Note: if Task 1 lands, the helper should itself use `log.Took` for the `took` attr, but the two tasks are independent — this task does not depend on Task 1 and must emit the same `took` attr either way.

**Outcome**: The clean-stale batch-summary emission contract (the Warn/Info attr lists, the `took` contract, the success/failure branch) lives in exactly one place. Both stores route their terminal summary through it. The zero-removal skip and per-entry DEBUG loops remain per-store. Emitted log output is byte-identical to today.

**Do**:
1. Add `emitCleanStaleSummary(logger *slog.Logger, removed int, start time.Time, saveErr error)` in a store-adjacent location shared by both packages, or in internal/log if that avoids a new import edge. When `saveErr != nil` it emits the Warn with `"op", "clean-stale", "entries", removed, "via", "internal", "error", saveErr, "error_class", fileutil.ClassifyWriteError(saveErr), "took", time.Since(start)`; otherwise the Info with `"op", "clean-stale", "entries", removed, "via", "internal", "took", time.Since(start)`. Both use the message `"clean-stale"`.
2. In internal/hooks/store.go CleanStale: keep Load, the liveKeys partition, the zero-removal early return, and the per-entry `hook_key` DEBUG loop. Replace the inline Warn (call `emitCleanStaleSummary(logger, len(removed), start, err)` before returning the wrapped error) and the inline Info (call `emitCleanStaleSummary(logger, len(removed), start, nil)`).
3. In internal/project/store.go CleanStale: keep Load, the os.Stat partition (including the permission-denied-retains arm), the zero-removal early return, and the per-entry `project`/`path` DEBUG loop. Replace the inline Warn and Info the same way.
4. Resolve the duplicated logger reference: the helper takes the logger explicitly so each store passes its own package `logger`.
5. Do NOT move the per-entry DEBUG loops or the partition logic into the helper — those legitimately differ.

**Acceptance Criteria**:
- A single `emitCleanStaleSummary` helper owns the clean-stale terminal Warn/Info emission; neither store constructs the terminal summary attr list inline.
- The success path emits the Info with attrs `op=clean-stale entries=<N> via=internal took=<duration>`; the save-failure path emits the Warn additionally carrying `error` and `error_class`. Output is byte-identical to current behaviour (attr keys, values, ordering, message string).
- The zero-removal early return still skips both Save and any summary in both stores (no summary line on a no-op clean).
- The per-entry DEBUG loops retain their store-specific attr keys (`hook_key` for hooks; `project`+`path` for projects).
- The wrapped error returns ("failed to save after cleaning stale hooks/projects") are unchanged.

**Tests**:
- Existing hooks-store and project-store CleanStale tests pass unmodified (success summary, save-failure summary with `error_class`, and zero-removal no-summary cases).
- A test capturing emitted records on a save-failure asserts the Warn carries `error_class` from `fileutil.ClassifyWriteError` and `took` is a duration — for both stores, confirming the shared helper produces the same shape.
- A zero-removal test confirms no summary record is emitted (only the early return) for both stores.
- `go test ./internal/hooks/... ./internal/project/...` passes.

## Task 3: Mark the hydrate exec-failure fallback before its bare `os.Exit(1)`
status: pending
severity: low
sources: standards

**Problem**: cmd/state_hydrate.go:434-437 — `defaultExecShell` calls `syscall.Exec` and, only if exec returns an error, falls through to a bare `os.Exit(1)` with no preceding terminal marker and no `log.Close`. The spec's Defensive-invariants rule states "Bare `os.Exit` is prohibited outside `main`" (PR-review reject), with exactly one sanctioned exception (the daemon self-eject, which pairs `daemon: self-eject` + `log.Close(0)` before exiting). This exec-failure fall-through is a second, un-sanctioned bare-exit. On that path the just-emitted `hydrate: exec` INFO marker becomes misleading (it announces a handoff that did not happen) and the process exits unmarked by any `process: exit`/`process: panic` line — the exact "process vanished without a terminal marker" shape the spec's four-way classification calls "genuinely alarming." The window is narrow (exec almost never fails after argv validation) and the line is dead on the happy path (syscall.Exec never returns on success), but it is a literal exception to a stated prohibition that the spec did not sanction.

**Solution**: Bring the exec-failure path inside the "every termination is marked" contract. Either (a) emit a terminal marker before exiting — call `log.Close(1)` so a paired `process: exit code=1` is recorded, mirroring the daemon self-eject's Close-before-exit discipline — or (b) have `defaultExecShell` return the exec error to the caller and let the normal return/Close path own termination. Pick the approach that fits the existing call shape with least disturbance; (a) is the smaller, more local change and directly mirrors the only sanctioned bare-exit's discipline.

**Outcome**: The exec-failure path no longer exits unmarked. A `process: exit` (code=1) marker is paired with the termination, so the `hydrate: exec` INFO is not left dangling as a phantom handoff, and the four-way "vanished without a terminal marker" alarm condition no longer applies to this path. No remaining bare `os.Exit` outside `main` and the sanctioned daemon self-eject.

**Do**:
1. At cmd/state_hydrate.go:434-437, on the `syscall.Exec` error fall-through, before exiting non-zero, route termination through `log.Close(1)` (so a `process: exit code=1` terminal marker is emitted) rather than calling a bare `os.Exit(1)`. If a WARN/error log of the exec failure itself is not already emitted, add one immediately before Close so the failure reason is captured.
2. Alternatively, if cleaner given the call site, change `defaultExecShell` to return the exec error to its caller and have the caller drive the existing return/Close termination path; ensure the result is still a non-zero exit with a paired terminal marker.
3. Confirm the happy path is untouched: on successful `syscall.Exec` the process is replaced and nothing after the call runs.
4. Verify no other bare `os.Exit` outside `main` is introduced and the daemon self-eject exception is unaffected.

**Acceptance Criteria**:
- The exec-failure fall-through in `defaultExecShell` no longer terminates via a bare `os.Exit(1)`; termination is paired with a terminal marker (a `process: exit` with a non-zero code, e.g. via `log.Close(1)`).
- The process still exits non-zero on exec failure (exit semantics preserved for callers/tests of the failure path).
- The happy path (successful exec handoff) is unchanged.
- No new bare `os.Exit` outside `main` exists; the daemon self-eject remains the only sanctioned bare exit.

**Tests**:
- A test exercising the exec-failure path (injected exec returning an error) asserts the process drives the Close/terminal-marker path and exits non-zero — capturing that a `process: exit` marker with code=1 is emitted (or, for approach (b), that the exec error is returned and the caller's Close path runs).
- Existing hydrate exec-handoff tests (the `hydrate: exec` INFO marker on the success path) still pass.
- `go test ./cmd -run Hydrate` (or the relevant hydrate test selector) passes.

## Task 4: Narrow the exported `SweepLogs` surface to drop its dead gated mode and ignored parameter
status: pending
severity: low
sources: architecture

**Problem**: internal/log/retention.go:131-139 — `SweepLogs(stateDir string, retentionDays int, gated bool) error` is the package's only exported retention entry point, but its parameter space does not match its consumption. The sole production caller, `cleanRotatedLogs` (cmd/clean.go:178), always invokes `SweepLogs(stateDir, 0, false)`. The `gated==true` arm is reachable from no production caller — the only gated (per-process startup) sweep runs through the unexported `runRetentionSweep` wired into the sink's dayRoll seam, never through `SweepLogs`. Worse, the parameters interact dangerously: the doc comment states `retentionDays` is honoured ONLY when `gated==false` and ignored when `gated==true`, so a hypothetical `SweepLogs(dir, 30, true)` would silently discard the 30 in favour of env resolution. This is the boolean-parameter + conditionally-meaningful-parameter anti-pattern (code-quality.md "Boolean parameters", "Untyped parameters when concrete types are known"): the exported signature advertises caller-chosen gating and a caller-chosen window the single real caller never uses and that, in the dead combination, behaves surprisingly. It widens internal/log's public surface beyond a coherent contract.

**Solution**: Narrow the exported surface to the one mode that is genuinely a public entry point — the explicit user-invoked `--logs` sweep (ungated, cutoff=today, delete-everything-older-than-today). Replace `SweepLogs` with a parameterless-mode signature such as `SweepLogsForClean(stateDir string) error` that delegates to `runRetentionSweepWithDays(stateDir, today, false, &zero)` (cutoff=today via retentionDays=0). Keep the gated per-startup path entirely behind the unexported `runRetentionSweep`/dayRoll seam where it already lives. This removes the dead `gated==true` branch from the public API and eliminates the silently-ignored-`retentionDays` footgun while preserving the single-source-of-truth `runRetentionSweepWithDays` algorithm.

**Outcome**: The public retention entry point matches its only real consumer: a single, clearly-named, parameterless-mode function for the `--logs` clean path. No reachable dead `gated==true` branch on the public API, no silently-ignored parameter. The shared `runRetentionSweepWithDays` walk/delete/prune logic is unchanged and still shared with the gated dayRoll path.

**Do**:
1. Add `SweepLogsForClean(stateDir string) error` to internal/log/retention.go. It computes today via `nowFunc().Format(dateLayout)` and delegates to `runRetentionSweepWithDays(stateDir, today, false, &zero)` where `zero` is an `int` 0 (cutoff == today — deletes every prior-day rotated file, leaves today's). Carry a doc comment stating it is the explicit user-invoked `--logs` sweep (ungated, cutoff=today) and that the gated per-process path lives behind `runRetentionSweep`/dayRoll, not here.
2. Update the sole caller `cleanRotatedLogs` (cmd/clean.go:178) to call `SweepLogsForClean(stateDir)` instead of `SweepLogs(stateDir, 0, false)`.
3. Remove the old `SweepLogs(stateDir, retentionDays, gated)` exported function (its `gated==true` branch had no production caller; the gated path remains via the unexported `runRetentionSweep`).
4. Verify the gated per-startup sweep path (sink dayRoll seam → `runRetentionSweep` → `runRetentionSweepWithDays(..., true, nil)`) is untouched and still env-resolves its retention window.
5. Update any tests referencing the old `SweepLogs` signature (e.g. internal/log/sweeplogs_test.go) to the new entry point, preserving their assertions.

**Acceptance Criteria**:
- `internal/log` exports `SweepLogsForClean(stateDir string) error` (or an equivalent parameterless-mode name) and no longer exports the three-parameter `SweepLogs`.
- `cleanRotatedLogs` (cmd/clean.go) calls the new entry point; the `portal clean --logs` behaviour (delete every prior-day rotated file, keep today's) is byte-for-byte preserved.
- No exported retention function carries a `gated bool` parameter or a `retentionDays` parameter that is conditionally ignored.
- The gated per-process dayRoll sweep path is unchanged and still resolves its window from `PORTAL_LOG_RETENTION_DAYS`.
- `runRetentionSweepWithDays` remains the single shared walk/delete/prune implementation (no duplicated walk logic introduced).

**Tests**:
- Existing retention/sweeplogs tests updated to the new signature still pass, asserting the `--logs` sweep deletes prior-day rotated files and retains today's.
- A test confirms the gated per-startup path (dayRoll → runRetentionSweep) still honours `PORTAL_LOG_RETENTION_DAYS` and is independent of the new clean entry point.
- `go test ./internal/log/... ./cmd -run Clean` passes.
