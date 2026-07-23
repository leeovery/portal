AGENT: duplication
FINDINGS:
- FINDING: detectInsideTmux re-states walkToBundle's return contract that its sibling propagates verbatim
  SEVERITY: low
  FILES: internal/spawn/detect_inside.go:105-117, internal/spawn/detect_outside.go:47
  DESCRIPTION: detectInsideTmux consumes walkToBundle and then re-branches its
    three-shape result (werr != nil -> return Identity{}, werr; id.IsNull() ->
    return Identity{}, nil; else -> return id, nil). walkToBundle already
    guarantees exactly those three shapes — (id, nil) on resolve, (Identity{},
    nil) on clean NULL, (Identity{}, transient-err) on failure — so the tail
    block is behaviourally identical to `return walkToBundle(winner.PID, walker,
    reader)`. Its sibling detectOutsideTmux (detect_outside.go:47) consumes the
    exact same callee and propagates it directly with a single return. The two
    sibling detection functions therefore handle the identical walkToBundle
    contract two different ways. This is near-duplicate logic that restates a
    contract the callee already owns: if walkToBundle's outcome shapes ever
    change, the inside path's hand-rolled branch can silently drift from both the
    callee and the outside path. The three comments do carry documentation value,
    but that same intent is already documented on walkToBundle itself.
  RECOMMENDATION: Collapse the tail of detectInsideTmux (after selecting the
    winner) to `return walkToBundle(winner.PID, walker, reader)`, matching how
    detectOutsideTmux propagates the walk contract, so both detection paths defer
    to walkToBundle's single source of truth and cannot diverge. Behaviour and
    the Detect() WARN/INFO folding are unchanged. If the per-outcome commentary
    is worth keeping, move it to a one-line note above the return rather than a
    re-implementation of the branches.
SUMMARY: The T1-1 change is a small, well-factored inversion that correctly reuses the shared walkToBundle, transient, and DI-seam helpers and the shared test fakes (localWalkSeams / walk_test.go fakes), so no significant duplication was introduced. The one low-severity item is that detectInsideTmux re-branches walkToBundle's return contract that its sibling detectOutsideTmux propagates verbatim — a behaviour-identical restatement that could be collapsed to remove drift risk.
