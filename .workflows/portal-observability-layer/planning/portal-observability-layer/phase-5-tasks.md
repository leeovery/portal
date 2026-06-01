---
phase: 5
phase_name: Cycle summaries and saver/daemon lifecycle catalogs
total: 11
---

## portal-observability-layer-5-1 | approved

### Task 5-1: Emit the daemon tick cycle summary `capture: tick complete` with sessions/panes/natural_churn/anomalous/took and per-pane DEBUG/WARN

**Problem**: The daemon's 1 Hz capture-and-commit loop is the highest-frequency machinery in portal and the centre of the motivating incident, yet it emits no per-tick summary an operator can reconstruct a window from. The spec's cycle-level catalog mandates exactly ONE INFO summary per tick under the `capture` component (promoted out of `daemon`), with the per-pane detail demoted to DEBUG (steady state) / WARN (anomaly). Today `captureAndCommit` logs only per-pane WARN lines on capture/write failure and nothing on the happy path.

**Solution**: Instrument `captureAndCommit` in `cmd/state_daemon.go` (the tick cycle, shape "Tick cycle" per the spec) to capture `start := time.Now()`, track `sessions` / `panes` / `natural_churn` / `anomalous` counts across the per-pane loop, emit a per-pane DEBUG breadcrumb for each pane processed, keep the existing per-pane WARN on capture/write failure (now incrementing `anomalous`), and emit exactly one INFO summary `capture: tick complete sessions=N panes=N natural_churn=N anomalous=N took=T` just before the successful `return nil`. The summary is emitted under a `capture`-bound logger (`log.For("capture")`), distinct from the daemon's `log.For("daemon")` logger.

**Outcome**: Each successful tick that performs capture work emits one INFO line `capture: tick complete sessions=N panes=N natural_churn=N anomalous=N took=T`; ctx-cancellation at any of the three observation points returns before the summary fires (no summary line); per-pane capture/write failures emit a per-pane WARN and are reflected in the summary's `anomalous` count.

**Do**:
- In `cmd/state_daemon.go`, add a package-level `var captureLogger = log.For("capture")` (component `capture` per the closed taxonomy — promoted out of `daemon`). The daemon's other lines stay on the existing `daemon`-bound logger.
- In `captureAndCommit`, capture `start := time.Now()` at the top (after the observation-point-1 ctx check returns cleanly), and declare counters `panes`, `naturalChurn`, `anomalous` (sessions count is `len(idx.Sessions)`).
- Inside the per-pane loop: for each pane actually processed (not in `skipSet`), increment `panes` and emit a per-pane DEBUG breadcrumb under `captureLogger` (e.g. `captureLogger.Debug("pane captured", "pane_key", paneKey, "session", sess.Name)`); on the existing `CaptureAndHashPane` / `WriteScrollbackIfChanged` failure branches, increment `anomalous` and keep a per-pane WARN (`captureLogger.Warn("capture pane", "error", err, "pane_key", paneKey)` / `"write scrollback"`) — the WARN passes the wrapped error directly (Phase 4) via the `error` attr.
- `natural_churn` is the count of items that ended cleanly mid-cycle by normal action (e.g. a session/pane the user closed during the tick, surfaced as a tmux "no such pane/session" expected outcome distinct from an anomalous capture failure). If the current code shape cannot distinguish a user-closed pane from an anomalous capture failure at this site, set `natural_churn=0` and note the gap in Context rather than inventing a classifier — see `[needs-info]` below.
- Immediately before the final `return nil` in `captureAndCommit` (after the `Commit` success), emit the single summary: `captureLogger.Info("tick complete", "sessions", len(idx.Sessions), "panes", panes, "natural_churn", naturalChurn, "anomalous", anomalous, "took", time.Since(start))`.
- Do NOT emit the summary on any of the three `case <-ctx.Done(): return nil` observation points — a cancellation must not produce a summary line (the existing comment already states "a cancellation must not produce a log line"). Do NOT emit the summary on a phase-boundary error return (`list markers` / `capture structure` / `commit` wrapped-error returns) — those propagate to `tick`, which WARNs.
- Leave `tick`'s no-op fast path (`!dirty && !gap` → early `return`) untouched: a no-op tick performs no capture work and therefore never calls `captureAndCommit`, so it emits no summary (this is the steady-state idle case). The `@portal-restoring` early-return in `tick` likewise emits no summary.

**Acceptance Criteria**:
- [ ] A tick that performs capture work and commits successfully emits exactly one INFO `capture: tick complete` line under component `capture` with `sessions`, `panes`, `natural_churn`, `anomalous`, and `took` attrs.
- [ ] A ctx-cancellation at any of the three observation points returns `nil` before the summary and emits no `tick complete` line.
- [ ] A per-pane `CaptureAndHashPane` or `WriteScrollbackIfChanged` failure increments `anomalous`, emits one per-pane WARN with the wrapped `error` attr, and the loop continues (one bad pane does not abort the cycle or the summary).
- [ ] A phase-boundary error (`list markers` / `capture structure` / `commit`) returns the wrapped error to `tick` without emitting a `tick complete` summary.
- [ ] An idle tick (`!dirty && !gap`) and a restoring tick emit no summary (no call into `captureAndCommit`).
- [ ] Per-pane DEBUG breadcrumbs are emitted under `capture` (silent at production INFO, visible at DEBUG); the summary is INFO.

**Tests**:
- `"it emits one capture: tick complete summary with sessions/panes/natural_churn/anomalous/took on a successful tick"`
- `"it emits no summary when ctx is cancelled at observation point 1/2/3"`
- `"it increments anomalous and emits a per-pane WARN on a capture-pane failure while continuing the loop"`
- `"it increments anomalous and emits a per-pane WARN on a write-scrollback failure"`
- `"it emits no summary when a phase-boundary error (commit) is returned"`
- `"it emits no summary on an idle tick (not dirty, no gap)"`
- `"it emits per-pane DEBUG breadcrumbs that are filtered at INFO and present at DEBUG"`
- `"it counts a user-closed session as natural_churn distinct from an anomalous capture failure"` (only if the natural_churn classifier is wired; otherwise assert natural_churn=0 per the `[needs-info]` note)

**Edge Cases**:
- Restoring/idle no-op tick produces no summary at all (capture work never runs).
- Ctx-cancel at any of the three observation points returns before the summary.
- Per-pane capture/write failure increments `anomalous` and fires a per-pane WARN; the cycle continues.
- A user-closed session/pane is `natural_churn`, distinct from an anomalous capture failure (see `[needs-info]`).
- The final-flush (shutdown) path also calls `captureAndCommit` (via `defaultShutdownFlush` with `context.Background()`), so it too emits a `tick complete` summary — acceptable and desirable (the final flush IS a capture cycle); ensure tests of `defaultShutdownFlush` tolerate the extra `capture:` summary line.
- The `capture` component is promoted out of `daemon` — the summary line's prefix is `capture:`, not `daemon:`.

**Context**:
> "Daemon tick (1 Hz capture + commit) | `capture` | `capture: tick complete sessions=N panes=N natural_churn=N anomalous=N took=T`" (spec § Cycle-level summary cadence and shape → Concrete cycle catalog)
>
> "`natural_churn` — items that ended cleanly mid-cycle by normal action (e.g. a session the user closed during the tick), distinct from a capture failure; `anomalous` — items that failed anomalously without terminating the cycle (each also emits a per-item WARN)." (spec § Cycle-level summary → Mechanical rule)
>
> "Per-item DEBUG breadcrumb ALWAYS for items where the per-item path is interesting (the capture loop's per-pane state…). These flood at DEBUG and are silent at INFO — the summary is the INFO truth. Per-item WARN ONLY for items that fail anomalously (count goes into the summary's `anomalous` attr)." (spec § Cycle-level summary → Per-item event level inside a cycle)
>
> "`capture` | The daemon's per-tick capture loop (promoted from inside `daemon`)." (spec § Subsystem prefix taxonomy → component table)
>
> `[needs-info]`: The current `captureAndCommit` per-pane loop classifies a failed `CaptureAndHashPane` / `WriteScrollbackIfChanged` as a WARN-and-continue (anomalous) but has no branch that recognises a *cleanly user-closed* pane/session mid-tick as a distinct outcome (a tmux "no such pane" during capture currently flows through the same WARN path). Producing a non-zero `natural_churn` requires classifying an expected "pane vanished cleanly" tmux error (e.g. `ErrNoSuchSession` / no-such-pane stderr) at the capture site as `natural_churn` rather than `anomalous`. The spec defines the attr but the existing code does not distinguish the two; the executor should either wire this classification using the Phase-4 boundary-preserved tmux sentinels (preferred, if a clean sentinel is available) or emit `natural_churn=0` and flag the gap. The summary attr key remains in the closed vocabulary either way.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (component table, closed cycle-summary attr keys)

---

## portal-observability-layer-5-2 | approved

### Task 5-2: Emit bootstrap per-step `bootstrap: step complete step=<StepName> took=T` and overall `bootstrap: orchestration complete steps=11 warnings=N took=T`

**Problem**: The eleven-step bootstrap orchestrator emits per-step `Debug("step N (…): entering")` breadcrumbs but no per-step completion summary and no overall orchestration summary. The spec's cycle catalog classifies the orchestrator as a "Sequence cycle" requiring ONE INFO per step (`step complete`) and ONE INFO at the post-step Return boundary (`orchestration complete`) carrying `steps=11`, the accumulated `warnings` count, and `took`.

**Solution**: Instrument `Orchestrator.Run` in `cmd/bootstrap/bootstrap.go` to (a) keep the existing per-step `Debug(... entering)` lines as DEBUG breadcrumbs, (b) emit one INFO `bootstrap: step complete step=<StepName> took=T` after each step's body completes (the step's own `took` measured around that step), and (c) emit one INFO `bootstrap: orchestration complete steps=11 warnings=N took=T` at the Return post-step boundary, with `took` measured from the top of `Run`.

**Outcome**: A successful bootstrap emits eleven `bootstrap: step complete step=<StepName> took=T` INFO lines (one per step, in order) plus one `bootstrap: orchestration complete steps=11 warnings=N took=T` line; a fatal-abort step (1, 2, 3, or 8) short-circuits with `Run` returning the `*FatalError` before the orchestration summary fires.

**Do**:
- In `cmd/bootstrap/bootstrap.go`, the bootstrap logger is the migrated `*slog.Logger` bound to component `bootstrap` (Phase 1 replaced the `Logger` interface field; per the foundation note these sites already hold `*slog.Logger` + `log.For("bootstrap")`). Use that logger for the new INFO lines.
- Capture `orchestrationStart := time.Now()` at the top of `Run` (after the no-op-logger substitution).
- For each of the eleven steps, wrap the step body with a `stepStart := time.Now()` immediately before the step's primary call and, after the step's body returns (and on the non-fatal continuation paths), emit `logger.Info("step complete", "step", "<StepName>", "took", time.Since(stepStart))`. Keep the existing `Debug(... entering)` line as the DEBUG breadcrumb above each.
- Use the closed `StepName` set, one literal per step, matching the orchestrator's own step names: `EnsureServer`, `RegisterPortalHooks`, `SetRestoring`, `SweepOrphanDaemons`, `EnsureSaver`, `Restore`, `EagerSignalHydrate`, `ClearRestoring`, `CleanStaleMarkers`, `SweepOrphanFIFOs`, `CleanStale`. (`step` is the bootstrap step name per the closed cycle-summary attr vocabulary.)
- For the four fatal-abort steps (Step 1 `EnsureServer`, Step 2 `RegisterPortalHooks`, Step 3 `Set @portal-restoring`, Step 8 `Clear @portal-restoring`): on the fatal branch, `Run` returns the `*FatalError` immediately (via `o.fatalf`) — do NOT emit a `step complete` line for the aborting step and do NOT emit the orchestration summary. The `o.fatal` ERROR line is the terminal record for that path (Phase 1/2).
- At the Return post-step boundary (replacing/augmenting the existing `Debug("Return: exiting with %d warning(s)", len(warnings))`), emit `logger.Info("orchestration complete", "steps", 11, "warnings", len(warnings), "took", time.Since(orchestrationStart))`. Keep the `steps` literal at `11` (the closed step count); `warnings` reflects the accumulated soft-warning slice at return.
- Do NOT add any new component/attr keys; `step`, `steps`, `warnings`, `took` are all in the closed cycle-summary/contextual vocabulary.

**Acceptance Criteria**:
- [ ] A successful bootstrap emits eleven INFO `bootstrap: step complete step=<StepName> took=T` lines in step order, each with the correct closed `StepName`.
- [ ] A successful bootstrap emits one INFO `bootstrap: orchestration complete steps=11 warnings=N took=T` line at the Return boundary, where `warnings` equals `len(warnings)`.
- [ ] A fatal abort at step 1/2/3/8 returns the `*FatalError` and emits neither a `step complete` line for the aborting step nor the orchestration summary.
- [ ] The existing per-step `Debug("step N (…): entering")` breadcrumbs are retained as DEBUG (filtered at production INFO).
- [ ] `warnings` reflects accumulated soft warnings (e.g. `SaverDownWarning`, `CorruptSessionsJSONWarning`) present at return; a clean run emits `warnings=0`.

**Tests**:
- `"it emits eleven step complete lines with the closed StepName set on a clean bootstrap"`
- `"it emits orchestration complete steps=11 warnings=0 took=T on a clean bootstrap"`
- `"it emits orchestration complete with warnings=N when soft warnings accumulated (saver down + corrupt sessions.json)"`
- `"it short-circuits before the orchestration summary on a fatal step 1 (EnsureServer) error"`
- `"it short-circuits before the orchestration summary on a fatal step 8 (Clear @portal-restoring) error and emits no step complete for step 8"`
- `"it retains the per-step entering Debug breadcrumbs"`

**Edge Cases**:
- Fatal-abort step (1/2/3/8 Clear) short-circuits before the orchestration summary; the aborting step gets no `step complete`.
- `warnings` count reflects the accumulated soft warnings at the moment of return (steps 5, 6 contribute the only warnings today).
- Closed `StepName` set — one literal per step, no ad-hoc names.
- Existing per-step `Debug` "entering" lines kept as DEBUG breadcrumbs (not promoted, not removed).
- The orchestration summary is emitted at the Return post-step boundary (not a numbered step).

**Context**:
> "Bootstrap orchestration | `bootstrap` | `bootstrap: orchestration complete steps=11 warnings=N took=T`" and "Each bootstrap step | `bootstrap` | `bootstrap: step complete step=<StepName> took=T`" (spec § Cycle-level summary → Concrete cycle catalog)
>
> "Sequence cycle — an orchestrator running discrete named steps (the 11-step bootstrap orchestrator, the two-phase restore engine)." (spec § Cycle-level summary → Mechanical rule)
>
> "(`step` = bootstrap step name…)" — closed cycle-summary attr keys include `steps`, `step`, `warnings`. (spec § Subsystem prefix taxonomy → Cycle-summary attr keys)
>
> Step ordering and the fatal-vs-soft classification are pinned by the orchestrator docstring: steps 1, 2, 3, 8 are fatal (return `*FatalError`); steps 4, 5, 6, 7, 9, 10, 11 are best-effort (log Warn and continue). (`cmd/bootstrap/bootstrap.go` `Run` doc + CLAUDE.md § Server bootstrap)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (Cycle-summary attr keys)

---

## portal-observability-layer-5-3 | approved

### Task 5-3: Emit restore phase A summary `restore: skeleton complete sessions=N windows=N panes=N took=T` over the per-session create loop

**Problem**: The two-phase restore engine reconstructs saved sessions at bootstrap but emits only per-session WARN lines on failure — no completion summary an operator can use to confirm "N sessions / W windows / P panes were re-created this boot." The spec's cycle catalog classifies restore phase A (skeleton) as a cycle requiring ONE INFO summary `restore: skeleton complete sessions=N windows=N panes=N took=T`.

**Solution**: Instrument `Orchestrator.Restore` / `restoreOne` in `internal/restore/restore.go` (the per-session create loop) to count the sessions actually restored (skeleton created), the windows and panes across those restored sessions, capture `start := time.Now()` before the loop, and emit one INFO `restore: skeleton complete sessions=N windows=N panes=N took=T` under component `restore` after the loop completes. Per-session failures remain isolated (logged + swallowed) and are excluded from the restored counts.

**Outcome**: A bootstrap that restores some sessions emits one INFO `restore: skeleton complete sessions=N windows=N panes=N took=T` where `sessions` counts sessions whose skeleton was created (not live-skipped, not underscore-skipped, not invalid-topology-skipped), and `windows`/`panes` are the saved-topology counts of those restored sessions; the early-return paths (no sessions.json, zero saved sessions, corrupt sessions.json) emit no summary (or `sessions=0` per the chosen shape).

**Do**:
- In `internal/restore/restore.go`, the `Orchestrator.Logger` field is the migrated `*slog.Logger` bound to `log.For("restore")` (Phase 1). Use it for the summary.
- In `Restore`, capture `start := time.Now()` immediately before the `for _, sess := range idx.Sessions` loop. Declare counters `restoredSessions`, `restoredWindows`, `restoredPanes`.
- Change `restoreOne` to report whether it restored the session (e.g. return a bool, or accept pointers to the counters) so the loop can tally only sessions that reached `sr.Restore(sess)` successfully. A session is counted as restored ONLY when it passes the underscore-prefix skip, the live-session skip, the `validateTopology` check, AND `sr.Restore(sess)` returns without error. Underscore-skipped, live-skipped, invalid-topology-skipped, and `sr.Restore`-errored sessions are NOT counted.
- For each restored session, add its saved-topology window count (`len(sess.Windows)`) to `restoredWindows` and its saved-topology pane count (sum of `len(w.Panes)`) to `restoredPanes`. Source these from the *saved* topology (the `state.Session` being restored), matching the cycle catalog's `sessions`/`windows`/`panes` unit keys — do NOT re-query live tmux for this count (the live re-query is phase B's concern; the arm-phase pane-count-mismatch WARN already surfaces drift).
- After the loop, emit `o.Logger.Info("skeleton complete", "sessions", restoredSessions, "windows", restoredWindows, "panes", restoredPanes, "took", time.Since(start))`.
- Early returns BEFORE the loop emit no summary: `handleReadIndexSkip` (no sessions.json → `(false,nil)`; corrupt → `(true, wrapped)`), `len(idx.Sessions) == 0` → `(false, nil)`, and `snapshotLiveSessions` failure → `(false, nil)`. The corrupt-sessions.json path returns `(true, err)` before reaching the loop, so no summary fires there.
- `windows` is a closed cycle-summary attr key (restore-skeleton window count); `sessions`, `panes`, `took` are in the closed vocabulary. Do NOT add new keys.

**Acceptance Criteria**:
- [ ] A restore that creates N session skeletons emits one INFO `restore: skeleton complete` with `sessions=N`, `windows`=sum of restored sessions' saved window counts, `panes`=sum of restored sessions' saved pane counts, and `took`.
- [ ] A live-skipped session (name already in `liveSet`) is excluded from all three counts.
- [ ] An underscore-prefixed session and an invalid-topology session (zero windows / a window with zero panes) are excluded from the restored counts (their existing WARN/skip behaviour is unchanged).
- [ ] A session whose `sr.Restore(sess)` returns an error is excluded from the counts; its per-session WARN still fires.
- [ ] The no-sessions.json, zero-saved-sessions, and list-sessions-failure early returns emit no summary.
- [ ] The corrupt-sessions.json path returns `(true, wrapped)` (wrapping `state.ErrCorruptIndex`) before the loop and emits no summary.

**Tests**:
- `"it emits skeleton complete sessions=N windows=W panes=P took=T after restoring N sessions"`
- `"it excludes a live-skipped session from sessions/windows/panes counts"`
- `"it excludes an underscore-prefixed session from the restored counts"`
- `"it excludes an invalid-topology session (zero windows / zero-pane window) from the restored counts"`
- `"it excludes a sr.Restore-errored session from the counts while still emitting its per-session WARN"`
- `"it emits no summary when sessions.json is absent / has zero sessions / list-sessions fails"`
- `"it returns (true, err) wrapping ErrCorruptIndex and emits no summary on corrupt sessions.json"`

**Edge Cases**:
- Zero saved sessions early-return emits no summary (the loop is never entered).
- Per-session isolate-and-continue: a failed `sr.Restore` does not abort the loop; the session is not counted as restored.
- Live / underscore-prefixed / invalid-topology skips are excluded from the restored counts.
- Corrupt sessions.json returns `(true, err)` before the summary.
- `windows`/`panes` counted from the saved topology of restored sessions, not from a live re-query.

**Context**:
> "Restore phase A (skeleton) | `restore` | `restore: skeleton complete sessions=N windows=N panes=N took=T`" (spec § Cycle-level summary → Concrete cycle catalog)
>
> "Loop cycle — a `for` loop iterating distinct items (sessions, panes, files, entries, orphans)." (spec § Cycle-level summary → Mechanical rule)
>
> "(`windows` = restore-skeleton window count…)" — closed cycle-summary attr keys. (spec § Subsystem prefix taxonomy → Cycle-summary attr keys)
>
> The Restorer contract: returns `(false, nil)` on happy path / after isolating per-session failures; returns `(true, err)` wrapping `state.ErrCorruptIndex` only when sessions.json is unparseable. (`internal/restore/restore.go` Orchestrator doc, `cmd/bootstrap.Restorer` interface)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (Cycle-summary attr keys)

---

## portal-observability-layer-5-4 | approved

### Task 5-4: Emit restore phase B summary `restore: geometry complete panes=N took=T` over the geometry/active-pane/zoom replay

**Problem**: Restore phase B (geometry: layout, active-pane selection, zoom) applies per-window operations best-effort, emitting per-step WARN on failures but no completion summary. The spec's cycle catalog classifies phase B as a cycle requiring ONE INFO `restore: geometry complete panes=N took=T`.

**Solution**: Instrument the geometry replay in `internal/restore/session.go` (`ApplyWindowGeometry`, called per session by `restoreOne`) so a phase-B summary is emitted once per restored session: capture `start := time.Now()`, count the live panes the geometry phase operated over (sourced from the live `[]tmux.PaneCoord` threaded in), tally `anomalous` from the best-effort layout/select-pane/zoom failures (which already emit per-step WARN), and emit `restore: geometry complete panes=N took=T` under component `restore` (with an `anomalous` attr when non-zero).

**Outcome**: Each session that reaches geometry replay emits one INFO `restore: geometry complete panes=N took=T` (plus `anomalous=N` when best-effort steps failed), where `panes` is the count of live panes from the threaded `[]tmux.PaneCoord`; empty saved-window groups are skipped (not counted) and a session that never reached arm/geometry (zero panes) emits no geometry summary.

**Do**:
- In `internal/restore/session.go`, `ApplyWindowGeometry(sess, livePanes)` is the phase-B cycle. The `SessionRestorer.Logger` is the migrated `*slog.Logger` bound to `log.For("restore")` (Phase 1).
- Capture `start := time.Now()` at the top of `ApplyWindowGeometry`. Track `panes` (count of live panes operated over — use `len(livePanes)`, which is the live `[]tmux.PaneCoord` the arm phase gathered) and `anomalous` (incremented when `applyLayoutWithFallback` hits its fallback/double-failure WARN, `applyActivePane` WARNs, or `applyZoom` WARNs).
- The existing per-step best-effort WARNs (`select-layout … failed`, `select-pane … failed`, `resize-pane -Z … failed`) are RETAINED — they are the per-item WARN for the anomalous count. Thread a small counter through `applyLayoutWithFallback` / `applyActivePane` / `applyZoom` (e.g. return a bool "ok" or increment a receiver-passed counter) so `ApplyWindowGeometry` can tally `anomalous` without duplicating the WARN lines.
- An empty saved-window group (`len(group) == 0`, "no live pane mapped to this saved window") is `continue`-skipped and contributes no work — it is not counted in `panes` beyond what `len(livePanes)` already reflects, and is not `anomalous`.
- After the per-window loop, emit `r.Logger.Info("geometry complete", "panes", len(livePanes), "took", time.Since(start))`, adding `"anomalous", anomalous` only when `anomalous > 0` (or always — choose always for grep-uniformity, matching the cycle-summary convention; prefer always-present `anomalous` for consistency with task 5-1).
- There is NO scrollback-replay step in this engine: scrollback replay is helper-driven via per-pane FIFOs (the hydrate helper), not part of `ApplyWindowGeometry`. The phase-B summary covers geometry only — do NOT invent a scrollback count here.
- A zero-pane session never reaches `ApplyWindowGeometry` (it is rejected by `validateTopology` upstream in `restore.go`, or `sr.Restore` errors out before geometry), so no geometry summary fires for it.
- Do NOT add new component/attr keys: `panes`, `took`, `anomalous` are all in the closed vocabulary.

**Acceptance Criteria**:
- [ ] A session reaching geometry replay emits one INFO `restore: geometry complete panes=N took=T` under component `restore`, where `panes == len(livePanes)`.
- [ ] A best-effort layout/select-pane/zoom failure increments `anomalous`, the existing per-step WARN still fires, and the geometry replay continues to the next step/window.
- [ ] An empty saved-window group is skipped and is not counted as anomalous.
- [ ] No scrollback-replay count appears in the summary (geometry only; replay is FIFO/helper-driven).
- [ ] A zero-pane session never reaches geometry and emits no geometry summary.

**Tests**:
- `"it emits geometry complete panes=N took=T over the live PaneCoord slice"`
- `"it increments anomalous and retains the per-step WARN on a select-layout double-failure"`
- `"it increments anomalous on a select-pane failure and on a zoom failure"`
- `"it skips an empty saved-window group without counting it as anomalous"`
- `"it emits no scrollback count in the geometry summary"`
- `"it does not emit a geometry summary for a session rejected before geometry (zero panes)"`

**Edge Cases**:
- Best-effort layout-fallback / select-pane / zoom failures counted as `anomalous` with the per-step WARN already present.
- Empty saved-window group skipped (not counted).
- Pane count sourced from the live `PaneCoord` slice (`len(livePanes)`).
- No scrollback-replay step in this engine (helper-driven via FIFO — summary covers geometry only).
- Zero-pane session never reaches geometry.

**Context**:
> "Restore phase B (geometry + replay) | `restore` | `restore: geometry complete panes=N took=T`" (spec § Cycle-level summary → Concrete cycle catalog)
>
> "`anomalous` — items that failed anomalously without terminating the cycle (each also emits a per-item WARN)." (spec § Cycle-level summary → Mechanical rule)
>
> Geometry is best-effort: a `select-layout` failure falls back to "tiled" and continues; any other per-step failure is logged and the next step/window proceeds. (`internal/restore/session.go` `ApplyWindowGeometry` / `applyLayoutWithFallback` / `applyActivePane` / `applyZoom` docs)
>
> Scrollback replay is driven by the per-pane hydrate helper through FIFOs (`respawn-pane -k` arms each pane with `portal state hydrate`), NOT by the restore engine — so phase B owns geometry only. (`internal/restore/session.go` package doc; CLAUDE.md § restore)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (Cycle-summary attr keys)

---

## portal-observability-layer-5-5 | approved

### Task 5-5: Emit the two cmd/bootstrap clean-sweep summaries `clean: orphan-daemon sweep complete killed=N` and `clean: marker sweep complete unset=N` (both with took)

**Problem**: The orphan-daemon sweep (`OrphanSweepCore.SweepOrphanDaemons`) and the stale-marker sweep (`MarkerCleanupCore.CleanStaleMarkers`) in `cmd/bootstrap` are loop cycles but emit only per-item lines (the orphan sweep emits a per-kill INFO `sweep: killed orphan daemon pid=N`; marker cleanup emits per-unset/per-malformed WARN). The spec's cycle catalog mandates ONE INFO summary each under component `clean`: `clean: orphan-daemon sweep complete killed=N took=T` and `clean: marker sweep complete unset=N took=T`. The existing per-kill INFO must be demoted to per-item DEBUG so the single summary is the steady-state truth.

**Solution**: Instrument both sweeps to capture `start := time.Now()`, track the sweep's outcome count (`killed` for orphan-daemon, `unset` for marker), demote the pre-existing per-kill INFO to DEBUG, and emit the cataloged INFO summary under component `clean` at the end of each sweep. The summary line flips the component to `clean` even though the surrounding orchestration runs under `bootstrap`.

**Outcome**: A bootstrap emits one `clean: orphan-daemon sweep complete killed=N took=T` (killed = count of successful SIGKILLs) and one `clean: marker sweep complete unset=N took=T` (unset = count of markers unset); identity-skips stay DEBUG, kill failures / unset failures / malformed lines stay WARN; the mass-unset-hazard deferral emits a summary with `unset=0` (or skips cleanly) rather than reporting a false unset.

**Do**:
- Add a package-level `var cleanLogger = log.For("clean")` in `cmd/bootstrap` (component `clean` per the closed taxonomy). The summary lines use `cleanLogger`; the per-item Debug/Warn lines stay on the existing `bootstrap`-bound logger seam (the `Logger` field), which is correct — the component flips to `clean` ONLY on the summary line.
- **Orphan-daemon sweep** (`cmd/bootstrap/orphan_sweep.go`, `SweepOrphanDaemons`): capture `start := time.Now()` at entry (after the no-op-logger / default-seam substitution). Declare `killed int`. On each successful `kill(pid)`, increment `killed` and DEMOTE the existing `logger.Info("sweep: killed orphan daemon pid=%d", pid)` line to a per-item DEBUG breadcrumb (`logger.Debug` under the bootstrap logger, or `cleanLogger.Debug("orphan killed", "target_pid", pid)` — pick one and keep `pid`/`target_pid` from the closed vocabulary). The identity-skip stays DEBUG (already is); the kill-failure and identity-check-failure stay WARN (already are). The `Pgrep`-failed and `SaverPanePID`-errored early-WARN paths: `Pgrep` failure returns before the loop — emit NO summary on that early return (the sweep did no work); the `SaverPanePID` error path proceeds with an empty legitimate set and reaches the summary normally.
- Before the final `return nil`, when the sweep actually ran (Pgrep succeeded), emit `cleanLogger.Info("orphan-daemon sweep complete", "killed", killed, "took", time.Since(start))`.
- **Marker sweep** (`cmd/bootstrap/stale_marker_cleanup.go`, `CleanStaleMarkers`): capture `start := time.Now()` at entry. Declare `unset int`. On each successful `Unsetter.UnsetServerOption(name)`, increment `unset`. Per-unset failures stay WARN (already are) and are NOT counted in `unset`. Malformed live-pane lines stay WARN (already, inside `parseLivePaneSet`).
- The two early returns in `CleanStaleMarkers` that return BEFORE any unset work: `ListSkeletonMarkers` error → `return err` (no summary — dependency failure); `ListAllPanesWithFormat` error → `return err` (no summary — dependency failure). The mass-unset-hazard guard (`len(live)==0 && len(markers)>0`) returns `nil` after a WARN — emit a summary `clean: marker sweep complete unset=0 took=T` here (the deferral processed zero unsets) OR skip the summary cleanly; the spec treats the deferral as a "successful soft outcome", so a `unset=0` summary is correct and must NOT report any false unset. The empty-markers + empty-live no-op (`return nil`) emits `unset=0`.
- Before the final `return` (the success/aggregate-error return after the unset loop), emit `cleanLogger.Info("marker sweep complete", "unset", unset, "took", time.Since(start))`. The aggregate `errors.Join` return value is unchanged; the summary is emitted regardless of per-unset failures (the failures are reflected as WARN, not in `unset`).
- Use the closed sweep-outcome keys: `killed` (orphan-daemon), `unset` (marker). Both summaries carry `took`. Do NOT add new keys.

**Acceptance Criteria**:
- [ ] The orphan-daemon sweep emits one INFO `clean: orphan-daemon sweep complete killed=N took=T` under component `clean`, where `killed` counts ONLY successful SIGKILLs.
- [ ] The pre-existing `sweep: killed orphan daemon pid=N` INFO is demoted to a per-item DEBUG (no longer INFO).
- [ ] Identity-skips remain DEBUG; kill-failures and identity-check-failures remain WARN; `killed` excludes skipped and failed kills.
- [ ] A `Pgrep` failure returns before the loop and emits no orphan-daemon summary; a `SaverPanePID` error proceeds with an empty legitimate set and still reaches the summary.
- [ ] The marker sweep emits one INFO `clean: marker sweep complete unset=N took=T` under component `clean`, where `unset` counts only successful unsets.
- [ ] The mass-unset-hazard deferral emits a summary with `unset=0` (or skips cleanly) — never a false unset — and the existing deferral WARN still fires; `ListSkeletonMarkers` / `ListAllPanesWithFormat` errors return before the summary.

**Tests**:
- `"it emits clean: orphan-daemon sweep complete killed=N counting only successful SIGKILLs"`
- `"it demotes the per-kill INFO to per-item DEBUG"`
- `"it keeps identity-skip at DEBUG and kill-failure at WARN, excluding both from killed"`
- `"it emits no orphan-daemon summary when Pgrep fails (returns before the loop)"`
- `"it proceeds to the summary with killed=0 when SaverPanePID errors (empty legitimate set)"`
- `"it emits clean: marker sweep complete unset=N counting only successful unsets"`
- `"it emits unset=0 (no false unset) on the mass-unset-hazard deferral and retains the deferral WARN"`
- `"it returns before the marker summary when ListSkeletonMarkers or ListAllPanesWithFormat fails"`

**Edge Cases**:
- Component flips from `bootstrap` to `clean` on the summary line only (per-item Debug/Warn stay under `bootstrap`).
- `killed` counts only successful SIGKILLs (identity-skip stays DEBUG, kill-failure stays WARN).
- Existing `sweep: killed orphan daemon pid=` INFO demoted to per-item DEBUG.
- Mass-unset hazard deferral emits `unset=0` (or skips cleanly), not a false unset.
- Empty-markers / empty-live no-op emits a `unset=0` summary.
- `Pgrep` / `list-panes` failure returns before the summary.

**Context**:
> "Orphan daemon sweep | `clean` | `clean: orphan-daemon sweep complete killed=N took=T`" and "Marker cleanup | `clean` | `clean: marker sweep complete unset=N took=T`" (spec § Cycle-level summary → Concrete cycle catalog)
>
> "Sweeps report their outcome keys instead (`reaped` / `killed` / `skipped` / `unset`)." (spec § Cycle-level summary → Mechanical rule)
>
> "Cycle-end summary at the end of a tick/iteration/batch (`tick complete`, `bootstrap step done`, `clean-stale entries=N`) | `Info`. Per-item DEBUG breadcrumb ALWAYS for items where the per-item path is interesting … silent at INFO." (spec § Log-level discipline → Mechanical level-selection table; § Cycle-level summary → Per-item event level)
>
> The mass-unset-hazard deferral "is a successful soft outcome ('skip this run; next bootstrap retries'), not a failure." (`cmd/bootstrap/stale_marker_cleanup.go` `CleanStaleMarkers` doc, step 4)
>
> Note: the foundation note pins that these sites already hold the migrated `*slog.Logger`; this task adds the `clean`-component summary and demotes the one INFO. The hooks `CleanStale` summary and the retention sweep summary are Phase 3 / Phase 2 respectively — NOT this task.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (Cycle-summary attr keys, component table)

---

## portal-observability-layer-5-6 | approved

### Task 5-6: Emit the orphan-FIFO sweep summary `clean: orphan-fifo sweep complete reaped=N skipped=N took=T` in `internal/state.SweepOrphanFIFOs`

**Problem**: `internal/state.SweepOrphanFIFOs` removes orphan `hydrate-*.fifo` files but emits a per-removal INFO (`removed orphan FIFO …`) under component `bootstrap` and no cycle summary. The spec's cycle catalog mandates ONE INFO summary under component `clean`: `clean: orphan-fifo sweep complete reaped=N skipped=N took=T`, and the per-removal INFO must be demoted to per-item DEBUG.

**Solution**: Instrument `SweepOrphanFIFOs` to capture `start := time.Now()`, track `reaped` (FIFOs removed) and `skipped` (non-FIFO siblings preserved + per-file lstat/remove failures skipped + live-marker-protected FIFOs left in place), demote the existing per-removal INFO to per-item DEBUG, flip the summary component from `bootstrap` to `clean`, and emit the cataloged summary at the end of the sweep.

**Outcome**: A sweep emits one INFO `clean: orphan-fifo sweep complete reaped=N skipped=N took=T` under component `clean`; a missing state dir / empty glob emits `reaped=0 skipped=0`; non-FIFO siblings and per-file failures are counted as `skipped`; a glob failure returns before the summary; the per-removal INFO is demoted to DEBUG.

**Do**:
- In `internal/state/fifo_sweep.go`, `SweepOrphanFIFOs(dir, liveMarkerKeys, logger)` is the cycle. Per the foundation note, `logger` is now the migrated `*slog.Logger` (the Phase 1 sweep replaced `*state.Logger`); the call site is bound to `log.For("clean")` for the summary, while the per-item lines (lstat/remove WARN) keep their wrapped-error attr. Confirm the function signature post-Phase-1 holds a `*slog.Logger`; if the production caller passes a `bootstrap`-bound logger, use a locally-derived `clean`-component logger for the summary (e.g. accept the component flip by binding `log.For("clean")` for the summary line) — the component flips to `clean` on the summary line per the catalog.
- Capture `start := time.Now()` at entry. Declare `reaped int`, `skipped int`.
- After the glob succeeds, iterate matches:
  - `os.Lstat` failure → keep the existing WARN (with the wrapped `error` attr per Phase 4), increment `skipped`, `continue`.
  - Non-FIFO sibling (`fi.Mode()&os.ModeNamedPipe == 0`) → preserve (existing behaviour), increment `skipped`, `continue`. (Comment the "why we preserve non-FIFO siblings" branch if Phase 4 task 4-6 did not already.)
  - Live-marker-protected FIFO (`liveMarkerKeys[paneKey]` present) → leave in place, increment `skipped`, `continue`.
  - `os.Remove` failure → keep the existing WARN (wrapped `error`), increment `skipped`, `continue`.
  - Successful removal → increment `reaped`, DEMOTE the existing `logger.Info("removed orphan FIFO %s", path)` to a per-item DEBUG (`logger.Debug("orphan fifo reaped", "path", path)` under the `clean` logger, using the `path` attr from the closed vocabulary).
- A `filepath.Glob` error returns `fmt.Errorf("glob fifos in %s: %w", dir, err)` BEFORE the loop — emit no summary on that early return (the orchestrator surfaces it as a soft warning).
- A missing state dir is "nothing to sweep" (the glob simply returns zero matches, not an error) — the loop body runs zero times and the summary fires with `reaped=0 skipped=0`.
- Before the final `return nil`, emit `cleanLogger.Info("orphan-fifo sweep complete", "reaped", reaped, "skipped", skipped, "took", time.Since(start))` (bind `cleanLogger = log.For("clean")` at package init in `internal/state` if not already present, or derive it locally — the summary's component prefix must be `clean`).
- Use the closed sweep-outcome keys `reaped` and `skipped` (`skipped` = orphan-fifo sweep skip count per the spec). Both with `took`. Do NOT add new keys.

**Acceptance Criteria**:
- [ ] A sweep emits one INFO `clean: orphan-fifo sweep complete reaped=N skipped=N took=T` under component `clean`.
- [ ] A missing state dir / empty glob emits `reaped=0 skipped=0` (the loop runs zero times).
- [ ] A non-FIFO sibling matching the glob is preserved and counted as `skipped`.
- [ ] A per-file `os.Lstat` or `os.Remove` failure emits its WARN (wrapped `error` attr), is counted as `skipped`, and the loop continues.
- [ ] A live-marker-protected FIFO is left in place and counted as `skipped`.
- [ ] The existing per-removal INFO is demoted to a per-item DEBUG.
- [ ] A `filepath.Glob` failure returns before the summary.

**Tests**:
- `"it emits clean: orphan-fifo sweep complete reaped=N skipped=N took=T"`
- `"it emits reaped=0 skipped=0 for a missing state dir / empty glob"`
- `"it preserves a non-FIFO sibling and counts it as skipped"`
- `"it WARNs and counts as skipped on an lstat failure / remove failure"`
- `"it leaves a live-marker-protected FIFO in place and counts it as skipped"`
- `"it demotes the per-removal INFO to per-item DEBUG"`
- `"it returns before the summary on a glob failure"`

**Edge Cases**:
- Missing state dir = nothing-to-sweep summary (`reaped=0 skipped=0`).
- Non-FIFO sibling preserved and counted as `skipped`.
- Per-file lstat/remove failure WARN and counted as `skipped`.
- Existing per-removal INFO demoted to per-item DEBUG.
- Component flips `bootstrap` → `clean` on the summary line.
- Glob failure returns before the summary.

**Context**:
> "Orphan FIFO sweep | `clean` | `clean: orphan-fifo sweep complete reaped=N skipped=N took=T`" (spec § Cycle-level summary → Concrete cycle catalog)
>
> "Sweeps report their outcome keys instead (`reaped` / `killed` / `skipped` / `unset`)." and "(`skipped` = orphan-fifo sweep skip count.)" (spec § Cycle-level summary → Mechanical rule; § Subsystem prefix taxonomy → Cycle-summary attr keys)
>
> "Non-FIFO files matching the glob (regular files, symlinks) are preserved so a misconfigured filesystem entry is not silently destroyed. Per-file errors are logged via logger and skipped — one bad entry must not abort the rest of the sweep. A missing state directory is treated as 'nothing to sweep' and returns nil silently." (`internal/state/fifo_sweep.go` `SweepOrphanFIFOs` doc)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Cycle-level summary cadence and shape (Mechanical rule, Concrete cycle catalog), § Subsystem prefix taxonomy (Cycle-summary attr keys, component table)

---

## portal-observability-layer-5-7 | approved

### Task 5-7: Emit saver create/respawn/ready lifecycle INFO events (`placeholder created`, `destroy-unattached off`, `respawn-daemon`, `daemon ready`) in portal_saver.go

**Problem**: The `_portal-saver` bootstrap flow (`BootstrapPortalSaver` in `internal/tmux/portal_saver.go`) creates the placeholder session, sets `destroy-unattached=off`, respawns the pane as `portal state daemon`, and waits for readiness — but emits no lifecycle events. The spec's saver/daemon lifecycle catalog mandates four INFO events under component `saver`, emitted by bootstrap observing the saver from outside: `placeholder created`, `destroy-unattached off`, `respawn-daemon`, `daemon ready`. The current code does not capture the placeholder pane's `tmux_pane` id or the placeholder/daemon pids that several of these events require.

**Solution**: Instrument `BootstrapPortalSaver` to emit the four cataloged `saver:` INFO events at their exact moments, capturing the pane id (`tmux_pane`) and the pre-respawn placeholder pid (`from_pid`) / post-respawn daemon pid (`to_pid`) / ready daemon pid (`target_pid`) via tmux queries added at the create branch. The respawn-daemon and daemon-ready events fire ONLY on the create branch (not the alive happy path); destroy-unattached-off fires on both the create branch and the defensive happy-path set-option.

**Outcome**: A bootstrap that creates the saver emits `saver: placeholder created tmux_pane=%N pid=…`, `saver: destroy-unattached off tmux_pane=%N`, `saver: respawn-daemon from_pid=P to_pid=D tmux_pane=%N`, and (on readiness success) `saver: daemon ready target_pid=D version=…`; the alive happy path emits only `saver: destroy-unattached off` (the defensive set-option) and neither respawn-daemon nor daemon-ready; a readiness timeout keeps its existing WARN and emits no `daemon ready`.

**Do**:
- Add a package-level `var saverLogger = log.For("saver")` in `internal/tmux/portal_saver.go` (component `saver` per the closed taxonomy).
- **`placeholder created`** — fires after `createPortalSaverWithRetry` succeeds (the create branch only). Required attrs: `tmux_pane` (the placeholder pane's live tmux pane id, e.g. `%42`) and `pid` (auto-baseline — the bootstrap process emitting this; it is NOT passed at the call site). To obtain `tmux_pane`, add a tmux query for the placeholder pane's id immediately after creation (e.g. `list-panes -t =_portal-saver -F '#{pane_id}'` via a new `Client` helper or an inline `Run`, since no `tmux_pane`-returning helper exists today). Emit `saverLogger.Info("placeholder created", "tmux_pane", paneID)`.
- **`destroy-unattached off`** — fires immediately after the `SetSessionOption(PortalSaverName, "destroy-unattached", "off")` call succeeds. This call runs on BOTH branches (create branch AND the alive happy-path defensive set), so the event fires on both. Required attr: `tmux_pane` (the saver pane id). Emit `saverLogger.Info("destroy-unattached off", "tmux_pane", paneID)`. (On the alive happy path the pane id must also be queried — reuse the same `#{pane_id}` helper.)
- **`respawn-daemon`** — fires on the create branch, around the `RespawnPane(PortalSaverName, portalSaverDaemonCommand)` call. Required attrs: `from_pid` (the placeholder pid, captured BEFORE the respawn — e.g. via `tmux.SaverPanePID(c, PortalSaverName)` pre-respawn), `to_pid` (the daemon pid, captured AFTER the respawn — `SaverPanePID` post-respawn), and `tmux_pane` (the pane id, unchanged by `-k`). Capture `from_pid` before `RespawnPane`, run the respawn, then capture `to_pid`. Emit `saverLogger.Info("respawn-daemon", "from_pid", fromPID, "to_pid", toPID, "tmux_pane", paneID)`. If a pid read fails, log the read failure per Phase 4 (wrapped `error`) and emit the event with the pid attr it could obtain (best-effort — the lifecycle event must still fire; do not abort the bootstrap for an observability pid-read miss).
- **`daemon ready`** — fires ONLY on readiness-barrier SUCCESS, NOT on timeout. The readiness barrier is `waitForSaverDaemonReady` (returns nil on both success and timeout, distinguishing internally via `isSaverDaemonReady`). Move/emit the `daemon ready` INFO at the success-return path inside `waitForSaverDaemonReady` (the `if isSaverDaemonReady(stateDir) { return nil }` branch), reading the daemon pid via `saver.ReadPID(stateDir)` to populate `target_pid`. Required attrs: `target_pid` (the daemon pid) and `version` (auto-baseline). Emit `saverLogger.Info("daemon ready", "target_pid", daemonPID)` (version is auto-injected). The timeout path keeps its existing WARN ("saver respawn: daemon did not come up within …") and emits NO `daemon ready`.
- `tmux_pane`, `from_pid`, `to_pid`, `target_pid` are all in the closed contextual/lifecycle attr vocabulary. Do NOT add new keys.
- These events are emitted by bootstrap observing the saver from outside (per the process/subsystem boundary) — they are additive to the saver process's own `process:` lines, never a substitute.

**Acceptance Criteria**:
- [ ] On the create branch, `saver: placeholder created tmux_pane=%N` is emitted after `createPortalSaverWithRetry` succeeds (pid is the auto-baseline of the bootstrap process).
- [ ] `saver: destroy-unattached off tmux_pane=%N` is emitted after the `destroy-unattached=off` set-option succeeds, on BOTH the create branch and the alive happy-path defensive set.
- [ ] On the create branch, `saver: respawn-daemon from_pid=P to_pid=D tmux_pane=%N` is emitted around `RespawnPane`, with `from_pid` captured pre-respawn and `to_pid` post-respawn.
- [ ] `saver: daemon ready target_pid=D` (with `version` auto-baseline) is emitted ONLY on readiness-barrier success; the readiness timeout keeps its WARN and emits no `daemon ready`.
- [ ] `respawn-daemon` and `daemon ready` are NOT emitted on the alive happy path (no respawn, no readiness wait).
- [ ] A pid-read failure while capturing `from_pid`/`to_pid`/`target_pid` does not abort the bootstrap; the event still fires best-effort and the read failure is logged with the wrapped `error` attr.

**Tests**:
- `"it emits placeholder created with tmux_pane after creating the placeholder"`
- `"it emits destroy-unattached off with tmux_pane on the create branch"`
- `"it emits destroy-unattached off on the alive happy-path defensive set-option"`
- `"it emits respawn-daemon with from_pid/to_pid/tmux_pane around respawn-pane -k"`
- `"it emits daemon ready with target_pid only on readiness-barrier success"`
- `"it emits no daemon ready and keeps the WARN on a readiness timeout"`
- `"it emits neither respawn-daemon nor daemon ready on the alive happy path"`
- `"it still emits respawn-daemon best-effort when a pane-pid read fails, logging the wrapped error"`

**Edge Cases**:
- `respawn-daemon` and `daemon ready` fire only on the create branch (not the alive happy path).
- `from_pid`/`to_pid` captured around `respawn-pane -k` (before/after).
- `daemon ready` fires only on readiness-barrier success (timeout path keeps its WARN with no `daemon ready`).
- `tmux_pane` source for the placeholder pane requires a new `#{pane_id}` query (no existing helper returns it).
- `destroy-unattached off` fires on create AND on the defensive happy-path set-option.
- `target_pid`/`version` attrs on `daemon ready` (version auto-injected).

**Context**:
> Saver lifecycle events (component `saver`): "Bootstrap creates the `_portal-saver` placeholder pane | `placeholder created` | `tmux_pane`, `pid` (auto-baseline)"; "Bootstrap turns off `destroy-unattached` on the placeholder session | `destroy-unattached off` | `tmux_pane`"; "Bootstrap respawns the placeholder pane as `portal state daemon` | `respawn-daemon` | `from_pid` (placeholder pid), `to_pid` (daemon pid, post-respawn), `tmux_pane`"; "Bootstrap observes the daemon up and ready (after the 2s readiness barrier) | `daemon ready` | `target_pid` (the daemon pid), `version` (auto-baseline)". (spec § Saver and daemon lifecycle event taxonomy → Mechanical rule, saver table)
>
> "`saver:` lines are emitted by bootstrap observing the saver from outside (a different observer than the saver's own `process:` lines), so they are not redundant." (spec § Saver and daemon lifecycle → Process/subsystem boundary)
>
> "`cmd/bootstrap/` — all `saver:` lifecycle events …" — though the actual create/respawn/ready code lives in `internal/tmux/portal_saver.go` (the kill-barrier escalation also lives there per the same § Calling code locations). The instrumentation lands at the code site; the component is `saver` either way. (spec § Saver and daemon lifecycle → Calling code locations)
>
> `BootstrapPortalSaver` flow: create branch = createPortalSaverWithRetry → SetSessionOption(destroy-unattached off) → RespawnPane(daemon) → waitForSaverDaemonReady; alive happy path = SetSessionOption only (no RespawnPane). `SaverPanePID(c, session)` reads `#{pane_pid}`; there is no existing `#{pane_id}` helper. (`internal/tmux/portal_saver.go`, `internal/tmux/tmux.go`)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Saver and daemon lifecycle event taxonomy (Mechanical rule saver table, Process/subsystem boundary, Calling code locations), § Subsystem prefix taxonomy (Lifecycle/contextual attr keys)

---

## portal-observability-layer-5-8 | approved

### Task 5-8: Emit saver kill-barrier lifecycle INFO events (`kill-barrier started`, `kill-barrier escalated reason=kill-session-timeout`, `placeholder died`)

**Problem**: The Component A kill-barrier (`killSaverAndWaitForDaemon` / `escalateKillToSIGKILL` in `internal/tmux/portal_saver.go`) kills a prior daemon and escalates to SIGKILL on timeout, but emits only WARN lines on the failure/skip paths and no lifecycle INFO. The spec's lifecycle catalog mandates three INFO events under component `saver`: `kill-barrier started`, `kill-barrier escalated reason=kill-session-timeout`, and `placeholder died`. The Phase-4 SIGKILL-escalation DEBUG breadcrumb already exists in `escalateKillToSIGKILL`; the `kill-barrier escalated` INFO must sit ABOVE it (do not duplicate the breadcrumb).

**Solution**: Instrument the kill-barrier to emit `kill-barrier started target_pid=X` once per kill invocation after the prior-PID alive probe confirms there is a live daemon to kill, `kill-barrier escalated target_pid=X reason=kill-session-timeout` on the identity-confirmed SIGKILL escalation branch (above the existing DEBUG breadcrumb), and `placeholder died target_pid=X reason=…` when the barrier observes the prior daemon's host process exit during the poll. All three are INFO under the `saver` component.

**Outcome**: A kill-barrier invocation that finds a live prior daemon emits `saver: kill-barrier started target_pid=X`; on poll timeout with identity confirmation it emits `saver: kill-barrier escalated target_pid=X reason=kill-session-timeout` (above the existing Phase-4 DEBUG breadcrumb); when the prior PID is observed to exit during the poll it emits `saver: placeholder died target_pid=X reason=<signal|exit|unknown>`; the no-prior-PID and already-dead tolerant-kill shortcuts emit none of these; the identity-skip WARN path emits no `escalated`; at most one WARN per invocation is preserved.

**Do**:
- Use the `saverLogger = log.For("saver")` from task 5-7 (or add it if 5-7 is not yet applied; either way component `saver`).
- **`kill-barrier started`** — fires once per kill invocation AFTER the prior-PID alive probe confirms a live daemon. In `killSaverAndWaitForDaemon`, the started event belongs at the point where the code commits to killing a live prior daemon: after `saver.Barrier.IsAlive(priorPID)` returns true (the "Prior daemon alive — issue kill-session once and wait for exit" branch), immediately before `c.KillSession(PortalSaverName)`. Emit `saverLogger.Info("kill-barrier started", "target_pid", priorPID)`. Do NOT emit it on the two tolerant-kill shortcuts: (a) `readErr != nil` (no usable prior PID), and (b) `!saver.Barrier.IsAlive(priorPID)` (prior daemon already dead) — neither path actually starts a barrier against a live daemon.
- **`kill-barrier escalated`** — fires in `escalateKillToSIGKILL`, ONLY on the `IdentifyIsPortalDaemon` escalation branch (the branch that actually sends SIGKILL). It sits ABOVE the existing Phase-4 DEBUG breadcrumb (which fires at the escalation decision just before the SIGKILL syscall). Required attrs: `target_pid` (priorPID) and `reason="kill-session-timeout"` (the only value in the closed reason space). Emit `saverLogger.Info("kill-barrier escalated", "target_pid", priorPID, "reason", "kill-session-timeout")` on the branch where `err == nil && result == state.IdentifyIsPortalDaemon`, immediately before (or paired with) the existing escalation DEBUG breadcrumb and the `saver.Barrier.SendSIGKILL(priorPID)` call. Do NOT emit it on the identity-skip branch (`err != nil || result != IdentifyIsPortalDaemon`), which keeps its existing WARN and returns nil without signalling. Do NOT add a second/duplicate DEBUG breadcrumb — that already exists from Phase 4 task 4-4.
- **`placeholder died`** — fires when the barrier observes the prior daemon's host process has exited (e.g. during the kill-barrier exit-poll). The natural site is when `waitForPriorPIDExit(priorPID, …)` returns `true` (observed exit) — both in the session-kill wait stage (`killSaverAndWaitForDaemon`) and the post-SIGKILL escalation poll (`escalateKillToSIGKILL`). Required attrs: `target_pid` (the dead pid) and `reason ∈ {signal, exit, unknown}`. Emit `saverLogger.Info("placeholder died", "target_pid", priorPID, "reason", reason)` on the observed-exit branch. The `reason` classification (signal vs exit vs unknown) is derived from how the exit was observed: after a SIGKILL escalation the exit is `signal`; after a clean `kill-session`-driven exit the host process typically exits via SIGHUP — classify as `signal` if the barrier sent/triggered a signal, `exit` if the process exited without a barrier-sent signal, `unknown` if the cause cannot be determined from the poll. See `[needs-info]` below on the limits of distinguishing these from the current poll, which observes only liveness (`IsAlive` true→false), not the exit cause.
- Preserve the at-most-one-WARN-per-invocation contract: the new INFO lifecycle events are additive and do not change the WARN paths (identity-skip WARN, post-SIGKILL-survival WARN).
- `target_pid`, `reason` are in the closed lifecycle attr vocabulary; `kill-session-timeout` and `{signal, exit, unknown}` are the closed reason value spaces. Do NOT introduce new reason values.

**Acceptance Criteria**:
- [ ] `saver: kill-barrier started target_pid=X` is emitted once per kill invocation, after the prior-PID alive probe confirms a live daemon and before `kill-session`.
- [ ] `kill-barrier started` is NOT emitted on the no-prior-PID (`readErr != nil`) or already-dead (`!IsAlive`) tolerant-kill shortcuts.
- [ ] `saver: kill-barrier escalated target_pid=X reason=kill-session-timeout` is emitted only on the `IdentifyIsPortalDaemon` escalation branch, above the existing Phase-4 DEBUG breadcrumb, with no duplicate breadcrumb.
- [ ] The identity-skip WARN path (`err != nil || result != IdentifyIsPortalDaemon`) emits no `escalated` line and keeps its single WARN.
- [ ] `saver: placeholder died target_pid=X reason=<signal|exit|unknown>` is emitted on an observed-exit poll branch with a closed reason value.
- [ ] At most one WARN per invocation is preserved (the new INFO events do not add or remove WARN lines).

**Tests**:
- `"it emits kill-barrier started target_pid once when the prior daemon is alive"`
- `"it emits no kill-barrier started on the no-prior-PID tolerant-kill shortcut"`
- `"it emits no kill-barrier started when the prior daemon is already dead"`
- `"it emits kill-barrier escalated reason=kill-session-timeout only on the IdentifyIsPortalDaemon branch, above the existing DEBUG breadcrumb"`
- `"it emits no kill-barrier escalated and keeps the single WARN on the identity-skip branch"`
- `"it emits placeholder died with a closed reason value on an observed exit"`
- `"it preserves the at-most-one-WARN-per-invocation contract"`

**Edge Cases**:
- `started` fires once per kill invocation after the prior-PID alive probe (not on the no-prior-PID / already-dead tolerant-kill shortcuts).
- `escalated` fires only on the `IdentifyIsPortalDaemon` escalation branch and sits above the Phase-4 escalation DEBUG breadcrumb (no duplicate).
- Identity-skip WARN path emits no `escalated` line.
- `placeholder died reason ∈ {signal, exit, unknown}` from exit-poll observation.
- At-most-one-WARN-per-invocation contract preserved.

**Context**:
> Saver lifecycle (component `saver`): "Bootstrap initiates the kill-barrier (Component A) for a prior daemon | `kill-barrier started` | `target_pid` (the prior daemon pid being killed)"; "Bootstrap escalates from `kill-session` to direct SIGKILL on the prior daemon | `kill-barrier escalated` | `target_pid`, `reason=\"kill-session-timeout\"`"; "Bootstrap observes a saver pane's host process has exited (e.g. during the kill-barrier exit-poll) | `placeholder died` | `target_pid` (the dead pid), `reason` ∈ {`signal`, `exit`, `unknown`}". (spec § Saver and daemon lifecycle event taxonomy → Mechanical rule, saver table)
>
> "`escalateKillToSIGKILL` … no breadcrumb on the SIGKILL escalation path | DEBUG breadcrumb at the escalation decision, beneath the `saver: kill-barrier escalated` INFO lifecycle event." (spec § Diagnostic context preservation → Enumerated gap-closure sites) — the breadcrumb is Phase 4 (task 4-4); the INFO above it is THIS task.
>
> "`kill-barrier escalated reason`: `kill-session-timeout` (only value today; new values require amendment). `placeholder died reason`: `signal` / `exit` / `unknown`." (spec § Saver and daemon lifecycle → Reason value spaces)
>
> "an unpaired `process: start` is alarming only if no `saver:`/`daemon:` line names that pid as an external kill … Bootstrap emits `saver: kill-barrier started/escalated target_pid=X` and `saver: placeholder died`." (spec § Defensive invariants → Externally-killed-process footnote)
>
> `[needs-info]`: The current kill-barrier poll (`waitForPriorPIDExit` / `saver.Barrier.IsAlive`) observes only liveness (alive true→false), not the exit cause — it cannot read a wait-status or distinguish a SIGHUP-driven graceful exit from a SIGKILL-forced one at the OS level for a non-child process. The `placeholder died reason` therefore must be derived from barrier context, not a syscall: `signal` after the barrier's SIGKILL escalation, `exit`/`signal` after the `kill-session` (kill-session triggers tmux to deliver SIGHUP to the daemon → arguably `signal`), `unknown` when the prior PID was observed gone on the first probe without the barrier acting. The executor should pick the most defensible mapping from observable barrier state and document it; if a single observed-exit site cannot cleanly distinguish `signal` vs `exit`, prefer `unknown` over inventing certainty. Do not add a new reason value.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Saver and daemon lifecycle event taxonomy (Mechanical rule saver table, Reason value spaces), § Diagnostic context preservation (Enumerated gap-closure sites), § Defensive invariants (Externally-killed-process footnote)

---

## portal-observability-layer-5-9 | approved

### Task 5-9: Emit daemon `lock acquired` (carrying tmux_pane, replacing the dropped `daemon: spawn`) and normal-path `shutdown` (reason + flush_completed)

**Problem**: The daemon has no lifecycle INFO for acquiring its singleton lock and no structured shutdown event. The spec's lifecycle catalog mandates `daemon: lock acquired` (carrying `tmux_pane`, since the redundant `daemon: spawn` event is dropped and its one unique attr `tmux_pane` moves here) and `daemon: shutdown reason=… flush_completed=…` on the normal return path. Today `defaultDaemonRun` acquires the lock with no INFO, and `defaultShutdownFlush` logs ad-hoc "skipping final flush" / "final flush" INFO lines under `daemon` without the cataloged `reason`/`flush_completed` shape.

**Solution**: Instrument `defaultDaemonRun` to emit `daemon: lock acquired pid=… tmux_pane=…` immediately after `acquireDaemonLock` + `WritePIDFile` succeed (the post-pre-check acquire), reading `tmux_pane` from `$TMUX_PANE`. Instrument `defaultShutdownFlush` to emit one `daemon: shutdown reason=… flush_completed=…` on the normal return path, carrying the shutdown reason and whether the final commit completed. The self-eject path (task 5-10) deliberately does NOT route through `defaultShutdownFlush`, so no `shutdown` line fires there.

**Outcome**: A daemon that acquires its lock emits `daemon: lock acquired pid=D tmux_pane=%N`; a normal shutdown emits exactly one `daemon: shutdown reason=<sighup|signal|exit> flush_completed=<bool>` (with `flush_completed=false` on the restoring-skip and capture-error branches); the lock-held and non-EWOULDBLOCK lock-error paths emit no `lock acquired` (keeping their WARN); the self-eject path emits no `shutdown`.

**Do**:
- Per the foundation note, `cmd/state_daemon.go` already holds `*slog.Logger` + `log.For("daemon")` (the migrated daemon logger). Use it for these events.
- **`lock acquired`** — in `defaultDaemonRun`, after the `acquireDaemonLock` success and the `WritePIDFile` success (the adjacency-pinned acquire+pidfile block), and after `daemonLockFile = lockFile`, emit `daemon: lock acquired`. Required attrs: `pid` (auto-baseline — the daemon's own pid) and `tmux_pane` (read from `$TMUX_PANE` via `os.Getenv("TMUX_PANE")`). Emit `daemonLogger.Info("lock acquired", "tmux_pane", os.Getenv("TMUX_PANE"))` (pid is auto-injected). Do NOT emit it on the two failure branches: `errors.Is(err, state.ErrDaemonLockHeld)` (lock held — keeps its WARN and returns nil) and the non-EWOULDBLOCK lock error (keeps its WARN and returns the wrapped error). The dropped `daemon: spawn` event is NOT re-introduced — `process: start process_role=daemon` (Phase 2) already marks the daemon's process startup; `lock acquired` is the additive subsystem milestone carrying the orphaned `tmux_pane` attr.
- **`shutdown`** — in `defaultShutdownFlush`, replace the ad-hoc INFO lines with the cataloged event on the normal return path. Required attrs: `reason ∈ {sighup, signal, exit}` and `flush_completed` (bool — whether the final commit completed). Emit exactly one `daemon: shutdown` before each `return nil`:
  - Restoring-skip branch (`restoring == true`): `flush_completed=false` (no flush performed).
  - Read-`@portal-restoring`-errored branch: `flush_completed=false` (conservatively skipped).
  - Flush-attempted branch: `flush_completed=true` if `captureAndCommit` returned nil, `false` if it returned an error (the existing WARN on `captureAndCommit` error is retained).
  Keep any of the existing diagnostic detail as DEBUG breadcrumbs if useful, but the cataloged INFO is the one `shutdown` line per invocation.
- **`reason` — `[needs-info]` (preserved from the designer):** the `reason ∈ {sighup, signal, exit}` distinction CANNOT be produced today. `cmd/state_daemon.go`'s `RunE` registers `signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)` into ONE channel and the goroutine only calls `cancel()` — it does NOT record WHICH signal arrived. Distinguishing `sighup` from `signal` (and from a non-signal `exit`, i.e. a `ctx` cancellation not driven by a signal) requires a small behavioural addition: capture the received `os.Signal` value (e.g. read it from `sigCh` into a package-or-closure-scoped var, or thread it into `daemonDeps`/the shutdown call) and map `syscall.SIGHUP → "sighup"`, `syscall.SIGTERM → "signal"`, and a non-signal ctx-cancel → `"exit"`. The executor MUST implement this capture so `reason` is real, OR, if scope-limited, emit `reason` from the best available signal and flag the residual gap — but the spec requires the three distinct values, so the capture is in-scope for this task. Do NOT hardcode a single reason.
- `tmux_pane`, `reason`, `flush_completed` are in the closed contextual/lifecycle attr vocabulary; `{sighup, signal, exit}` is the closed reason value space. Do NOT add new keys/values.
- The self-eject path is handled in task 5-10 and must NOT emit `shutdown` (it bypasses `daemonShutdownFunc`).

**Acceptance Criteria**:
- [ ] `daemon: lock acquired pid=D tmux_pane=%N` is emitted after the lock + pidfile acquire succeed, with `tmux_pane` read from `$TMUX_PANE` and `pid` as the auto-baseline.
- [ ] The lock-held (`ErrDaemonLockHeld`) and non-EWOULDBLOCK lock-error paths emit no `lock acquired` and keep their WARN.
- [ ] No `daemon: spawn` event is emitted (it is dropped; `process: start process_role=daemon` covers startup).
- [ ] `defaultShutdownFlush` emits exactly one `daemon: shutdown reason=… flush_completed=…` per invocation on the normal return path.
- [ ] `flush_completed=false` on the restoring-skip and read-error branches; `flush_completed` reflects the `captureAndCommit` result on the flush-attempted branch.
- [ ] `reason` distinguishes `sighup` / `signal` / `exit` via the captured received-signal value (a behavioural addition per the `[needs-info]`); it is not hardcoded.

**Tests**:
- `"it emits daemon: lock acquired with tmux_pane from $TMUX_PANE after a successful acquire"`
- `"it emits no lock acquired and keeps the WARN when the lock is held (ErrDaemonLockHeld)"`
- `"it emits no lock acquired and keeps the WARN on a non-EWOULDBLOCK lock error"`
- `"it emits one daemon: shutdown reason=sighup flush_completed=true on a clean SIGHUP shutdown"`
- `"it emits daemon: shutdown flush_completed=false on the restoring-skip branch"`
- `"it emits daemon: shutdown flush_completed=false when the final captureAndCommit errors"`
- `"it maps SIGTERM to reason=signal and a non-signal ctx-cancel to reason=exit"`
- `"it emits no daemon: shutdown on the self-eject path"` (cross-check with task 5-10)

**Edge Cases**:
- `tmux_pane` read from `$TMUX_PANE` per the process/subsystem boundary.
- Lock-held and non-EWOULDBLOCK lock-error paths emit no `lock acquired` (keep their WARN).
- `shutdown reason` distinguishes `sighup`/`signal`/`exit` (`[needs-info]` — requires capturing the received signal value; current `signal.Notify` merges SIGHUP+SIGTERM into one channel and only calls `cancel()`).
- `flush_completed=false` on the restoring-skip and capture-error shutdown branches.
- `shutdown` NOT emitted on the self-eject path (task 5-10).

**Context**:
> Daemon lifecycle (component `daemon`): "Daemon acquires `daemon.lock` (post-pre-check) | `lock acquired` | `pid` (auto-baseline), `tmux_pane`"; "Daemon shutdown via the normal return path (SIGHUP, signal, normal exit) | `shutdown` | `reason` ∈ {`sighup`, `signal`, `exit`}, `flush_completed` (bool — whether the final commit completed). **Not emitted on the self-eject path**." (spec § Saver and daemon lifecycle event taxonomy → Mechanical rule, daemon table)
>
> "A redundant `daemon: spawn` event (it would fire at the same instant as `process: start process_role=daemon`, carrying the same data) is therefore **dropped**; its one unique attr (`tmux_pane`) moves onto `daemon: lock acquired`." (spec § Saver and daemon lifecycle → Process/subsystem boundary)
>
> "`daemon shutdown reason`: `sighup` / `signal` / `exit`. (`self-eject` is **not** a `daemon: shutdown` reason …)" (spec § Saver and daemon lifecycle → Reason value spaces)
>
> "`cmd/state_daemon.go` — daemon lifecycle (`lock acquired`, `self-eject`, `shutdown`). The daemon's process startup is marked by `process: start process_role=daemon`, not a `daemon:` event." (spec § Saver and daemon lifecycle → Calling code locations)
>
> Current shutdown handler shape: `RunE` does `signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM)` then a goroutine `<-sigCh; cancel()` — the received signal value is discarded. `defaultShutdownFlush` has three return paths (read-error skip, restoring skip, flush-attempted). (`cmd/state_daemon.go`)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Saver and daemon lifecycle event taxonomy (Mechanical rule daemon table, Process/subsystem boundary, Reason value spaces, Calling code locations), § Subsystem prefix taxonomy (Lifecycle attr keys)

---

## portal-observability-layer-5-10 | approved

### Task 5-10: Emit daemon `self-eject ticks=N threshold=N` then `log.Close(0)` then `os.Exit(0)` at the hysteresis trip with load-bearing ordering

**Problem**: The daemon's self-supervision self-eject currently logs an ad-hoc INFO `"self-supervision: saver-membership lost for N consecutive ticks, exiting"` and calls `osExit(0)` directly, bypassing `daemonShutdownFunc`. The spec replaces that line with the cataloged `daemon: self-eject ticks=N threshold=N` AND requires the eject to be a paired terminal marker: it must FIRST emit `self-eject`, THEN call `log.Close(0)` (which emits `process: exit code=0` without itself exiting), THEN call `osExit(0)`. Without `log.Close(0)`, the SIGKILL-style direct exit would leave an unpaired `process: start` — the exact "process vanished without a terminal marker" alarm class the feature exists to eliminate.

**Solution**: In `defaultDaemonTickLoop` (`cmd/state_daemon.go`), at the hysteresis trip (`consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks`), replace the existing ad-hoc INFO with the cataloged sequence: `daemonLogger.Info("self-eject", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)`, then `log.Close(0)`, then `osExit(0)`. Per-tick probe failures below the threshold stay DEBUG with no INFO. `daemonShutdownFunc` is deliberately NOT called (no `daemon: shutdown` on this path).

**Outcome**: At the trip, the daemon emits `daemon: self-eject ticks=N threshold=3` followed by `process: exit code=0` (via `log.Close(0)`) and then exits via `osExit(0)`; no `daemon: shutdown` line fires; per-tick probe failures before the trip are DEBUG only; the four-way terminal classification holds (the daemon's `process: start` is paired by the `process: exit`).

**Do**:
- In `cmd/state_daemon.go` `defaultDaemonTickLoop`, locate the eject branch (`if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks { … }`).
- REPLACE the existing `deps.Logger.Info(... "self-supervision: saver-membership lost for %d consecutive ticks, exiting" ...)` line with the cataloged event: `daemonLogger.Info("self-eject", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)` (component `daemon`; `ticks` = the consecutive-absence count at the trip; `threshold` = `selfSupervisionHysteresisTicks`). Both attrs are in the closed lifecycle vocabulary.
- IMMEDIATELY AFTER the `self-eject` INFO, call `log.Close(0)` (the Phase 1/2 marker-emitter, which emits `process: exit code=0 took=…` and does NOT call `os.Exit`). This pairs the daemon's terminal marker.
- THEN call `osExit(0)` (the package-level `os.Exit` seam — NOT bare `os.Exit`, so tests can record the eject without terminating the test process). The ordering `self-eject → log.Close(0) → osExit(0)` is LOAD-BEARING: emit before close before exit. Do not reorder, do not skip `log.Close(0)`.
- Do NOT call `daemonShutdownFunc` / `defaultShutdownFlush` on this path — the divergent-view daemon must NOT run one more `captureAndCommit` cycle on its way out (same reasoning as Component A's straight-to-SIGKILL). This means NO `daemon: shutdown` line on the self-eject path (task 5-9's shutdown event is the normal-return path only).
- Per-tick probe failures BELOW the threshold (`consecutiveAbsenceTicks++` increments that have not yet reached the threshold) stay DEBUG and emit NO INFO until the trip — keep/add a DEBUG breadcrumb per failing probe (e.g. `daemonLogger.Debug("saver-membership probe failed", "ticks", consecutiveAbsenceTicks, "threshold", selfSupervisionHysteresisTicks)`) consistent with the hysteresis-internal-failures clarification. A passing probe resets the counter to 0 (existing behaviour, unchanged).
- The eject is reached from inside the ticker `select`; after `osExit(0)` the unreachable `return nil` is retained (existing pattern) so the test seam that stubs `osExit` to a no-op falls through cleanly.

**Acceptance Criteria**:
- [ ] At the trip, the daemon emits `daemon: self-eject ticks=N threshold=3` where `ticks == consecutiveAbsenceTicks` at the trip and `threshold == selfSupervisionHysteresisTicks`.
- [ ] The ordering is exactly `self-eject` INFO → `log.Close(0)` → `osExit(0)`; `log.Close(0)` emits `process: exit code=0` and does not itself exit.
- [ ] No `daemon: shutdown` line is emitted on the self-eject path (`daemonShutdownFunc` is not called).
- [ ] Per-tick probe failures below the threshold emit DEBUG only (no INFO until the trip); a passing probe resets the counter to 0.
- [ ] The eject uses the `osExit` seam, not bare `os.Exit`.
- [ ] The existing ad-hoc "self-supervision: saver-membership lost" INFO line is removed (replaced by `self-eject`).

**Tests**:
- `"it emits daemon: self-eject ticks=N threshold=3 at the hysteresis trip"`
- `"it emits self-eject then process: exit (via log.Close) then calls osExit in that order"`
- `"it does not emit daemon: shutdown on the self-eject path"`
- `"it emits DEBUG-only per-tick probe failures below the threshold with no INFO"`
- `"it resets the absence counter to 0 on a passing probe"`
- `"it uses the osExit seam (recorded in test) not bare os.Exit"`
- `"it removes the legacy self-supervision: saver-membership lost INFO line"`

**Edge Cases**:
- Ordering `self-eject → Close → osExit` is paired-terminal-marker critical (no double terminal marker, no `daemon: shutdown`).
- `ticks == consecutiveAbsenceTicks` at the trip; `threshold == selfSupervisionHysteresisTicks`.
- Per-tick probe failures stay DEBUG with no INFO until the trip.
- `daemonShutdownFunc` deliberately bypassed.
- `osExit` seam used, not bare `os.Exit`.
- Replaces the existing "self-supervision: saver-membership lost" INFO line.

**Context**:
> Daemon lifecycle (component `daemon`): "Daemon's self-supervision counter trips threshold and ejects | `self-eject` | `ticks` (consecutive-absence count at trip), `threshold` (configured ejection threshold)"; "Daemon's self-supervision counter increments toward eject | (no INFO — DEBUG per the level-discipline 'hysteresis-internal failures' clarification)". (spec § Saver and daemon lifecycle event taxonomy → Mechanical rule, daemon table)
>
> "One sanctioned exception — the daemon self-eject. The daemon's self-supervision self-eject calls `os.Exit(0)` directly … To stay marked, it FIRST emits `daemon: self-eject ticks=N threshold=N`, THEN calls `log.Close(0)` (which emits `process: exit code=0` and does not itself exit), THEN `os.Exit(0)`. So the termination is fully paired and the four-way classification holds. It does NOT run `daemonShutdownFunc`, so **no `daemon: shutdown` line fires on the self-eject path.**" (spec § Defensive invariants → § sanctioned exception)
>
> "Hysteresis-internal failures … DEBUG per spurious tick. ONE INFO or WARN on the trip (when the threshold is crossed and the eject decision lands)." (spec § Log-level discipline → Placement clarifications)
>
> Current eject site: `defaultDaemonTickLoop`'s `if consecutiveAbsenceTicks >= selfSupervisionHysteresisTicks { deps.Logger.Info(...); osExit(0); return nil }`. `osExit` is the package-level seam over `os.Exit` (`var osExit = os.Exit`); `selfSupervisionHysteresisTicks = 3`. The eject deliberately bypasses `daemonShutdownFunc`. (`cmd/state_daemon.go`)

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Saver and daemon lifecycle event taxonomy (Mechanical rule daemon table), § Defensive invariants against log destruction (sanctioned exception, `process: exit` shape), § Log-level discipline (Placement clarifications)

---

## portal-observability-layer-5-11 | approved

### Task 5-11: Home the `signal` component — re-attribute `EagerSignalHydrate`'s WARN and instrument the `internal/state` FIFO signal plumbing under `signal`

**Problem**: The closed subsystem taxonomy defines `signal` as the component owning "the FIFO signaling **mechanism** — `EagerSignalHydrate` and the lower-level FIFO signal send/receive plumbing in `internal/state`." Phase 1 deliberately did NOT pre-introduce `signal` (it kept `EagerSignalHydrate`'s existing write-failure WARN under `hydrate`/`ComponentHydrate`, promising the component would be homed "where Phase 5/6 promote them"). The sibling deferred components `capture` and `saver` are promoted in tasks 5-1 and 5-7/5-8, but `signal` is never homed — so `EagerSignalHydrate`'s WARN stays mis-attributed under `hydrate` and `grep "signal:" portal.log` produces nothing, leaving one of the 15 closed components un-wired.

**Solution**: Home the `signal` component. (1) Re-attribute `EagerSignalHydrate`'s per-FIFO write-failure WARN from `hydrate` to `signal` by binding `var logger = log.For("signal")` in `cmd/bootstrap/eager_signal_hydrate.go` (replacing the Phase-1-migrated `hydrate` binding for these lines). (2) Apply the § Call-site logging pattern mechanical rule to the lower-level `internal/state` FIFO signal send/receive plumbing (`signal_hydrate.go` — `WriteFIFOSignal` / `SendHydrateSignal`): a DEBUG breadcrumb on the retry-ladder transitions and a WARN on the recoverable write-failure path, all under `signal`. The hydrate helper's own exit-path lines (incl. `signal timeout`) stay under `hydrate` per the Hook-firing catalog (Phase 6) — this task touches only the signaling *mechanism*, not the helper's exec-chain.

**Outcome**: `EagerSignalHydrate`'s write-failure WARN renders under `signal:` (not `hydrate:`); the `internal/state` FIFO signal send/receive plumbing emits its breadcrumbs/WARN under `signal`; `grep "signal:" portal.log` reconstructs the FIFO-signaling mechanism's behaviour; the 15-component closed taxonomy is fully wired.

**Do**:
- In `cmd/bootstrap/eager_signal_hydrate.go`, add `import "github.com/leeovery/portal/internal/log"` and bind a package-level `var logger = log.For("signal")` (component literal `signal` per the closed taxonomy). Re-point the existing per-FIFO write-failure WARN (currently `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)`, Phase-1-migrated to a `hydrate` slog WARN) to the `signal`-bound logger: `logger.Warn("eager-signal write fifo failed", "path", fifoPath, "error", err, "error_class", "unexpected")`. Use only closed-vocabulary attrs (`path`, `error`, `error_class`). Per the level-discipline table, a write-failure that leaves a pane's helper un-signalled drops a unit of work → WARN with `error_class="unexpected"`. Pass the wrapped `err` directly (not `.Error()`) per the Phase-4 convention.
- Per the § Call-site logging pattern mechanical rule (DEBUG breadcrumb at each meaningful transition; WARN per recoverable error path), add a DEBUG breadcrumb in `EagerSignalHydrate`'s per-FIFO loop on the successful-signal path (e.g. `logger.Debug("fifo signalled", "path", fifoPath)`) so the per-pane signalling is reconstructible at DEBUG; the WARN above is the anomaly line. Do NOT add a cycle-summary INFO — `EagerSignalHydrate` is bootstrap step 7 and its `bootstrap: step complete` summary is owned by task 5-2; the § Cycle-level summary "Concrete cycle catalog" lists no separate signal-sweep summary, so none is mandated here.
- Instrument the lower-level FIFO signal send/receive plumbing in `internal/state/signal_hydrate.go`. `WriteFIFOSignal` / `SendHydrateSignal` currently take NO logger (they return errors the caller logs). `[needs-info]`: homing these under `signal` requires either (a) binding a package-level `var logger = log.For("signal")` in `internal/state` and emitting the breadcrumb/WARN inside `WriteFIFOSignal`/`SendHydrateSignal` directly (matching the model-observer seam used for the stores), or (b) leaving them error-returning and relying on the caller (`EagerSignalHydrate`) to emit under `signal` — in which case the lower-level plumbing carries only a DEBUG retry-ladder breadcrumb if any. Resolve and document the chosen seam; prefer (a) for the retry-ladder DEBUG breadcrumb (the retryable-FIFO-error retry decision in `isRetryableFIFOError`/`SendHydrateSignal` is a meaningful transition worth a DEBUG line under `signal`), keeping the whole-operation WARN at the `EagerSignalHydrate` call site so it carries the `path`. Confirm the import-cycle guard holds (`internal/state` may import `internal/log`; `internal/log` must not import `internal/state` — Task 1-8's invariant).
- Confirm no behaviour change beyond component re-attribution + additive breadcrumbs: the retry ladder, the FIFO open/write semantics, the orchestrator's Warn-and-swallow of `EagerSignalHydrate`'s return, and the bootstrap step-7 control flow are all unchanged.
- Update any test that asserts the `EagerSignalHydrate` write-failure WARN renders under `hydrate` to expect `signal` instead.

**Acceptance Criteria**:
- [ ] `EagerSignalHydrate`'s per-FIFO write-failure WARN renders under component `signal` (prefix `signal:`), NOT `hydrate`, with `path`/`error`/`error_class=unexpected` attrs and the wrapped error passed directly.
- [ ] A per-FIFO successful signal emits a DEBUG breadcrumb under `signal` (silent at production INFO, present at DEBUG).
- [ ] The lower-level `internal/state` FIFO signal send/receive plumbing logs its retry-ladder breadcrumb / write-failure under `signal` per the resolved seam (the `[needs-info]` choice), with no `internal/log → internal/state` import cycle.
- [ ] No `bootstrap`/`hydrate`-prefixed line is emitted for the signaling mechanism that the spec assigns to `signal`; `grep "signal:" portal.log` reconstructs the FIFO-signaling behaviour.
- [ ] No new attr keys are introduced (only closed-vocabulary attrs); no cycle-summary INFO is added (none is mandated by the Concrete cycle catalog).
- [ ] The retry ladder, FIFO open/write semantics, and bootstrap step-7 control flow are behaviourally unchanged; tests asserting the old `hydrate` attribution are updated to `signal`.

**Tests**:
- `"it emits the eager-signal write-failure WARN under component=signal with path and error_class=unexpected"`
- `"it emits a per-FIFO signalled DEBUG breadcrumb under signal (filtered at INFO, present at DEBUG)"`
- `"the internal/state FIFO signal plumbing logs its retry/write breadcrumb under signal"` (per the resolved seam)
- `"no signaling-mechanism line renders under hydrate or bootstrap after the re-attribution"`
- `"it adds no new attr keys and no cycle-summary line"`

**Edge Cases**:
- EagerSignalHydrate write-failure WARN moves from `hydrate` to `signal` (component re-attribution, not a new line).
- The hydrate helper's own exit-path lines (incl. `signal timeout`, Phase 6) stay under `hydrate` — this task touches only the signaling mechanism, never the helper exec-chain.
- Lower-level `internal/state` signal plumbing currently takes no logger — seam/signature change needed (`[needs-info]`: emit inside the plumbing vs at the EagerSignalHydrate caller); resolve and document, preserve the import-cycle guard.
- No cycle-summary mandated (the signal sweep is not in the Concrete cycle catalog); only the call-site/level-discipline pattern applies.
- `error_class="unexpected"` per the level table (an un-signalled pane drops a unit of work).

**Context**:
> "`signal` | FIFO signaling **mechanism** — `EagerSignalHydrate` and the lower-level FIFO signal send/receive plumbing in `internal/state`. (The hydrate helper's own exit-path outcome lines — incl. `signal timeout` — render under `hydrate` per the Hook-firing catalog, which governs the helper's exec-chain.)" (spec § Subsystem prefix taxonomy → component-ownership table)
>
> "This list is the **single source of truth for the component count.**" — the closed 15-component space; every component is consulted by contributors and presumed wired. (spec § Subsystem prefix taxonomy → Closed component value space)
>
> "Grep idiom preserved: `grep "hydrate:" portal.log` produces the per-subsystem audit trail." — the same per-prefix reconstruction is the point of every closed component, including `signal`. (spec § Subsystem prefix taxonomy → Rendering mechanism)
>
> Mechanical rule (per function authored or amended): DEBUG breadcrumbs at each meaningful state transition; WARN per recoverable error path with `error_class`. (spec § Call-site logging pattern → Mechanical rule)
>
> Current code: `EagerSignalHydrate` (`cmd/bootstrap/eager_signal_hydrate.go:96`) emits `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)` — under `hydrate`, not `signal`. The lower-level plumbing (`internal/state/signal_hydrate.go`: `WriteFIFOSignal`, `SendHydrateSignal`, `OpenFIFOForSignal`, `DefaultFIFOSignaler`, `isRetryableFIFOError`) takes no logger and returns errors to the caller. Phase 1 task 1-9 explicitly deferred `signal` to Phase 5/6 promotion.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Subsystem prefix taxonomy (Closed component value space — `signal` ownership, single-source-of-truth, Rendering mechanism), § Call-site logging pattern (Mechanical rule); planning Phase 1 task 1-9 `signal` deferral
