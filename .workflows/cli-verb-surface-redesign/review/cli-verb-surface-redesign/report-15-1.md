TASK: cli-verb-surface-redesign-15-1 — Extract a store-owned staleness predicate so doctor's diagnosis and the prune cannot drift

ACCEPTANCE CRITERIA:
1. Hooks ∉ staleness classification exists in exactly one place in internal/hooks; both hooks.Store.CleanStale and cmd/doctor.go's checkStaleHooks derive from it; countStaleHookKeys is deleted.
2. Project os.Stat tri-state classification exists in exactly one place in internal/project; both project.Store.CleanStale and checkStaleProjects derive from it; the duplicated switch in doctor is deleted.
3. doctor's stale-hook and stale-project checks remain strictly read-only (no Save/prune/side-effects) and return identical checkResult status/detail strings.
4. Mass-deletion hazard guard behaviourally unchanged; NOT moved into the shared predicate.
5. Project staleness still treats permission-denied / non-ErrNotExist os.Stat errors as retained (NOT stale) on both paths.
6. go build ./..., go test ./... (unit lane), golangci-lint run all pass.

STATUS: Complete

SPEC CONTEXT:
Spec §"portal doctor --fix" (specification.md:309-312) mandates the diagnose→repair pairing for stale hooks/projects, and the down-server data-loss safety: with the server down the live-pane enumeration is empty so every hook would falsely look orphaned — doctor must report dead-pane-hook staleness as not-evaluable (never "all stale") and --fix must perform NO hook pruning in that state (user-authored on-resume commands are not reconstructable). The stale-project prune is filesystem-only and may still run. This task is the internal DRY refactor that makes the read-only diagnosis and the prune share one predicate so they cannot drift, while preserving that guard exactly.

IMPLEMENTATION:
- Status: Implemented (clean, faithful to plan)
- Hooks predicate: internal/hooks/store.go:244 `StaleKeys(persisted, live)` — pure ∉ classifier over the already-loaded map; documented as single owner and explicitly NOT encoding the hazard guard. CleanStale derives removals from it (store.go:288). Doctor derives via hooks.StaleKeys(persisted, live) (cmd/doctor.go:453).
- countStaleHookKeys: DELETED — zero references anywhere in the tree (verified by grep across all *.go).
- Projects predicate: internal/project/store.go:192 `partitionByExistence(projects)` — single home of the os.Stat tri-state (nil→kept, ErrNotExist→removed, default→kept). StaleEntries (store.go:178) exposes the read-only removed set for doctor; CleanStale partitions through the identical classifier (store.go:243). The duplicated switch in doctor is gone — checkStaleProjects (cmd/doctor.go:473) now just calls store.StaleEntries().
- Read-only: checkStaleHooks only Load()s + enumerates + counts; checkStaleProjects only calls StaleEntries() (Loads+classifies, never Saves). Neither prunes.
- Hazard guard: UNCHANGED and correctly left OUT of the predicate. It stays a cmd-layer policy in checkStaleHooks (cmd/doctor.go:444-452 — empty/errored live + hooks present → checkNotEvaluable; empty live + no hooks → checkPass "no hooks") and in runHookStaleCleanup (cmd/run_hook_stale_cleanup.go:119-126 — deferral, no delete). StaleKeys' doc explicitly states an empty live set makes every key stale there and that the guard is a separate cmd-layer concern.
- Permission-denied retained: partitionByExistence default branch keeps the entry on both diagnosis (StaleEntries) and prune (CleanStale) paths.
- Prune paths unchanged: pruneDoctorStaleHooks delegates to runHookStaleCleanup verbatim (cmd/doctor.go:297-303); project prune still calls deps.ProjectStore.CleanStale() (cmd/doctor.go:315).

TESTS:
- Status: Adequate — precisely scoped, no over/under-testing.
- internal/hooks/store_test.go:726 TestStaleKeys — general ∉, empty-result (all live), all-stale (empty live set), empty-persisted cases. store_test.go:782 TestCleanStaleRemovesExactlyStaleKeys — proves CleanStale removes exactly StaleKeys' set.
- internal/project/store_test.go:551 TestStaleEntries — present→live, missing(ErrNotExist)→stale, permission-denied(0000 parent)→retained. store_test.go:618 TestCleanStaleRemovesExactlyStaleEntries — CleanStale removes exactly StaleEntries' set.
- cmd/doctor_test.go:1040 TestDoctorStaleHooksParityWithPredicate — table test asserting reported count == len(hooks.StaleKeys(...)) for past-guard inputs; hazard-guard cases (enumeration error / empty-live-with-hooks / empty-live-no-hooks) map to checkNotEvaluable/checkPass AND assert byte-identical hooks.json (read-only proven).
- cmd/doctor_test.go:1132 TestDoctorStaleProjectsParityWithPredicate — count == StaleEntries(); load error → checkNotEvaluable.
- Down-server safety preserved: cmd/doctor_test.go:1337 TestDoctorFixProtectsUserHooksWhenLiveSetEmptyOrErrored (empty + errored live, hooks.json byte-for-byte unchanged) and :1375 TestDoctorFixDownServerPrunesProjectsButNotHooks (projects pruned, hooks deferred). Happy-path :1275 TestDoctorFixPrunesStaleEntriesThenRediagnosesClean intact.
- Tests assert behaviour (removed set == predicted set, byte-equal store, status/detail strings), not implementation internals. Not redundant.

CODE QUALITY:
- Project conventions: Followed. Store-owns-its-classification matches the codebase's "chokepoint at the store seam" pattern (same shape as the audit-breadcrumb design). Predicate names role-descriptive.
- SOLID: Good. Single responsibility sharpened — the store owns "what is stale", doctor owns "how to report", the cmd layer owns "when it's safe to act" (the hazard guard). DRY duplication removed without over-abstraction (guard deliberately left as a cmd policy, not folded in).
- Complexity: Low. StaleKeys is a set-difference; partitionByExistence is a three-arm switch; both are trivially readable.
- Modern idioms: Yes (maps.Clone for the kept map, slices helpers elsewhere).
- Readability: Good. Doc comments on StaleKeys / partitionByExistence / StaleEntries / checkStaleHooks / checkStaleProjects all explicitly call out the single-owner invariant and why the guard is separate — this is exactly the drift the task set out to prevent, documented at the source.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/project/store_test.go:566 and cmd/doctor_test.go:1167 — the permission-denied / load-error cases rely on chmod 0000, which does not produce a permission error when the suite runs as root; a `if os.Geteuid()==0 { t.Skip(...) }` guard would make them robust. Marginal: matches an established codebase convention (no CI, developer never runs tests as root), so this is consistency-preserving to leave as-is; flagged only for completeness.
