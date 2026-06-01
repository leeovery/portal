---
status: complete
created: 2026-06-01
cycle: 3
phase: Plan Integrity Review
topic: portal-observability-layer
---

# Review Tracking: portal-observability-layer - Integrity

Follow-up (cycle 3) standalone-document integrity review of the plan (6 phases / 52 tasks across `planning.md` + `phase-1..6-tasks.md`, mirrored in tick topic `tick-7ac1a9`). This cycle (a) verifies the cycle-2 integrity fix — the task 5-11 `signalLogger` naming-precision change — is faithfully applied in both `phase-5-tasks.md` and the tick task `tick-137c05`; (b) re-confirms the cycle-1 5-7/5-8 `saverLogger` ownership fix remains present; and (c) re-checks the whole plan end-to-end for any remaining structural issues or cascades introduced by those changes.

## Cycle-2 fix verification — VERIFIED FIXED

**Task 5-11 `signalLogger` naming-precision fix — verified FIXED in both surfaces.**

- `phase-5-tasks.md` Task 5-11 now uses `var signalLogger = log.For("signal")` (NOT a bare `logger`) at BOTH instrumentation sites, with the shadow-hazard rationale spelled out:
  - **Solution (line 593)**: "binding a package-level `var signalLogger = log.For("signal")` in `cmd/bootstrap/eager_signal_hydrate.go` and pointing the WARN call at it (the function-local `logger := c.Logger` already in `EagerSignalHydrate` would otherwise shadow a bare `logger` package var)."
  - **Do bullet 1 (line 598)**: binds `var signalLogger`, explains the function-local `logger := c.Logger` at line 76 shadows a bare package `logger`, and makes the receiver-replacement explicit ("the receiver MUST be `signalLogger`, not the function-local `logger`, so the re-attribution actually takes effect"), with the Phase-5 convention cross-reference (`captureLogger`/`cleanLogger`/`saverLogger`/`daemonLogger`).
  - **Do bullet 2 (line 599)**: the success-path DEBUG breadcrumb uses `signalLogger.Debug("fifo signalled", "path", fifoPath)` — "same `signalLogger` package var, NOT the function-local `logger`."
  - **Do bullet 3 (line 600)**: the `internal/state` `[needs-info]` option (a) binds `var signalLogger = log.For("signal")` (NOT bare `logger`) "because `logger` is the pervasive function-parameter name in `internal/state` and would shadow a package-level `logger`."
- The tick task `tick-137c05` description mirrors all of the above and additionally carries an explicit edge-case note: "The `signalLogger` package var must NOT be a bare `logger` at either site (`cmd/bootstrap` and `internal/state`) — a function-local `logger := c.Logger` / `logger` parameter would shadow it and silently no-op the re-attribution."

The function-local-shadow hazard at both the `cmd/bootstrap` and `internal/state` sites is fully resolved; the var name now matches the uniform Phase-5 `<component>Logger` convention; the instruction now actually achieves 5-11's stated Outcome (the WARN re-attribution takes effect).

## Cycle-1 fix re-verification — STILL PRESENT

**5-7/5-8 `saverLogger` ownership fix (cycle 1) — re-verified PRESENT.**

- `phase-5-tasks.md` Task 5-7 (line 364) remains the sole unconditional owner: "Add a package-level `var saverLogger = log.For("saver")` in `internal/tmux/portal_saver.go`."
- Task 5-8 Do bullet 1 (line 422) references it without re-declaring: "Use the package-level `var saverLogger = log.For("saver")` introduced by task 5-7 … do NOT re-declare it here (a second `var saverLogger` in the same file is a duplicate-declaration compile error)."
- The tick mirror is consistent: `tick-cb4c1b` (5-7) declares the var; `tick-af09b2` (5-8) references it ("do NOT re-declare"), carries the added edge-case note "saverLogger is owned/declared by task 5-7; 5-8 only references it," and encodes the dependency edge structurally (`tick-af09b2` `blocked_by tick-cb4c1b`).

## Summary

**No findings. The plan is implementation-ready and of high structural quality, and both prior integrity fixes are faithfully and durably applied across `phase-5-tasks.md` and the tick mirror.**

Full re-read of all six phases / 52 tasks confirms:

- **Task-template compliance**: every task carries the full canonical template (Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference). Problem statements explain WHY; Solution describes WHAT; Outcome defines the verifiable end state; acceptance criteria are concrete pass/fail; Tests enumerate edge cases, not just happy paths.
- **Vertical slicing**: each task is a single coherent, independently-verifiable TDD cycle (e.g. one resolver, one handler step, one store's mutation seam, one boundary class, one lifecycle catalog, one component homing, one hydrate exit path).
- **Phase structure**: logical foundation → rotation/lifecycle/defensive → config audit trail → boundary preservation → cycle/lifecycle catalogs → hydrate trail progression; each phase has clear acceptance criteria and is independently testable.
- **Dependencies/ordering**: no circular dependencies; cross-phase predecessor relationships (Phase-2 markers filling Phase-1 seams; 5-8 above the Phase-4 4-4 DEBUG breadcrumb; 5-10 not emitting the 5-9 shutdown line; Phase-6 exit-path INFOs preceding the 6-1 exec INFO) are each self-documented in the dependent task body, so natural creation-date ordering produces correct execution order; the one genuine intra-phase convergence point (5-8 needing 5-7's shared var) carries an explicit tick `blocked_by` edge.
- **Self-containment & acceptance-criteria quality**: each task pulls the relevant spec decisions into Context and Spec Reference; an implementer can execute any single task without reading siblings; criteria are pass/fail, not subjective.

The deliberate `[needs-info]` flags (e.g. 5-1 `natural_churn` classifier, 5-8 `placeholder died reason`, 5-9 shutdown `reason` capture, 5-11 `internal/state` plumbing seam, 3-1 `write-failed-fsync`, 3-3/3-4 single-batched-Save per-entry WARN, 3-5 emission point / `error_class` mapping, 3-6 per-entry key / migrate WARN class, 6-3 `fifo missing` row) are intentional implementer-decision markers grounded in the live code — NOT integrity defects — and each is correctly framed (resolve-and-document, do-not-invent). The cycle-1 Minor (Do-section length) was acknowledged won't-fix and is NOT re-raised.

### Cascade check (no finding)

One cross-task interaction was examined and found NOT to rise to a finding: tasks 4-4 (Phase 4, SIGKILL-escalation DEBUG breadcrumb) and 5-7 (Phase 5, `var saverLogger` declaration) both touch `internal/tmux/portal_saver.go` and both want a `saver`-component logger. Unlike the cycle-1 5-8/5-7 hazard — which contained a literal "or add it if 5-7 is not yet applied" duplicate-declaration *instruction* — task 4-4 contains no instruction to *declare* a new var; its worked example uses the bare `logger` receiver and it says only to "bind/use a `log.For("saver")` logger." Phase 1 task 1-8 already establishes `var logger = log.For("saver")` in this file, so 4-4's "use a saver-bound logger" naturally resolves to reusing that existing Phase-1 `logger` var (which lands before Phase 5). There is no forced duplicate-declaration and no design decision pushed onto the implementer: the natural reading produces correct, compiling code, and any redeclaration attempt yields an immediate, trivially-resolved compile error rather than a silent mis-attribution. This is below the Important threshold and, consistent with the cycle-1 proportionality stance, is not raised.

No circular dependencies, orphaned/duplicate tasks, or incoherent phase boundaries were found. No new cascading issues surfaced from the cycle-2 changes.

## Findings

None.
