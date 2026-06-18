---
phase: 4
phase_name: Preview chrome + edge/UX states
total: 7
---

## spectrum-tui-design-4-1 | approved

### Task spectrum-tui-design-4-1: Notice-band primitive + single-slot arbiter + flash-driven viewport-height recompute (F10)

**Problem**: Today the Sessions page inserts a persistent By-Tag signpost row AND a transient flash row independently and unconditionally — `viewSessionList` in `internal/tui/model.go` calls `insertRowBelowTitle(...)` once for `m.byTagSignpost` and again for `m.flashText`, so both can insert at once, the two rows are styled with ad-hoc `#888888` literals (`byTagSignpostStyle`, `flashRowStyle`), neither carries the MV `▌` left-bar treatment, and the inserted rows are not accounted for in the list's height budget. The spec (§11 intro) requires a single shared notice-slot that holds **at most one band**, rendered as a `▌` left-bar accent line directly under the title separator and above the section header, with the section header + list shifting down — and the band's appearance/clear must recompute the list viewport height (F10) so the one-row-per-delegate pagination invariant holds.

**Solution**: Build the shared `▌`-band render primitive (a single function that styles a notice band with a role-coloured `▌` left-bar + message, on the band's tint, full-width under the title separator), plus the single-slot arbiter that decides which band — at most one — currently occupies the slot, plus the F10 height tie-in that recomputes the list viewport height whenever the band appears or clears. This is shared infrastructure consumed by tasks 4-2 (inline flash), 4-3 (no-tags signpost), and 4-4 (command-pending banner). The arbiter encodes the new logic: a persistent mode band (signpost §11.3, command-pending §11.4) owns the slot while its mode is active; a transient flash (§11.2) temporarily takes the slot, replacing any persistent band for its duration, then the persistent band returns; a persistent violet info band and a transient flash never display at once — the transient wins while shown.

**Outcome**: A single notice band renders in the slot when any notice is active; never two at once; the band sits directly under the title separator above the section header; the list/section-header shift down; and the list's viewport height is recomputed on band appear/clear so pagination never overflows or miscounts. The visible colour/glyph effect is verified through the bands that consume this primitive (4-2/4-3/4-4); this task's own acceptance is the band primitive structure, the single-slot arbitration, and the F10 recompute (behaviour + structure tests).

**Do**:
- In `internal/tui/model.go` (or a new `internal/tui/notice_band.go` sibling — keep it in the `tui` package), add a band-role enum/type with the three MV role variants from §11 intro: transient/warning (`accent.orange`), transient/success (`state.green`), mode/info (`accent.violet`). Bind colours to the Phase 1 §2.9 tokens (`accent.orange`, `state.green`, `accent.violet`, and the per-band tint where applicable, e.g. `bg.warning` for the warning flash) — no literal hex; the per-band tint and on-band text token are passed/selected by the consuming task.
- Add a shared band-render primitive `renderNoticeBand(role, message, width, ...)` that emits a full-width single-line band: a `▌` left-bar in the role colour pinned at the far left, then the message text in the band's on-band text token, padded to the slot width. Reuse the canvas/full-width treatment from the Phase 1 owned-canvas paint and the Phase 2 shared chrome so the band aligns under the title separator. Under `NO_COLOR` (§2.5, Phase 1 task 1-8 carve-out) the band drops tint + bar colour but keeps the `▌` bar, position, and message text (and, for the flashes, the glyph + bold/dim — passed by 4-2).
- Replace the dual independent `insertRowBelowTitle` calls in `viewSessionList` with a single arbitration step: compute the one active band (if any) and insert it once. Encode the single-slot rule: when `m.flashText != ""` (transient) it wins the slot regardless of any persistent band; otherwise the active persistent band (e.g. `m.byTagSignpost` in By-Tag, or command-pending on Projects via `viewProjectList`) occupies the slot. Keep `insertRowBelowTitle` as the placement mechanic (under the title/filter line) but funnel both source rows through the arbiter so the band is inserted at most once.
- Preserve the existing flash lifecycle in `internal/tui/sessions_flash.go` (`flashGen`/`flashTickCmd`/`isActionableKey`) and the `setFlash`/`clearFlash` primitives + the generation guard (the `msg.Gen == m.flashGen` compare in the `flashTickMsg` handler, and the actionable-key clear at the top of `updateSessionList`) — the arbiter only changes *which* row renders, never the flash's auto-clear timing or generation-guard behaviour.
- Wire the F10 height recompute: when the band appears or clears, recompute the list viewport height before the list lays out (the same recompute `applySessionListSize`/`applyListSize` already performs for the manual footer). The notice band is chrome — it consumes one row of the height budget while present and releases it when cleared; reuse the existing one-row-per-delegate recompute path so the list never overflows or miscounts. Account for the band row in the height budget passed to `applyListSize` (subtract one row while a band is active).

**Acceptance Criteria**:
- [ ] A single shared `▌`-band render primitive exists, parameterised by the three role variants (orange/warning, green/success, violet/info), emitting a full-width single-line band with a far-left `▌` left-bar; no literal hex — colours come from the §2.9 tokens.
- [ ] The notice slot holds at most one band: with both a persistent band condition and a transient flash active, only the transient flash renders; when the flash clears, the persistent band returns; the two never render simultaneously.
- [ ] The band sits directly under the title separator, above the section header, full-width; the section header + list shift down by one row.
- [ ] On band appear and on band clear the list viewport height is recomputed (the band consumes one row while present, releases it when cleared); the one-row-per-delegate pagination invariant holds — no overflow, no miscount.
- [ ] The existing flash auto-clear lifecycle is preserved: the generation guard still drops superseded ticks, and an actionable keypress still clears an active flash (parity vs the pre-reskin `flashGen`/`flashTickCmd`/`isActionableKey` behaviour).
- [ ] Under `NO_COLOR` the band drops tint + bar colour but stays present via the `▌` bar + position + message (verified at the primitive level; glyph/bold-dim contributions are passed by the consuming flash task 4-2).
- [ ] vhs verification: this primitive has no standalone Paper frame — its visible effect is verified through the bands it powers (4-2/4-3/4-4). Produce no separate frame compare here; instead the band-structure/colour-role match is asserted in 4-2's `Sessions — inline flash (MV)`, 4-3's `Sessions — no tags signpost (MV)`, and 4-4's `Projects — command pending (MV)` captures. This task's `vhs` obligation is discharged by the band rendering correctly in those three downstream captures.

**Tests**:
- `"it renders a single notice band with a far-left ▌ left-bar in the role colour for each of the three role variants"`
- `"it inserts at most one band: with both a persistent band and a transient flash active, only the transient flash row renders"`
- `"it returns the persistent band to the slot after the transient flash clears"`
- `"it never renders a persistent violet info band and a transient flash simultaneously"`
- `"it places the band directly under the title separator above the section header and shifts the section header + list down one row"`
- `"it recomputes the list viewport height when the band appears (one row consumed) and when it clears (one row released) — pagination count unchanged otherwise"` (edge: band present↔absent recomputes list height, one-row-per-delegate invariant, no overflow/miscount)
- `"it preserves the flash generation guard — a superseded flashTickMsg does not early-clear the current flash"` (edge: existing flashGen guard preserved)
- `"it clears an active flash on the next actionable keypress"` (edge: flash auto-clear on next keypress)
- `"it clears an active flash after the short timeout"` (edge: flash auto-clear on short timeout)
- `"it keeps the ▌ bar and band position under NO_COLOR while dropping tint and bar colour"` (edge: NO_COLOR drops tint/bar colour but keeps ▌ bar + position)

**Edge Cases**:
- At-most-one band: replaces today's independent signpost+flash double-insert (both could insert at once) with a single arbitrated insertion.
- Persistent↔transient hand-off: transient flash wins the slot while shown, then the persistent band returns when the flash clears.
- Flash auto-clear on next keypress / short timeout: the existing `flashGen` generation guard is preserved so a stale in-flight tick cannot early-clear a current flash.
- Band present↔absent recomputes the list height (F10): consumes one row while present, releases it when cleared; the one-row-per-delegate invariant must hold (no overflow / miscount).
- `NO_COLOR`: drops tint + bar colour but keeps the `▌` bar + position (Phase 1 task 1-8 carve-out, §2.5).

**Context**:
> §11 intro (shared convention): inline notices use a `▌` left-bar accent line — `accent.orange` = transient/warning, `state.green` = transient/success, `accent.violet` = mode/info. Placement: the band sits directly under the title separator, above the section header (full-width); the section header + list shift down.
> §11 intro (single-slot rule): the notice slot holds at most one band. Persistent mode notices (no-tags signpost §11.3, command-pending §11.4) own the slot while their mode is active; a transient flash (§11.2) takes the slot temporarily, replacing any persistent band for its duration, then the persistent band returns. The flash auto-clears on the next keypress or after a short timeout. A persistent (violet info) band and a transient flash never display at once — the transient flash wins while shown.
> §11.2 (F10 — flash vs pagination): the flash band is chrome — when it appears/clears, the list viewport height is recomputed (the same recompute the one-row-per-delegate invariant already mandates), so the list never overflows or miscounts rows.
> §2.5 (NO_COLOR): notice bands drop their tint and bar colour — the band stays present via its `▌` left-bar and position — and carry state through the message text plus (on the flashes) the `⚠`/`✓` glyph and bold/dim.
> Current code (`internal/tui/model.go`): `viewSessionList` calls `insertRowBelowTitle(listView, renderByTagSignpostRow())` when `m.byTagSignpost` AND `insertRowBelowTitle(listView, m.renderFlashRow())` when `m.flashText != ""` — independently, so both insert at once today. `insertRowBelowTitle` splits the first line (title/filter) and inserts the row after it. The single-slot arbitration is the new logic that prevents both inserting at once.
> Current code (`internal/tui/sessions_flash.go`): `flashGen`/`setFlash`/`clearFlash`/`flashTickCmd`/`flashTickMsg`/`isActionableKey` lifecycle to reuse unchanged. The generation guard (`if msg.Gen == m.flashGen { m.clearFlash() }` in the `flashTickMsg` handler) and the `if m.flashText != "" && isActionableKey(msg) { m.clearFlash() }` actionable-key clear in `updateSessionList` are parity-load-bearing.
> Depends on Phase 1 (§2.9 tokens, owned canvas, NO_COLOR carve-out — tasks 1-3/1-4/1-6/1-8) and Phase 2 (shared chrome / section header / title separator — tasks 2-2/2-3). This task is shared infrastructure; the band/arbiter/recompute it builds are consumed by tasks 4-2, 4-3, 4-4.
> Per §15.4 the implementer produces and self-checks the capture before handoff; for this infrastructure task the capture obligation is satisfied via the downstream 4-2/4-3/4-4 frames (this primitive has no standalone Paper frame).

**Spec Reference**: §11 intro (shared left-bar notice convention + single-slot rule), §11.2 (F10 flash vs pagination), §2.5 (NO_COLOR notice-band carve-out), §2.9 (token table), §3.1–§3.4 (shared chrome placement). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-2 | approved

### Task spectrum-tui-design-4-2: Inline flash band reskin (warning ⚠ + success ✓) routed through the single-slot arbiter

**Problem**: The Sessions inline flash (the transient band shown e.g. when a session is killed externally) currently renders via `renderFlashRow`/`flashRowStyle` in `internal/tui/model.go` with an ad-hoc `#888888` foreground and no `▌` left-bar, no warning tint, no glyph, and no MV treatment. The spec (§11.2) requires it to be an `accent.orange` left-bar band with a `⚠` glyph and the message on a `bg.warning` tint with `text.on-warning` message text, auto-clearing — plus a `state.green` `✓` success variant so success stays glyph-distinct from the warning (not colour-only, §2.2).

**Solution**: Restyle the inline flash to the §11.2 MV treatment by routing it through the task-4-1 notice-band primitive: the warning variant uses the orange role (`accent.orange` `▌` left-bar + `⚠` glyph) with the message on a `bg.warning` tint in `text.on-warning`; the success variant uses the green role (`state.green` `▌` left-bar + `✓` glyph) so success is glyph-distinct. The flash takes the single slot over any persistent band for its duration (via the 4-1 arbiter), then the persistent band returns; the auto-clear lifecycle (generation-guarded tick + actionable-key clear) is preserved exactly. Appearance/clear triggers the F10 viewport-height recompute (via the 4-1 tie-in).

**Outcome**: The inline flash renders as the MV warning band (`accent.orange ▌ ⚠ <message>` on `bg.warning` with `text.on-warning`) and, for the success path, the MV success band (`state.green ▌ ✓ <message>`); it auto-clears on the next actionable keypress or after the short timeout exactly as before; it occupies the single notice slot over a persistent band for its duration then yields it back; and the list height recomputes on appear/clear.

**Do**:
- In `internal/tui/model.go`, replace `flashRowStyle` (the `#888888` literal) and `renderFlashRow` with a call into the task-4-1 band primitive using the warning role: pass `accent.orange` (bar), `bg.warning` (tint), `text.on-warning` (message text), and the `⚠` glyph; render `⚠ <m.flashText>`. No literal hex — colours from §2.9 tokens.
- Add the success variant: a flash carrying the success role renders the `state.green` `▌` left-bar + `✓` glyph with the message (per §11.2 / §2.9 `state.green` role). Distinguish warning vs success on the flash record (e.g. a flash-kind field on the model alongside `flashText`, defaulting to warning so the existing externally-killed bail stays a warning). The `✓` (success) vs `⚠` (warning) glyph keeps the two variants distinct without relying on colour (§2.2).
- Keep `formatSessionGoneFlash` (in `internal/tui/sessions_flash.go`) as the source of the warning message wording for the externally-killed bail (parity) — restyle the row, not the wording or the lifecycle. Confirm the warning band still reads with the spec's example shape `<name> closed externally — list updated` is illustrative in §11.2; the actual wording stays whatever `formatSessionGoneFlash` already produces (parity — do not change copy unless a Phase 1-2 task already changed it).
- Route the flash through the task-4-1 single-slot arbiter so it wins the slot over any persistent band (no-tags signpost / command-pending) while shown, then the persistent band returns when the flash clears. Do not re-implement insertion — consume the 4-1 arbiter.
- Preserve the auto-clear lifecycle byte-for-byte: `setFlash` bumps `flashGen`; the `flashTickCmd(m.flashGen)` schedule + the `flashTickMsg` generation-guard clear; and the actionable-key clear (`if m.flashText != "" && isActionableKey(msg)`). The reskin must not perturb the timing or the generation guard.
- Ensure the F10 recompute (from task 4-1) fires on flash appear and clear: the `bg.warning`/`state.green` band consumes one list row while present and releases it when cleared.
- Verify the text-on-tint pair clears the contrast floor: `text.on-warning` on `bg.warning` (pinned + co-tuned in Phase 1 task 1-4 / §2.9 — both ratios clear simultaneously). The reskin must use the co-tuned pair, not invent a new pairing.
- Under `NO_COLOR`, the flash drops tint + bar colour but keeps the `▌` bar + position + the `⚠`/`✓` glyph + bold/dim (§2.5 / §2.2) — carried through the 4-1 primitive's NO_COLOR path; pass the glyph + bold/dim so state survives colourlessly.

**Acceptance Criteria**:
- [ ] The warning flash renders as `accent.orange ▌` left-bar + `⚠` glyph + message on a `bg.warning` tint with `text.on-warning` message text; no literal hex (colours from §2.9 tokens).
- [ ] The success flash renders as `state.green ▌` left-bar + `✓` glyph + message; `✓` (success) and `⚠` (warning) keep the two variants glyph-distinct, not colour-only (§2.2).
- [ ] The flash auto-clears on the next actionable keypress and after the short timeout, with the generation guard intact — parity with the pre-reskin `formatSessionGoneFlash`/`flashGen` lifecycle.
- [ ] The flash takes the single notice slot over any persistent band for its duration, then the persistent band returns when it clears (via the task-4-1 arbiter).
- [ ] The `text.on-warning` on `bg.warning` pair clears the contrast floor (uses the Phase 1 co-tuned pair, both ratios clear simultaneously).
- [ ] The list viewport height recomputes on flash appear and clear (F10 via task 4-1); pagination never overflows or miscounts.
- [ ] Under `NO_COLOR` the flash keeps the `▌` bar + position + `⚠`/`✓` glyph + bold/dim while dropping tint and bar colour.
- [ ] vhs verification: a `vhs` tape drives the TUI to the inline-flash state (trigger an externally-killed-session bail against the seeded fixture) and writes `testdata/vhs/sessions-inline-flash.png`; the capture is compared against the named Paper frame `Sessions — inline flash (MV)` (§15.1) for layout / structure / colour-role; the success variant follows the same pattern (not separately mocked). Behaviour parity vs the pre-reskin flash (lifecycle + wording) is confirmed (§1).

**Tests**:
- `"it renders the warning flash as accent.orange ▌ + ⚠ + message on bg.warning with text.on-warning"`
- `"it renders the success flash as state.green ▌ + ✓ + message"`
- `"it keeps ✓ and ⚠ glyph-distinct so success is not conveyed by colour alone"` (edge: success glyph-distinct from warning, §2.2)
- `"it auto-clears the flash on the next actionable keypress"` (edge: parity with formatSessionGoneFlash auto-clear)
- `"it auto-clears the flash after the short timeout with the generation guard intact"` (edge: superseded tick does not early-clear)
- `"it takes the slot over a persistent band then restores the persistent band on clear"` (edge: takes slot over persistent band then restores it)
- `"it keeps the text.on-warning on bg.warning pair above the contrast floor"` (edge: warning text-on-tint pair clears floor)
- `"it recomputes the list height on flash appear and clear"` (edge: list height recomputed on appear/clear)
- `"it keeps the ▌ bar + ⚠/✓ glyph + bold/dim under NO_COLOR"` (edge: NO_COLOR keeps ▌ + glyph + bold/dim)
- `"it preserves the externally-killed bail wording from formatSessionGoneFlash"` (edge: wording parity)

**Edge Cases**:
- Success glyph-distinct from warning (`✓` vs `⚠`, not colour-only — §2.2).
- Warning text-on-tint pair clears the floor (`text.on-warning` on `bg.warning`, co-tuned in Phase 1 task 1-4).
- Auto-clears on next actionable keypress / timeout (parity with `formatSessionGoneFlash` + the generation guard).
- Takes the slot over a persistent band, then restores it (via the task-4-1 arbiter).
- `NO_COLOR` keeps the `▌` bar + glyph + bold/dim while dropping tint/bar colour.
- List height recomputed on appear/clear (F10 via task 4-1).

**Context**:
> §11.2 (inline flash): a transient band under the title separator — an `accent.orange` left-bar + `⚠` + message (e.g. `folio-Jiz4el closed externally — list updated`), on a `bg.warning` tint with `text.on-warning` message text; auto-clears. The success variant uses `state.green` with a `✓` glyph so success stays glyph-distinct from the warning `⚠`, not colour-only (§2.2, matching the §2.9 `state.green` role).
> §2.9: `text.on-warning` (`#E8C9A0` dark / `#7A4B12` light) on `bg.warning` (`#241B10` dark / light amber) — the text-carrying tint is co-tuned with its on-band text token; both ratios clear simultaneously (pinned in Phase 1 task 1-4). `state.green` carries live/positive signals including the success flash. `accent.orange` carries the warning flash `⚠`.
> §2.2: state is never carried by hue alone — `⚠` warning, `✓` success — colour only reinforces the glyph; this makes the `NO_COLOR` path work and protects colour-blind users.
> Current code (`internal/tui/model.go`): `flashRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))` and `renderFlashRow` render the flash text verbatim with the `#888888` placeholder — restyle to tokens. `formatSessionGoneFlash` (in `internal/tui/sessions_flash.go`) produces the externally-killed bail wording (preserve). The auto-clear lifecycle (`setFlash`/`clearFlash`/`flashGen`/`flashTickCmd`/`flashTickMsg` + `isActionableKey`) must be preserved (parity).
> Routes through the task-4-1 single-slot arbiter + the F10 height-recompute tie-in. Depends on Phase 1 (§2.9 tokens incl. the co-tuned `bg.warning`/`text.on-warning` pair, NO_COLOR carve-out) and task 4-1.
> Per §15.4 the implementer produces the `vhs` capture and self-checks it against `Sessions — inline flash (MV)` before handoff.

**Spec Reference**: §11.2 (inline flash, F10 recompute), §2.2 (glyph + colour, never hue alone), §2.5 (NO_COLOR band carve-out), §2.9 (`accent.orange` / `state.green` / `bg.warning` / `text.on-warning` tokens, co-tuned pair), §15.1 (`Sessions — inline flash (MV)`). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-3 | approved

### Task spectrum-tui-design-4-3: "No tags yet" signpost reskin — accent.violet left-bar band over the flat list, via the single-slot arbiter

**Problem**: When By-Tag mode is active but no project carries any tag, the Sessions page degrades to the flat list with a signpost row — but today that row renders via `renderByTagSignpostRow`/`byTagSignpostStyle` in `internal/tui/model.go` with an ad-hoc `#888888` italic foreground, no `▌` left-bar, and stale wording (`"No tags yet — add tags on the projects page"`). The spec (§11.3) requires an `accent.violet` left-bar signpost in `text.strong` with the wording `No tags yet — add tags in a project's editor: press x for projects, then e to edit`, rendered over the flat list (degrade-with-message, not silent flatten — §5.3), routed through the single notice slot.

**Solution**: Restyle the signpost to the §11.3 MV treatment by routing it through the task-4-1 notice-band primitive using the violet info role (`accent.violet` `▌` left-bar + message in `text.strong`), and update the wording constant to the spec's exact string. The signpost is a persistent mode band: it owns the slot while By-Tag-with-zero-tags is active, yields the slot to a transient flash for that flash's duration, then returns. The grouping machinery and the "renders over the flat list, zero pane reads" behaviour (§5.4) stay intact — the reskin only changes the row's appearance and routes it through the slot.

**Outcome**: The no-tags signpost renders as an `accent.violet ▌` left-bar band with the §11.3 wording in `text.strong`, sitting under the title separator over the flat list; it persists while By-Tag has zero tags anywhere; it yields the slot to a transient flash then returns; and it performs zero pane reads (the flat-list / signpost path is preserved).

**Do**:
- In `internal/tui/model.go`, replace `byTagSignpostStyle` (the `#888888` italic literal) and `renderByTagSignpostRow` with a call into the task-4-1 band primitive using the violet info role: `accent.violet` `▌` left-bar + message in `text.strong`. No literal hex — colours from §2.9 tokens.
- Update `byTagSignpostText` to the spec-exact wording: `No tags yet — add tags in a project's editor: press x for projects, then e to edit` (§11.3). Source it as a single constant (the message wording is sourced as a constant, per the task table edge). This replaces the current `"No tags yet — add tags on the projects page"` string.
- Route the signpost through the task-4-1 single-slot arbiter as a persistent violet info band: it owns the slot while `m.byTagSignpost` is true, but a transient flash (task 4-2) takes the slot for its duration, then the signpost returns. Remove the standalone `insertRowBelowTitle(listView, renderByTagSignpostRow())` call from `viewSessionList` — the arbiter now owns insertion.
- Preserve the existing gate and grouping machinery byte-for-byte: `m.byTagSignpost = m.sessionListMode == prefs.ModeByTag && !anyTagsExist(m.projects)` in `rebuildSessionList`, the `case m.byTagSignpost: items = ToListItems(filtered)` arm (flat items, no grouping), and the §5.4 invariant that the signpost/flat path performs **zero pane reads** (`resolveSessionDirs` is invoked only from the grouped arms, never the signpost arm). The reskin must not move grouping logic or introduce a pane read on this path.
- Ensure the band's appear/clear participates in the F10 height recompute via task 4-1 (the signpost consumes one list row while present).
- Under `NO_COLOR`, the signpost drops tint + bar colour but keeps the `▌` bar + position + message (via the 4-1 NO_COLOR path).

**Acceptance Criteria**:
- [ ] The signpost renders as an `accent.violet ▌` left-bar band with the message in `text.strong`; no literal hex (colours from §2.9 tokens).
- [ ] The wording is the spec-exact `No tags yet — add tags in a project's editor: press x for projects, then e to edit`, sourced as a single constant.
- [ ] The signpost shows only in By-Tag mode with zero tags anywhere, over the flat list (degrade-with-message); the grouping machinery (`anyTagsExist` gate, `ToListItems` flat-items arm) is unchanged (parity).
- [ ] The signpost performs zero pane reads (the flat / signpost path never invokes `resolveSessionDirs` — §5.4 preserved).
- [ ] The signpost is a persistent band: it yields the slot to a transient flash for the flash's duration, then returns (via the task-4-1 arbiter).
- [ ] Under `NO_COLOR` the signpost keeps the `▌` bar + position while dropping tint + bar colour.
- [ ] vhs verification: a `vhs` tape drives the TUI to By-Tag with a zero-tags fixture and writes `testdata/vhs/sessions-no-tags-signpost.png`; the capture is compared against the named Paper frame `Sessions — no tags signpost (MV)` (§15.1) for layout / structure / colour-role. Behaviour parity vs the pre-reskin signpost (gate + flat-list + zero-pane-reads) is confirmed (§1).

**Tests**:
- `"it renders the signpost as an accent.violet ▌ left-bar band in text.strong"`
- `"it uses the spec-exact wording sourced as a constant"` (edge: message wording sourced as a constant)
- `"it shows the signpost only in By-Tag mode with zero tags anywhere"` (edge: shows only in By-Tag with zero tags anywhere, parity)
- `"it renders the signpost over the flat list with zero pane reads"` (edge: renders over flat list, zero pane reads §5.4)
- `"it leaves the grouping machinery untouched (anyTagsExist gate + ToListItems flat arm)"` (edge: grouping machinery untouched)
- `"it yields the slot to a transient flash then returns the signpost on flash clear"` (edge: persistent band yields slot to transient flash then returns)
- `"it keeps the ▌ bar + position under NO_COLOR"` (edge: NO_COLOR keeps ▌ + position)

**Edge Cases**:
- Shows only in By-Tag with zero tags anywhere (parity; grouping machinery untouched — `anyTagsExist` gate + `ToListItems` flat arm).
- Persistent band yields the slot to a transient flash, then returns (via the task-4-1 arbiter).
- Renders over the flat list with zero pane reads preserved (§5.4 — the signpost/flat path never invokes `resolveSessionDirs`).
- `NO_COLOR` keeps the `▌` bar + position while dropping tint/bar colour.
- Message wording sourced as a constant (the spec-exact string).

**Context**:
> §11.3 ("No tags yet" signpost): By-Tag with zero tags anywhere — an `accent.violet` left-bar signpost (`No tags yet — add tags in a project's editor: press x for projects, then e to edit`, `text.strong`) over the flat list — degrade-with-message, not a silent flatten (§5.3).
> §5.3: by Tag with no project anywhere carrying tags shows the "No tags yet" signpost over the flat list instead.
> §5.4: pane reads are gated to grouped modes only — Flat and the zero-tags signpost perform zero pane reads (the lazy dir-resolution fallback is invoked only from the grouped render arms).
> Current code (`internal/tui/model.go`): `byTagSignpostStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)`, `byTagSignpostText = "No tags yet — add tags on the projects page"` (stale wording), `renderByTagSignpostRow()`, and the `case m.byTagSignpost: items = ToListItems(filtered)` arm + the `m.byTagSignpost = ...` gate in `rebuildSessionList`. `viewSessionList` inserts the signpost via `insertRowBelowTitle(listView, renderByTagSignpostRow())` — this standalone call is replaced by the task-4-1 arbiter.
> Routes through the task-4-1 single-slot arbiter (persistent violet info band) + the F10 recompute. Depends on Phase 1 (§2.9 tokens, NO_COLOR carve-out) and task 4-1.
> Per §15.4 the implementer produces the `vhs` capture and self-checks it against `Sessions — no tags signpost (MV)` before handoff.

**Spec Reference**: §11.3 (no-tags signpost), §5.3 (zero-tags → signpost rule), §5.4 (zero pane reads on flat/signpost path), §2.5 (NO_COLOR band carve-out), §2.9 (`accent.violet` / `text.strong` tokens), §15.1 (`Sessions — no tags signpost (MV)`). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-4 | approved

### Task spectrum-tui-design-4-4: Command-pending banner reskin — accent.violet left-bar + orange command chip + swapped footer, over full Projects chrome

**Problem**: When Projects is invoked to run a command, today's `View()` in `internal/tui/model.go` inserts a plain status line `"Select project to run: " + strings.Join(m.command, " ")` after the list title, and the footer is sourced from `commandPendingHelpKeys` (currently `enter run here` / `n new in cwd` / `q quit`). The spec (§11.4) requires an `accent.violet` left-bar banner reading `Pick a project to run` with the command in an `accent.orange` chip, a footer of `⏎ run here · n run in cwd · esc cancel`, all over the FULL Projects chrome (green `Projects` header + `/ to filter`) — not a stripped page.

**Solution**: Restyle the command-pending banner to the §11.4 MV treatment by routing it through the task-4-1 notice-band primitive using the violet info role (`accent.violet` `▌` left-bar) with the fixed text `Pick a project to run` and the command rendered in an `accent.orange` chip beside it; swap the footer to `⏎ run here · n run in cwd · esc cancel`; and keep the full Projects chrome intact (the banner sits on top). The dispatched actions (run here / run in cwd / cancel) are parity-preserved.

**Outcome**: In command-pending mode the Projects page keeps its full chrome (green `Projects` header + `/ to filter`) with an `accent.violet ▌` banner (`Pick a project to run`) and the command in an `accent.orange` chip sitting under the title separator; the footer reads `⏎ run here · n run in cwd · esc cancel`; and run-here / run-in-cwd / cancel dispatch exactly as before.

**Do**:
- In `internal/tui/model.go` `View()` (the `case PageProjects: if m.commandPending` arm), replace the plain `statusLine := "Select project to run: " + ...` insertion with the task-4-1 violet info band: `▌` left-bar in `accent.violet` + fixed text `Pick a project to run` (source as a constant) + the command joined (`strings.Join(m.command, " ")`) rendered in an `accent.orange` chip. The chip is `accent.orange`-styled per §11.4; no literal hex — colours from §2.9 tokens. Route insertion through the task-4-1 arbiter as a persistent violet band (so a transient flash could not collide — though Projects has no flash today, the slot rule is honoured uniformly).
- Keep the full Projects chrome: `viewProjectList()` still renders the green `Projects` header + count + `/ to filter` hint (from Phase 3 task 3-2 / §6.1) — do NOT strip the page; the banner sits on top under the title separator. The screen is the normal Projects page plus the banner, not a dedicated command-pending page.
- Swap the footer for command-pending: it must read `⏎ run here · n run in cwd · esc cancel` (§11.4). Update `commandPendingHelpKeys` (currently `enter run here` / `n new in cwd` / `q quit`) so the footer renders the three spec keys with the spec labels — `⏎ run here` / `n run in cwd` / `esc cancel`. The footer for command-pending is drawn from `commandPendingHelpKeys` via `projectFooterBindings(&m.projectList, m.commandPending)` (parity of the binding-source mechanism), restyled to MV glyphs/labels (`accent.blue` glyphs / `text.detail` labels per the Phase 2/3 footer). Verify the dispatched actions match: `Enter` → run here (`handleProjectEnter` in command-pending), `n` → run in cwd (`handleNewInCWD`), `Esc` → cancel — all parity-preserved; do not change the dispatch, only the footer presentation/labels.
- Confirm the command-pending mode gate is unchanged: `m.commandPending` set via the `WithCommand` option; `s`/`x`/`e`/`d` are suppressed while command-pending (the existing `if m.commandPending { return m, nil }` guards in `updateProjectsPage`) — parity, do not alter.
- Ensure the banner participates in the F10 height recompute via task 4-1 (the banner consumes one list row while present).
- Under `NO_COLOR`, the banner drops tint + bar colour but keeps the `▌` bar + position + the chip's glyph/position (via the 4-1 NO_COLOR path); the orange chip degrades to a colourless chip that stays distinguishable by position.

**Acceptance Criteria**:
- [ ] In command-pending mode the banner renders as an `accent.violet ▌` left-bar with `Pick a project to run` and the command in an `accent.orange` chip; no literal hex (colours from §2.9 tokens).
- [ ] The full Projects chrome is preserved — green `Projects` header + count + `/ to filter` hint — and the banner sits on top (not a stripped page).
- [ ] The footer reads `⏎ run here · n run in cwd · esc cancel`, drawn from `commandPendingHelpKeys` via the existing binding-source mechanism, restyled to MV.
- [ ] The dispatched actions are parity-preserved: `Enter` runs here, `n` runs in cwd, `Esc` cancels — identical to the pre-reskin command-pending behaviour.
- [ ] The banner is routed through the task-4-1 single-slot arbiter as a persistent violet band; the list height recomputes on banner appear/clear.
- [ ] Under `NO_COLOR` the banner keeps the `▌` bar + position + chip position/glyph while dropping tint + bar colour.
- [ ] vhs verification: a `vhs` tape launches Portal in command-pending mode (seeded command) and writes `testdata/vhs/projects-command-pending.png`; the capture is compared against the named Paper frame `Projects — command pending (MV)` (§15.1) for layout / structure / colour-role. Behaviour parity vs the pre-reskin command-pending mode (dispatched actions, full chrome) is confirmed (§1).

**Tests**:
- `"it renders the banner as accent.violet ▌ + 'Pick a project to run' with the command in an accent.orange chip"`
- `"it joins the command into the orange chip"` (edge: command joined into orange chip)
- `"it keeps the full Projects chrome (green header + / to filter), not a stripped page"` (edge: keeps full Projects chrome, not stripped)
- `"it renders the command-pending footer as ⏎ run here · n run in cwd · esc cancel from commandPendingHelpKeys"` (edge: footer drawn from commandPendingHelpKeys, parity)
- `"it dispatches run-here on Enter, run-in-cwd on n, cancel on Esc identically to the pre-reskin behaviour"` (edge: parity of dispatched actions)
- `"it routes the banner through the single-slot arbiter as a persistent violet band"` (edge: banner routed through arbiter)
- `"it keeps the ▌ bar + chip glyph/position under NO_COLOR"` (edge: NO_COLOR keeps ▌ + chip glyph/position)

**Edge Cases**:
- Keeps the full Projects chrome (green header + `/ to filter`, not stripped) — the banner sits on top of the normal Projects page.
- Command joined into the orange chip (`strings.Join(m.command, " ")`).
- Footer drawn from `commandPendingHelpKeys` (parity of the binding source + dispatched actions); only the presentation/labels change.
- Banner routed through the task-4-1 single-slot arbiter as a persistent violet band.
- `NO_COLOR` keeps the `▌` bar + chip glyph/position while dropping tint/bar colour.

**Context**:
> §11.4 (command-pending banner): when Projects is invoked to run a command — an `accent.violet` left-bar banner (`Pick a project to run`) with the command in an `accent.orange` chip; the footer becomes `⏎ run here · n run in cwd · esc cancel`. The screen keeps the full Projects chrome (green `Projects` header + `/ to filter`) — not a stripped page; the banner sits on top.
> §6.1: Projects section header — `Projects` (`state.green`) + count (`text.detail`) on the left; the `/ to filter` hint on the right (the full chrome the banner sits over).
> §2.9: `accent.violet` (mode bar / banner), `accent.orange` (the command chip); chips are `text.primary` on a tint as a general rule, but §11.4 explicitly specifies the command in an `accent.orange` chip for this banner.
> Current code (`internal/tui/model.go`): `View()` `case PageProjects: if m.commandPending` inserts `statusLine := "Select project to run: " + strings.Join(m.command, " ")` after the list title. `commandPendingHelpKeys()` returns `enter run here` / `n new in cwd` / `q quit` — restyle to the spec footer `⏎ run here · n run in cwd · esc cancel`. `projectFooterBindings(&m.projectList, m.commandPending)` already swaps in `commandPendingHelpKeys` when command-pending (parity of the binding source). The `if m.commandPending { return m, nil }` guards on `s`/`x`/`e`/`d` in `updateProjectsPage` stay (parity). The dispatched actions (Enter run-here, n run-in-cwd, Esc cancel) are parity-preserved.
> Routes through the task-4-1 single-slot arbiter (persistent violet band) + the F10 recompute. Depends on Phase 1 (§2.9 tokens, NO_COLOR carve-out), Phase 3 task 3-2 (Projects chrome), and task 4-1.
> Per §15.4 the implementer produces the `vhs` capture and self-checks it against `Projects — command pending (MV)` before handoff.

**Spec Reference**: §11.4 (command-pending banner), §6.1 (Projects section header / full chrome), §2.5 (NO_COLOR band carve-out), §2.9 (`accent.violet` / `accent.orange` tokens), §15.1 (`Projects — command pending (MV)`). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-5 | approved

### Task spectrum-tui-design-4-5: Empty sessions + empty projects states reskin — centred glyph/message/hint + replaced footer from the keymap descriptor

**Problem**: An empty Sessions list (no sessions exist) and an empty Projects list have no MV empty-state treatment today — they render as a bare list with the standard footer. The spec (§11.1) requires a centred empty state: a dim block glyph `▌ ▌ ▌` (`text.faint`), `No sessions yet` (`text.primary`), and a hint `Press n to start one in the current directory · x for projects` (`text.detail`) — with the footer REPLACED by the keys relevant when there are no sessions (`n new in cwd · x projects · / filter · ? help`, drawn from the page's full keymap §12.1, not a subset of the standard footer). Empty projects mirrors it with `No projects yet` + an open-a-directory hint.

**Solution**: Render the centred empty-sessions and empty-projects states reusing the Phase 2 task 2-9 centred-empty-state pattern (the same centring + token treatment as the no-matches state), but as a distinct surface ("no sessions/projects exist", no active query). The standard footer is replaced — not subset — with the no-sessions-relevant keys drawn from the page's keymap descriptor (§12.1). Empty projects mirrors the same pattern with its own message + hint.

**Outcome**: When the Sessions list is empty, a centred `▌ ▌ ▌` glyph (`text.faint`), `No sessions yet` (`text.primary`), and the hint `Press n to start one in the current directory · x for projects` (`text.detail`) render, with the footer replaced by `n new in cwd · x projects · / filter · ? help`; empty projects mirrors it with `No projects yet` + an open-a-directory hint and the projects-relevant replaced footer; the one-row-per-delegate pagination invariant is unperturbed.

**Do**:
- In `internal/tui/model.go`, add an empty-sessions render branch in `viewSessionList` (gated on the session list being empty AND not filtering — distinct from the no-matches state, which has an active query). Reuse the centred-empty-state helper introduced by Phase 2 task 2-9 (the `⌀`/`No sessions match` no-matches renderer) — same centring + token treatment — passing the empty-sessions content: glyph `▌ ▌ ▌` in `text.faint`, message `No sessions yet` in `text.primary`, hint `Press n to start one in the current directory · x for projects` in `text.detail`. No literal hex — colours from §2.9 tokens. Source the message + hint strings as constants.
- Replace the footer for the empty-sessions state with the no-sessions-relevant keys `n new in cwd · x projects · / filter · ? help`, drawn from the page's full keymap descriptor (§12.1) — NOT a subset of the standard condensed footer. Select the four relevant bindings (`n`, `x`, `/`, `?`) from the Sessions keymap descriptor (the single-source descriptor from Phase 2 task 2-1) and render them through the same footer renderer (`accent.blue` glyphs / `text.detail` labels / `?` glyph `accent.violet`). The footer is fully replaced, not the standard footer with items hidden.
- In `viewProjectList`, add the mirroring empty-projects branch (gated on the projects list being empty AND not filtering): centred glyph (mirror the sessions glyph treatment), message `No projects yet` (`text.primary`), an open-a-directory hint (`text.detail`) — source the projects message + hint as constants. Replace the footer with the projects-relevant keys drawn from the Projects keymap descriptor (§12.1 Projects keymap, the single-source descriptor from Phase 3 task 3-3). Empty projects mirrors the pattern; it is not separately mocked.
- Preserve the one-row-per-delegate pagination invariant: the empty state replaces the list body (the list has zero delegate rows), so there is nothing to overflow; ensure the centred block is sized against the list's height budget exactly as the no-matches state is (reuse 2-9's sizing).
- Confirm the empty state renders only when the underlying list is genuinely empty (no items), distinct from the no-matches state (items exist, query filters to zero) — the two are separate surfaces.
- Under `NO_COLOR` the empty state renders colourless on the native bg (inherited from the Phase 1 carve-out + the 2-9 pattern) — the glyph + message + hint stay legible.

**Acceptance Criteria**:
- [ ] The empty-sessions state renders a centred `▌ ▌ ▌` glyph (`text.faint`), `No sessions yet` (`text.primary`), and the hint `Press n to start one in the current directory · x for projects` (`text.detail`); no literal hex (colours from §2.9 tokens).
- [ ] The empty-sessions footer is fully REPLACED by `n new in cwd · x projects · / filter · ? help` drawn from the Sessions keymap descriptor (§12.1) — not a subset of the standard footer.
- [ ] The empty-projects state mirrors the pattern: centred glyph + `No projects yet` (`text.primary`) + open-a-directory hint (`text.detail`) + a replaced footer drawn from the Projects keymap descriptor; not separately mocked.
- [ ] The empty state renders only when the list is genuinely empty (no items), distinct from the no-matches state (Phase 2 task 2-9, query filters to zero).
- [ ] The empty state reuses the Phase 2 task 2-9 centred-empty-state pattern (same centring + sizing); the one-row-per-delegate pagination invariant is unperturbed.
- [ ] Under `NO_COLOR` the empty state renders colourless on the native bg with the glyph + message + hint legible.
- [ ] vhs verification: a `vhs` tape drives the TUI to the empty-sessions state (seed zero sessions) and writes `testdata/vhs/sessions-empty.png`; the capture is compared against the named Paper frame `Sessions — empty (MV)` (§15.1) for layout / structure / colour-role; empty projects mirrors (not separately mocked). Behaviour parity vs the pre-reskin empty-list rendering (the list still functions; keys still dispatch) is confirmed (§1).

**Tests**:
- `"it renders the centred ▌ ▌ ▌ glyph + 'No sessions yet' + hint for the empty sessions state"`
- `"it replaces the footer with 'n new in cwd · x projects · / filter · ? help' from the keymap descriptor"` (edge: footer fully replaced, drawn from §12.1 descriptor, not a subset)
- `"it renders the empty-projects state with 'No projects yet' + an open-a-directory hint and the projects-relevant replaced footer"` (edge: empty projects mirrors with own message/hint, not separately mocked)
- `"it renders the empty state only when the list has zero items"` (edge: renders only when list is empty)
- `"it keeps the empty state distinct from the no-matches state (items exist + active query)"` (edge: distinct from no-matches state)
- `"it reuses the 2-9 centred-empty-state pattern without perturbing the one-row-per-delegate invariant"` (edge: reuses 2-9 pattern, pagination invariant holds)
- `"it renders the empty state legibly under NO_COLOR on the native bg"`

**Edge Cases**:
- Renders only when the list is empty (zero items), distinct from the no-matches state (Phase 2 task 2-9, where items exist and a query filters to zero).
- Footer fully replaced (not a subset) drawn from the keymap descriptor (§12.1).
- Empty projects mirrors with its own message/hint (not separately mocked).
- Reuses the Phase 2 task 2-9 centred-empty-state pattern; the one-row-per-delegate invariant holds.

**Context**:
> §11.1 (empty states): empty sessions — centred a dim block glyph `▌ ▌ ▌` (`text.faint`), `No sessions yet` (`text.primary`), hint `Press n to start one in the current directory · x for projects` (`text.detail`); the footer is replaced by the keys relevant with no sessions — `n new in cwd · x projects · / filter · ? help` (drawn from the page's full keymap §12.1, NOT a subset of the standard footer). Empty projects mirrors it — `No projects yet` + an open-a-directory hint (same pattern; not separately mocked).
> §12.1 Sessions keymap: `↑`/`↓` move · `Ctrl+↑`/`Ctrl+↓` page · `/` filter · `Enter` attach · `Space` preview · `s` cycle grouping · `r` rename · `k` kill · `n` new-in-cwd · `x` → Projects · `q` quit · `Esc`. §12.1 Projects keymap: `↑`/`↓` move · `Ctrl+↑`/`Ctrl+↓` page · `/` filter · `Enter` new-session-from-project · `x` → Sessions · `e` edit · `d` delete · `n` new-in-cwd · `q` quit · `Esc`. The replaced footer draws its keys from these descriptors.
> §7.3 (over-filtered no-matches): the centred `⌀` no-matches state (Phase 2 task 2-9) — the pattern this task reuses for a distinct surface (empty-list vs filtered-to-zero).
> Reuse the centred-empty-state pattern from Phase 2 task 2-9 (the no-matches renderer): same centring + token + sizing treatment; this is "no sessions exist" with no active query. The footer-replacement draws from the keymap descriptor (Phase 2 task 2-1 Sessions descriptor / Phase 3 task 3-3 Projects descriptor).
> Current code (`internal/tui/model.go`): `viewSessionList` / `viewProjectList` render the list + footer with no empty-state branch. Add the empty branches gated on an empty (non-filtering) list.
> Depends on Phase 1 (§2.9 tokens, NO_COLOR carve-out), Phase 2 task 2-9 (centred-empty-state pattern) + task 2-1 (Sessions descriptor), Phase 3 task 3-3 (Projects descriptor).
> Per §15.4 the implementer produces the `vhs` capture and self-checks it against `Sessions — empty (MV)` before handoff.

**Spec Reference**: §11.1 (empty states), §12.1 (per-screen keymaps — footer key source), §7.3 (no-matches state — the reused pattern), §2.5 (NO_COLOR), §2.9 (`text.faint` / `text.primary` / `text.detail` tokens), §15.1 (`Sessions — empty (MV)`). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-6 | approved

### Task spectrum-tui-design-4-6: Preview overlay chrome reskin (peek mode) — accent.cyan top bar + cyan content frame, captured ANSI content untouched

**Problem**: The read-only Preview overlay's chrome in `internal/tui/pagepreview.go` is hand-composed in a pre-MV form: a hand-built top-edge row carrying counters (`Window M of N · Pane M of N`) and the keymap embedded in the border (`verboseKeymap`/`compactKeymap`), with a single `previewBorderColor` (migrated from `AdaptiveColor` to an explicit value in Phase 1 task 1-2) framing all four edges. The spec (§9.1) requires the MV "peek mode" chrome: a top bar `⊙ preview` (`accent.cyan`) + `<session>` (`text.primary`) + `Window x/y · Pane x/y` (`text.detail`) with right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` (`text.detail`), and a cyan border (`accent.cyan`) framing the read-only content area — while the captured ANSI content stays untouched (the documented palette exception, §9.2).

**Solution**: Restyle the preview chrome to the §9.1 peek-mode treatment: point `previewBorderColor` at the `accent.cyan` token; compose the top bar with the `⊙ preview` marker (`accent.cyan`), the session name (`text.primary`), and the `Window x/y · Pane x/y` counters (`text.detail`), with the right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` (`text.detail`); frame the content area with the cyan border. The captured content area is left as untouched real ANSI (only the chrome is themed). Scroll/nav/attach/back behaviour and the width-cascade tiers are parity-preserved.

**Outcome**: The Preview overlay renders the `accent.cyan` peek-mode chrome (`⊙ preview` top bar with session + `Window x/y · Pane x/y` + right-aligned nav hints, cyan content frame), the captured ANSI content area is unchanged real ANSI, and every preview key (scroll, `Tab`, `]`/`[`, `Enter` attach, `Space`/`Esc` back) behaves identically to before.

**Do**:
- In `internal/tui/pagepreview.go`, point `previewBorderColor` at the `accent.cyan` token (Phase 1 §2.9). The Phase 1 task 1-2 v2 migration already converted it from `lipgloss.AdaptiveColor` to an explicit colour; this task re-targets it at the token. No literal hex — the cyan comes from the §2.9 `accent.cyan` token.
- Restyle the top bar to the §9.1 layout: a `⊙ preview` marker in `accent.cyan`, then `<session>` (`m.session`) in `text.primary`, then `Window x/y · Pane x/y` counters in `text.detail`, with right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` in `text.detail`. This is a presentation change to the hand-composed chrome (`composeChromeLine`/`composeChromeLineParts`/`selectChromeTier` + the `verboseKeymap`/`compactKeymap` constants): update the segment content + glyphs + token colours to match §9.1 (the counters become `Window x/y · Pane x/y`, the right-aligned hints become the §9.1 nav set, the `⊙ preview` + session marker is added). Preserve the width-cascade tier mechanism (truncate name → drop segment → compact → collapse) so narrow widths still degrade gracefully — re-style the tiers, do not remove them.
- Frame the read-only content area with the cyan border (`accent.cyan` via `previewBorderColor`) — the existing `bodyBorderStyle` (`lipgloss.RoundedBorder()` with `BorderForeground(previewBorderColor)`) re-targets at the cyan token. The content area itself stays the untouched real ANSI (`injectSGRResets(m.viewport.View())`) — only the chrome (top bar + frame) is themed (§9.2). Do NOT theme the captured content.
- Confirm the Preview overlay is a FULL-SCREEN OVERLAY, not a modal — the §8.1 blank-screen rule does NOT apply (§9). The reskin must not route preview through the blank-screen modal path (Phase 3 task 3-1); preview remains its own full-screen overlay reached by `Space`.
- Preserve all preview key behaviour byte-for-byte (parity): scroll `↑↓` + `Ctrl+↑/↓`, `Tab` next pane, `]`/`[` window, `⏎` attach (this pane), `Space`/`Esc` back (§9.3 / §12.1) — these live in `previewModel.Update`; the reskin touches only `View()` and the chrome composers, not the key handling. Confirm `currentRawIndices` (raw tmux indices for attach) and the `degenerate` single-window/pane no-op behaviour stay intact.
- Under `NO_COLOR`, the chrome renders colourless on the native bg (the Phase 1 carve-out + lipgloss/termenv handling); the cyan chrome degrades to colourless but the structure (top bar + frame + glyphs) stays present (§9.2 / §2.5).
- The cyan-on-canvas contrast is covered by the §2.9 re-verification pass (Phase 1 task 1-9) — confirm the `accent.cyan` chrome clears the floor against the canvas it renders on (the chrome margins paint `canvas`; the content area is untouched ANSI).

**Acceptance Criteria**:
- [ ] The top bar renders `⊙ preview` (`accent.cyan`) + `<session>` (`text.primary`) + `Window x/y · Pane x/y` (`text.detail`) with right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` (`text.detail`); no literal hex (colours from §2.9 tokens).
- [ ] The read-only content area is framed by a cyan border (`accent.cyan` via `previewBorderColor`); the captured ANSI content is left untouched (only the chrome is themed — §9.2).
- [ ] The Preview is a full-screen overlay, not a modal — the §8.1 blank-screen rule does NOT apply; preview is not routed through the blank-screen modal path.
- [ ] Scroll (`↑↓`/`Ctrl+↑↓`), `Tab` next pane, `]`/`[` window, `Enter` attach (this pane), `Space`/`Esc` back behave identically to the pre-reskin implementation (parity — `previewModel.Update` unchanged).
- [ ] The width-cascade tiers are preserved (re-styled, not removed) so narrow widths still degrade gracefully.
- [ ] `previewBorderColor` points at the `accent.cyan` token (re-targeting the Phase 1 task 1-2 explicit-colour migration).
- [ ] Under `NO_COLOR` the chrome renders colourless on the native bg with structure (top bar + frame + glyphs) intact; the cyan-on-canvas contrast clears the §2.9 floor.
- [ ] vhs verification: a `vhs` tape drives the TUI to the Preview overlay (`Space` on a seeded session) and writes `testdata/vhs/preview-screen.png`; the capture is compared against the named Paper frame `Preview Screen (MV)` (§15.1) for layout / structure / colour-role. Behaviour parity vs the pre-reskin preview (scroll/nav/attach/back, content untouched) is confirmed (§1).

**Tests**:
- `"it renders the top bar as ⊙ preview (cyan) + session (primary) + Window x/y · Pane x/y (detail) with right-aligned nav hints"`
- `"it frames the content area with the accent.cyan border and leaves the captured ANSI content untouched"` (edge: content area real ANSI untouched, only chrome themed §9.2)
- `"it keeps the Preview a full-screen overlay (no §8.1 blank-screen)"` (edge: full-screen overlay NOT a modal)
- `"it preserves scroll / Tab next pane / ]·[ window / Enter attach / Space·Esc back behaviour"` (edge: scroll/pane/window-nav/attach/back parity)
- `"it points previewBorderColor at the accent.cyan token"` (edge: AdaptiveColor → accent.cyan token, Phase 1 v2 migration)
- `"it preserves the width-cascade tiers (re-styled, not removed)"` (edge: width-cascade tiers preserved)
- `"it renders the chrome colourless on the native bg under NO_COLOR with structure intact"` (edge: NO_COLOR colourless chrome on native bg)
- `"it keeps the cyan-on-canvas chrome above the §2.9 contrast floor"` (edge: cyan-on-canvas contrast §2.9)

**Edge Cases**:
- Full-screen overlay, NOT a modal — the §8.1 blank-screen rule does not apply (do not route through the blank-screen modal path).
- Content area is real ANSI, untouched — only the chrome is themed (§9.2).
- Scroll/pane/window-nav/attach/back parity (`Space`/`Esc`, `]`/`[`, `Tab`, `Enter` — `previewModel.Update` unchanged).
- `previewBorderColor` AdaptiveColor → `accent.cyan` token (re-target the Phase 1 task 1-2 explicit-colour migration).
- Width-cascade tiers preserved (re-styled, not removed).
- `NO_COLOR` colourless chrome on native bg.
- Cyan-on-canvas contrast covered by the §2.9 re-verification pass (Phase 1 task 1-9).

**Context**:
> §9 (Preview screen): a full-screen overlay (not a modal — the blank-screen rule of §8.1 does not apply), reached by `Space`. Its chrome is `accent.cyan`-framed to signal "peek mode" — deliberately distinct from the violet main UI.
> §9.1 (chrome): top bar `⊙ preview` (`accent.cyan`) + `<session>` (`text.primary`) + `Window x/y · Pane x/y` (`text.detail`), with right-aligned nav hints `[ ] window · ↹ pane · ⏎ attach · ␣ back` (`text.detail`); a cyan border (`accent.cyan`) frames the read-only content area.
> §9.2 (captured content out-of-theme): the pane content is the real captured ANSI output, rendered read-only — not theme tokens (the documented palette exception). Only the chrome is themed; the content is whatever the pane actually printed. The `canvas` colour paints the preview chrome + surrounding margins; the content area is left as the untouched real ANSI. The cyan chrome's contrast against the canvas is covered by the §2.9 re-verification pass.
> §9.3 (keys): scroll `↑↓` + `Ctrl+↑/↓`; `Tab` next pane; `]`/`[` window; `⏎` attach (this pane); `Space`/`Esc` back — unchanged.
> §2.9: `accent.cyan` carries the Preview chrome (`#7DCFFF` dark / `#0E7490` light, floor 4.5). The one documented palette exception is the Preview scrollback capture (real ANSI), not the chrome.
> Current code (`internal/tui/pagepreview.go`): `previewBorderColor` (was `lipgloss.AdaptiveColor{Light:..., Dark:...}`, migrated to explicit in Phase 1 task 1-2 — now point at `accent.cyan`); the hand-composed chrome `composeChromeLine`/`composeChromeLineParts`/`selectChromeTier` with the `verboseKeymap`/`compactKeymap` constants and the `Window %d of %d · Pane %d of %d` counters — restyle the segments + glyphs + token colours to §9.1 (`⊙ preview` + session + `Window x/y · Pane x/y` + the §9.1 nav hints) while preserving the width-cascade tiers; `View()` composes the top bar + the `bodyBorderStyle` content frame; `previewModel.Update` owns the key handling (do NOT touch — parity). The content stays `injectSGRResets(m.viewport.View())` (untouched ANSI).
> Depends on Phase 1 (§2.9 tokens incl. `accent.cyan`, owned canvas, NO_COLOR carve-out, the v2 migration that made `previewBorderColor` explicit, and the §2.9 cyan-on-canvas re-verification pass — task 1-9).
> Per §15.4 the implementer produces the `vhs` capture and self-checks it against `Preview Screen (MV)` before handoff.

**Spec Reference**: §9 (Preview overlay, not a modal), §9.1 (chrome), §9.2 (captured content out-of-theme), §9.3 (keys), §2.5 (NO_COLOR), §2.9 (`accent.cyan` token + re-verification pass), §15.1 (`Preview Screen (MV)`). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-4-7 | approved

### Task spectrum-tui-design-4-7: Preview `?` help wiring (Phase-3 carry) — Preview keymap descriptor + `?` overlays the generic help renderer without blanking the preview

**Problem**: Phase 3 task 3-4 built the generic `?` help modal (a help-modal type + a generic two-column renderer over the per-page keymap descriptor) and bound `?` on Sessions and Projects, but EXPLICITLY deferred the Preview keymap descriptor and the Preview `?`-help wiring to Phase 4 (the §8.5 / §9.3 / §8.1 exception case). Today `previewModel.Update` in `internal/tui/pagepreview.go` has no `?` handling, so there is no help on the Preview overlay. The spec requires a `?` help opened from Preview to OVERLAY the preview (not blank it — the §8.1 blank-screen exception), driven by the Preview keymap descriptor added to the single-source descriptor type, reusing the 3-4 generic renderer.

**Solution**: Add the Preview keymap descriptor (§12.1 Preview keymap) to the single-source descriptor type (the Phase 2 task 2-1 / Phase 3 task 3-3 type that drives footers + help), and bind `?` on the Preview overlay to open the Phase 3 task 3-4 generic help renderer as an OVERLAY over the preview content (not a blank-screen clear). Toggle-close on `?`/`Esc`, key-exclusive — `Esc` dismisses the help and does NOT fall through to the preview "back" action. No hand-authored Preview help copy — the renderer is generated from the descriptor.

**Outcome**: Pressing `?` on the Preview overlay opens the generic help modal listing Preview's complete keymap (scroll, `Tab`, `]`/`[`, `Enter` attach, `Space`/`Esc` back) overlaid on the preview WITHOUT blanking it; pressing `?` again or `Esc` dismisses the help (and `Esc` while help is open does not trigger the preview "back"); the content is descriptor-driven with no hand-authored Preview copy.

**Do**:
- Add the Preview keymap descriptor to the single-source descriptor type (introduced in Phase 2 task 2-1, extended in Phase 3 task 3-3). The Preview keymap (§12.1): scroll `↑`/`↓` + `Ctrl+↑`/`Ctrl+↓`, `Tab` next pane, `]`/`[` window, `Enter` attach (this pane), `Space`/`Esc` back. This descriptor is the single source for the Preview help content (and any Preview footer/hint) — no separate copy to author.
- In `internal/tui/pagepreview.go` `previewModel.Update`, add a `?` case that toggles an in-overlay help flag on `previewModel` (e.g. `helpOpen bool`). When opening, the help renders over the preview via the Phase 3 task 3-4 generic help renderer fed the Preview descriptor. Add help-open state to `previewModel` and render the overlay in `previewModel.View()` (the help draws on top of the existing composed preview frame — NOT a blank-screen clear).
- The help OVERLAYS the preview (does not blank it — §8.1 exception / §9). Render the help modal composited over the preview's existing View output (reuse the overlay compositing primitive from `internal/tui/modal.go` `renderModal` — the foreground-over-background overlay path, NOT the Phase 3 task 3-1 blank-screen clear path). The preview content stays visible behind the help (per §8.1 the Preview help is the documented exception that overlays rather than blanks).
- Make the help key-exclusive while open: when `helpOpen` is true, `previewModel.Update` consumes `?` (toggle-close) and `Esc` (dismiss help) and does NOT let `Esc` fall through to the preview-back action (`previewDismissedMsg`). All other preview keys are inert while help is open (key-exclusive, §8.1). When help is closed, `Esc`/`Space` resume their normal "back" behaviour.
- Reuse the Phase 3 task 3-4 generic descriptor-driven renderer verbatim — pass it the Preview descriptor; do NOT hand-author Preview help content. The header (`? Keybindings`) + right-aligned `esc close` and the two-column glyph/action layout (`accent.blue` glyph / `text.strong` action) come from the shared renderer.
- Confirm the help lists Preview's COMPLETE keymap (§12.1), including keys not shown in any footer (the help is the full reference per §8.5).
- Under `NO_COLOR`, the help overlay renders colourless (inherited from the Phase 3 renderer + the Phase 1 carve-out) over the preview.

**Acceptance Criteria**:
- [ ] Pressing `?` on the Preview overlay opens the help modal listing Preview's complete keymap (scroll, `Tab`, `]`/`[`, `Enter` attach, `Space`/`Esc` back) from the Preview descriptor.
- [ ] The help OVERLAYS the preview without blanking it (the §8.1 exception) — the preview content stays visible behind the help; the help is NOT routed through the Phase 3 task 3-1 blank-screen clear path.
- [ ] The help reuses the Phase 3 task 3-4 generic descriptor-driven renderer — no hand-authored Preview copy; the `? Keybindings` header + `esc close` + two-column layout come from the shared renderer.
- [ ] The help is key-exclusive: `?` toggle-closes, `Esc` dismisses the help and does NOT fall through to the preview "back" action; other preview keys are inert while help is open.
- [ ] The Preview keymap descriptor is added to the single-source descriptor type (the type that drives footers + help); the help lists the complete keymap (§12.1), including keys not in any footer.
- [ ] Under `NO_COLOR` the help overlay renders colourless over the preview.
- [ ] vhs verification: a `vhs` tape drives the TUI to the Preview overlay then presses `?` and writes `testdata/vhs/preview-help.png`; Preview help is not separately mocked (§15.1 frame map) — the capture is verified for the overlay-without-blanking behaviour (preview visible behind the help) and the descriptor-driven content (the audited Preview keymap), following the audited Preview keymap + the 3-4 renderer. Behaviour parity vs the pre-reskin preview keys (when help is closed) is confirmed (§1).

**Tests**:
- `"it opens the Preview help listing the complete Preview keymap from the descriptor on ?"` (edge: descriptor lists Preview's complete keymap §12.1)
- `"it overlays the help over the preview without blanking it"` (edge: help overlays preview without blanking, distinct from blank-screen modal path)
- `"it reuses the 3-4 generic descriptor-driven renderer (no hand-authored Preview copy)"` (edge: reuses 3-4 renderer)
- `"it toggle-closes the help on a second ?"` (edge: toggle-close on ?)
- `"it dismisses the help on Esc and does not fall through to the preview back action"` (edge: Esc dismisses help, not fall-through to preview back, key-exclusive)
- `"it keeps other preview keys inert while help is open"` (edge: key-exclusive while help open)
- `"it resumes normal Esc/Space back behaviour when help is closed"` (edge: parity of preview back when help closed)
- `"it renders the help overlay colourless under NO_COLOR over the preview"`

**Edge Cases**:
- Help overlays the preview without blanking (distinct from the Phase 3 task 3-1 blank-screen modal path — §8.1 exception / §9).
- Reuses the Phase 3 task 3-4 generic descriptor-driven renderer (no hand-authored Preview copy).
- Toggle-close on `?`/`Esc`, key-exclusive: `Esc` dismisses the help, does NOT fall through to the preview "back".
- The descriptor lists Preview's complete keymap (§12.1).
- Preview help not separately mocked (follows the audited keymap — §15.1).

**Context**:
> §9.3 + §8.5: a `?` help opened from Preview OVERLAYS the preview — does NOT blank it (the §8.1 exception). This is the Phase-3-deferred Preview help wiring (Phase 3 task 3-4 deferred the Preview descriptor + the Preview `?` wiring to Phase 4).
> §8.5: the help modal is generated from the page's keymap descriptor — the single source of truth that also drives the footer and §12.1 — not hand-authored per page. Opened from Preview, it overlays the preview (doesn't blank it — §9). The help modal closes on `?` (toggle) or `Esc`; while open it is key-exclusive (§8.1), so `Esc` dismisses it and does not fall through to the page's back action.
> §8.1: modals render on a blank screen, EXCEPT the Preview screen — a `?` help opened from Preview overlays the preview without blanking it.
> §12.1 Preview keymap: `↑`/`↓` + `Ctrl+↑`/`Ctrl+↓` scroll · `Tab` next pane · `]`/`[` window · `Enter` attach (this pane) · `Space`/`Esc` back.
> §14.4 / Phase 3 task 3-4: the generic `?` help renderer over the per-page keymap descriptor (the single source of truth) + the help-modal type already exist; this task adds the Preview descriptor and wires `?` on Preview to that renderer as an overlay.
> Current code (`internal/tui/pagepreview.go`): `previewModel.Update` has no `?` handling today (Phase 3 task 3-4 deferred it). `internal/tui/modal.go` `renderModal` is the foreground-over-background overlay compositing primitive (the path the Preview help uses — overlay, NOT blank-screen). The single-source keymap descriptor type lives in the `tui` package (introduced in Phase 2 task 2-1, extended in Phase 3 task 3-3).
> Depends on Phase 2 task 2-1 (descriptor type), Phase 3 task 3-3 (descriptor extension) + task 3-4 (generic help renderer + help-modal type), Phase 1 (NO_COLOR carve-out), and task 4-6 (the restyled Preview chrome the help overlays).
> Per §15.4 the implementer produces the `vhs` capture (Preview-with-help-overlay) and self-checks it for the overlay-without-blanking behaviour + descriptor-driven content before handoff (Preview help is not separately mocked — §15.1).

**Spec Reference**: §9.3 (Preview keys + `?` overlay), §8.5 (`?` help generated from descriptor, key-exclusive, Preview overlays), §8.1 (blank-screen rule + the Preview overlay exception), §12.1 (Preview keymap), §14.4 (generic help renderer over the descriptor), §2.5 (NO_COLOR), §15.1 (Preview help not separately mocked). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`
