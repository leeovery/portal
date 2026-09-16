# Review Report: Open With Forced Filter (Cycle 2)

## Stats

- Total findings: 1
- Deduplicated findings: 1
- Proposed tasks: 1

## Summary

The second cycle's prep routed a single finding, `13-1-1`, and it carries `replan` without a blocking marker — it is the whole of this synthesis. Task 13-1 closed the within-generation collision, but its withholding rule is applied to the entries the source currently holds rather than to the session the handed target came from, so a filter pass running behind a list rebuild over a colliding filter value can still surface a row neither of whose fields contains the search term. The review left the fix's shape open across three candidates; it is settled here on the specification's own failure direction plus the source's measured lifetime, so the proposal carries a direction rather than a question.

## Spec Defects

None. The action does not indict the specification. §4.4 states the rule the code fails to hold — "Withholding is the only safe answer available… ranking both would surface a session neither of whose fields contains the term, precisely what §4.3 separates the fields to prevent" — and its neighbouring sentence, "A filter pass answering for a list generation the source has since rebuilt omits in the same direction", is a claim the current code falsifies in the compound case. The claim is the right one and the code is the wrong side: the specification prescribes omission as the sanctioned failure and surfacing a non-matching row as the forbidden one, and closing the finding restores that sentence's truth rather than requiring it to be rewritten. The 2026-09-16 corrigendum (`specification.md:500`) that records the carve-out points at task 13-1's commit `2fe01e014` as the settlement; the settlement is incomplete, not misdescribed. No other claim in the specification is reached by this action.

## Discarded Findings

None. Prep discarded nothing this cycle, and routed nothing `do-now` or `out-of-scope` — the one finding it carried is the one proposal above.
