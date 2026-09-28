TASK: lazy-resume-on-attach-6-5 — The Pending Dot Packs Beside the Attached One

ACCEPTANCE CRITERIA:
- A row whose session is in the pending set renders the attention-token dot; one that is not renders nothing in its cell.
- A row with only a pending dot puts it hard right, at exactly the column an attached-only row puts its dot.
- A row with both renders attached then pending, with the attached dot pushed left by one cell — neither indicator holds a reserved lane.
- The row's total width is identical across all four combinations: neither, attached alone, pending alone, both.
- A session holding several waiting panes carries exactly one dot, because the set is keyed on the session name.
- A gone row still replaces the whole trailing region with the badge and shows neither indicator, whatever the pending set says.
- Under `NO_COLOR` the four combinations render as nothing, `A`, `P` and `AP`, in the same cells and the same order, with no second row geometry.
- A nil pending set marks no row and never panics: every row renders exactly as it does under an empty set.
- The pending dot is `accent.attention`; no raw hex appears at the call site.

STATUS: complete

SPEC CONTEXT: The spec's section on the picker's session row makes pending-resume a second, independent indicator beside the (now word-less) attached dot, in `accent.attention`, set when any pane in the session waits (one dot per session, not per pane). The dots pack right in the fixed order attached-then-pending with no reserved lanes; under NO_COLOR each becomes a letter in the same cell (`A`, `P`, `AP`) so state stays glyph-backed. A gone row is unaffected — the `session gone` badge owns the whole trailing region. Wiring the set from the pending read is task 6-6; the help legend is 6-7; the selected-row contrast guard is 6-10.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/session_item.go:41 — `pendingIndicatorLetter = "P"` beside `attachedIndicatorLetter`
  - internal/tui/session_item.go:47-50 — `rowIndicators` gains `pending`
  - internal/tui/session_item.go:54-57 — `indicatorSlotWidth` re-derived from the widest cluster (attached+pending) in both colour modes, so the reservation tracks the renderer and no call site restates it
  - internal/tui/session_item.go:141-144 — `SessionDelegate.Pending map[string]struct{}`, keyed on `Session.Name` like `Selected`/`GoneFlagged`; nil marks nothing (`isSelected` on a nil map is a safe lookup)
  - internal/tui/session_item.go:316-328 — gone branch untouched; non-gone branch feeds `pending: isSelected(d.Pending, it.Session.Name)` into the cluster and left-pads the slot, so the cluster is right-packed
  - internal/tui/session_item.go:341-358 — `indicatorCluster` concatenates attached (`StatePositive`, `A`) then pending (`AccentAttention`, `P`) with no separator; `indicator` shares one geometry between the dot and the letter
- Notes: Matches every criterion and the "Do" list. Slot = 2 cells (`●●` / `AP`), clusters of width 0/1/1/2 always pad to it, so row width and the count column are invariant across the four combinations; a lone indicator of either kind lands at the same hard-right cell. Token referenced through `d.Theme.AccentAttention`, no hex. Wiring into the model (`model.go:951`) belongs to 6-6 and is present.

TESTS:
- Status: Adequate
- Coverage: internal/tui/session_row_pending_test.go covers all nine planned tests — pending dot present/absent with exact suffix and absence of the attention SGR on a non-pending row (dark + light); lone pending vs lone attached at the same column across widths 40/80/120 in both colour modes; attached-then-pending order proved by the opening SGR of each dot (state.positive then accent.attention) and the pair's column; width identity plus fixed count column across all four combos at four widths in both modes; gone row byte-identical with and without a pending set, badge owning the suffix; NO_COLOR table over the four combinations asserting exact cells; nil set byte-identical to an empty set across rows and selections; attention fg over selection and canvas backgrounds. Existing goldens and alignment assertions (row_style_helpers_test.go, session_style_consolidation_test.go, colourless_nocolor_test.go, multi_select_marker_test.go, session_item_test.go) were moved to the widened slot, and the dir-narrowing test's width moved 41→42 to keep its subject's name budget unchanged. The narrow-width backstop tests derive from `indicatorSlotWidth` and track the change.
- Notes: The several-waiting-panes test (`markedSet("alpha","alpha","alpha")`) can only observe a one-member set at the delegate; the per-session collapse it names is exercised where the set is built (internal/tmux/tmux_test.go "it counts panes and sets sessions"), so the criterion is covered across the change-set.

CODE QUALITY:
- Project conventions: Followed (token-only colour, no t.Parallel, glyph-backed NO_COLOR, comments free of process artifacts)
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
