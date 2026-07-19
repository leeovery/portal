---
status: complete
created: 2026-07-19
cycle: 1
phase: Plan Integrity Review
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Integrity

## Outcome

**No findings.** The plan meets structural quality standards for implementation readiness.

## Scope of Review

Standalone integrity pass over the planning file and all 32 tasks across 6 phases
(`phase-{1..6}-tasks.md`), evaluated against every dimension in `review-integrity.md`:
task template compliance, vertical slicing, phase structure, dependencies/ordering,
task self-containment, scope/granularity, and acceptance-criteria quality. Spec
traceability was reviewed separately and is out of scope here.

Per the orchestrator's context, the following were treated as intentional and NOT
re-flagged: the deliberately-empty dependency graph (natural authoring order encodes
the sequence), concrete file/function grounding, inline planner decisions where the
spec was silent, the two explicit user-decided burst behaviours, the retained
out-of-scope `internal/spawn`/`spawn`-component/`@portal-spawn-*` surface, and the
atomic-green-commit bundling in Phases 4/5.

## What Was Checked (and held)

**Template compliance.** Every task carries Problem, Solution, Outcome, Do,
Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference. Problem
statements explain why each task exists; Solutions describe what is built; Outcomes
define verifiable end states; ACs are concrete pass/fail; Tests include edge cases,
not just happy paths.

**Ordering (traced, no hazards).** Verified no task depends on a symbol authored by a
later task, intra-phase or cross-phase:
- Phase 2: `openResolved` created in 2-1, consumed by 2-2/2-3/2-4/2-6.
- Phase 3: `orderedOpenTargets` (3-2) → 3-4; `Surface`/`resolveOpenSurfaces` (3-3) →
  3-4/3-5/3-6; `composeOpenArgv`/`Burster.Run([]Surface)` (3-5) → 3-6;
  `runOpenBurst` (3-6) → 3-7/3-8.
- Phase 4: `DoctorDeps`/`checkResult` framework (4-1) → 4-2/4-3/4-4/4-5. Confirmed
  `doctor --fix` (4-5) reuses the underlying `runHookStaleCleanup` /
  `project.Store.CleanStale` / `log.SweepLogsForClean` — NOT the clean.go wrappers
  deleted later in 4-7 — so the 4-5-before-4-7 order is safe. `ErrDoctorUnhealthy`
  added to `IsSilentExitError` in 4-1, `ErrStatusUnhealthy` removed in 4-7 (correct
  sequence).
- Phase 5/6: deletions and presentation changes correctly depend on the finalised
  flag surface from Phases 1-3 (`--session`, `--ack`, `--alias`, `alias.Store.Keys()`).

**Internal consistency (traced the subtle edges).**
- The `-s` glob single-target-first-match dispatch (2-1) vs. the Phase-3 engine
  K-expansion (3-4 routing on `HasGlobMeta`) is an explicitly-documented
  forward-evolution (3-3 refactors `ResolveSessionPin` to delegate to
  `ResolveSessionPinAll`, so the match rule is single-sourced, not contradicted).
- The Phase-3 routing gate (`len>=2 OR (len==1 && HasGlobMeta)`) correctly sends a
  glob-metacharacter `-p` value (e.g. `/tmp/foo[1]`) into the engine while a plain
  `-p /gone` falls through to the Phase-2 pin block; both emit the identical
  `Directory not found` shape, so no user-visible divergence. The single-target `-p`
  clause in `missAbortError` is reachable exactly for the glob-value case — correct,
  not dead.
- The unsupported-terminal atomic block (3-6, no self-connect) and the
  permission-wall-still-self-connects rule (3-8) are distinct cases, each with an
  explicit reconciling decision note — not a contradiction.
- The `--ack` write (3-1) sits in `openResolved` after the 2-6 mint-scoped command
  guard; attach windows carry no command (3-7), so the guard never blocks a spawned
  attach receiver — consistent.

**Acceptance-criteria quality.** ACs are pass/fail and cover behaviour (exact
message strings asserted byte-for-byte where load-bearing, e.g. `No session found:`,
`nothing resolved for '<t>' — try -f <t>`, the runtime-not-running message, the
uninstall completion lines, the `Pick a project to run` banner), edge/boundary cases
(empty session set, malformed glob, zero-match glob, down-server not-evaluable hooks,
permission-denied stat retained), and negative cases (never mints, never pops the
picker, never touches the exit code). Verification-only tasks (5-3, 6-5) still deliver
substantive regression guards.

**Grounding (spot-verified against the codebase).** `state.ReadIndex`'s documented
`(_,true,nil)` / `(_,true,err wrapping ErrCorruptIndex)` / `(idx,false,nil)` contract
matches Task 4-1's claims exactly; every referenced `internal/spawn` and
`internal/state` helper the tasks lean on (`QuoteJoin`, `UnsupportedNoopMessage`,
`PartitionResults`, `FirstPermission`, `LogBatchSummary`/`LogPermission`/
`LogWindowResults`, `NewDetector`, `CollectStatus`, `Dir`/`EnsureDir`, `DaemonAlive`)
and the `cmd/open.go` symbols (`buildSessionConnector`, `parseCommandArgs`,
`FallbackResult`, `initialFilter`) exist.

**Scope/granularity.** Task 3-3 is the largest single task (Surface type +
K-returning resolver variants + refactors + the cmd engine, touching three
boundaries), but it is one cohesive, independently-verifiable vertical slice (the
read-only resolution engine) with a coherent test set; splitting the resolver
variants out would produce a task whose only consumer is the next task (a
"too-small" step). The planners' merge is a reasonable judgment, so it is not raised
as a defect. All deletion/migration bundles in Phases 4/5 are the required
atomic-green-commit shape.

## Findings

None.

---
