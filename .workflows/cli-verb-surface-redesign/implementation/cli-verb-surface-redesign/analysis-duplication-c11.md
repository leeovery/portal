AGENT: duplication
FINDINGS:
- FINDING: Twin prefix-filter completers are structurally identical
  SEVERITY: low
  FILES: cmd/completion.go:40-48, cmd/completion.go:81-89
  DESCRIPTION: completeSessionNames and completeAliasKeys — both new in this
    redesign's Tab Completion feature — are byte-for-byte the same shape: allocate
    a matches slice, range a candidate source keeping strings.HasPrefix(x,
    toComplete), and return (matches, cobra.ShellCompDirectiveNoFileComp). They
    diverge only in the candidate-source call (completionSessionNames() vs
    completionAliasKeys()) and the loop variable name (name vs key). The
    prefix-filter-then-NoFileComp logic is the reusable part; today it is authored
    twice, so a change to the completion directive or the match rule must be made in
    lockstep in two places. Below the strict Rule-of-Three bar (two instances, ~9
    lines each), so this is a low-value tidy, not an urgent consolidation — but it
    is a clean, exact duplicate.
  RECOMMENDATION: Extract a package-private helper, e.g.
    prefixCompletions(toComplete string, candidates []string) ([]string,
    cobra.ShellCompDirective), and reduce both completers to
    `return prefixCompletions(toComplete, completionSessionNames())` /
    `... completionAliasKeys())`. Keeps the two seam functions and their doc
    comments; only the shared loop body collapses.

- FINDING: Mass-deletion hazard guard re-implemented in the read-only stale-hooks check
  SEVERITY: low
  FILES: cmd/doctor.go:444-452, cmd/run_hook_stale_cleanup.go:119-126
  DESCRIPTION: The safety-critical "empty live-pane set + persisted hooks present =>
    do NOT treat every entry as stale" predicate is authored twice. The prune side
    is single-sourced in runHookStaleCleanup (called verbatim by both the daemon's
    maybeRunHookCleanup and doctor --fix's pruneDoctorStaleHooks), but doctor's
    read-only diagnosis checkStaleHooks re-derives the same `len(live)==0 &&
    len(persisted)>0` branch inline (with a both-empty pass sub-case) to emit a
    checkResult instead of logging+returning. The two outputs legitimately differ
    (a diagnosis vs a deferral), and doctor.go:426-429 explicitly acknowledges the
    guard is "applied here (and in runHookStaleCleanup)". The shared, load-bearing
    part is the *condition* — the invariant that a zero-length live set must never
    make hooks.StaleKeys classify everything as orphaned. Duplicating that condition
    across the diagnosis and the prune is precisely the drift class the codebase
    otherwise closes with StaleKeys/StaleEntries.
  RECOMMENDATION: Lift the condition (not the branch bodies) into one predicate the
    hooks package owns alongside StaleKeys — e.g. hooks.LivePanesUnreliable(live,
    persisted []) bool returning true when len(live)==0 && len(persisted)>0 — and
    have both checkStaleHooks and runHookStaleCleanup gate on it, each keeping its
    own divergent output (checkNotEvaluable vs Warn-and-defer). This anchors the
    safety invariant in one place while preserving the two distinct result shapes.
SUMMARY: Cycle 11 finds a heavily-consolidated surface — the two burst paths, log
  emission, stale-classification predicates, message renderers, and DI-deps idiom
  are all deliberately single-sourced with guard tests. Only two minor, low-severity
  near-duplicates remain: the twin completion prefix-filters and the mass-deletion
  hazard condition re-authored in doctor's read-only stale-hooks check.
