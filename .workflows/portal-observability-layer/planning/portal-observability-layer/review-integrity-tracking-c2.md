---
status: in-progress
created: 2026-06-01
cycle: 2
phase: Plan Integrity Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Integrity

Follow-up (cycle 2) standalone-document integrity review of the plan (6 phases / 52 tasks across `planning.md` + `phase-1..6-tasks.md`, mirrored in tick topic `tick-7ac1a9`). This cycle (a) verifies the cycle-1 integrity fix — the 5-8/5-7 shared-`saverLogger` ownership clarification plus the new `signal`-homing task 5-11 — and (b) re-checks the whole plan for cascading structural issues introduced by those changes.

## Cycle-1 fix verification

**Finding 1 (5-8/5-7 `saverLogger` ownership) — verified FIXED.** Task 5-8's first Do bullet (`phase-5-tasks.md` line 422) now reads "Use the package-level `var saverLogger = log.For("saver")` introduced by task 5-7 … This task depends on 5-7 for that shared logger var — do NOT re-declare it here (a second `var saverLogger` in the same file is a duplicate-declaration compile error)." Task 5-7 (line 364) remains the sole unconditional owner of the var declaration. The explicit dependency edge is encoded in tick: `tick-af09b2` (5-8) `blocked_by tick-cb4c1b` (5-7), and 5-8's tick description carries the same clarified bullet plus an added edge-case note ("saverLogger is owned/declared by task 5-7; 5-8 only references it"). The duplicate-declaration hazard and the missing convergence edge are both resolved.

**New task 5-11 (`signal` component homing) — structurally sound, one defect (Finding 1 below).** 5-11 carries the full canonical template (Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference), is a single coherent vertical slice (home one closed component end-to-end), and correctly closes the last un-wired component in the 15-component closed taxonomy (`bootstrap`, `daemon`, `restore`, `hydrate`, `notify`, `hooks`, `preview`, `saver`, `capture`, `signal`, `log-rotate`, `clean`, `aliases`, `projects`, `process` — `signal` was the sole gap after cycle-1 confirmed `capture`→5-1 and `saver`→5-7/5-8). Its Problem statement accurately cites the Phase-1 deferral (verified at `phase-1-tasks.md` line 446: "`capture`/`saver`/`signal` … introduced where Phase 5/6 promote them … Do NOT pre-introduce Phase 5/6 components"). The `[needs-info]` on the `internal/state` plumbing seam is a legitimate intentional implementer-decision marker grounded in live code (the lower-level plumbing currently takes no logger), not a defect. The planning.md Phase-5 acceptance bullet (line 146) and task-table row (line 164) for 5-11 are present and consistent. No new dependency/ordering issues are introduced (the `signal` logger lives in files distinct from the other Phase-5 component loggers; no cross-task shared-var convergence is created).

The cycle-1 Minor finding (Do-section length) was acknowledged as won't-fix and is NOT re-raised.

## Summary

The plan remains implementation-ready and of high structural quality, and the cycle-1 fix is correctly applied. One Important finding remains: the newly-added task 5-11 instructs binding a package-level **`var logger`** (bare name) in `cmd/bootstrap/eager_signal_hydrate.go`, but the function it must re-instrument (`EagerSignalHydrate`) already declares a function-local `logger := c.Logger` that would shadow it — so following the instruction literally would leave the write-failure WARN routed through the injected (old-component) logger and silently fail to render under `signal`, defeating the task's stated outcome. The same bare-`logger` name is also hazardous in the `internal/state` option (a), where `logger` is the pervasive function-parameter name. The fix is a naming-precision change (`signalLogger`) consistent with the rest of Phase 5's component-suffixed convention (`captureLogger`/`cleanLogger`/`saverLogger`/`daemonLogger`).

No circular dependencies, orphaned/duplicate tasks, or incoherent phase boundaries were found. No other cascading issues surfaced from the cycle-1 changes.

## Findings

### 1. Task 5-11's package-level `var logger` collides with the function-local `logger := c.Logger` in `EagerSignalHydrate` (re-attribution silently no-ops) and breaks Phase-5's logger-naming convention

**Severity**: Important
**Plan Reference**: `phase-5-tasks.md` Task 5-11 (`portal-observability-layer-5-11`), Solution (line 593) and Do bullets 1 + 3 (lines 598, 600); mirrored in tick `tick-137c05`
**Category**: Task Self-Containment / Acceptance Criteria Quality (an instruction whose literal execution does not achieve the task's stated outcome; an implementer would have to notice and resolve an unstated naming collision)
**Change Type**: update-task

**Details**:
Task 5-11 Do bullet 1 instructs: "In `cmd/bootstrap/eager_signal_hydrate.go` … bind a package-level `var logger = log.For("signal")` … Re-point the existing per-FIFO write-failure WARN … to the `signal`-bound logger: `logger.Warn("eager-signal write fifo failed", …)`."

But in the live code the function this WARN lives in — `EagerSignalCore.EagerSignalHydrate()` (`cmd/bootstrap/eager_signal_hydrate.go:71`) — already declares a **function-local** `logger := c.Logger` (line 76), and the WARN site is `logger.Warn(...)` at line 96. After Phase 1 (task 1-8) retypes the `Logger` field to `*slog.Logger` and removes the `if logger == nil` substitution, that local `logger` (the injected per-core field) remains. A package-level `var logger = log.For("signal")` added to the same file would be **shadowed** by the function-local `logger` inside the exact function it must re-instrument: the WARN at the call site would resolve to the local (injected, old-`hydrate`-component) logger, NOT the new `signal`-bound package var. The package-level var would compile (package-level vars are exempt from "declared and not used") but be dead, and the task's Outcome ("the write-failure WARN renders under `signal:`") and its first Acceptance Criterion would silently FAIL even though the implementer followed the instruction verbatim.

The bare name `logger` is also the lone outlier in Phase 5: every other component logger introduced in this phase uses a component-suffixed package var (`captureLogger` in 5-1, `cleanLogger` in 5-5, `saverLogger` in 5-7, `daemonLogger` referenced in 5-9/5-10). The same hazard recurs in Do bullet 3's `internal/state` option (a), which suggests binding a package-level `var logger = log.For("signal")` in `internal/state` — a package where `logger` is the pervasive function-parameter name (e.g. `SweepOrphanFIFOs(dir, liveMarkerKeys, logger)` per task 5-6, and the many `logger *slog.Logger` params retyped in tasks 1-8/1-9), so a package-level `var logger` there is shadowed by every such parameter.

This is not the `[needs-info]` seam question (option a vs b for the `internal/state` plumbing) — that decision is correctly deferred to the implementer. This is a concrete, resolvable naming defect in the instruction itself: the prescribed var name does not work at the prescribed site. The fix is to rename the package-level binding to `signalLogger` (matching the `*Logger` convention used throughout Phase 5) at both sites, and to make explicit that the `cmd/bootstrap` re-attribution must point the WARN site at `signalLogger` rather than the function-local `logger := c.Logger` (i.e. replace the WARN call's receiver, not merely add an unused package var).

**Current** (`phase-5-tasks.md` Task 5-11):

Solution (line 593):
> **Solution**: Home the `signal` component. (1) Re-attribute `EagerSignalHydrate`'s per-FIFO write-failure WARN from `hydrate` to `signal` by binding `var logger = log.For("signal")` in `cmd/bootstrap/eager_signal_hydrate.go` (replacing the Phase-1-migrated `hydrate` binding for these lines). (2) Apply the § Call-site logging pattern mechanical rule to the lower-level `internal/state` FIFO signal send/receive plumbing (`signal_hydrate.go` — `WriteFIFOSignal` / `SendHydrateSignal`): a DEBUG breadcrumb on the retry-ladder transitions and a WARN on the recoverable write-failure path, all under `signal`. The hydrate helper's own exit-path lines (incl. `signal timeout`) stay under `hydrate` per the Hook-firing catalog (Phase 6) — this task touches only the signaling *mechanism*, not the helper's exec-chain.

Do bullet 1 (line 598):
> - In `cmd/bootstrap/eager_signal_hydrate.go`, add `import "github.com/leeovery/portal/internal/log"` and bind a package-level `var logger = log.For("signal")` (component literal `signal` per the closed taxonomy). Re-point the existing per-FIFO write-failure WARN (currently `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)`, Phase-1-migrated to a `hydrate` slog WARN) to the `signal`-bound logger: `logger.Warn("eager-signal write fifo failed", "path", fifoPath, "error", err, "error_class", "unexpected")`. Use only closed-vocabulary attrs (`path`, `error`, `error_class`). Per the level-discipline table, a write-failure that leaves a pane's helper un-signalled drops a unit of work → WARN with `error_class="unexpected"`. Pass the wrapped `err` directly (not `.Error()`) per the Phase-4 convention.

Do bullet 3 (line 600), final clause only:
> … `[needs-info]`: homing these under `signal` requires either (a) binding a package-level `var logger = log.For("signal")` in `internal/state` and emitting the breadcrumb/WARN inside `WriteFIFOSignal`/`SendHydrateSignal` directly (matching the model-observer seam used for the stores), or (b) …

**Proposed** (`phase-5-tasks.md` Task 5-11):

Solution (line 593):
> **Solution**: Home the `signal` component. (1) Re-attribute `EagerSignalHydrate`'s per-FIFO write-failure WARN from `hydrate` to `signal` by binding a package-level `var signalLogger = log.For("signal")` in `cmd/bootstrap/eager_signal_hydrate.go` and pointing the WARN call at it (the function-local `logger := c.Logger` already in `EagerSignalHydrate` would otherwise shadow a bare `logger` package var). (2) Apply the § Call-site logging pattern mechanical rule to the lower-level `internal/state` FIFO signal send/receive plumbing (`signal_hydrate.go` — `WriteFIFOSignal` / `SendHydrateSignal`): a DEBUG breadcrumb on the retry-ladder transitions and a WARN on the recoverable write-failure path, all under `signal`. The hydrate helper's own exit-path lines (incl. `signal timeout`) stay under `hydrate` per the Hook-firing catalog (Phase 6) — this task touches only the signaling *mechanism*, not the helper's exec-chain.

Do bullet 1 (line 598):
> - In `cmd/bootstrap/eager_signal_hydrate.go`, add `import "github.com/leeovery/portal/internal/log"` and bind a package-level `var signalLogger = log.For("signal")` (component literal `signal` per the closed taxonomy; use the `signalLogger` name — NOT a bare `logger` — because `EagerSignalHydrate` already declares a function-local `logger := c.Logger` at line 76 that would shadow a package-level `logger`, silently leaving the WARN on the injected `hydrate`-bound logger). Re-point the per-FIFO write-failure WARN at the `signal`-bound logger: replace the existing `logger.Warn(state.ComponentHydrate, "eager-signal: write fifo %s: %v", fifoPath, err)` call (Phase-1-migrated to a `hydrate` slog WARN) with `signalLogger.Warn("eager-signal write fifo failed", "path", fifoPath, "error", err, "error_class", "unexpected")` — the receiver MUST be `signalLogger`, not the function-local `logger`, so the re-attribution actually takes effect. Use only closed-vocabulary attrs (`path`, `error`, `error_class`). Per the level-discipline table, a write-failure that leaves a pane's helper un-signalled drops a unit of work → WARN with `error_class="unexpected"`. Pass the wrapped `err` directly (not `.Error()`) per the Phase-4 convention. (Naming matches the Phase-5 convention: `captureLogger` (5-1), `cleanLogger` (5-5), `saverLogger` (5-7), `daemonLogger` (5-9/5-10).)

Do bullet 3 (line 600), `[needs-info]` clause:
> … `[needs-info]`: homing these under `signal` requires either (a) binding a package-level `var signalLogger = log.For("signal")` in `internal/state` (use the `signalLogger` name — NOT a bare `logger` — because `logger` is the pervasive function-parameter name in `internal/state` and would shadow a package-level `logger`) and emitting the breadcrumb/WARN inside `WriteFIFOSignal`/`SendHydrateSignal` directly (matching the model-observer seam used for the stores), or (b) …

Also add the matching success-path DEBUG breadcrumb in Do bullet 2 to use `signalLogger` (currently line 599 writes `logger.Debug("fifo signalled", "path", fifoPath)`): change to `signalLogger.Debug("fifo signalled", "path", fifoPath)`. Update the corresponding tick task (`tick-137c05`) description identically.

**Resolution**: Pending
**Notes**: Low-risk, self-contained naming-precision fix; no scope or architecture change. It makes 5-11's instruction actually achieve its stated Outcome/Acceptance Criterion and aligns the var name with the uniform Phase-5 `<component>Logger` convention. The `[needs-info]` seam choice (a vs b) and all other content of 5-11 are sound and unchanged. Worth confirming the same `signalLogger` rename is applied to Do bullet 2's success-path DEBUG breadcrumb so all three `signal` emission sites in `cmd/bootstrap` use the one package var.

---
