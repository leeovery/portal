TASK: remote-trigger-spawns-on-local-terminal-2-1 — Clean up T1-1 gate-inversion residues in detect_inside.go (chore / pure refactor, no behaviour change)

ACCEPTANCE CRITERIA:
- The `ClientActivity` type doc contains no "local-only tiebreak" / "disambiguate among host-local clients" phrasing and describes `Activity` as the cross-client winner-selection signal, consistent with the function docstring.
- `detectInsideTmux` returns `walkToBundle(winner.PID, walker, reader)` directly after selecting the winner, with no hand-rolled re-branching of its three-shape result.
- No behaviour change: `Detect()`'s NULL+WARN folding and all existing detection outcomes (resolved→drive, clean-NULL→no-op, transient→NULL+ErrDetectTransient) are unchanged.
- `internal/tmux/clients.go` is unchanged (out of scope).

STATUS: complete

SPEC CONTEXT:
The spec's "Owned Behaviour Change: Dropped Walk-Resilience Property" section (specification.md:58) explicitly names the pre-fix contract phrases that must NOT remain after the T1-1 gate inversion — line 66 flags *"client_activity is used ONLY to disambiguate among host-local clients — never as a cross-client primary signal"* as "the exact inversion of the new rule (activity now selects the winner across all clients, local and remote alike)." T2-1 is the residue-cleanup task that removes the spawn-package echo of this falsified phrasing and collapses the redundant walk re-branch. Detection semantics (winner-select-then-locality-gate) were established by T1-1 and are unchanged here.

IMPLEMENTATION:
- Status: Implemented (clean, matches task Do steps 1-3 exactly)
- Location: internal/spawn/detect_inside.go:9-13 (type doc), :103-111 (collapsed tail); commit e14945d5
- Notes:
  - Type doc (detect_inside.go:9-13) now reads "...its last-activity timestamp (the cross-client winner-selection signal — the most-active client is the burst's trigger)." No "local-only tiebreak" or "disambiguate among host-local clients" phrasing remains. Consistent with the already-correct `detectInsideTmux` function docstring below it (:49-91) and the `selectTriggeringClient` doc (:114-117). AC1 met.
  - `detectInsideTmux` tail (:103-111) now selects the winner then returns `walkToBundle(winner.PID, walker, reader)` directly, matching `detectOutsideTmux`'s single-return propagation (detect_outside.go:47). The per-outcome commentary (resolve / clean-NULL / transient) is preserved as a one-line note block above the return rather than re-implemented as branches — exactly as Do step 2 suggested. AC2 met.
  - Behaviour-identical: the removed three branches were `werr != nil → (Identity{}, werr)`, `id.IsNull() → (Identity{}, nil)`, `else → (id, nil)`. `walkToBundle` (walk.go:60-93) already returns exactly `(Identity{}, transient-err)`, `(Identity{}, nil)` for every NULL exit (cycle/root/hop-bound all return the zero `Identity{}`), or `(resolvedID, nil)`. In the former NULL branch `id` was already the zero `Identity{}`, so returning it directly is identical to the old fresh `Identity{}`. No outcome, error-wrapping, or `IsNull()` semantics change. AC3 met.
  - Out-of-scope mirror `tmux.ClientInfo` (clients.go:9-16) left untouched — `git show e14945d5 --stat` shows the diff touches only `.tick/tasks.jsonl`, the tick manifest, and `internal/spawn/detect_inside.go`; clients.go is absent. AC4 met.

TESTS:
- Status: Adequate (existing suite, no new test required — correct for a doc-only + behaviour-identical refactor)
- Coverage: internal/spawn/detect_inside_test.go covers the full three-shape contract that the collapse-to-walkToBundle must preserve:
  - resolved→drive: single local, highest-activity-local (both list orders), first-on-tie, and the reported-bug mirror (local winner drives despite an idle remote bystander).
  - clean-NULL→no-op: all-remote, most-active-remote-with-local-bystander (the reported bug shape), zero clients.
  - transient→NULL+ErrDetectTransient: list-clients failure, sole-local walk failure, and the winner-walk-transient-failure fail-safe (dropped walk-resilience locked in) — each asserts `errors.Is(ErrDetectTransient)` + underlying-cause preservation + NULL identity.
- Notes: The commit adds/modifies no test files, which is correct — the refactor is behaviour-identical and the pre-existing matrix already pins every outcome shape, so it proves no regression. No over- or under-testing introduced by this task.

CODE QUALITY:
- Project conventions: Followed. Single-return callee propagation now mirrors the sibling `detectOutsideTmux`; doc-comment style (godoc sentence form, DI-seam explanations) consistent with the package.
- SOLID principles: Good. DRY improved — `walkToBundle`'s three-shape contract is now single-sourced across both detection paths rather than duplicated in a hand-rolled re-branch that could silently drift.
- Complexity: Low. Removed a 12-line three-branch block for a one-line return; cyclomatic complexity of `detectInsideTmux` reduced.
- Modern idioms: Yes. Idiomatic Go error/identity propagation.
- Readability: Good. Intent (defer to the single source of truth, never fall back on uncertainty) is documented in one concise comment.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tmux/clients.go:11-12 — The mirror `tmux.ClientInfo` type doc still carries the identical now-falsified phrase ("Activity is the local-only tiebreak used to choose among 2+ host-local clients"). This task DELIBERATELY excluded it (Do step 3) and it is correctly untouched, so this is NOT a defect in T2-1 — recording it only as a follow-up: the same one-line cross-client-winner-selection rewrite applied to detect_inside.go should eventually be applied here so the two mirror docs stay consistent. Out of scope for this plan.
