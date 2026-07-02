---
status: complete
created: 2026-07-02
cycle: 2
phase: Traceability Review
topic: Skip Bootstrap When Warm
---

# Review Tracking: Skip Bootstrap When Warm - Traceability

## Summary

Cycle 2 re-ran both directions against the specification in full (spec re-read from
scratch, all three phase task files re-read, all grounding anchors re-verified against
the live codebase). The single cycle-1 finding — an unanchored DEBUG log line in Task
2-1's `Do` section — was verified as **applied**: Task 2-1's `Do` third bullet now ends
at the transient-error-as-absent fold with no logging instruction, matching the cycle-1
Proposed text.

**No new findings.** The plan remains a faithful, complete translation of the
specification.

**Direction 1 (Spec → Plan, completeness):** Every specification element has plan
coverage with implementer-level depth.

- **Version-Stamped Latch** (storage, four-outcome "satisfied" semantics, parse-free
  value format, injectable running version, dev-build nuance) → Task 1-1 (helper +
  `BootstrappedMarkerName` const + three-way verdict), Task 2-3 (version compare at the
  entry path). The reuse anchors the plan claims are all present in the codebase
  (`RestoringChecker.TryGetServerOption`, `SetServerOption`/`TryGetServerOption`/
  `UnsetServerOption`, `RestoringMarkerName` precedent).
- **Latch Set-Point & Timing** (end-of-`Run` write, atomic-with-success across both
  invocation modes, soft-vs-fatal gating, best-effort/never-fatal write, insertion after
  the fatal-error gate + before the orchestration-complete summary, concurrent-path
  ordering before the terminal `Done`) → Task 1-2. The `bootstrap_progress.go`
  no-change claim is accurate: `runner.Run` returns before the `Done:true` event is sent.
- **CleanStale removal (11 → 10)** — step + `StaleCleaner` seam + adapter + `NoOpStaleCleaner`
  + `WithClean`/defaults plumbing + `totalSteps`/`emitStep(11,…)` + all "eleven" doc
  comments → Task 1-3. `loading_progress.go`'s two independent constants
  (`stepLabelTable` key 11, `totalBootstrapSteps` denominator) → Task 1-4. Both target
  the real live 11-step code confirmed in-tree.
- **Latch-Check Placement & Abridged-Path Wiring** (single read computed once, abridged
  gate upstream of `shouldRunConcurrentBootstrap`, context injection with
  `serverStarted=false` and NO `deferredBootstrapKey`, warnings sink reuse, loading-screen
  trigger re-keyed to latch-not-satisfied, `serverStarted` force-true correctness +
  comment reword, full outcome matrix) → Tasks 2-2, 2-3, with integration in 2-4.
- **Abridged EnsureSaver — Liveness-Only** (new package-`cmd` helper, `SaverPanePIDOrAbsent`
  → `BootstrapPortalSaver` composition, transient-error-as-absent, `SaverDownWarning`
  funnel, never calls `EnsurePortalSaverVersion`, kill-barrier-race dissolution,
  restore-window note) → Task 2-1.
- **Daemon-Owned Hooks Cleanup** (store + `lastCleanup` on `daemonDeps` via `loadHookStore()`
  resolving the same `hooks.json`, `lastCleanup` init to daemon-start, throttled
  `time.Since >= interval` gate, pinned `runHookStaleCleanup` args
  lister/store/`swallowListError=true`/`onRemoved=nil`, mass-delete guard +
  `EmitCleanStaleSummary` reuse with no new audit event, idle-branch tick placement after
  the `@portal-restoring` check, WARN-and-swallow failure posture, daemon-as-sole-caller
  guard) → Tasks 3-1, 3-2, 3-3, with real-tmux integration in 3-4.
- **Test Strategy** (branch selection, set-point gating, abridged self-heal regression
  guard, daemon cleanup cadence, real-tmux integration under `IsolateStateForTest`,
  design-for-test injectable version) → distributed across 1-1/1-2/1-3, 2-1/2-3/2-4,
  3-2/3-3/3-4. All three "branch selection" sub-assertions (satisfied → EnsureSaver-only;
  not-satisfied → 10-step Run + stamped latch; daemon is sole CleanStale caller) map to
  concrete task tests.
- **Edge Cases & Latch Invalidation** — every invalidation/failure mode is represented as
  a task edge case (auto-invalidation, upgrade invalidation, two-markers-can't-both-set,
  latch-write failure, manual escape hatch, abridged EnsureSaver hard-failure). The
  **accepted residues** (cold-boot cleanup leftovers, daemon-death vs cleanup home,
  flapping daemon starves cleanup) correctly generate **no tasks** — they are tolerated
  per spec and the plan invents no handling for them.
- **Out-of-scope directives honored** — no task adds a `portal`-level unset command, a
  `portal clean` unset flag/subcommand, or any help-text unset surface; production code
  never unsets the latch. The plan adds no invented scope.

**Direction 2 (Plan → Spec, fidelity):** Every task's Problem, Solution, implementation
detail, acceptance criteria, tests, and edge cases trace to a specific spec section (each
task carries an explicit Spec Reference plus inline `Spec §` quotes). The name/signature/
interval decisions the tasks pin beyond the spec's letter (`BootstrappedLatchSatisfied`,
`ensureSaverLiveness`, `HookStore` field, `maybeRunHookCleanup`,
`hookCleanupInterval = 10s`, `lastCleanup` reset-after-body) are each explicitly flagged
as ambiguity resolutions and grounded in the spec's semantics — legitimate planning-level
specification, not invention. The `10s` interval is spec-anchored ("10s default"). No
untraceable plan content remains after the cycle-1 removal.

## Findings

None. The plan is a faithful, complete translation of the specification.

---
