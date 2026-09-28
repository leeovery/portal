TASK: lazy-resume-on-attach-6-4 — The attached indicator loses its word

ACCEPTANCE CRITERIA:
- An attached row renders the dot alone in the trailing region — the word `attached` appears nowhere in a rendered row.
- An attached row, an unattached row and a gone row rendered at the same list width are all exactly that width.
- The name's flex budget grows by exactly the cells the word gave back: at a fixed width, a name that truncated before now renders more of itself, and no blank hole is left where the word was.
- A gone row still replaces the whole trailing region with the badge, at the same right-edge position as today, with the row's width unchanged.
- The dot keeps `state.positive`, on a selected row as on an unselected one.
- Under `NO_COLOR` an attached row renders `A` in the indicator cell rather than a bare dot, and an unattached row renders neither.
- `indicatorSlotWidth` is derived from the rendered cluster, so no call site restates the slot's width.
- No raw hex appears at the call site — `internal/tui`'s colour-literal guard passes with no exemption added.

STATUS: issues_found

SPEC CONTEXT: The session row drops the word `attached` so a second (pending-resume) dot fits; indicators pack hard right in a fixed order (attached, then pending) rather than holding reserved lanes. Under NO_COLOR each indicator is a letter in the same cell (`A`, `P`), because state must stay glyph-backed. A gone row is unaffected: the `session gone` badge still replaces the whole trailing region. The wider row rework (window count, paths, right-hand strip) stays parked on `picker-row-redesign`. The help legend rides in a separate task (6-7).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/session_item.go:37-42 — `rowIndicatorGlyph`, `attachedIndicatorLetter` (plus `pendingIndicatorLetter`, added by task 6-5)
  - internal/tui/session_item.go:44-45 — `goneBadge` with the one-line reservation note replacing the old "must not exceed" constraint
  - internal/tui/session_item.go:52-57 — `indicatorSlotWidth`, taken as the max of the rendered cluster's width in both colour modes. The renderer is called at package init, so no call site restates the slot's width.
  - internal/tui/session_item.go:276-281 — per-row `trailingWidth` (badge width on a gone row, slot + margin otherwise) feeding `used`
  - internal/tui/session_item.go:316-328 — the gone branch renders the badge as before (pad of 0). The non-gone branch renders left-pad to the slot, then the cluster, then the margin.
  - internal/tui/session_item.go:339-358 — `indicatorCluster` / `indicator`. The glyph is resolved through `rowToken(…, StatePositive, selected)`, and the letter is used when `Colourless`.
- Notes:
  - HEAD carries task 6-5's extension: the pending dot, and a 2-cell `indicatorSlotWidth`. The 6-4 contract still holds under it.
  - Gone row byte-identity: `used` stays 27 on a gone row (2+2+11+12), the same as the old 2+2+11+10+2, so the golden in `TestSessionRow_GoneBadgeReplacesTheWholeTrailingRegion` matches the old formula byte for byte. Checked by hand at w=60: pads of 16 and 13.
  - Deliberate consequence the plan accepted: the reservation is now per row, so a gone row's window count sits 8 cells left of its neighbours' counts at HEAD (9 at the 6-4 commit). The pre-flight-abort design frame (`testdata/vhs/reference/sessions-multi-select-preflight-abort-mv.png`) shows that count aligned. The executor dropped the old count-column assertion from `TestSessionRow_GoneFlaggedWidthByteUnchanged` on purpose, and the task's edge cases trade this for the transient flag. Recorded here, not raised as a finding.
  - Deliberate divergence from the committed row frame: `testdata/vhs/reference/sessions-pending-resume-dot-nord.png` keeps the count column where it was and leaves the reclaimed cells empty between the count and the dots. The plan explicitly chose to give those cells to the name ("no blank hole"), and the criteria encode that choice. Not a finding.

TESTS:
- Status: Adequate
- Coverage: every planned test exists in internal/tui/session_row_anatomy_test.go:
  - "renders without its word" → `TestSessionRow_AttachedIndicatorRendersWithoutItsWord` (:635)
  - "same row width" → `TestSessionRow_SameWidthAttachedUnattachedAndGone` (:650): 4 widths × colour/colourless × selected/unselected
  - "reclaimed cells to the name" → `TestSessionRow_ReclaimedCellsGoToTheName` (:670): the name delta equals the word's width minus the slot, and the count starts at name end + gap
  - "gone badge replaces the trailing region" → `TestSessionRow_GoneBadgeReplacesTheWholeTrailingRegion` (:706): byte goldens, badge hard right, no indicator
  - "positive token when selected" → `TestSessionRow_IndicatorKeepsPositiveTokenWhenSelected` (:739): checks the SGR that opens the glyph for fg plus the selection or canvas bg
  - "A under NO_COLOR" → `TestSessionRow_ColourlessAttachedRendersLetterNotDot` (:755)
  - "nothing for unattached" → `TestSessionRow_UnattachedRendersNothingInTheIndicatorCell` (:778)
  - "packs hard right" → `TestSessionRow_IndicatorPacksHardRight` (:793)

  The six listed suites (plus model_test.go and multi_select_marker_test.go) were moved off the `● attached` / `attachedMarker` strings. No literal remains in the tree.
- Notes:
  - The width-equality test can only catch underflow, because the `ansi.Truncate` backstop masks overflow. The byte golden and the reclaimed-cells test cover the overflow direction, so together they are sufficient.
  - One test table was left behind by the flex-budget change (see FINDINGS).

CODE QUALITY:
- Project conventions: Followed (tokens only, resolved through rowToken; no raw hex; the colour-literal guard is untouched; comments carry no process artefacts)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes (built-in `max`)
- Readability: Good. The comments at :44 and :52-53 and the doc on `indicatorCluster` hold against the code.
- Issues: none in production code

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/session_row_anatomy_test.go:366 — The `"directory dropped below the floor"` case of `TestSessionRow_DirColumnKeepsTheRowExactlyTheListWidth` still renders at width 34. Before this task, the 12-cell trailing reservation left a 1-cell directory budget there, so the directory was dropped. With the reclaimed cells, the flat row's `used` is 19 at HEAD. The name flex is then 15 and the budget 9, so `fitSessionDir` returns `…/tui` and the case now exercises a left-truncated directory, not a dropped one. This task moved the two sibling tests at :421 (50→41) and :440 (34→25) for exactly this reason but missed this table. Its neighbour at :365 (`"left-truncated directory"`, width 50) sits one cell from its own threshold: at the 6-4 commit (1-cell slot) it rendered the directory whole, and it truncates at HEAD only because 6-5 widened the slot. Fix: set the widths to 41 and 25, matching the siblings. — FAILS: no exact-width assertion covers a ShowDir row whose directory is dropped. `TestSessionRow_DirColumnDropsTheDirectoryBelowTheFloor` checks only the name and the absence of `/~`, and the backstop sweep checks only the right-margin suffix. A padding regression in the dropped-directory branch, such as `namePad` short by the withheld separator cell, would pass while the case's name claims it is covered.

UNSETTLED:
- "No raw hex appears at the call site — `internal/tui`'s colour-literal guard passes with no exemption added." — Reading shows no hex in `internal/tui/session_item.go` and no change to `internal/tui/colour_literal_guard_test.go` in this feature. Settling "passes" needs a run of `go test ./internal/tui -run ColourLiteral`.
