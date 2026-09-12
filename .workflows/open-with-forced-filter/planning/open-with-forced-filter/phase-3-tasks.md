---
phase: 3
phase_name: The Directory Column in a Search-Opened Picker
total: 4
---

# Phase 3: The Directory Column in a Search-Opened Picker — 4 tasks

## open-with-forced-filter-3-1

### Task 3-1: Fit a session's recorded directory into a column width

**Problem**: Phase 2 made a session findable by its recorded directory, so a search-opened list returns rows the user cannot account for — `api-work` appears under `/port` because it lives in the Portal checkout, while the row shows a name, a window count and an attached marker and nothing else. The column that answers that has a width rule the existing row machinery cannot supply: `renderSessionRow` clamps with `ansi.Truncate` (`internal/tui/session_item.go:256`), which drops the **tail** — exactly the half of a path a human recognises a checkout by. The directory needs the opposite cut (shortened from the left, `…/Code/portal` rather than `/Users/leeovery/Cod…`), the home abbreviation applied at every width rather than as a rung of that ladder, and a floor below which it is dropped rather than truncated to noise. That decision is a pure function of a string and a width; deciding it inside the delegate would make every width case need a rendered row to test and would tangle the floor rule with the row's styling.

**Solution**: One pure helper in `internal/tui` that answers "what does this recorded directory look like in N cells" — the home-abbreviated value whole when it fits, the longest separator-anchored tail behind an ellipsis when it does not, and nothing at all when not even the last whole segment fits beside that ellipsis.

**Outcome**: `fitSessionDir("<home>/Code/portal", 40)` is `~/Code/portal`; the same value at a width that cannot hold it returns a `…/`-prefixed tail cut at a separator, never mid-segment; a width that cannot hold the ellipsis plus the last whole segment returns the empty string; a session carrying no recorded directory returns the empty string at every width.

**Do**:
- Add `internal/tui/session_dir_column.go` with `const dirTruncationPrefix = "…"` and `func fitSessionDir(dir string, width int) string`.
- Return `""` when `dir == ""` or `width <= 0`.
- Abbreviate once through `resolver.AbbreviateHome(dir)` (added in task 1-2; `internal/tui` already imports `internal/resolver`), and return that value unchanged when its `lipgloss.Width` is `<= width`.
- Otherwise scan the abbreviated value's `/` positions left to right and return the first candidate `dirTruncationPrefix + value[i:]` whose `lipgloss.Width` is `<= width`, skipping any candidate whose text after the prefix is separators only; return `""` when no candidate qualifies.
- Add `internal/tui/session_dir_column_test.go` (`package tui`) as a table over the shapes in **Tests**, with every home-relative case anchored on `t.Setenv("HOME", <t.TempDir()>)` so no verdict depends on the developer's account name.
- Wire the helper nowhere: its only caller is the row renderer (task 3-2).

**Acceptance Criteria**:
- [ ] A value that fits is returned home-abbreviated and otherwise byte-identical — `~/Code/portal`, never the absolute path tmux recorded
- [ ] The abbreviation is applied before the width is consulted, so it holds at every width and is never reached only as a rung of the shortening ladder
- [ ] A value too wide for the column is shortened from the left at a separator, so the surviving tail is whole segments prefixed by `…/` and never a mid-segment fragment
- [ ] The longest tail the width admits is the one returned
- [ ] A width that cannot hold the ellipsis plus the last whole segment returns `""` — the floor drops the directory rather than truncating it to noise
- [ ] A value carrying no separator at all is returned whole when it fits and `""` when it does not
- [ ] A value whose last segment is empty (a stamped trailing separator) never yields a separators-only tail
- [ ] An empty `dir` returns `""`, and a zero or negative width returns `""`
- [ ] A directory equal to the home directory returns a bare `~`; a directory outside the home directory is returned unabbreviated
- [ ] Widths are measured as display width, so a path carrying wide characters is fitted by cells rather than bytes
- [ ] The helper reads no file, issues no tmux call and consults nothing beyond `AbbreviateHome`'s `$HOME` read
- [ ] `go test ./...` passes

**Tests**:
- `"it returns the home-abbreviated value when it fits"` — `<home>/Code/portal` at width 40 → `~/Code/portal`
- `"it abbreviates at the narrowest width that fits"` — the same value at exactly `len("~/Code/portal")`
- `"it shortens from the left at a separator so the tail survives whole"` — `<home>/Code/portal/internal/tui` at a width admitting `…/internal/tui`
- `"it returns the longest tail the width allows"` — two widths over one deep path, asserting the tail grows with the width
- `"it never cuts inside a segment"` — sweep every width from 1 to the value's full width and assert each non-empty result is `…` followed by whole `/`-delimited segments of the input
- `"it drops the directory when the last segment cannot fit beside the ellipsis"` — returns `""`
- `"it returns a value carrying no separator whole or not at all"` — `scratchpad` at a width that fits and one that does not
- `"it never emits a separators-only tail"` — `<home>/Code/` at a narrow width returns `""`
- `"it returns a bare tilde for the home directory itself"`
- `"it leaves a directory outside the home directory unabbreviated"` — `/opt/tools`
- `"it returns the empty string for an empty recorded directory"` — at a generous width
- `"it returns the empty string for a zero or negative width"`
- `"it measures display width rather than byte length"` — a path segment of wide characters fitted by cells
- `"it returns the raw path when the home directory cannot be resolved"` — `t.Setenv("HOME", "")`

**Edge Cases**:
- `ansi.TruncateLeft` is deliberately not used: it drops a caller-supplied number of cells and would cut mid-segment, which is the `…l` rendering the floor exists to prevent. Every cut this helper makes lands on a separator, so a suffix scan is exact and the ANSI-aware variant buys nothing over plain text
- The first candidate (dropping only a leading `~` or the leading `/`) is never wider than the whole value minus nothing, so it self-rejects on the same width test rather than needing a special case
- A recorded directory is whatever was stamped: nothing normalises it, so a trailing separator, a doubled separator and a relative-looking value all reach this helper and must each answer without panicking
- The floor is derived rather than declared: the shortest candidate the scan can produce is `…` plus the last separator-prefixed segment, so a width that cannot hold that holds nothing
- The value is plain text — a recorded directory carries no ANSI — so display width comes from `lipgloss.Width`, matching the measurement the rest of the row uses
- The abbreviation is a pure string test with no `filepath.EvalSymlinks` and no `os.Stat`, so the same value fits identically on a machine where the directory exists and one where it does not

**Context**:
> When the row is too narrow for both, the directory gives way and the session name never does. The directory is shortened **from the left**, so its tail survives — `…/Code/portal`, not `/Users/leeovery/Cod…` — because the tail is the segment a human recognises a checkout by, and the whole premise of showing it is that the directory is recognisable where the name is not. A path under the user's home is always displayed abbreviated to `~/`, at any width — the abbreviation is how the value is rendered rather than a rung of the truncation ladder, and it commonly reclaims enough width that no truncation is needed at all.
>
> Below a floor the directory is dropped rather than truncated to noise. When the remaining width cannot hold an ellipsis plus one whole path segment, the row shows the session name alone — a row ending in `…l` says nothing and reads as damage, where a bare name at least reads as a name.
>
> The rendering never narrows the search. A term is tested against the whole recorded directory in its home-abbreviated form, which is fixed before any row is laid out, so a row can survive on a segment the width pushed out of view or the floor dropped altogether. What the column carries is the recognisable tail of the value that was matched, not a promise that the matched run itself is on screen.
>
> Only the recorded directory is ever displayed: the value is never derived from a pane read, for the same reason the match is not.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.1, §6.2

## open-with-forced-filter-3-2

### Task 3-2: Render the recorded directory beside the session name in a search-opened row

**Problem**: The session row renders the name, a fixed count slot, a fixed attached slot and a right margin (`renderSessionRow`, `internal/tui/session_item.go:206`), and `TestSessionRow_FlatIsNameOnly` pins that a directory never leaks into it. A search-opened list has to show one: `/port` returns `api-work` on the strength of a path the row never displays, which sends the user back to previewing each candidate — the ceremony the whole feature removes. The column cannot be bolted on as an extra region, because the row's count and attached slots are fixed and column-aligned, and the whole row must stay exactly the list width: the directory has to come out of the name's own flex budget, after the name has taken what it needs, without moving a single trailing slot.

**Solution**: An off-by-default flag on `SessionDelegate` that, when set, renders task 3-1's fitted directory one space after the session name inside the name's flex region, in the same colour token the row's window count takes, with the name's pad shortened by exactly what the directory and its space consumed.

**Outcome**: A delegate carrying the flag renders `portal-a1b2 ~/Code/portal` with the count and attached slots in the same columns as every other row and the row width unchanged; a delegate without it renders today's row byte-for-byte; the name is truncated exactly as it is today and never gives way to the directory; below the floor the row shows the name alone.

**Do**:
- Add `ShowDir bool` to `SessionDelegate` (`internal/tui/session_item.go`), beside `MultiSelect` — the zero value is off, so every existing construction site (including the hand-built `SessionDelegate{}` in the anatomy suite) keeps today's row.
- In `renderSessionRow`'s sized branch (`total > 0`), after `visibleName` is computed, derive `dirText := ""` and, when `d.ShowDir`, set it to `fitSessionDir(it.Session.Dir, nameWidth-lipgloss.Width(visibleName)-1)` — the one cell subtracted is the single separating space.
- In the unsized branch (`total <= 0`), set `dirText` to `resolver.AbbreviateHome(it.Session.Dir)` when `d.ShowDir` and the session carries a directory, matching that branch's existing render-whole-and-do-not-truncate flow.
- Assemble a `dirCell` string: empty when `dirText == ""`, otherwise `bg.Render(" ")` followed by `d.rowToken(lipgloss.Style{}, countTok, selected).Render(dirText)` — the same `countTok` value the count slot already resolved, so the selected row's brighter rung comes for free and no new token is referenced. Insert it between `name` and `namePad` in the row assembly, and subtract `1 + lipgloss.Width(dirText)` from the `namePad` width when it is non-empty.
- Leave the `HeaderItem` arm, the gone-badge branch, the marked/selector bar, `countSlotWidth`, `attachedSlotWidth`, `rowRightMargin` and the final `ansi.Truncate` guard untouched.
- Extend `internal/tui/session_row_anatomy_test.go` (`package tui`, the `renderRow` / `visibleColOf` harness) with the cases in **Tests**, pinning `t.Setenv("HOME", …)` wherever an abbreviated form is asserted.

**Acceptance Criteria**:
- [ ] With `ShowDir` false the row is byte-identical to the same row rendered over a session whose `Dir` is empty — the field is not read at all — and `TestSessionRow_FlatIsNameOnly` passes unmodified
- [ ] With `ShowDir` true the directory's first cell sits exactly one column right of the name's last cell, at every name length
- [ ] The directory is rendered in the same token the row's window count takes — `text.muted` unselected, `text.secondary` selected — and no new token, no `lipgloss.Color` literal and no raw hex is introduced (`colour_literal_guard_test.go` stays green with no exemption)
- [ ] The row width is exactly the list width with the directory present, absent, left-truncated and dropped, on selected and unselected rows, in colour and colourless
- [ ] The count and attached slots stay column-aligned across rows whose name and directory lengths differ
- [ ] The session name is truncated exactly as today: a name alone wider than the flex region still truncates with `…`, and the directory is dropped rather than the name being shortened to make room
- [ ] A session carrying no recorded directory renders no separating space and no separator glyph — its row is byte-identical to the same row with `ShowDir` false
- [ ] A grouped row (non-empty `GroupKey`) takes its indent out of the same budget, so its directory region is exactly `groupRowIndent` narrower
- [ ] An unsized list renders name, one space and the whole abbreviated directory without truncating either
- [ ] The colourless render carries the same single space with no bracket, glyph or separator standing in for the weight — the stripped text is identical to the coloured render's
- [ ] `go test ./...` passes

**Tests**:
- `"it renders the recorded directory one space after the session name"` — `visibleColOf` on the directory equals the name's column plus its width plus one
- `"it renders today's row when the column is off"` — one session with a `Dir`, `ShowDir` false, byte-compared against the same session with `Dir` cleared
- `"it renders no separator for a session carrying no recorded directory"` — `ShowDir` true, empty `Dir`, byte-compared against the `ShowDir` false render
- `"it keeps the row exactly the list width"` — table over present / absent / truncated / dropped directories at several widths
- `"it keeps the count and attached slots aligned across differing name and directory lengths"`
- `"it left-truncates the directory and never the name"` — a width where both compete: the name is whole, the directory carries the `…/` tail
- `"it drops the directory below the floor and renders the name alone"`
- `"it truncates an over-long name exactly as it does with the column off"` — the directory absent from that row
- `"it takes the window count's token"` — `tokenFgSeq` for `text.muted` unselected and `text.secondary` selected, both themes
- `"it renders the directory on the selected row's background"` — the `bg.selection` tint covers the separating space and the directory
- `"it renders the same single space with no glyph when colourless"` — `Colourless: true`, stripped row compared to the coloured render's stripped text
- `"it narrows the directory region by a grouped row's indent"` — same session, `GroupKey` set and unset, at a width where the extra two cells change the fitted value
- `"it renders the whole directory on an unsized list"` — `renderRow` at width 0
- `"it never overflows at narrow widths with the column on"` — the existing width sweep (1, 5, 10, 20, 25, 26, 29, 40, 80) re-run with `ShowDir` true and a long recorded directory

**Edge Cases**:
- The name keeps absolute priority: it is truncated against the full flex region first, and the directory takes only what is left, so a long name silently costs the directory rather than the reverse
- The separating space must be painted by the row background style, or the selected row shows an untinted gap between the name and the directory
- The unsized branch is a pre-first-`WindowSizeMsg` transient with no budget to flex against; it renders the abbreviated value whole because that branch already renders the name whole, and the final width guard is skipped there exactly as it is today
- The delegate is a value type constructed fresh on every restyle, so the flag carries no state and needs no invalidation — it is read at render time from whatever the model built (task 3-3)
- `HeaderItem` rows never reach `renderSessionRow`, so a group heading can never acquire a directory
- The gone-badge branch replaces the attached slot and the right margin, not the name region, so a gone row carries its directory unchanged
- The final `ansi.Truncate(row, total, "…")` guard stays as the backstop for pathological widths where the floored name region plus the fixed slots already exceed the list width; the directory arithmetic must not rely on it, which is what the width assertions pin

**Context**:
> The directory begins one space after the session name — packed against it rather than anchored to a column of its own, so its start moves with the name's length and both its edges are ragged down the list. Balance comes from colour rather than alignment. The directory takes the same colour token as the row's window count — the muted rung of the text ramp, the role paths, counts and subtitles already take elsewhere in the picker — so it reads as a second-weight annotation on the name it follows, and the ragged edges do not register as misalignment. A right-anchored column would buy a scannable edge at the cost of a wide, arbitrary gap on every short-named row, and would make the two pieces read as separate columns rather than as one row about one session. This introduces no new colour token and no new convention; the selected row's own treatment one step brighter is the established pattern the sigil row reuses.
>
> Where colour is off entirely the row is unchanged — the same single space, no bracket, glyph or separator introduced to stand in for the weight. The established carve-out asks that *state* never be carried by colour alone; a directory annotates the name rather than reporting state, and a path is legible as a path by its own separators.
>
> The row already flexes the name against a fixed count slot, a fixed attached slot and a right margin, and already truncates; the directory takes whatever width remains after the name and the fixed slots, and applies the left-truncation above.
>
> A session carrying no recorded directory shows none beside its name — the slot is simply empty. The displayed value is never derived from a pane read, for the same reason the match is not: the row must show what the search actually matched against.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §6.1, §6.2

## open-with-forced-filter-3-3

### Task 3-3: Turn the directory column on for a search-opened picker and nowhere else

**Problem**: Task 3-2 built the column but nothing switches it on, and its scope is narrow by decision: the accounting is owed where a row can be present on the strength of a path — the picker a search opened — and nowhere else, so a `-f` launch and a plain `x` must render their rows exactly as they do today. The gate has two tempting wrong readings. Gating on the term (`searchTerm != ""`) would drop the column for the term-less form, which is explicitly in scope. Gating on the containment filter's own test would drop it the moment the user edits the filter text — but a hand edit returns the *matching rule* to the picker's own without taking the column with it, because the rows can still be there on the strength of their directory. The delegate is also rebuilt constantly — every restyle, every marked-set mutation, every regroup — so the gate has to live where every one of those rebuilds reads it, or the column flickers out on the first `s` press.

**Solution**: One field assignment at the model's single delegate construction point, driven by the invocation-scoped `searchForm` flag the Phase 1 landing sets and never clears.

**Outcome**: A picker opened by `portal open /port` — or by `portal open /` — renders every session row with its recorded directory in Flat, By Project and By Tag, and keeps rendering it across a regroup, a preview and back, a sessions refresh, a theme swap and any edit of the filter text; a `-f` picker and a no-argument picker render byte-identically to today.

**Do**:
- Set `ShowDir: m.searchForm` in `m.sessionDelegate()` (`internal/tui/model.go:878`) — the single delegate construction point, which `refreshSessionDelegate` (`:890`) and `applyCanvasMode` (`:906`, via `applyPageListCanvasMode`) both route through, so a marked-set mutation, an `s` regroup, a preview dismissal, a sessions refresh and a live `ApplyTheme` each rebuild a delegate carrying the gate.
- Gate on `m.searchForm` alone: never on `m.searchTerm != ""`, and never on the containment filter's committed-text test.
- Change nothing else — the `HeaderItem` render arm, the grouping builders (which keep embedding the session verbatim), `m.derivedDirs` and the filter machinery are all untouched.
- Add `internal/tui/search_dir_column_test.go` (`package tui`), building models through `New(lister, WithSearchForm(...), ...)` and the `newRebuildTestModel` seams where a grouped rebuild is needed, and reading rendered rows through the `renderRow` harness or `m.sessionList`'s current delegate.

**Acceptance Criteria**:
- [ ] A model built with a search form renders each session row with its recorded directory, in Flat, By Project and By Tag
- [ ] The term-less search form carries the column — the gate is the form, not the term
- [ ] A `-f` picker, a no-argument picker and a command-pending picker render their session rows byte-identically to today (existing suites green unmodified)
- [ ] The column survives an `s` regroup, a `Space` preview and back, a `SessionsMsg` refresh, a marked-set mutation and a live theme swap
- [ ] The column survives a hand edit of the committed filter text, including clearing it
- [ ] A group header row renders no directory in either grouped mode
- [ ] A session whose directory is known only to grouping (recorded empty, derived present) shows no directory, and the column issues no pane read on any path
- [ ] A width narrow enough to left-truncate or drop the directory changes no row's membership in the list
- [ ] `go test ./...` passes

**Tests**:
- `"it renders the directory column in a search-opened picker"` — a term-carrying form, Flat
- `"it renders the column for the term-less search form"`
- `"it renders the column in By Project and By Tag"`
- `"it leaves a -f picker's rows unchanged"` and `"it leaves a no-filter picker's rows unchanged"`
- `"it keeps the column after an s regroup"` — drive the `s` key through `Update`
- `"it keeps the column after a preview and back"` — `Space` then dismiss
- `"it keeps the column after a sessions refresh"` — a second `SessionsMsg`
- `"it keeps the column after a live theme swap"` — `ApplyTheme`
- `"it keeps the column after the filter text is edited"` and `"… after the filter is cleared"`
- `"it renders no directory on a group header row"`
- `"it shows no directory for a session whose directory is known only to grouping"` — recorded empty, derived populated by a grouped rebuild
- `"it issues no pane read for the column"` — a recording dir-reader seam with zero calls in Flat
- `"it keeps every matching row in the list at a width that drops the directory"` — the visible set is identical at a wide and a narrow list width

**Edge Cases**:
- `searchForm` is invocation state that the Phase 1 landing never clears, which is exactly what makes the column last "for as long as that picker is open" without a second piece of state to keep in step
- Option ordering inside `New`'s loop is not load-bearing: an option that refreshes the delegate mid-loop (`WithInitialMultiSelect`) is superseded by the construction-time `applyCanvasMode`, but the flag must still be set during construction rather than after, or the first frame paints a delegate without it
- The delegate is a value copied into the list, so the gate is re-read on every rebuild rather than cached — which is why the single construction point is the whole change
- The grouped modes reach the same recorded value the Flat mode does: grouping reads the *derived* directory for group membership only, and the item it emits still carries the recorded one verbatim (task 1-1), so a regroup can never put a path beside a name that showed none
- The picker reached any other way keeps rendering rows without the column even though the widened match domain can surface a row there on the strength of a directory no row displays — an accepted cost of the shared match fields, not a gap to close here
- Row membership is decided by the filter value, which is computed before any row is laid out, so no width can remove a row; the width decides only how much of the directory is legible

**Context**:
> Every row in a sigil-opened list shows its recorded directory beside the session name — the rows found by their directory and the rows found by their name alike, so the column is uniform rather than a signal in itself. In every grouping mode, not only Flat: the picker reopens in whichever session-list grouping mode the user last left it in, so the sigil's narrowed list lands in Flat, By Project or By Tag depending on that history. By Project does not carry the information by another route — the grouping survives a committed filter but the headings do not, because a header row's filter value is empty, so a filtered By-Project list shows grouped-indented names with neither heading nor directory.
>
> Scoped to the picker session a sigil opened — the term-less form included — across every grouping mode that list can be in, and for as long as that picker is open. A hand edit of the filter text returns the matching rule to the picker's own but does not take the column with it: the rows can still be present on the strength of their directory, so the accounting is still owed. How Sessions rows render when the picker is reached any other way is untouched.
>
> A session carries the recorded directory and the grouping-derived one as separate values. Neither the match nor the directory column ever reads the derived one, so a regroup can never make a session findable by a path it was not findable by a moment earlier, and can never put a path beside a name that showed none.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §4.1, §6.1, §6.3

## open-with-forced-filter-3-4

### Task 3-4: Capture the search-opened list as a fixture and check the rendered frame

**Problem**: The directory column is this feature's only visual change, and Portal cannot be run from a scratch build to look at one — a scratch build disturbs the running daemon and its bootstrap touches real state. The offline harness (`cmd/capturetool` over `internal/capture`) is the only route to seeing the screen, and it has no fixture that opens the picker through a search form, so the column is unreachable there. Coverage depends on it too: the swap-and-diff completeness guard renders whatever fixtures exist and names none, so a screen with no fixture is a screen the guard quietly does not check. The capture carries one trap of its own — the fixture data lives under `/home/user/…`, which `AbbreviateHome` will not abbreviate on a machine whose home is `/Users/leeovery`, so an un-pinned `$HOME` renders absolute paths on one machine and `~/…` on another, and the frame the human judges would not be the frame the specification describes.

**Solution**: One registry fixture that builds a search-opened picker over session data exercising every branch of the column at once, a Go assertion over its rendered frame with `$HOME` pinned, and a `vhs` tape (plus a colourless variant) that captures it for the human visual check.

**Outcome**: `capture.FixtureNames()` carries `sessions-search-results`; it resolves, is enumerated by the coverage assertion with no exemption, and its frame shows a `~/`-abbreviated path, a row matched only by its directory, a row with an empty directory slot and a left-truncated `…/` tail — checked by eye against the specified placement, weight and truncation before sign-off.

**Do**:
- Add a `search *tui.SearchForm` field to `Fixture` (`internal/capture/fixtures.go`) and pass it through as `Search: f.search` in `Deps`, nil for every existing fixture.
- Add `sessionsSearchResultsFixture()` named `sessions-search-results`, registered in `fixtureBuilders()`, declaring **no** render size (the geometry-fixture assertion requires it to render at the caller's), `initialMode: prefs.ModeFlat`, and `search: &tui.SearchForm{Term: "port"}`. Seed it with sessions covering every branch in one frame — a name match with a short home path (`portal-a1b2`, `/home/user/code/portal`), a **directory-only** match (`api-work`, `/home/user/code/portal-gateway`), a match carrying **no** recorded directory (`legacy-port-shim`, `Dir: ""`), a deep path long enough to **left-truncate** (`portal-design-exports-review`, `/home/user/code/portal/internal/capture/testdata/reference/design-exports/frames`), a path **outside** home that stays unabbreviated (`portal-notes`, `/opt/portal-tools`), and one session matching neither field (`evvi-sync-engine`, `/home/user/code/evvi`) so the narrowing is visible by its absence. Give it a small project store (with tags) so `s` reaches By Project and By Tag live.
- Add `TestSessionsSearchResultsFixture` and a `FixtureNames` inclusion assertion to `internal/capture/capture_test.go`, following the `fx.ModelAt(…)` → `ansi.Strip(m.View().Content)` idiom, with `t.Setenv("HOME", "/home/user")` so the abbreviated forms are what the assertions see.
- Write `testdata/vhs/sessions-search-results.tape` modelled on the existing tapes: `Set FontFamily "JetBrains Mono"`, `Set FontSize 16`, fixed `Width`/`Height` with the resulting column/row count recorded in a comment, `Set Shell "bash"`. **Build the tool first and pin `$HOME` only on the run** — `go build -o /tmp/portal-capturetool ./cmd/capturetool` then `HOME=/home/user /tmp/portal-capturetool --fixture sessions-search-results --theme tokyo-night` — then `Screenshot "testdata/vhs/sessions-search-results.png"`. Add a second tape for the colour-off row that runs the same binary with `HOME=/home/user NO_COLOR=1` and screenshots `sessions-search-results-nocolor.png`; do **not** add a second fixture carrying the `noColor` field, which would exclude it from the swap guard's coverage.
- Hash the PNG before and after each `vhs` run and confirm it changed before trusting or reviewing it; run `vhs` with the sandbox disabled (it needs loopback networking).
- Present the rendered frame inline at the gate alongside the live-view command — `go build -o /tmp/portal-capturetool ./cmd/capturetool && HOME=/home/user /tmp/portal-capturetool --fixture sessions-search-results` — and check it against the specification's placement (one space after the name), weight (the count's muted rung, one step brighter on the selected row) and truncation (`…/` tail, name never giving way, empty slot for the directory-less row).

**Acceptance Criteria**:
- [ ] `sessions-search-results` is enumerated by `capture.FixtureNames()`, resolves through `FixtureByName`, and is covered by the swap-and-diff guard and every registry assertion with no exemption anywhere
- [ ] The fixture declares no render size, so `TestFixtureRenderSize_DeclaredOnlyByTheGeometryFixtures` passes over it unchanged
- [ ] The model it builds lands on the Sessions page with the term committed, and its visible rows are the containment set — the non-matching session is absent from the frame
- [ ] With `HOME=/home/user` the rendered frame carries, in one screen: a `~/code/…` abbreviated path, a row whose name does not contain the term while its directory does, a row showing a name with an empty directory slot, an unabbreviated `/opt/…` path, and a `…/`-prefixed left-truncated tail
- [ ] The directory on every row begins exactly one column after its session name, and the count and attached slots stay aligned down the frame
- [ ] `s` reaches By Project and By Tag live on the same fixture, with the column present in both — no second fixture is added for them
- [ ] A `NO_COLOR` capture of the same fixture shows the directory with no bracket, glyph or separator standing in for the weight
- [ ] The tape(s) and PNG(s) are committed with this task; `testdata/vhs/reference/*.png` is untouched
- [ ] The human visual check is recorded before sign-off, with the still capture and the live-view command presented
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it enumerates the search-results fixture"` — `FixtureNames` inclusion plus `FixtureByName` resolution
- `"it builds a search-opened model landing on the Sessions page with the term committed"`
- `"it narrows the list to the containment set"` — the non-matching session absent from the frame
- `"it renders a home-abbreviated directory beside the session name"` — `t.Setenv("HOME", "/home/user")`
- `"it renders a row matched only by its recorded directory"`
- `"it renders an empty directory slot for a session carrying none"` — the name present, no path on that row
- `"it leaves a directory outside the home directory unabbreviated"`
- `"it left-truncates the longest path in the frame"` — the row carries a `…/` tail at the harness width
- `"it renders the column in By Project and By Tag"` — replay `s` through the fixture's model
- `"it declares no render size"` — the frame is the caller's dimensions

**Edge Cases**:
- `go run` cannot be used under a pinned `$HOME`: Go resolves `GOCACHE` and `GOPATH` from it, so `HOME=/home/user go run ./cmd/capturetool` fails to initialise the build cache. Build with the real home, run the built binary with the pinned one — that split is why the tape has two typed lines
- The Go tests render the fixture under the developer's real `$HOME`, where `/home/user/…` is not abbreviated; any assertion on a `~/` form must set `HOME` itself, and the swap guard (which scans colour runs, not text) is unaffected either way
- The deep path must still left-truncate at **both** the Go harness width (120×40) and the tape's terminal width — verify in the rendered output and lengthen the fixture path rather than assuming; a frame with no visible `…/` tail leaves the truncation rung uncaptured
- The fixture deliberately carries one session with an empty `Dir`, against the file's existing "every session carries a stamped Dir" convention: the dir-resolution seams are nil in `Deps`, so a grouped rebuild resolves nothing and routes that session to the catch-all rather than issuing a pane read the harness has no server for
- The colourless capture is driven by the environment rather than by a fixture flag, because `Fixture.Colourless()` excludes a flagged fixture from the swap guard — the harness already honours `NO_COLOR` at render time
- `vhs` runs a tape, reports no error and silently writes no PNG; a stale or absent image reads as "the change didn't render" or, worse, as a false pass against a previous capture
- No design reference frame exists for this screen — `testdata/vhs/reference/` holds no directory-column export — so the gate is judged against the specification's own placement, weight and truncation rules, with `filtering-list-active-mv.png` as the nearest existing frame of a filtered sessions list for chrome comparison only
- The tape and PNGs are scaffolding cleared at the **feature's** sign-off; the Go fixture is permanent, and deleting one silently shrinks the completeness guard rather than failing it

**Context**:
> Every row in a sigil-opened list shows its recorded directory beside the session name — the rows found by their directory and the rows found by their name alike, so the column is uniform rather than a signal in itself. Matching on the recorded directory means `/port` can return a row the user cannot account for: a session named `api-work` appears because it lives in the Portal checkout, while the row shows only a name, a window count and an attached marker.
>
> The directory begins one space after the session name and takes the same colour token as the row's window count. When the row is too narrow for both, the directory gives way and the session name never does, shortened from the left so its tail survives; below a floor it is dropped rather than truncated to noise. A path under the user's home is always displayed abbreviated to `~/`, at any width.
>
> The capture harness renders a named deterministic fixture of the real TUI through the shared `tui.Build` constructor with every tmux seam faked — it opens no tmux server and reads no real config. Determinism is load-bearing, which is what makes an un-pinned `$HOME` a defect in the capture rather than a cosmetic difference: the same fixture would render two different frames on two machines.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §6.1, §6.2, §6.3
