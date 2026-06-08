# Analysis — Duplication — Cycle 1

STATUS: findings
FINDINGS_COUNT: 3 (1 medium, 2 low)

## FINDING: buildByProject and buildByTag share a near-identical group-build skeleton
- SEVERITY: medium
- FILES: internal/tui/grouping.go:43-78, internal/tui/grouping.go:106-138
- DESCRIPTION: The two builders are structurally parallel near-duplicates. Both (a) iterate sessions, partitioning into a typed []SessionItem "resolved" slice and a []list.Item catch-all slice; (b) sort the resolved slice with a byte-identical slices.SortFunc comparing GroupKey then Session.Name (grouping.go:66-71 vs 126-131 are the same five lines); (c) convert the typed resolved slice into []list.Item via the same make(...); for range; append loop (grouping.go:73-77 vs 133-137); (d) hand off to appendCatchAll. Only the per-session resolution (project-match vs tag-expansion) genuinely differs. A future ordering or item-construction change must be made in lockstep in both — copy-paste-drift risk.
- RECOMMENDATION: Extract the shared tail — the (GroupKey, Session.Name) sort + the typed→[]list.Item conversion + the appendCatchAll handoff — into one helper, e.g. assembleGroups(resolved []SessionItem, catchAll []list.Item, heading string) []list.Item. Each builder then owns only its resolution loop.

## FINDING: []SessionItem → []list.Item conversion loop repeated three times
- SEVERITY: low
- FILES: internal/tui/grouping.go:73-77, internal/tui/grouping.go:133-137, internal/tui/grouping.go:225-230
- DESCRIPTION: The same idiom (allocate capacity-sized []list.Item, loop []SessionItem boxing each via append) appears three times. Necessary loop (Go can't assign typed slice to []list.Item) but past Rule-of-Three.
- RECOMMENDATION: Add sessionItemsToList(items []SessionItem) []list.Item and call from all three sites. If assembleGroups extraction is taken, two collapse automatically. Low — mechanical.

## FINDING: Edit-modal Tags block mirrors the Aliases block in handler and renderer
- SEVERITY: low
- FILES: internal/tui/model.go:1653-1709, internal/tui/model.go:2142-2190
- DESCRIPTION: Tags field cursor-clamp/x-remove/type-into-add/render block each a structural copy of the Aliases arm with field names swapped. Per spec this is a conscious mirror ("Tags behaves exactly like Aliases"). On merit should NOT be force-extracted in v1: aliases and tags have diverging persistence semantics (collision-checked SetAndSave/DeleteAndSave vs AddTag/RemoveTag-with-normalise). Recorded for visibility only.
- RECOMMENDATION: Leave as-is for v1 (accepted intentional mirror). No action this cycle.

SUMMARY: Two grouping builders share a duplicated sort comparator + item-conversion loop + catch-all handoff to consolidate into an assembler helper (medium); related boxing loop recurs three times (low). Modal Tags/Aliases mirror is deliberate/spec-sanctioned — visibility only. Catch-all helpers, prefs path resolution, ListSessions parser, CanonicalDirKey, NormaliseTag all single-sourced and reused correctly.
