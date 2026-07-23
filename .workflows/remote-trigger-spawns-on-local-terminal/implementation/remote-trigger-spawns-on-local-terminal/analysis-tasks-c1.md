---
topic: remote-trigger-spawns-on-local-terminal
cycle: 1
total_proposed: 1
---
# Analysis Tasks: Remote Trigger Spawns On Local Terminal (Cycle 1)

## Task 1: Clean up T1-1 gate-inversion residues in detect_inside.go
status: pending
severity: low
sources: architecture, duplication

**Problem**: The T1-1 gate inversion in `internal/spawn/detect_inside.go` left two residues from the same change. (1) The `ClientActivity` type doc (detect_inside.go:11) still describes the `Activity` field as "the local-only tiebreak used to choose among 2+ host-local clients" — the exact contract phrase the fix inverted. `Activity` now selects the triggering winner across ALL clients (local and remote alike), and the spec's "Owned Behaviour Change" section explicitly names leaving this old contract text in place ("describing behaviour the code no longer has") as a must-not-remain residue. The already-rewritten `detectInsideTmux` function docstring below it is correct, so the type doc is now internally inconsistent and invites re-introduction of filter-then-tiebreak reasoning. (2) `detectInsideTmux` (lines 105-117) re-branches `walkToBundle`'s three-shape return contract (werr != nil → return Identity{}, werr; id.IsNull() → return Identity{}, nil; else → return id, nil), even though `walkToBundle` already guarantees exactly those three shapes and its sibling `detectOutsideTmux` (detect_outside.go:47) propagates the same callee with a single `return walkToBundle(...)`. The hand-rolled branch is behaviour-identical but can silently drift from both the callee and the outside path if `walkToBundle`'s outcome shapes ever change.

**Solution**: Update the `ClientActivity` type doc so the `Activity` field's role matches the new cross-client contract, and collapse the tail of `detectInsideTmux` (after winner selection) to defer to `walkToBundle` as its sibling does. Both are pure cleanup with no runtime behaviour change.

**Outcome**: The type doc no longer describes behaviour the code no longer has (satisfying the spec's must-not-remain requirement) and stays consistent with the corrected function docstring; both detection paths defer to `walkToBundle`'s single source of truth so they cannot diverge. `Detect()`'s WARN/INFO folding and every existing detection outcome are unchanged.

**Do**:
1. In `internal/spawn/detect_inside.go`, at the `ClientActivity` type doc (~line 11), replace the "local-only tiebreak used to choose among 2+ host-local clients" phrasing with a description of `Activity` as the cross-client winner-selection signal — e.g. "its last-activity timestamp (the cross-client winner-selection signal — the most-active client is the burst's trigger)". Keep it consistent with the already-correct `detectInsideTmux` function docstring below it.
2. In `detectInsideTmux` (~lines 105-117), after selecting the winner, replace the three-branch restatement of `walkToBundle`'s result with a single `return walkToBundle(winner.PID, walker, reader)`, matching how `detectOutsideTmux` propagates the walk contract. If the per-outcome commentary (resolve / clean-NULL / transient) is worth keeping, move it to a one-line note above the return rather than re-implementing the branches.
3. Leave the out-of-scope mirror `tmux.ClientInfo` type doc (`internal/tmux/clients.go:11-12`) untouched — it carries the identical now-falsified phrase but is outside this plan's scope.

**Acceptance Criteria**:
- The `ClientActivity` type doc contains no "local-only tiebreak" / "disambiguate among host-local clients" phrasing and describes `Activity` as the cross-client winner-selection signal, consistent with the function docstring.
- `detectInsideTmux` returns `walkToBundle(winner.PID, walker, reader)` directly after selecting the winner, with no hand-rolled re-branching of its three-shape result.
- No behaviour change: `Detect()`'s NULL+WARN folding and all existing detection outcomes (resolved→drive, clean-NULL→no-op, transient→NULL+ErrDetectTransient) are unchanged.
- `internal/tmux/clients.go` is unchanged (out of scope).

**Tests**:
- Existing `internal/spawn` detection tests pass unchanged — in particular the inside-tmux winner-walk cases (resolved→drive, clean-NULL→no-op, transient→NULL+ErrDetectTransient-wrapped-WARN), confirming the collapse to `walkToBundle` preserves the three-shape contract.
- No new test required (doc-only edit plus a behaviour-identical refactor); the existing suite proves no regression.
