---
status: complete
created: 2026-07-19
cycle: 2
phase: Plan Integrity Review
topic: CLI Verb Surface Redesign
---

# Review Tracking: CLI Verb Surface Redesign - Integrity

## Outcome

**No findings.** Cycle-2 fresh full pass confirms the plan meets structural
quality standards for implementation readiness. This was a complete re-evaluation
of the whole plan (planning file + all 33 tasks across 6 phases), NOT a delta check
of cycle-1's fixes.

## Scope of Review

Standalone integrity pass over the planning file and every task in
`phase-{1..6}-tasks.md` (5 + 7 + 8 + 8 + 3 + 5 = 36 task entries; the substantive
tasks including the cycle-1-added daemon stale-project prune, 4-8), evaluated against
every dimension in `review-integrity.md`: task template compliance, vertical slicing,
phase structure, dependencies/ordering, task self-containment, scope/granularity, and
acceptance-criteria quality. Spec traceability was reviewed separately and is out of
scope here.

Per the orchestrator's context, the following were treated as intentional and NOT
re-flagged: the deliberately-empty dependency graph (natural authoring order encodes
the sequence), concrete file/function grounding, inline planner decisions where the
spec was silent, the two explicit user-decided burst behaviours (unsupported-terminal
atomic block; permission-wall trigger still self-connects), the retained out-of-scope
`internal/spawn` / `spawn`-component / `@portal-spawn-*` surface, the atomic-green-commit
bundling in Phases 4/5, and Task 4-8's placement (last in Phase 4, reusing
`project.Store.CleanStale`, no ordering dependency beyond that reuse). The large,
cohesive Task 3-3 was re-evaluated and again accepted as a single vertical slice.

## What Was Checked (and held)

**Template compliance.** Every task carries Problem, Solution, Outcome, Do,
Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference. Problem
statements explain why each task exists; Solutions describe what is built; Outcomes
define verifiable end states; ACs are concrete pass/fail; Tests include edge cases,
not just happy paths.

**Ordering (traced fresh, no hazards).** Verified no task depends on a symbol
authored by a later task, intra-phase or cross-phase:
- Phase 2: `openResolved` created in 2-1, consumed by 2-2/2-3/2-4/2-6.
- Phase 3: `orderedOpenTargets` (3-2) → 3-4; `Surface`/`resolveOpenSurfaces` (3-3) →
  3-4/3-5/3-6; `composeOpenArgv`/`Burster.Run([]Surface)` (3-5) → 3-6;
  `runOpenBurst`/`OpenBurstDeps` (3-6) → 3-7/3-8. Signature evolutions (3-1 threads
  ack into 2-1's `openResolved`; 3-7 adds `command` to 3-5's `composeOpenArgv`) are
  explicitly anticipated by the earlier task ("leave a clear seam for it").
- Phase 4: `DoctorDeps`/`checkResult` framework (4-1) progressively extended by
  4-2/4-3/4-4/4-5. `AllPaneLister` referenced by 4-3 still lives in `clean.go` until
  4-7 relocates it (same package → references compile before and after). `doctor --fix`
  (4-5) reuses `runHookStaleCleanup` / `project.Store.CleanStale` /
  `log.SweepLogsForClean` directly — NOT the `clean.go` wrappers deleted in 4-7 — so
  the 4-5-before-4-7 order is safe. `ErrDoctorUnhealthy` added to `IsSilentExitError`
  in 4-1, `ErrStatusUnhealthy` removed in 4-7 (correct sequence).
- Phase 5: 5-1-before-5-2 required and satisfied by natural order (5-1 leaves the cmd
  `spawnLogger` var declared-but-unused — Go tolerates unused package-level vars, so
  the build stays green; 5-2 deletes it once attach.go's consumption is gone, and its
  AC restores golangci-lint cleanliness). `TerminalDetector`/`buildResolver` consumed
  by 4-4 still live in `cmd/spawn.go` until 5-2's single relocation (same package, no
  call-site edits) — Phase 4 correctly precedes Phase 5 so `doctor`'s host-terminal
  line is the ready replacement when `spawn --detect` is deleted.
- Phase 6: rename/hide/completion changes correctly depend on the finalised flag
  surface (`--session`, `--ack`, `--alias`, `alias.Store.Keys()`) from Phases 1-3.

**Internal consistency (re-traced the subtle edges).**
- The `-s` glob single-target-first-match (2-1) vs. the Phase-3 engine K-expansion
  (3-4 routing on `HasGlobMeta`) is a documented forward-evolution: 3-3 refactors
  `ResolveSessionPin`/`ResolveAliasPin` to delegate to the `…All` variants, so the
  match rule is single-sourced, not contradicted.
- The routing gate `len(targets) >= 2 OR (len == 1 && HasGlobMeta)` sends a
  glob-metacharacter `-p` value (`/tmp/foo[1]`) into the engine while a plain
  `-p /gone` falls through to the Phase-2 pin block; both emit the identical
  `Directory not found` shape, so no user-visible divergence, and the single-target
  `-p` clause in `missAbortError` is reachable exactly for the glob-value case.
- The unsupported-terminal atomic block (3-6, no self-connect) and the
  permission-wall-still-self-connects rule (3-8) are distinct cases, each with an
  explicit reconciling decision note — not a contradiction.
- The `--ack` write (3-1) sits in `openResolved` after the 2-6 mint-scoped command
  guard; the burst trigger's local mint (3-7) bypasses `openResolved` so it correctly
  writes no marker, while spawned single-target windows route through `openResolved`
  and write theirs — consistent.
- The daemon-alive check (4-1, state-based) gated by 4-2's `ServerRunning` front-gate
  (down-server → distinct runtime-not-running message; up-server → 4-1's state check)
  is coherently and explicitly described; no double-report.
- Resolve-log emission is single per bare guessing-chain target across both the
  single-target Phase-1 path (`open blog`) and the multi-target engine (`open blog
  api`), with globs/pins emitting none — no double emission at the burst boundary.

**Acceptance-criteria quality.** ACs are pass/fail and cover behaviour (exact
message strings asserted byte-for-byte where load-bearing: `No session found:`,
`nothing resolved for '<t>' — try -f <t>`, `No alias found:`, `No zoxide match for:`,
the `Portal runtime not running — run \`portal open\` to start` message, the two
uninstall completion lines, the `Pick a project to run` banner), edge/boundary cases
(empty session set, malformed glob, zero-match glob, down-server not-evaluable hooks,
permission-denied stat retained, per-window ack timeout timed from each window's own
spawn), and negative cases (never mints, never pops the picker, never drives the exit
code). Verification-only tasks (5-3, 6-5) still deliver substantive regression guards.

**Scope/granularity.** Task 3-3 (Surface type + K-returning resolver variants +
single-method refactors + the cmd engine) remains the largest single task but is one
cohesive, independently-verifiable vertical slice — the read-only resolve/classify
engine — with a coherent test set; splitting the resolver variants out would yield a
task whose only consumer is the next task. The Phase 4/5 deletion + dependent-test
migration + helper-relocation bundles are the required atomic-green-commit shape. No
task is mechanical boilerplate or a single-line change lacking a meaningful test.

**Self-containment.** Cross-references ("Phase-1 helper", "Task 2-6 guard", "reuse the
Task-3-5 builder") point only to already-built artifacts an in-order implementer will
have; each task restates enough Problem/Solution/Do/Context to be executed without
reading the referenced task's body. Acceptable self-containment for a sequential plan.

## Findings

None.

---
