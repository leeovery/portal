---
phase: 5
phase_name: Multi-Select TUI Mode
total: 8
---

## restore-host-terminal-windows-5-1 | approved

### Task 5.1: Multi-select mode state machine + session-identity selection set

**Problem**: The Sessions page has no way to mark more than one session. The whole feature turns on an explicit **multi-select mode** entered with `m` — a real mode you can sit in with zero selected — where `m` again toggles the cursor row's *session* in/out of a selection set. That set must be keyed on **session identity (name)**, not the list row, because By-Tag renders a multi-tag session as several rows (Pattern B) and marking any one row must mark the underlying session exactly once. Today `updateSessionList` has no mode flag, no selection set, and no `m` binding.

**Solution**: Add the mode + selection state to `Model` (`internal/tui/model.go`) — a `multiSelectMode bool` and a `selectedSessions map[string]struct{}` keyed on `tmux.Session.Name` — plus a `handleMultiSelectToggle` dispatched from a new `case isRuneKey(msg, "m")` arm in `updateSessionList`, an `Esc` branch that exits-and-clears, and a `m`-labelled help-only entry in `sessionsKeymap()` (`internal/tui/keymap.go`) with a matching probe in `keymap_dispatch_guard_test.go`. First `m` enters the mode with an empty set (no mark); subsequent `m` toggles the cursor row's `Session.Name`; `Esc` (when the filter is not focused) exits the mode and drops the whole set.

**Outcome**: From the normal Sessions list, `m` flips `multiSelectMode` true with `selectedSessions` empty; a second `m` on a session row adds that session's name; a third `m` on the same row removes it (idempotent pair); `m` while the cursor is on a `HeaderItem` (or when no session row is selectable) is a no-op; toggling any one row of a multi-tag By-Tag session adds/removes the single underlying name (so the set never double-counts); `Esc` exits the mode and empties the set. Uppercase `M` does nothing. All verified as Bubble Tea model unit tests.

**Do**:
- In `internal/tui/model.go`, add to `Model`:
  - `multiSelectMode bool` — true while the mode is active.
  - `selectedSessions map[string]struct{}` — the selection **set keyed on `Session.Name`** (the same identity `selectedSessionItem`/attach key on, per the `session_item.go` doc: "Selection and attach key on `Session.Name` … every view of a session resolves to the same underlying target"). Lazily initialise on first insert so a zero-value model needs no constructor change.
  - Test accessors (mirroring the existing `SessionList*` accessor convention): `func (m Model) MultiSelectActive() bool`, `func (m Model) IsSessionSelected(name string) bool`, and `func (m Model) SelectedSessionCount() int` (returns `len(m.selectedSessions)`). These let 5.1 assert set membership without a render.
- Add `func (m Model) handleMultiSelectToggle() (tea.Model, tea.Cmd)`:
  - If `!m.multiSelectMode`: set `m.multiSelectMode = true`, ensure `m.selectedSessions` is a fresh empty map, return `m, nil` (enter-only, **no mark** — "not an implicit mark-on-entry").
  - Else: resolve the cursor row via `m.selectedSessionItem()` (already returns `(SessionItem, ok)` and yields `ok=false` for a `HeaderItem` / nil). On `!ok` return `m, nil` (no-op — a `HeaderItem` row can't be marked). Otherwise toggle `si.Session.Name` in the set (delete if present, insert if absent) and return `m, nil`.
- Add `func (m Model) exitMultiSelect() Model` (or inline) that sets `multiSelectMode = false` and clears the set (`m.selectedSessions = nil`). Used by the `Esc` branch here and reused by 5.7's N=0 commit.
- Wire dispatch in `updateSessionList` (`internal/tui/model.go`):
  - Add `case isRuneKey(msg, "m"): return m.handleMultiSelectToggle()` **inside** the rune `switch` that sits below the `if m.sessionList.SettingFilter() { break }` guard — so `m` is a literal filter character while the `/` input is focused (the guard already delivers that; do not hoist the case above it).
  - In the existing `keyIsCode(msg, tea.KeyEscape)` case, add a leading branch: `if m.multiSelectMode { m = m.exitMultiSelect(); return m, nil }` (placed before the `FilterApplied` progressive-back check, so Esc exits the mode rather than quitting/clearing-filter). Reachable only when the filter is not focused (the `SettingFilter` guard breaks out earlier), satisfying "multi-select Esc applies only when the filter is not focused".
- In `internal/tui/keymap.go` `sessionsKeymap()`, add a **help-only** (non-`Core`) entry so the mode is discoverable in the `?` help modal but stays out of the condensed footer (the delivered normal-Sessions footer has no `m`): `{Key: "m", Action: "multi-select", HelpAction: "Multi-select mode"}`. Place it in descriptor order near the other action keys (e.g. after `s`).
- In `internal/tui/keymap_dispatch_guard_test.go` `TestSessionsDescriptorDispatchParity`, add a probe `"m": {press: tea.KeyPressMsg{Code: 'm', Text: "m"}, honour: func(t) bool { … }}` — press `m` on a fresh Sessions model and assert `MultiSelectActive()` became true (the bound effect). This keeps descriptor↔dispatch parity green (Direction 1: descriptor `m` is honoured; Direction 2: the probed `m` is in the descriptor).
- If the `?` help-modal body is byte-asserted (`help_modal_frame_test.go` / `help_modal_test.go`), update the expected body to include the new `m multi-select` row.

**Acceptance Criteria**:
- [ ] `m` from the normal list sets `MultiSelectActive()==true` with `SelectedSessionCount()==0` (enter with zero selected, no implicit mark).
- [ ] A second `m` on a session row sets `IsSessionSelected(name)==true` and `SelectedSessionCount()==1`; a third `m` on the same row returns it to unselected with count 0 (idempotent toggle pair).
- [ ] In By-Tag mode, toggling any one row of a multi-tag session marks/unmarks the single underlying `Session.Name` — `SelectedSessionCount()` changes by exactly 1 regardless of how many rows that session spans.
- [ ] `m` while the selected item is a `HeaderItem` (or no selectable session row exists) leaves the set unchanged (no-op).
- [ ] `Esc` (filter not focused) sets `MultiSelectActive()==false` and `SelectedSessionCount()==0` (exit and clear the whole set).
- [ ] Uppercase `M` (Text `"M"`) is a no-op: it neither enters the mode nor toggles a mark.
- [ ] `keymap_dispatch_guard_test.go` passes with the new `m` descriptor entry + probe (no descriptor↔dispatch drift).

**Tests** (Bubble Tea model unit tests in `internal/tui`, no `t.Parallel()`):
- `"it enters multi-select mode with zero selected on first m"`
- `"it toggles the cursor session on the second m and untoggles on a third (idempotent pair)"`
- `"it marks a multi-tag By-Tag session once when toggled via any single row"`
- `"it is a no-op when m is pressed with a HeaderItem selected"`
- `"it exits the mode and clears the whole set on Esc"`
- `"it ignores uppercase M (stays retired)"`
- `"it keeps sessionsKeymap descriptor and updateSessionList dispatch in parity for m"`

**Edge Cases**:
- Enter mode with zero selected (first `m` marks nothing).
- Toggle the same row twice (idempotent add/remove pair returns to unmarked).
- Multi-tag By-Tag session toggled via one row marks the underlying session once (set keyed on name).
- `m` on a `HeaderItem` row is a no-op (`selectedSessionItem` returns `ok=false`; the `skipHeaderRow` invariant already keeps the cursor off headers, this is the defensive backstop).
- `Esc` exits and clears the whole set.
- Uppercase `M` stays retired (no binding — `isRuneKey` matches Text `"m"` only).

**Context**:
> Spec *Multi-Select Mode → Trigger & marking*: "`m` enters an explicit multi-select mode from the normal Sessions list. It is a real mode you can sit in with **zero selected** — not an implicit mark-on-entry. `M` (uppercase) stays retired… `m` again toggles the cursor (highlighted) row in/out of the selection. The same key both enters the mode and toggles marks — no second key… `Esc` = exit mode and clear selection… Grouping `HeaderItem` rows are non-selectable and skipped by marking/navigation (existing `skipHeaderRow` invariant)."
> Spec *Multi-Select Mode → Granularity*: "**Selection is keyed on session identity, not the list row.** By Tag mode renders a multi-tag session as multiple rows (one per tag heading — Pattern B). Marking is on the **underlying session**: `m`-toggling any one of its rows marks the *session*… The selection model is a set of session identities, so a mark survives regroup/filter/paging even though the session spans multiple list items."
> `session_item.go` doc: "Selection and attach key on `Session.Name` (task 2-6), so every view of a session resolves to the same underlying target." This is why the set keys on `Session.Name`, not on the `SessionItem`/`GroupKey`.
> Scope boundary: the N≥2 spawn burst, detection, and the pre-flight prune are Phase 6 — this task only establishes the mode + set. The `m` dispatch sits below the `SettingFilter` break so `m` is a literal filter char while filtering (the filter-focus routing is fully covered in 5.5).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode (TUI Interaction) → Trigger & marking / Granularity: per-session only*; *Testing Strategy & DI Seams → Mode/keymap state machine*.

---

## restore-host-terminal-windows-5-2 | approved

### Task 5.2: `●` selection markers on session rows

**Problem**: In multi-select mode the marked rows must be **unmistakably distinct**, glyph-backed (never colour-only, per MV's NO_COLOR rule). The delivered Paper frame shows a violet `●` in the far-left column of every selected row — including the cursor row, which also carries the `bg.selection` highlight band. The `SessionDelegate` (`internal/tui/session_item.go`) currently renders only the `▌` selector on the cursor row and has no knowledge of the multi-select set.

**Solution**: Teach `SessionDelegate` about the multi-select selection: add fields (`MultiSelect bool` + `Selected map[string]struct{}`) propagated from the model in `applyCanvasMode` (`internal/tui/model.go`), and render a violet `●` in the fixed left-bar column for any `SessionItem` whose `Session.Name` is in the set. The `●` replaces the `▌` in that column for a selected row; the cursor row keeps its `bg.selection` band underneath, so a selected cursor row shows the band **and** the `●`. Re-apply the delegate on every toggle so the marker tracks the set live.

**Outcome**: With three sessions marked (matching the frame), every marked `SessionItem` row renders a violet `●` at the far-left column; a multi-tag By-Tag session shows the `●` on **all** of its rows; the cursor row that is also marked shows the `bg.selection` band together with the `●`; `HeaderItem` rows never carry a `●`; when `multiSelectMode` is false no row carries a `●`; under NO_COLOR the `●` glyph survives on the terminal's native fg/bg while the violet hue drops.

**Do**:
- In `internal/tui/session_item.go`:
  - Add a `multiSelectMarker = "●"` const near `selectorBar` (reuse the exact glyph the frame + spec pin — `●`, U+25CF, the same bullet the attached badge uses).
  - Add fields to `SessionDelegate`: `MultiSelect bool` and `Selected map[string]struct{}` (the model's selection set; nil-tolerant — a nil map means nothing selected).
  - In `renderSessionRow`, compute `marked := d.MultiSelect && isSelected(d.Selected, it.Session.Name)`. Render the left-bar column so that:
    - `marked` → the violet `●` at col 0 + the trailing blank cell (same 2-cell `leftBarColumnWidth` geometry as the `▌` selector), painted through `d.rowToken(lipgloss.Style{}, theme.MV.AccentViolet, selected)` so it carries the row's `bg.selection` tint on a selected row and the canvas otherwise. The `●` takes precedence over the `▌` selector in this column (a selected+marked cursor row shows `●`, matching the frame's `fab-flowx-explore`).
    - not `marked` → the existing `renderLeftBarColumn` behaviour (violet `▌` on the cursor row, two blank cells otherwise).
  - Keep the marker inside the existing fixed 2-cell left-bar column so the name's left edge and all trailing-slot alignment are unchanged (no new column, no width-budget change — the row stays exactly one delegate line, preserving the §3.5 pagination invariant).
  - Leave the `HeaderItem` render arm untouched — headers never get a marker.
- In `internal/tui/model.go` `applyCanvasMode` (both the colourless and coloured `SetDelegate` branches), set the new delegate fields from the model: `SessionDelegate{Mode: m.canvasMode, Colourless: …, MultiSelect: m.multiSelectMode, Selected: m.selectedSessions}`. This is the single delegate-construction chokepoint, so every rebuild/refresh already re-propagates the current set.
- Ensure a `m`-toggle re-renders the marker: after `handleMultiSelectToggle` mutates the set (5.1), call the delegate refresh (invoke `(&m).applyCanvasMode()` — or a narrower `(&m).refreshSessionDelegate()` helper that only re-sets the delegate) before returning, so the `●` reflects the new set on the next frame. (Entering the mode with `m` and exiting with `Esc` must likewise refresh so markers appear/vanish.)

**Acceptance Criteria**:
- [ ] A marked `SessionItem` row renders `●` in the left-bar column in `accent.violet` (dark canvas); an unmarked row does not.
- [ ] In By-Tag mode a multi-tag session that is marked shows `●` on **every** one of its rows (all rows share the same `Session.Name`).
- [ ] A row that is both the cursor and marked renders the `bg.selection` band **and** the `●` (the `●` occupies the left-bar column, the band spans the row).
- [ ] `HeaderItem` rows never render a `●`.
- [ ] With `multiSelectMode == false` no row renders a `●` (the marker is gated on `MultiSelect`).
- [ ] Under NO_COLOR (`Colourless == true`) the `●` glyph renders (structurally present) with no violet hue and no canvas/selection background — glyph-backed, never colour-only.
- [ ] The row remains exactly one delegate line and the name/count/attached column alignment is byte-unchanged from the non-marked row (no width-budget shift).

**Tests** (unit tests in `internal/tui`, asserting the rendered row string — mirror `session_item_test.go` / `session_row_anatomy_test.go` patterns):
- `"it renders a violet ● on a marked session row"`
- `"it renders ● on every row of a marked multi-tag By-Tag session"`
- `"it renders the selection band and ● together on a marked cursor row"`
- `"it never renders ● on a HeaderItem row"`
- `"it renders no ● when not in multi-select mode"`
- `"it keeps the ● glyph and drops the violet hue under NO_COLOR"`

**Edge Cases**:
- `●` on every row of a multi-tag By-Tag session (set keyed on name → all instances marked).
- NO_COLOR: glyph survives, violet hue + background drop (the shared `rowToken`/`rowBgStyle` colourless branches already return bare styles).
- Cursor row also marked → shows the `bg.selection` band + `●` (marker in the left-bar column, band across the row).
- `HeaderItem` rows never carry `●`.
- No `●` at all when `multiSelectMode` is false.

**Context**:
> Spec *Multi-Select Mode → Mode affordance (visual)*: "**Selected rows carry a glyph marker + the mode colour, never colour-only** (MV's NO_COLOR / colourless-render rule)… **violet** reused as the selection accent, `●` marker on selected rows… No new colour tokens."
> Spec *Design References*: "Sessions — Multi-Select (active) — `design/sessions-multi-select-active.png`. Violet `3 selected` banner…; violet `●` markers on selected rows **incl. the cursor row**."
> The delivered frame (`design/sessions-multi-select-active.png`) shows `●` at the far-left column on `agentic-workflows-codify`, `fab-flowx-explore` (the cursor row, also banded), and `designlab-web-r8suyU` — the marker sits in the same 2-cell column the `▌` selector uses, so the name left edge is unchanged.
> The existing `rowTokenStyle`/`rowBgStyle` free functions already encode the NO_COLOR carve-out (bare style under `colourless`) and the selected-vs-canvas background role — route the `●` through `d.rowToken`/`d.rowBg` so the marker inherits both rules with no new colour decision.
> `AccentViolet` is the only accent used — no new colour token (per scope boundary).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → Mode affordance (visual)*; *Design References → Sessions — Multi-Select (active) / Tokens*.

---

## restore-host-terminal-windows-5-3 | approved

### Task 5.3: Multi-select banner + notice-band single-slot precedence

**Problem**: Multi-select must read as unmistakably a distinct mode, modelled on filter mode: while in the mode the header region shows an `N selected` banner (the filter-line analogue). The banner must count **distinct sessions** (a multi-tag marked session counts once), update live as marks toggle, show `0 selected` at N=0, and it must arbitrate correctly against the other header-region claimants (the focused filter line, the transient flash, and the no-tags signpost) so exactly one thing owns the slot.

**Solution**: Render the multi-select banner as a **section-header-row variant** (like the FilterApplied query header) — violet `N selected` on the left, a right-aligned dim `esc cancel` — via a new `renderMultiSelectHeader` invoked from `applySectionHeader` (`internal/tui/model.go`), routed through the existing `renderSectionHeaderRow` right-anchor geometry (`internal/tui/section_header.go`). Handle precedence so: filter-focused → the orange filter input owns the row (banner steps aside); otherwise in-mode → the banner owns the row; and the no-tags signpost is **suppressed** while in multi-select mode (`activeNoticeBand` in `internal/tui/notice_band.go`), so the banner is the sole mode indicator. The count comes from `len(m.selectedSessions)`.

**Outcome**: In multi-select mode with three distinct sessions marked, the section-header row reads `3 selected` (violet) left + `esc cancel` (dim) right — no `Sessions ··· N` label; a multi-tag session marked via multiple rows still contributes `1`; at N=0 the row reads `0 selected`; each `m` toggle updates the count live; when the filter input is focused the banner steps aside and the orange filter line shows; while in mode the By-Tag "No tags yet" signpost does not render (the banner owns the mode indication); a transient flash still renders in the notice band above.

**Do**:
- In `internal/tui/section_header.go` add `func renderMultiSelectHeader(count, width int, mode theme.Mode, colourless bool) string`:
  - Left cluster: `fmt.Sprintf("%d selected", count)` rendered in `theme.MV.AccentViolet` (the violet mode accent), over the owned canvas via `headerStyle`.
  - Right hint: `"esc cancel"` in `theme.MV.TextDetail` (the dim chrome token — match the frame's grey `esc cancel`; verify the exact token against the frame at the 5.8 visual gate).
  - Compose them through the **shared** right-anchored section-header assembler (`renderSectionHeaderRow` / `assembleRightAnchoredRow`) so the banner's right-alignment, canvas-painted flex spacer, and §2.7 narrow-degrade match the standard section header exactly. Under NO_COLOR every hue + the canvas drop; the text survives.
  - The banner occupies exactly one row (line 0 of `listView`), so the one-row-per-delegate pagination budget is unchanged (it replaces the section header, it does not add a row).
- In `internal/tui/model.go` `applySectionHeader`, insert the mode branch (precedence order at line 0):
  1. `FilterState() == list.Filtering` → return `listView` untouched (the live filter input owns the row; banner steps aside).
  2. else if `m.multiSelectMode` → swap the first line for `renderMultiSelectHeader(len(m.selectedSessions), m.contentWidth(), m.canvasMode, m.colourless)` (same first-line-replacement mechanism the FilterApplied branch uses: find the first `'\n'`, keep the body).
  3. else if `FilterState() == list.FilterApplied` → the existing query header.
  4. else → the existing `renderSectionHeader`.
  (So a committed/applied filter while in multi-select shows the banner, not the query header — consistent with the spec's "otherwise → the multi-select banner".)
- In `internal/tui/notice_band.go` `activeNoticeBand`, suppress the persistent signpost while in the mode: change the `byTagSignpost` arm to `if m.byTagSignpost && !m.multiSelectMode { return bandInfo, byTagSignpostText, true }`. The transient flash arm stays first (unchanged) so a flash still wins the notice band. This delivers "banner outranks the no-tags signpost" and "transient flash outranks the banner" (the flash remains the loudest, always-shown notice band above the section-header row).
- Do **not** route the banner through `renderNoticeBand`'s `▌`-barred band — the delivered frame shows the banner as a bare `N selected` / `esc cancel` section-header analogue with **no** `▌` left bar. (The banner render is distinct from the `▌` notice band; the notice band continues to host the flash/signpost.)

**Acceptance Criteria**:
- [ ] In multi-select mode with N distinct marked sessions the section-header row reads `N selected` (violet) with a right-aligned `esc cancel` (dim), and the `Sessions ··· N` section header is not shown.
- [ ] A multi-tag session marked via several rows contributes exactly `1` to the count (distinct-session count via `len(m.selectedSessions)`).
- [ ] N=0 in mode renders `0 selected` (the banner shows even with an empty set).
- [ ] The count updates live: each `m` toggle changes the rendered number by exactly 1.
- [ ] While the filter input is focused (`Filtering`) the banner steps aside and the orange filter line owns the row.
- [ ] While in multi-select mode the By-Tag "No tags yet" signpost is suppressed (does not render).
- [ ] A transient flash (`flashText != ""`) still renders in the notice band while in the mode (flash outranks the banner).
- [ ] The banner is exactly one row and does not perturb the list height budget (no `Sessions ··· N` row plus banner — the banner replaces it).

**Tests** (unit tests in `internal/tui`; assert the composed `viewSessionList` / section-header substring):
- `"it renders N selected in violet with a right-aligned esc cancel while in mode"`
- `"it counts a multi-tag marked session once in the banner"`
- `"it renders 0 selected when in mode with nothing marked"`
- `"it updates the banner count live as marks toggle"`
- `"it shows the orange filter line and hides the banner while the filter input is focused"`
- `"it suppresses the no-tags signpost while in multi-select mode"`
- `"it keeps a transient flash visible in the notice band while in mode"`

**Edge Cases**:
- Counts distinct sessions once (multi-tag marked = 1 selected).
- Transient flash outranks the banner (flash stays in the notice band; banner still shows the count at the section-header row).
- Banner outranks the no-tags signpost (signpost suppressed in mode).
- Filter-focused → filter line owns the slot (banner steps aside; `Filtering` state).
- N=0 in-mode still shows `0 selected`.
- Count updates live on toggle.

**Context**:
> Spec *Multi-Select Mode → Mode affordance (visual)*: "Its own **mode colour** + a **banner**… (single-slot arbiter — the multi-select banner owns the slot while in mode)… **Notice-band precedence** (single slot, highest wins): filter line (filter focused) → in-burst `Opening n/N…` (burst pending) → transient error/guidance flash → multi-select banner (in mode) → unsupported-terminal banner → no-tags signpost."
> Spec *Multi-Select Mode → Granularity*: "the `N selected` banner counts **distinct sessions** (a multi-tag session counts once)."
> Spec *Design References*: "Violet `3 selected` banner (**filter-line analogue**)."
> **Placement decision (design-anchored, ambiguity flagged):** the delivered frame (`design/sessions-multi-select-active.png`) renders the banner at the **section-header row** as a filter-line analogue — violet `3 selected` left + `esc cancel` right, with **no** `▌` left bar — not through the `▌`-barred notice band. The spec's abstract "banner in the notice-band slot" language predates the delivered design; the golden frame governs placement (it shows exactly one row between the header rule and the list, replacing `Sessions ··· N`). Because the in-flight `Opening n/N…` and unsupported-terminal claimants are Phase 6, this task implements only the Phase-5 subset of the precedence: filter-focused filter line > transient flash (notice band, unchanged) > multi-select banner (section-header row) > no-tags signpost (suppressed in mode). The strict single-row collapse of flash-vs-banner is a Phase-6 concern (that is where pre-flight-abort/spawn-failure flashes originate).
> `renderFilterQueryHeader` (the FilterApplied section-header analogue) is the structural reference for the first-line swap; `renderSectionHeaderRow` is the shared right-anchor assembler to reuse so the banner's geometry matches the standard header.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → Mode affordance (visual) / Filter as an inner sub-state / Granularity*; *Design References*.

---

## restore-host-terminal-windows-5-4 | approved

### Task 5.4: Multi-select footer copy

**Problem**: The mode needs its own footer keymap, distinct from the standard Sessions footer and the filter footers. The delivered Paper frame fixes the copy exactly: `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel` — five entries, no right-aligned `? help` anchor. Nothing yet renders this footer or wires it into the footer-for-filter-state resolver.

**Solution**: Add a `renderMultiSelectFooter` (`internal/tui/footer.go`) that renders the five spec-exact entries as a dot-separated left cluster over the shared 1px `border.footer` top rule, with **no** right-aligned `? help` anchor (per the frame), reusing the filter-footer entry machinery (`filterFooterEntry` / `renderFilterCluster`) for per-glyph colouring. Wire it into `renderSessionsFooterForFilterState` (`internal/tui/model.go`) so that in multi-select mode and not filter-focused the multi-select footer renders; while the filter input is focused the filter footer renders instead.

**Outcome**: In multi-select mode (filter not focused) the footer reads exactly `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel` on one row over the footer rule, with no `? help` on the right; while the filter input is focused within the mode the input-active filter footer renders instead; the footer degrades gracefully at narrow widths (leading entries kept, trailing dropped with an ellipsis, one line, never wrapping); under NO_COLOR the glyphs survive and the hues drop.

**Do**:
- In `internal/tui/footer.go` add `func renderMultiSelectFooter(width int, mode theme.Mode, colourless bool) string`:
  - Build the entry list (spec-exact copy + glyphs, reusing the existing project key-glyph conventions): `↑↓` navigate, `m` toggle, `␣` preview (U+2423, the same space glyph `sessionsKeymap` uses for preview), `⏎` open (U+23CE), `esc` cancel. Key glyphs in `theme.MV.AccentBlue`, labels in `theme.MV.TextDetail` — the standard MV footer colour convention (verify exact per-glyph colour against the frame at the 5.8 visual gate).
  - Render as a left cluster over the shared 1px `border.footer` top rule (`footerTopRule`), with **no** right anchor: pass `rightSeg == ""` to `assembleRightAnchoredRow`, which pads the cluster to width via `headerPadRight` — so there is no `? help` hint (the delivered frame has none). Do **not** route through `renderFilterFooter`/`filterFooterRow` unchanged, since those hardcode the `sessionsKeymap` `? help` anchor.
  - Reuse `renderFilterCluster` (the `filterFooterEntry` per-glyph path) for the cluster body so the dot separators, canvas-painted gaps, and NO_COLOR carve-out match the other footers byte-for-byte.
  - Keep the footer exactly two rows (rule + entry row) so it is height-neutral against the reserved `sessionFooterHeight` budget (the swap must not change the list height).
- In `internal/tui/model.go` `renderSessionsFooterForFilterState`, insert the mode branch so it takes precedence over the standard footer but yields to the focused filter footer:
  - Keep the existing `sessionListNoMatches` / `sessionListEmpty` guards.
  - `case list.Filtering:` → `renderFilteringFooter` (filter focused; multi-select footer steps aside).
  - else if `m.multiSelectMode` → `renderMultiSelectFooter(...)` (covers both the unfiltered and the FilterApplied-in-mode states — consistent with 5.3's "otherwise → multi-select footer").
  - else `case list.FilterApplied:` → `renderFilterAppliedFooter`; `default:` → `renderSessionsFooter`.
- Author the copy as spec-exact constants where practical (mirroring how `commandBandText` is a single source of truth) so the wording can't drift from a paraphrase.

**Acceptance Criteria**:
- [ ] In multi-select mode (filter not focused) the footer entry row reads exactly `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`.
- [ ] The multi-select footer has **no** right-aligned `? help` anchor (matches the delivered frame).
- [ ] While the filter input is focused within the mode, the input-active filter footer renders (not the multi-select footer).
- [ ] The footer is two rows over the shared `border.footer` rule and is height-neutral (does not change the list's reserved height budget).
- [ ] At a narrow width the left cluster degrades on one line (leading entries kept, trailing dropped with an ellipsis) and never wraps to a second row.
- [ ] Under NO_COLOR the glyphs + labels render on the terminal's native fg/bg (hues + canvas drop) with no crash.

**Tests** (unit tests in `internal/tui`, mirror `footer_test.go` / `sessions_footer_switch_view_test.go`):
- `"it renders the exact multi-select footer copy in mode"`
- `"it renders no ? help anchor on the multi-select footer"`
- `"it renders the filter footer (not the multi-select footer) while the filter input is focused"`
- `"it degrades the multi-select footer to one line with an ellipsis at narrow width"`
- `"it keeps footer glyphs and drops hues under NO_COLOR"`
- `"it keeps the multi-select footer height-neutral against the standard footer"`

**Edge Cases**:
- Exact delivered copy from the Paper design (`↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`).
- Filter-focused within mode renders the filter footer instead.
- Narrow-width ellipsis degrade (one line, no wrap).
- NO_COLOR (glyphs survive, hue drops).

**Context**:
> Spec *Multi-Select Mode → Mode affordance (visual)*: "footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`. No new colour tokens."
> Spec *Design References → Sessions — Multi-Select (active)*: "footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`."
> The delivered frame (`design/sessions-multi-select-active.png`) shows these five entries and **no** `? help` on the right — so the multi-select footer drops the right anchor (unlike the standard/filter footers). Verify the exact per-glyph colour (blue keys / dim labels vs the frame) at the 5.8 visual gate; anchor on the frame, not a paraphrase.
> Reuse the existing footer plumbing: `footerTopRule` (the shared 1px `border.footer` rule), `renderFilterCluster` (per-glyph coloured left cluster), and `assembleRightAnchoredRow` with an empty right segment (the §2.7 narrow-degrade + width-pad path).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → Mode affordance (visual) / Filter as an inner sub-state*; *Design References*.

---

## restore-host-terminal-windows-5-5 | approved

### Task 5.5: Keymap coexistence + filter-focus key routing

**Problem**: Inside multi-select mode some existing Sessions keys must be **suppressed** (`k` kill, `x` page-toggle, `r` rename — row actions that don't compose with a marked set) while others stay **live** (`Space` preview, `/` filter, `s` regroup — you filter/regroup to find things to mark). The filter is an **inner sub-state**: while the `/` input is focused, `s` and `m` are literal filter characters and the filter input owns `Enter` (commit-to-browse) and `Esc` (clear-filter), so multi-select's `Enter`/`Esc` must not fire. `q`/`Ctrl+C` must still quit. Out of the mode, `k`/`x`/`r` must be unchanged.

**Solution**: Gate the suppressed row-action arms in `updateSessionList` (`internal/tui/model.go`) on `!m.multiSelectMode`, leaving `Space`/`/`/`s` (and their existing delegation) untouched, and rely on the existing `if m.sessionList.SettingFilter() { break }` guard — which already sits above the rune switch — to make `s`/`m` literal while filtering and to hand `Enter`/`Esc` to the list (commit / clear-filter). Route `Enter`/`Esc` to the multi-select handlers only when the filter is not focused.

**Outcome**: In multi-select mode `k`/`x`/`r` do nothing (no kill modal, no page switch, no rename modal); `Space` still opens the preview, `/` still starts filtering, `s` still cycles the grouping; while the filter input is focused `s`/`m` type into the query and `Enter`/`Esc` commit/clear the filter (multi-select `Enter`/`Esc` do not fire); `q` and `Ctrl+C` still quit from within the mode; and outside the mode `k`/`x`/`r` behave exactly as before.

**Do**:
- In `internal/tui/model.go` `updateSessionList`, within the rune `switch` (which is below the `SettingFilter` break):
  - Gate the suppressed arms on the mode. Change `case isRuneKey(msg, "k"):`, `case isRuneKey(msg, "x"):`, and `case isRuneKey(msg, "r"):` so that when `m.multiSelectMode` is true they are no-ops (`return m, nil`) and otherwise run their existing handlers. Concretely, either add a leading `if m.multiSelectMode { return m, nil }` inside each arm, or guard the arm condition. Keep the arms present (do not delete them) so `keymap_dispatch_guard_test.go`'s probes — which run against a **non-multi-select** model — still see them honoured.
  - Leave `case isRuneKey(msg, "s"):` (regroup), `case isRuneKey(msg, "m"):` (5.1 toggle), and the pre-switch `Space` handler unchanged — they stay live in the mode.
  - Route `Enter`: `case keyIsCode(msg, tea.KeyEnter):` → `if m.multiSelectMode { return m.handleMultiSelectEnter() }` (defined in 5.7) `else return m.handleSessionListEnter()`. This arm is reached only when the filter is not focused (the `SettingFilter` break hands `Enter` to the list to commit-to-browse), so a focused-filter `Enter` never triggers multi-select open.
  - `Esc` routing (5.1 already added the exit branch): confirm the `keyIsCode(msg, tea.KeyEscape)` case runs `exitMultiSelect` only when `m.multiSelectMode` — and, because that case sits below the `SettingFilter` break, a focused-filter `Esc` is delegated to the list (clear-filter) and never exits the mode.
  - Leave the early `keyIsCtrlC(msg)` → `tea.Quit` and the `isRuneKey(msg, "q")` → `tea.Quit` arms untouched: both must still quit regardless of the mode.
- `/` needs no explicit arm — it is delegated to `list.Update` (bubbles/list binds `/` to start filtering) and stays live in the mode; assert it in tests (pressing `/` in mode transitions `FilterState()` to `Filtering`).
- Do not alter the `sessionsKeymap` descriptor for the suppression (the descriptor drives display/parity against the **default** dispatch; the in-mode suppression is a runtime gate, not a descriptor change).

**Acceptance Criteria**:
- [ ] In multi-select mode, `k` opens no kill modal, `x` does not switch to Projects, and `r` opens no rename modal (all no-ops).
- [ ] In multi-select mode, `Space` opens the preview, `/` starts filtering (`FilterState()==Filtering`), and `s` cycles the grouping mode.
- [ ] While the filter input is focused, `s` and `m` type into the filter query (they do not regroup / toggle-mark).
- [ ] While the filter input is focused, `Enter` commits-to-browse and `Esc` clears the filter — multi-select's open/exit do not fire.
- [ ] `q` and `Ctrl+C` still quit from within multi-select mode.
- [ ] Out of the mode, `k`/`x`/`r` behave exactly as before (kill modal / Projects page / rename modal).
- [ ] `keymap_dispatch_guard_test.go` stays green (the suppressed arms are still honoured for the default-mode probes).

**Tests** (Bubble Tea model unit tests in `internal/tui`):
- `"it makes k, x, and r no-ops while in multi-select mode"`
- `"it keeps Space, /, and s live while in multi-select mode"`
- `"it treats s and m as literal filter characters while the filter input is focused"`
- `"it commits-to-browse on Enter and clears on Esc while the filter is focused (multi-select open/exit do not fire)"`
- `"it still quits on q and Ctrl+C from within multi-select mode"`
- `"it leaves k, x, and r unchanged outside multi-select mode"`

**Edge Cases**:
- `k`/`x`/`r` no-op in mode.
- `Space`/`/`/`s` stay live in mode.
- `s`/`m` literal while the filter is focused (the existing `SettingFilter` break handles this — verify it holds).
- Filter-focused `Enter` = commit-to-browse and `Esc` = clear-filter (multi-select `Enter`/`Esc` do not fire).
- `q`/`Ctrl+C` still quit.
- Out-of-mode `k`/`x`/`r` unchanged.

**Context**:
> Spec *Multi-Select Mode → Key coexistence within the mode*: "**Live in mode:** `Space` (preview…), `/` (filter), `s` (regroup)… **Suppressed in mode:** `k` (kill), `x` (page-toggle), `r` (rename), and other row actions. While the `/` filter is focused, `s` and `m` are literal filter characters (the filter input owns typing)."
> Spec *Multi-Select Mode → Filter as an inner sub-state*: "**The focused filter input owns `Enter`/`Esc`.** While the filter is focused it keeps its normal meaning (`⏎`/`↓` commit-to-browse, `Esc` clear-filter); multi-select's `⏎` (open-marked) and `Esc` (exit-mode) apply **only when the filter is not focused**."
> The `updateSessionList` layering already delivers most of this for free: the `if m.sessionList.SettingFilter() { break }` guard sits above the rune switch (so `s`/`m` are literal and `Enter`/`Esc` are delegated while filtering), `keyIsCtrlC` is handled before any mode logic, and `/` is delegated to `list.Update`. This task only adds the in-mode suppression of `k`/`x`/`r` and the `Enter` mode-branch. Keep the suppressed arms present so `keymap_dispatch_guard_test.go` (which probes the default model) stays green.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → Key coexistence within the mode / Filter as an inner sub-state*; *Testing Strategy & DI Seams → Mode/keymap state machine*.

---

## restore-host-terminal-windows-5-6 | approved

### Task 5.6: Sticky selection across filter/paging/regroup + Space-preview prune

**Problem**: Because selection is keyed on session identity, marks must be **sticky** across filtering, paging, regrouping, and the `Space`-preview round-trip — a filtered-out row stays marked and reappears when the filter clears; a marked session that moves buckets on an `s`-regroup stays marked. The one exception is a selection whose session was **externally killed** during the preview: it can't be opened, so it must be **pruned** (survivors kept), consistent with the pre-flight rule.

**Solution**: The selection set (a `map[string]struct{}` keyed on `Session.Name`, from 5.1) is model state independent of the list items, so it already survives filter/paging/regroup rebuilds — this task **verifies** that stickiness end-to-end and adds the one required mutation: prune the set to sessions still present in `m.sessions` whenever the sessions list is refreshed (the post-preview refresh path), and preserve `multiSelectMode` + the set across the `Space`-preview round-trip.

**Outcome**: Marks survive an `s`-regroup and paging (the `●` reappears on the same sessions, the banner count is unchanged); a row filtered out by a query stays in the set and its `●` reappears when the filter clears; entering the preview with `Space` and dismissing returns to the Sessions page still in multi-select mode with the selection intact and re-rendered; a session externally killed during the preview is dropped from the set on the post-dismiss refresh while every surviving mark is kept; a marked session that moves to a different bucket on regroup is still marked.

**Do**:
- Confirm the selection set is **not** cleared on any rebuild path: audit `rebuildSessionList`, `handleSwitchViewKey` (the `s`-regroup), the filter transitions, and `applySessions` (the `SessionsMsg` refresh) — none may reset `m.selectedSessions` or `m.multiSelectMode`. (They mutate `sessionList` items, not the set.) Add a regression test per path.
- Preserve the mode + set across the `Space`-preview round-trip: the `Space` handler (in `updateSessionList`) sets `activePage = pagePreview` and must leave `multiSelectMode`/`selectedSessions` untouched; the preview-dismiss transition (`pagePreview → pageSessions`) must return in-mode. Verify the dismiss handler does not reset either.
- Add the externally-killed prune. In `internal/tui/model.go`, in the sessions-refresh chokepoint (`applySessions`, which folds a fresh `SessionsMsg` from `refreshSessionsAfterPreviewCmd` after preview dismissal), after `m.sessions` is updated and before/with `rebuildSessionList`, prune the set to live names:
  - Build a set of current `m.sessions[i].Name`; delete from `m.selectedSessions` any name not present. This drops a session killed during the preview (it no longer appears in the refreshed list) and keeps every survivor — the same prune-what's-gone rule the pre-flight/preview round-trip specifies. Do this via a small helper `func (m *Model) pruneSelectionToLiveSessions()` so 5.7 / Phase 6 can reuse it.
  - Keep the prune silent (no flash) for the multi-select set — the spec's multi-select prune is "prune only a selection whose session was externally killed," not a user-facing flash. (The single-attach preview-bail flash is a separate, pre-existing path and is out of scope here.)
- Re-apply the delegate after the prune (5.2's propagation via `applyCanvasMode`, already hit by `rebuildSessionList`) so the `●` disappears from the pruned session and remains on survivors.

**Acceptance Criteria**:
- [ ] Marks survive an `s`-regroup: after cycling the grouping mode, the same sessions remain selected (`IsSessionSelected` true) and the banner count is unchanged.
- [ ] Marks survive paging: navigating across pages does not clear the set.
- [ ] A row filtered out by an active query stays selected (in the set) and its `●` reappears when the filter is cleared.
- [ ] The `Space`-preview round-trip returns to the Sessions page in multi-select mode with the selection intact and re-rendered.
- [ ] A session externally killed during the preview is removed from the selection on the post-dismiss refresh; every surviving marked session stays selected.
- [ ] A marked session that moves to a different bucket on an `s`-regroup (e.g. By-Project → By-Tag) stays marked.

**Tests** (Bubble Tea model unit tests in `internal/tui`; drive `SessionsMsg` refresh via the existing test helpers, mirror `pagepreview_externalkill_test.go` / `switch_view_key_test.go`):
- `"it keeps marks across an s-regroup"`
- `"it keeps marks across paging"`
- `"it keeps a filtered-out session marked and restores its ● when the filter clears"`
- `"it returns in-mode with selection intact after a Space-preview round-trip"`
- `"it prunes a session killed during preview and keeps the survivors marked"`
- `"it keeps a session marked when it moves buckets on regroup"`

**Edge Cases**:
- Marks survive `s`-regroup and paging.
- A filtered-out row stays selected and reappears on clear.
- Preview round-trip returns in-mode with selection intact.
- Externally-killed session pruned during preview (survivors kept) — the same prune rule as the pre-flight gate.
- Marked session that moves buckets on regroup stays marked (set keyed on name → bucket-independent).

**Context**:
> Spec *Multi-Select Mode → Sticky selection*: "Selection is **sticky** across filtering, paging, regrouping, **and the `Space`-preview round-trip**. On return from preview, `rebuildSessionList` re-renders **in-mode with the selection intact**, pruning only a selection whose session was **externally killed** during the preview (consistent with the pre-flight rule — a gone session can't be opened). A row filtered out stays selected and reappears when the filter clears."
> Spec *Multi-Select Mode → Granularity*: "The selection model is a set of session identities, so a mark survives regroup/filter/paging even though the session spans multiple list items."
> The set is model state keyed on `Session.Name`, independent of the `list.Model` items — so stickiness is largely a property of *not* clearing it on rebuild; the only active mutation is the live-session prune on refresh. The `refreshSessionsAfterPreviewCmd` → `SessionsMsg` → `applySessions` path already exists for the single-attach externally-killed case (see `pagepreview_externalkill_test.go`); the prune hooks the same chokepoint. Reuse `pruneSelectionToLiveSessions` in 5.7 / Phase 6 (the pre-flight prune is the same rule).

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → Sticky selection / Granularity*; *Burst & Partial-Failure Contract → Pre-flight (the shared prune rule)*.

---

## restore-host-terminal-windows-5-7 | approved

### Task 5.7: N=0 / N=1 Enter commit boundary (N≥2 no-op stub)

**Problem**: `Enter` in multi-select mode commits the **marked set** (the cursor/highlight is irrelevant). Two boundary cases must be handled now: **N=0** (nothing marked) is a no-op that exits the mode and stays in the picker (same effect as `Esc`); **N=1** (one marked) degenerates to a plain single attach in the current window via the existing connector — no special-casing. **N≥2** is where the spawn burst lives, but that is Phase 6 — so N≥2 Enter must be an explicit **no-op stub** that leaves the mode and selection intact until Phase 6 wires it.

**Solution**: Add `handleMultiSelectEnter` (`internal/tui/model.go`), dispatched from the `Enter` mode-branch (5.5). It reads `len(m.selectedSessions)`: N=0 → exit mode (reuse `exitMultiSelect`), stay open; N=1 → set `m.selected` to the one marked name and `tea.Quit` (the exact single-attach commit `handleSessionListEnter` performs, driving the existing `AttachConnector`/`SwitchConnector` at the cmd layer); N≥2 → return `m, nil` unchanged (documented Phase-6 stub).

**Outcome**: In multi-select mode with nothing marked, `Enter` exits the mode with Portal still open and nothing opened; with exactly one session marked, `Enter` selects that session and quits so the current window attaches to it via the existing connector; with two or more marked, `Enter` does nothing and leaves the mode + marks intact (Phase 6 wires the burst); and in every case the cursor/highlight is ignored — only the marked set decides.

**Do**:
- In `internal/tui/model.go` add `func (m Model) handleMultiSelectEnter() (tea.Model, tea.Cmd)`:
  - `n := len(m.selectedSessions)`.
  - `n == 0`: `m = m.exitMultiSelect(); return m, nil` (no-op exit — Portal stays open, nothing opened, same effect as `Esc`; **not** `tea.Quit`). Refresh the delegate (5.2) so the markers/banner clear.
  - `n == 1`: extract the single name from the set (range over the one-element map), set `m.selected = name`, `return m, tea.Quit`. This is byte-identical in effect to `handleSessionListEnter` (set `m.selected`, quit) — the cmd layer's existing self-attach path (`AttachConnector` outside tmux / `SwitchConnector` inside) opens it in the current window. **No special-casing** and no adapter (single attach needs none).
  - `n >= 2`: `return m, nil` **unchanged** — an explicit Phase-6 stub. Add a comment: `// Phase 6 wires the N≥2 spawn burst (internal/spawn) here; until then N≥2 Enter is a deliberate no-op leaving the mode + selection intact.` Do **not** clear the set, do **not** exit the mode, do **not** call `internal/spawn` (it exists but is not called by the TUI until Phase 6).
- Ensure the `Enter` dispatch (5.5) routes to `handleMultiSelectEnter` only when `m.multiSelectMode` and the filter is not focused (the `SettingFilter` break already excludes the focused-filter Enter).
- Optionally reuse `pruneSelectionToLiveSessions` (5.6) is **not** required here — the Phase-6 pre-flight `has-session` gate owns the gone-session check; keep this task purely the N-count branch so it stays independently testable without a live tmux seam.

**Acceptance Criteria**:
- [ ] N=0 Enter sets `MultiSelectActive()==false`, empties the set, does **not** quit (`Selected()==""`, no `tea.Quit`), and opens nothing (same effect as `Esc`).
- [ ] N=1 Enter sets `Selected()` to the one marked session's name and returns `tea.Quit` (drives the existing single-attach connector — no adapter, no special-casing).
- [ ] N≥2 Enter is a no-op: `MultiSelectActive()` stays true, the set is unchanged, `Selected()` stays empty, and no `tea.Quit`/spawn is issued.
- [ ] The commit ignores the cursor/highlight: with a highlighted-but-unmarked row and exactly one *other* session marked, N=1 Enter opens the **marked** session, not the highlighted one.

**Tests** (Bubble Tea model unit tests in `internal/tui`):
- `"it exits the mode and opens nothing on Enter with zero marked (same as Esc)"`
- `"it selects the single marked session and quits on Enter with one marked"`
- `"it opens the marked session, not the highlighted-but-unmarked cursor row, on N=1 Enter"`
- `"it is a no-op leaving the mode and selection intact on Enter with two or more marked (Phase 6 stub)"`

**Edge Cases**:
- N=0 Enter exits mode with nothing opened (Portal stays open — not a quit).
- N=1 Enter attaches that one session via the existing connector (no special-casing, no adapter).
- N≥2 Enter is a no-op leaving the mode intact (Phase 6 wires the burst — do not call `internal/spawn`).
- Highlighted-but-unmarked cursor row is irrelevant at Enter (only the marked set commits).

**Context**:
> Spec *Multi-Select Mode → N=0 / N=1 boundary*: "**N=1** (one marked, Enter): zero windows to spawn — the picker self-attaches to that one session, i.e. it **degenerates to a plain single attach** in the current window. No special-casing. **N=0** (nothing marked, Enter): a **no-op that exits multi-select mode**, dropping back to the standard picker (Portal stays open) — same effect as `Esc`. Nothing opens."
> Spec *Trigger-Context Matrix → Enter opens the marked set only*: "The cursor/highlight at Enter time is irrelevant — a highlighted-but-unmarked row is **not** opened (marking is `m`, not Enter). Enter always commits the `m`-marked set."
> Scope boundary (Phase 5 vs 6): the N≥2 in-process spawn burst, async `tea.Cmd`, detection gate, pre-flight abort, `Opening n/N…`, cancellation, and leave-what-opened selection mutation are **all Phase 6**. This task authors N≥2 Enter as an explicit no-op stub — `internal/spawn` exists (Phases 1–4) but is not called by the TUI until Phase 6.
> `handleSessionListEnter` (set `m.selected`; `tea.Quit`) is the exact shape N=1 mirrors; the cmd layer already turns `Selected()` into an `AttachConnector`/`SwitchConnector` self-attach.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Multi-Select Mode → N=0 / N=1 boundary*; *Trigger-Context Matrix & Open Order → Enter opens the marked set only*.

---

## restore-host-terminal-windows-5-8 | approved

### Task 5.8: Visual gate — `sessions-multi-select-active` capture fixture + reference

**Problem**: The multi-select mode's first visual gate needs a deterministic capture that matches the delivered Paper frame (`design/sessions-multi-select-active.png`) — the violet `3 selected` banner, `●` markers on the selected rows including the cursor row, and the multi-select footer — so the built TUI can be verified against the reference. The `capturetool` / `capture` harness has no fixture that opens the model in multi-select mode with a preset selection.

**Solution**: Add a `sessions-multi-select-active` fixture to `internal/capture/fixtures.go` (registered in `FixtureByName` + `FixtureNames`), seeding the `sessions-flat` session set opened directly in multi-select mode with three sessions marked and the cursor on a marked row — driven through the shared `tui.Build` constructor via a new seed seam (mirroring how `InitialFlash` seeds the otherwise-transient flash). Add the `vhs` tape(s) + committed reference PNG under `testdata/vhs/`, and move the delivered design frame into `testdata/vhs/reference/` per the visual-gate process.

**Outcome**: `go run ./cmd/capturetool --fixture sessions-multi-select-active` renders the mode with `3 selected` (violet) + `esc cancel`, `●` on `agentic-workflows-codify` / `fab-flowx-explore` (the banded cursor row) / `designlab-web-r8suyU`, and the footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel` — matching the Paper frame; the NO_COLOR variant renders glyph-backed (`●`/`▌`/`esc`) on the native bg without crashing; `capture.FixtureNames()` lists the fixture; and the capturetool import guard + `FixtureNames` tests stay green. Dark appearance only (light-mode variant deferred per spec).

**Do**:
- Add a seed seam so a fixture can open the model already in multi-select mode:
  - In `internal/tui/build.go` add a `Deps` field `InitialMultiSelect []string` (the session names to pre-mark; empty → normal mode) and, in `Build`, apply it via a new option `WithInitialMultiSelect(names)` that sets `multiSelectMode = true`, seeds `selectedSessions` from the names, and refreshes the delegate — mirroring the always-applied `WithInitialFlash` seeding order (before `armAppearanceDetection`). Nil/empty leaves the model in normal mode.
  - Position the cursor on a marked row: seed the initial selected index so the fixture's cursor lands on `fab-flowx-explore` (a marked, banded row per the frame). Either extend `WithInitialMultiSelect` to accept a cursor anchor name, or add a small `WithInitialCursor(name)` option that calls the list's select-by-index after items load. Keep it capture-only (production never sets it).
- In `internal/capture/fixtures.go`:
  - Add `sessionsMultiSelectActiveFixture()` reusing the `sessionsFlatFixture()` session set (same 12 sessions, same order), `initialMode: prefs.ModeFlat`, and a new fixture field `initialMultiSelect []string` = `{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"}` with the cursor anchored on `fab-flowx-explore`. Map the field into `Deps()` (`InitialMultiSelect` + the cursor anchor).
  - Register `"sessions-multi-select-active"` in `FixtureByName` (new `case`) and add it to the `FixtureNames()` slice (kept sorted).
- Add `testdata/vhs/sessions-multi-select-active.tape` (mirroring `sessions-flat.tape`: run `capturetool --fixture sessions-multi-select-active`, dark appearance) and capture the reference PNG `testdata/vhs/sessions-multi-select-active.png`. Add a NO_COLOR variant tape `testdata/vhs/sessions-multi-select-active-nocolor.tape` (inline `NO_COLOR=1`, mirroring `sessions-flat-nocolor.tape`) and its PNG. **Verify a fresh write** (hash changed) before trusting the capture — the vhs harness silently fails to write PNGs (re-run + confirm the file hash changed, per the project's capture-flake note).
- Move the delivered design frame into the reference tree: copy `design/sessions-multi-select-active.png` → `testdata/vhs/reference/sessions-multi-select-active-mv.png` (the reference-first convention used by the other MV frames, e.g. `sessions-inline-flash-mv.png`), so the executor and any reviewer Read it and self-check the capture against it.
- Update the `cmd/capturetool` / `internal/capture` fixture-count/list tests (`FixtureNames`, `import_guard_test.go`, `capture_test.go`) for the new registration.

**Acceptance Criteria**:
- [ ] `capture.FixtureByName("sessions-multi-select-active")` returns a fixture and `capture.FixtureNames()` includes it (sorted).
- [ ] `go run ./cmd/capturetool --fixture sessions-multi-select-active` renders in multi-select mode: violet `3 selected` banner + right-aligned `esc cancel`, `●` on the three marked rows (including the banded cursor row `fab-flowx-explore`), and the multi-select footer copy.
- [ ] The captured `sessions-multi-select-active.png` matches the delivered Paper frame (`testdata/vhs/reference/sessions-multi-select-active-mv.png`) — banner, markers (cursor row also marked), footer, and layout.
- [ ] The NO_COLOR variant renders glyph-backed (`●`/`▌`/`esc`) on the native bg without crashing (no violet hue, no canvas).
- [ ] Dark appearance only — no light-mode fixture/tape is added (deferred per spec).
- [ ] The capturetool import guard + fixture-list tests pass with the new fixture.

**Tests**:
- `"it registers sessions-multi-select-active in FixtureByName and FixtureNames"`
- `"it builds the fixture in multi-select mode with three sessions marked and the cursor on a marked row"`
- `"it renders the NO_COLOR variant glyph-backed without crashing"`
- Visual gate (manual, at the gate): compare `sessions-multi-select-active.png` against `testdata/vhs/reference/sessions-multi-select-active-mv.png`; provide the live-view command `go run ./cmd/capturetool --fixture sessions-multi-select-active --appearance dark`.

**Edge Cases**:
- Fixture matches the Paper frame (cursor row `fab-flowx-explore` is also marked — band + `●`).
- NO_COLOR variant renders glyph-backed without crashing.
- Dark appearance only (light-mode variant deferred per spec).

**Context**:
> Spec *Design References → Sessions — Multi-Select (active)*: "`design/sessions-multi-select-active.png`. Violet `3 selected` banner (filter-line analogue); violet `●` markers on selected rows incl. the cursor row; `Space` still preview; footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`."
> Spec *Design References → Tokens*: "Dark-mode; light-mode variants deferred unless requested."
> Spec *Design References → Visual-gate process*: "the committed `design/` frames are the implementation reference… re-capture fresh frames via the `capturetool` / `vhs` harness once the feature is built — moving them to `testdata/vhs/reference/` when wiring the visual gate."
> The harness never opens a real tmux server or reads real config — the fixture injects the session set in-memory via `fakeLister` and drives the exact production model through `tui.Build` (the seed seam mirrors `InitialFlash`, the established pattern for capturing an otherwise-transient state). The delivered frame (`design/sessions-multi-select-active.png`) is the golden layout: 12 sessions, `3 selected`, cursor on `fab-flowx-explore`.
> Project convention (capture flake): the vhs harness silently fails to write PNGs — verify a fresh write (file hash changed) and retry before pixel-checking or trusting the capture.

**Spec Reference**: `.workflows/restore-host-terminal-windows/specification/restore-host-terminal-windows/specification.md` — *Design References → Sessions — Multi-Select (active) / Tokens / Visual-gate process*; *Testing Strategy & DI Seams → Irreducible manual/integration residue*.
