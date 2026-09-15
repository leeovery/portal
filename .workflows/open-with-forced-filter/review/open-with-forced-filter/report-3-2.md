TASK: Render the recorded directory beside the session name in a search-opened row (tick-36ae2d / open-with-forced-filter-3-2)

ACCEPTANCE CRITERIA:
- With `ShowDir` false the row is byte-identical to the same row rendered over a session whose `Dir` is empty — the field is not read at all — and `TestSessionRow_FlatIsNameOnly` passes unmodified
- With `ShowDir` true the directory's first cell sits exactly one column right of the name's last cell, at every name length
- The directory is rendered in the same token the row's window count takes — `text.muted` unselected, `text.secondary` selected — and no new token, no `lipgloss.Color` literal and no raw hex is introduced (`colour_literal_guard_test.go` stays green with no exemption)
- The row width is exactly the list width with the directory present, absent, left-truncated and dropped, on selected and unselected rows, in colour and colourless
- The count and attached slots stay column-aligned across rows whose name and directory lengths differ
- The session name is truncated exactly as today: a name alone wider than the flex region still truncates with an ellipsis, and the directory is dropped rather than the name being shortened to make room
- A session carrying no recorded directory renders no separating space and no separator glyph — its row is byte-identical to the same row with `ShowDir` false
- A grouped row (non-empty `GroupKey`) takes its indent out of the same budget, so its directory region is exactly `groupRowIndent` narrower
- An unsized list renders name, one space and the whole abbreviated directory without truncating either
- The colourless render carries the same single space with no bracket, glyph or separator standing in for the weight — the stripped text is identical to the coloured render's
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
§6.1 requires every row in a sigil-opened list to show its *recorded* directory beside the session name, in every grouping mode, never a value derived from a pane read — the row must show what the search actually matched against. §6.2 fixes the placement and weight: one space after the name (packed, ragged, not a right-anchored column), in the same colour token the window count takes (the muted rung of the text ramp, one step brighter on the selected row), with no new token and no raw colour at the call site; where colour is off the row is unchanged with the same single space and no glyph standing in for the weight. When the row is too narrow the directory gives way and the name never does, shortened from the left so the tail survives, dropped entirely below a floor; a home-relative path is always displayed `~/`-abbreviated at any width. §6.3 scopes the column to the picker a sigil opened and keeps it on for that picker's life even after a hand edit of the filter text.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/session_item.go:118-123` — `ShowDir bool` on `SessionDelegate`, beside `MultiSelect`, zero value off
  - `internal/tui/session_item.go:263-283` — the sized/unsized branch that derives `dirText` and re-budgets `namePad`
  - `internal/tui/session_item.go:285-289` — `dirCell` assembly (`bg.Render(" ")` + `d.rowToken(lipgloss.Style{}, countTok, selected)`)
  - `internal/tui/session_item.go:311` — row assembly with `dirCell` between `name` and `namePad`
  - `internal/tui/model.go:938` — the single production construction that sets it (`ShowDir: m.searchForm`, task 3-3)
- Notes:
  - Every criterion settles by reading. The width arithmetic closes: `fitSessionDir` is called with `remaining-1` and its contract returns a value of at most that width (`internal/tui/session_dir_column.go:28-45`), so `remaining -= 1 + lipgloss.Width(dirText)` can never go negative and `name + dirCell + namePad` always measures exactly `nameWidth`. `used` (line 261) includes `lipgloss.Width(indent)`, so a grouped row's directory region is exactly `groupRowIndent` narrower, and the count/attached slots keep their columns whatever the name and directory lengths are.
  - `ShowDir` false reads `it.Session.Dir` nowhere: the sized branch guards the `fitSessionDir` call with `if d.ShowDir` (line 275) and the unsized branch short-circuits on `d.ShowDir &&` (line 267).
  - The name keeps absolute priority — it is truncated against the whole `nameWidth` before `remaining` exists, so a long name silently costs the directory and never the reverse.
  - The spec's "only the recorded directory" rule holds end to end: the row reads `it.Session.Dir`, and the grouping-derived value lives in the separate `m.derivedDirs` map (`internal/tui/model.go:267-272`), which is never written back into `Session.Dir`.
  - The comment on `ShowDir` claims the flag "stays on for that picker's life however the filter text is edited afterwards" — true against the code: `m.searchForm` is set once at construction (`internal/tui/model.go:567`) and never cleared.
  - Two later commits touched these files (`bb1522dfc` reworked `SessionItem.FilterValue`, `39541750e` corrected the `fitSessionDir` doc comment); neither altered the render path this task owns.

TESTS:
- Status: Adequate
- Coverage: Fourteen cases added to `internal/tui/session_row_anatomy_test.go:308-620`, one per property the plan's Tests section names — one-space placement across three name lengths (`:308`), column-off byte-equality across both themes (`:327`), no separator without a recorded directory (`:342`), exact list width over present/absent/left-truncated/dropped × selected/unselected × colour/colourless (`:354`), trailing-slot alignment across differing name and directory lengths (`:384`), left-truncation with the name intact (`:416`), the floor drop (`:435`), an over-long name byte-identical with the column on and off (`:451`), the `text.muted`/`text.secondary` token over canvas/`bg.selection` in both themes (`:472`), the selection tint covering the separating space cell-by-cell (`:504`), colourless stripped-text equality (`:527`), the grouped-indent narrowing at a width where the fitted value actually changes (`:540`), the whole abbreviated value on an unsized list (`:561`), and the narrow-width sweep (`:574`).
- Notes:
  - Each test would fail if the property it names broke — the placement, token, tint and grouped-narrowing cases assert exact columns and exact rendered sequences rather than mere containment.
  - `TestSessionRow_DirColumnNeverLeansOnThePathologicalWidthBackstop:597` is the guard the attempt-1 fix round added: it sweeps every width from the narrowest sized row to 120 over three directory shapes × three name lengths × attached/unattached and asserts the right margin survives, which is what distinguishes "the arithmetic fitted" from "the final `ansi.Truncate(row, total, "…")` at line 316 chopped the overrun". Without it the exact-width assertions cannot fail for an off-by-one in the withheld separator cell.
  - `TestSessionRow_FlatIsNameOnly:250` is unmodified — the commit touched the file by appending only.
  - Not over-tested: the three width-family tests assert three different properties (exact width, no overflow at pathological widths, backstop did not fire) rather than restating one.

CODE QUALITY:
- Project conventions: Followed. No new colour token, no `lipgloss.Color` and no hex at the call site — the directory reuses the already-resolved `countTok`, so `internal/tui/colour_literal_guard_test.go` needs no exemption and was not touched by this feature. No `t.Parallel()` in the added tests; `t.Setenv("HOME", …)` is used wherever an abbreviated value is asserted, and `resolver.AbbreviateHome` reads `$HOME` per call (`internal/resolver/path.go:94-106`) with no cached value to defeat it.
- SOLID principles: Good. The fit rule stays in `fitSessionDir`; the delegate only budgets and paints.
- Complexity: Low. One added branch in each arm of an existing branch, plus a three-line cell assembly.
- Modern idioms: Yes.
- Readability: Good. The three added comments each carry a reason the code cannot state — the withheld separator cell, why the space is painted by the row background, and why the flag is off by default — and none is falsified by the code.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — needs the unit lane executed (`go test ./...` from the project root); reading settles the arithmetic and the assertions but not the suite's actual result.
