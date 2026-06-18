---
phase: 2
phase_name: Shared chrome + Sessions surfaces (flat, grouped, filtering)
total: 9
---

## spectrum-tui-design-2-1 | approved

### Task spectrum-tui-design-2-1: Sessions keymap descriptor (footer/help data source) + §12.2 keymap revision

**Problem**: Today the Sessions footer is derived ad hoc from `sessionHelpKeys()` (`internal/tui/model.go`) which carries the wrong/legacy keymap: a `p` alias for Projects (`key.WithHelp("p/x", "projects")`), and the binding set predates the §12.2 revision. There is no single keymap descriptor that drives both the footer (§3.4) and the future `?` help modal (§8.5 / §14.4) — the spec requires one source of truth. The §12.2 keymap revision (arrows-only nav, `k`=kill, de-overload `s`/`x`, drop the `p` Sessions→Projects alias, no uppercase) must land here because the new footer (task 2-4) consumes the descriptor, and the dispatch in `updateSessionList` must be brought into agreement so each key has exactly one meaning.

**Solution**: Introduce a per-page **keymap descriptor** — an ordered, declarative model of every Sessions binding (key glyph + action label + a flag marking whether it appears in the condensed footer "core" set vs help-only) — as the single source of truth that will drive the footer (task 2-4) and the `?` help modal (Phase 3). Apply the §12.2 revision to both the descriptor and the live `updateSessionList` key dispatch: remove the `p` rune case (so `x` is the sole Sessions⟷Projects toggle and `s` is Sessions-only cycle), confirm navigation is arrows + `Ctrl+↑/↓` only with no vim/uppercase aliases reaching Sessions, and model the `? help` hint in the descriptor without binding `?` (the `?` swallow stays until Phase 3 binds it).

**Outcome**: A Sessions keymap descriptor exists and enumerates exactly the §12.1 Sessions keymap; the descriptor distinguishes the §3.4 core-footer keys (`↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` + right-aligned `? help`) from the help-only keys (`n`/`r`/`k`/`q`/paging); the live dispatch no longer has a `p` Sessions→Projects case; every retained binding behaves byte-identically to today.

**Do**:
- In `internal/tui/`, add a keymap-descriptor type (e.g. a `keymapEntry` with `Key string` glyph, `Action string` label, `Core bool` for footer-core membership, and a `RightAligned bool` for the trailing `? help`) and a `sessionsKeymap()` constructor returning the ordered Sessions entries. Glyphs/labels must match §3.4/§12.1 exactly: `↑↓ navigate`, `⏎ attach`, `/ filter`, `␣ preview`, `s switch view`, `x projects` (core); `? help` (core, right-aligned); `n new in cwd`, `r rename`, `k kill`, `q quit`, `Ctrl+↑/↓ page` (help-only, `Core:false`).
- Keep the descriptor a pure data source — no rendering here (task 2-4 renders the footer from it). It must be the single source the §8.5 help modal will also consume; do not duplicate the binding list.
- In `internal/tui/model.go`, revise the `updateSessionList` rune dispatch (~lines 1997-2028): delete the `case isRuneKey(msg, "p")` arm so `p` no longer flips to Projects; leave the `case isRuneKey(msg, "x")` arm as the sole Sessions→Projects toggle and `case isRuneKey(msg, "s")` as the Sessions-only cycle (it must stay below the `if m.sessionList.SettingFilter() { break }` guard so `s` remains a literal filter char while the `/` input is focused — unchanged).
- Remove/replace `sessionHelpKeys()`'s `p/x` binding so the descriptor is authoritative; if `sessionHelpKeys()` is still consumed by the old three-column footer (it is, via `sessionFooterBindings`), keep the footer compiling for now but ensure the `p` rune is gone from the displayed labels — the descriptor is the forward path and task 2-4 retires the old footer plumbing. Do NOT bind `?` here; leave the `if isRuneKey(msg, "?") { return m, nil }` swallow in place (Phase 3 binds it).
- Audit the Sessions dispatch against §12.2: confirm no `h`/`j`/`k`(nav)/`l`/`g`/`G`/`PgUp`/`PgDn`/`Home`/`End`/uppercase bindings reach Sessions. `k` must dispatch kill (it already does via `handleKillKey`). Navigation/paging come from `bubbles/list`'s `KeyMap` — verify the list's nav keymap does not re-introduce vim aliases after the Phase 1 v2 upgrade; if it does, unbind them on the Sessions list so move is `↑/↓` and page is `Ctrl+↑/↓` only.

**Acceptance Criteria**:
- [ ] A Sessions keymap descriptor exists and is the single declarative source for the Sessions footer and (later) help content; it enumerates the §12.1 Sessions keymap with core vs help-only classification matching §3.4.
- [ ] The `p` Sessions→Projects rune case is removed from `updateSessionList`; `x` toggles Sessions⟷Projects and `s` is the Sessions-only cycle — each key has exactly one meaning.
- [ ] No vim aliases (`h`/`j`/`k`-as-up/`l`/`g`/`G`), no `PgUp`/`PgDn`/`Home`/`End`, and no uppercase bindings are dispatchable on Sessions; `k` dispatches kill; navigation is `↑/↓`, paging is `Ctrl+↑/↓`.
- [ ] `?` is NOT bound (the swallow remains); the descriptor models `? help` for the footer hint only.
- [ ] Behaviour parity: every retained action (`Enter`, `Space`, `/`, `s`, `x`, `r`, `k`, `n`, `q`, `Esc`, `Ctrl+C`, nav, paging) dispatches to the same handler/result as before this task (traced against the pre-change `updateSessionList`).
- [ ] PRIMARY VISUAL ACCEPTANCE: this task has no standalone rendered surface of its own (the descriptor is data). Its visible effect is the footer, rendered in task 2-4. Defer the `vhs`+frame capture to task 2-4 (which is the descriptor's first consumer); 2-1's own acceptance is the binding-layer parity + descriptor-as-single-source above. State this explicitly in the handoff: no `vhs` capture is produced by 2-1.

**Tests**:
- `"it returns the Sessions keymap descriptor enumerating exactly the §12.1 Sessions bindings"`
- `"it marks the §3.4 core-footer keys (↑↓/⏎/// ␣/s/x and right-aligned ? help) as Core and the rest (n/r/k/q/paging) as help-only"`
- `"it no longer dispatches p to the Projects page (p is inert / not a page toggle)"`
- `"it dispatches x to the Projects page (sole Sessions→Projects toggle)"`
- `"it dispatches s to the grouping cycle and treats s as a literal filter character while the / input is focused"` (edge case: `s` literal while filter focused — unchanged)
- `"it excludes filter-mode bindings from the core footer set"` (edge case: filter-mode bindings excluded from core footer)
- `"it has no uppercase or vim-alias bindings dispatchable on Sessions"` (edge case: no uppercase aliases)
- `"it dispatches k to kill, r to rename, n to new-in-cwd, Enter to attach, Space to preview unchanged"` (parity of every dispatched action)
- `"it leaves the ? swallow in place (? is not bound to open help)"`

**Edge Cases**:
- `s` must remain a literal filter character while the `/` filter input is focused (the dispatch case stays below the `SettingFilter()` guard) — unchanged behaviour.
- Filter-mode bindings (clear-filter, accept-while-filtering) are not part of the condensed core footer set.
- No uppercase aliases anywhere; dropping `p` must not orphan the Projects toggle (`x` covers it both directions — Projects→Sessions `x` already exists in `updateProjectsPage`).
- Parity: a dispatched action's downstream effect (modal opened, page flipped, cmd returned) must be unchanged for every key that is retained.

**Context**:
> §12 keybindings are audited against code; §12.2 lists the deliberate changes: drop all vim aliases (`h`/`j`/`k`/`l`, `g`/`G`) and `PgUp`/`PgDn`/`Home`/`End`; `k`=kill; no uppercase; `x` toggles Sessions⟷Projects both directions; `s` is Sessions-only cycle; drop the former `p` (Sessions→Projects) and `s` (Projects→Sessions) aliases so each key has one meaning. §8.5/§14.4: the per-page keymap descriptor is the single source of truth that drives footer AND help; introduce it here because the footer (task 2-4) is its first consumer. The `?` binding + help modal are Phase 3 — the `?` swallow (`if isRuneKey(msg, "?") { return m, nil }`) stays until then; the descriptor only models `? help` for the footer hint. Current code: `internal/tui/model.go` `sessionHelpKeys()` (~line 612) holds the `p/x` binding to revise; the `updateSessionList` rune switch (~line 1997-2028) holds the `p`/`s`/`x` dispatch and the `?` swallow. The Projects-side `s` (Projects→Sessions) alias drop is Phase 3's scope (Projects page) — this task only revises the Sessions side; do not touch `updateProjectsPage` beyond what is needed to keep `x` working both directions (it already handles `x`).

**Spec Reference**: §12 (keybindings, audited), §12.1 (per-screen Sessions keymap), §12.2 (keymap revision), §3.4 (footer core keys), §8.5 (help generated from the keymap descriptor), §14.4 (the descriptor as single source of truth driving footer + help). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-2 | approved

### Task spectrum-tui-design-2-2: Header block — PORTAL wordmark + violet caret + subtitle + 2px separator rule

**Problem**: Portal's TUI has no header/wordmark block today — the Sessions view begins straight at the `bubbles/list` title. §3.1 calls for a shared header (uppercase letter-spaced `PORTAL` wordmark + violet `▌` caret + right-aligned `session manager` subtitle + full-width 2px separator rule) above the list. This is the one genuinely new-but-small chrome element (§14.3), and it must be added without breaking the load-bearing one-row-per-delegate pagination invariant: any rows the header consumes must be subtracted from the list's height budget (§3.5).

**Solution**: Build a header-block renderer (a Lipgloss `JoinVertical` of the wordmark+caret line, the right-aligned subtitle, and the 2px separator rule) consuming the Phase 1 §2.9 tokens (`text.primary` wordmark, `accent.violet` caret, `text.detail` subtitle, `border.separator` rule) and the owned canvas. Compose it above the Sessions list in `viewSessionList`, and fold its rendered height into the list's size budget so pagination stays exact. Implement the §2.7 narrow degrade (compact wordmark, drop subtitle) below the minimum width.

**Outcome**: The Sessions surface renders a PORTAL header block with violet caret and right-aligned `session manager` subtitle over a full-width 2px separator rule, in MV tokens on the owned canvas; the list's pagination remains exact (one delegate line per row, no overflow); below the minimum width the wordmark collapses to compact and the subtitle drops; the captured `vhs` PNG matches the header region of `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)`.

**Do**:
- Add a header-block render function in `internal/tui/` (new file e.g. `header.go`): render `PORTAL` in `text.primary`, uppercase + letter-spaced (≈0.26em → approximate as a per-glyph space in terminal cells; heavy/bold weight), with a solid block `▌` caret in `accent.violet` immediately to its right; right-align `session manager` in `text.detail` on the same band (use `lipgloss.JoinHorizontal` + a flex spacer sized to terminal width); under that, a full-width rule using the 2px treatment in `border.separator` (terminal "2px" → a heavy/thick horizontal rule line, e.g. a `▔`/`━`/box-drawing run or a 2-row rule — match the Paper frame's weight).
- Reference Phase 1 tokens via the token layer — NO literal hex at the call site (§2.9 closed vocabulary). Leaf styles carry `.Background(canvas)` per Phase 1 so the header paints its own cells; do NOT re-implement the outer full-terminal fill (Phase 1 task 1-6 owns it).
- Compose the header above the list in `viewSessionList` (`internal/tui/model.go` ~line 2329) via `lipgloss.JoinVertical` — header first, then the existing list+flash+signpost+footer block.
- Fold the header height into the list size budget: extend `applyListSize`/`applySessionListSize` (`internal/tui/model.go` ~lines 884-893) to subtract `lipgloss.Height(header)` (in addition to the footer height it already subtracts) so `bubbles/list` paginates against the reduced height and the one-row-per-delegate invariant holds. Verify the header height is counted at every `SetSize` call site (`WindowSizeMsg`, `rebuildSessionList`, construction seed).
- Implement §2.7 narrow degrade progressively: step 1 drop the right-side subtitle, step 2 wordmark→compact form, as width shrinks below the minimum-width threshold (pin the exact thresholds as an implementation detail). The degrade must never overflow.
- Produce the `vhs` capture: extend/add the Sessions tape (the harness from Phase 1 task 1-1) to launch Portal from seeded fixture state, reach the Sessions flat view, `Screenshot` to the harness dir (e.g. `testdata/vhs/sessions-flat.png`, overwritten in place). Compare the header region against the committed Paper reference export of `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)` for layout/structure/colour-role.

**Acceptance Criteria**:
- [ ] The header renders `PORTAL` (uppercase, letter-spaced, heavy, `text.primary`) + an immediately-right `▌` caret (`accent.violet`) + a right-aligned `session manager` subtitle (`text.detail`) over a full-width 2px `border.separator` rule, all via tokens (no literal hex at call sites).
- [ ] The header height is subtracted from the Sessions list height budget at every size-apply call site; the one-row-per-delegate pagination invariant holds (no overflow, no title/cursor scrolled off — re-verify against grouped views' page count too).
- [ ] Narrow degrade: below the minimum width the subtitle drops then the wordmark collapses to compact; the UI never overflows (§2.7 progressive, per-dimension).
- [ ] The header paints on the owned canvas with no edge bleed (leaf `.Background(canvas)`); the outer full-terminal fill from Phase 1 is unmodified.
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape drives the TUI to the Sessions flat view and writes a PNG to the harness dir; the captured header region matches `Sessions — Modern Vivid v2` (dark) and `Sessions — Modern Vivid (Light)` for layout/structure/colour-role (agent/user-judged, not pixel-diff, §15.2); the implementer self-checks the capture against the committed Paper reference before handoff (§15.4).
- [ ] Behaviour parity: adding the header changes only the rendered chrome; list navigation, selection, filtering, and every key dispatch behave identically to pre-task (the only behavioural delta is the recomputed list height, which must keep pagination exact).

**Tests**:
- `"it renders the PORTAL wordmark with a violet caret and right-aligned session-manager subtitle over a 2px separator rule"`
- `"it subtracts the header height from the session list height budget so pagination stays exact"` (edge case: pagination invariant unperturbed)
- `"it counts the header height at every SetSize call site (window resize, rebuild, construction)"`
- `"it drops the subtitle then collapses the wordmark to compact below the minimum width"` (edge case: narrow degrade)
- `"it never overflows the viewport at the minimum supported terminal size"` (edge case: never break)
- `"it paints the header cells on the owned canvas without edge bleed"` (edge case: owned-canvas render)
- `"it leaves list navigation/selection/filtering behaviour unchanged after adding the header"` (parity)

**Edge Cases**:
- Narrow degrade is progressive and per-dimension (§2.7): drop subtitle first, then compact wordmark; a short-but-wide terminal keeps the full wordmark.
- The header must not perturb the one-row-per-delegate pagination invariant — its height is part of the list's height budget, not an uncounted extra band (contrast the historic in-delegate-heading overflow bug).
- Owned-canvas render: leaf `.Background(canvas)` only; do not re-add the outer fill (Phase 1 owns it) — verify no edge bleed on full-width terminals.
- Zero/unset terminal dimensions: fall back to the same default (80x24) used elsewhere so the header still composes without panic.

**Context**:
> §3.1: wordmark `PORTAL` uppercase letter-spaced (≈0.26em) heavy `text.primary`; caret solid block `▌` `accent.violet` immediately right; subtitle right-aligned `session manager` `text.detail` small + letter-spaced; separator rule full-width 2px `border.separator`. §2.7 narrow degrade: wordmark→compact, drop subtitle. §3.6: no full-screen frame — structure is the two horizontal rules + per-element treatment, never a box around the whole UI. §14.3 flags the header/wordmark + separator block as new-but-small (≈ Lipgloss `JoinVertical`). §2.9 tokens are pinned in Phase 1 (light + dark variants); reference them, do not re-derive. The owned canvas (leaf `.Background(canvas)` + outer fill) is Phase 1 task 1-6; this task adds only the leaf-painted header and must not touch the outer fill. Measurements are Paper-frame reference values — exact cell mapping (the "2px" rule weight, the 0.26em letter-spacing) is finalised at implementation against the frame and the real terminal (§3 intro, §15.3). Current `viewSessionList` is `internal/tui/model.go` ~line 2329; size helpers `applyListSize`/`applySessionListSize` ~lines 884-893.

**Spec Reference**: §3.1 (header — wordmark/caret/subtitle/rule), §2.7 (narrow degrade), §3.5 (pagination invariant — height budget), §3.6 (no full-screen frame), §2.9 (token table), §14.3 (new-but-small header block), §15.1/§15.2 (frame + harness). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-3 | approved

### Task spectrum-tui-design-2-3: Section header + count — Sessions label, state.green count, mode suffix, right-aligned `/ to filter` hint

**Problem**: The Sessions section header today is the plain `bubbles/list` title string produced by `sessionListTitleForMode` (`internal/tui/model.go`), styled by the list's default `Styles.Title`. §3.2/§4.2 require a restyled section header directly under the separator rule: the label `Sessions` in `accent.cyan`, the count in `state.green` at the SAME font size distinguished by dim colour not size (§13.6), the mode suffix (`— by project` / `— by tag`) in `text.detail`, and a persistent right-aligned `/ to filter` hint in `text.detail`. The text content (mode suffix) and the inside-tmux `(current: …)` decoration must be preserved.

**Solution**: Restyle the section-header render so the label/count/mode-suffix are token-coloured and a right-aligned `/ to filter` hint sits on the same row. Reuse `sessionListTitleForMode` for the mode-suffix text (parity for the text), but restyle the colour/layout: split the label from the count so each gets its token colour at the same cap-height, and append the right-aligned hint. Implement §2.7 narrow degrade dropping the right-side hint.

**Outcome**: The Sessions section header renders `Sessions` (`accent.cyan`) + count (`state.green`, same font size, dim-by-colour) + optional mode suffix (`text.detail`) on the left and a right-aligned `/ to filter` hint (`text.detail`) on the right, on the owned canvas; the inside-tmux `(current: …)` decoration is preserved; below the minimum width the right hint drops; the capture matches the section-header region of `Sessions — Modern Vivid v2` / `(Light)`.

**Do**:
- Add a section-header render function in `internal/tui/` that takes the active mode (and inside-tmux state) and renders the left cluster — `Sessions` in `accent.cyan`, the live count in `state.green` (count comes from the visible session count, same source the list title/count uses), and the mode suffix `— by project` / `— by tag` in `text.detail` — using `sessionListTitleForMode` (`internal/tui/model.go` ~line 659) as the source of the suffix text so the strings stay parity-identical (including the `(current: %s)` inside-tmux decoration). The count must render at the same font size as the label — distinguished only by its dim `state.green` colour, not by being smaller (§13.6); share the baseline/cap-height (a same-line same-size token, no superscript/smaller glyph).
- Right-align a persistent `/ to filter` hint in `text.detail` on the same row (flex spacer to terminal width). This hint shows on every filterable session view (Flat / by Project / by Tag); `s switch view` lives ONLY in the footer — never duplicate it here (§3.2).
- Restyle the `bubbles/list` `Styles.Title` (and disable the list's own title row if the section header is rendered separately above the list) so the section header is the new token-coloured row rather than the default-styled list title. Decide and document whether the section header is (a) rendered by restyling `Styles.Title` in place or (b) rendered as a separate row above the list with the list's title row suppressed; either is acceptable, but the chosen path must keep the list height budget exact (count any separately-rendered row in `applyListSize`, mirroring task 2-2's header treatment).
- Implement §2.7 narrow degrade: drop the right-side `/ to filter` hint below the threshold (the second width step after the header subtitle), never overflowing.
- All colours via Phase 1 §2.9 tokens (`accent.cyan`, `state.green`, `text.detail`) — no literal hex at the call site. Leaf `.Background(canvas)`.
- Produce the `vhs` capture: drive the Sessions tape to flat view; also capture a grouped view (by project / by tag) so the mode suffix is visible; `Screenshot` to the harness dir, compare the section-header region against `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role.

**Acceptance Criteria**:
- [ ] The section header renders `Sessions` in `accent.cyan`, the count in `state.green` at the SAME font size distinguished by dim colour not size (§13.6 — shares baseline/cap-height), and the mode suffix (`— by project`/`— by tag`) in `text.detail`, all via tokens.
- [ ] A persistent right-aligned `/ to filter` hint renders in `text.detail` on every session view; `s switch view` is NOT duplicated in the section header (footer-only).
- [ ] The mode-suffix text comes from `sessionListTitleForMode` (parity for the text), and the inside-tmux `(current: %s)` decoration is preserved.
- [ ] Narrow degrade drops the right-side hint below the threshold without overflow.
- [ ] If the section header is rendered as a separate row, its height is folded into the list size budget so pagination stays exact (mirrors task 2-2).
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape captures the Sessions flat (and a grouped) view to the harness dir; the section-header region matches `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper reference before handoff.
- [ ] Behaviour parity: the change is cosmetic — the count value, mode suffix text, and inside-tmux decoration are unchanged vs the current `sessionListTitleForMode` output; only colour/layout/the added hint differ.

**Tests**:
- `"it renders Sessions in accent.cyan with the count in state.green at the same font size (dim by colour not smaller)"`
- `"it renders the mode suffix from sessionListTitleForMode in text.detail for by-project and by-tag"` (parity for the suffix text)
- `"it renders a right-aligned / to filter hint in text.detail on flat, by-project, and by-tag views"`
- `"it does not duplicate s switch view in the section header"` (footer-only)
- `"it preserves the inside-tmux (current: <name>) decoration"` (edge case: inside-tmux decoration preserved)
- `"it drops the right-side / to filter hint below the minimum width"` (edge case: narrow degrade)
- `"it keeps the count value and mode suffix text byte-identical to the pre-reskin title"` (parity)

**Edge Cases**:
- The count shares the label's cap-height/baseline and the same font size — it is dim-by-colour, never rendered smaller (§13.6).
- Inside-tmux: `sessionListTitleForMode` appends `(current: %s)`; this decoration must survive the restyle (it is the one documented spec divergence the function already carries).
- Narrow degrade drops the right hint independently of the header subtitle drop (per-dimension width steps).
- The count source must match the visible session count (after inside-tmux exclusion / filtering as today) — do not introduce a new count that disagrees with the list.

**Context**:
> §3.2 section header: page/mode label + count on the left, optional hint on the right; label `accent.cyan` (Sessions) or `state.green` (Projects); mode suffix `— by project`/`— by tag` in `text.detail`; count at the SAME font size as the label, distinguished by dim colour — `state.green` for the Sessions count; right side carries the persistent `/ to filter` hint (`text.detail`) on every filterable view; `s switch view` lives in the footer only (never duplicated here). §4.2: `Sessions` (`accent.cyan`) + count (`state.green`); empty list shows the empty state (§11.1 — Phase 4, out of scope here). §13.6: counts beside labels render at the same font size distinguished by dim colour, sharing baseline + cap-height. §2.7 narrow degrade drops the right-side header hint. `sessionListTitleForMode` (`internal/tui/model.go` ~line 659) produces the three base strings + the inside-tmux `(current: %s)` decoration — reuse it for the suffix text (parity), restyle the colour/layout around it. The empty-state footer swap (§11.1) is Phase 4 — not this task. §2.9 tokens are Phase 1.

**Spec Reference**: §3.2 (section header), §4.2 (Sessions section header + count), §13.6 (counts beside labels — same size, dim colour), §2.7 (narrow degrade), §2.9 (token table), §15.1/§15.2 (frame + harness). `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-4 | approved

### Task spectrum-tui-design-2-4: Condensed footer + right-aligned `? help` — single row of core keys from the keymap descriptor

**Problem**: Today the Sessions footer is a manual three-column keymap built by `renderKeymapFooter` / `chunkBindingsIntoThreeColumns` / `sessionFooterBindings` (`internal/tui/model.go`) — it tries to fit every binding and can't, which is the footer-space problem §3.4 solves. §3.4 requires a single condensed row of exactly the Sessions core keys (`↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects`) with a right-aligned `? help`, keys in `accent.blue`, labels in `text.detail`, `?` glyph in `accent.violet`; `n`/`r`/`k`/`q`/paging move to `?` help and are NOT in the footer. This footer is the first consumer of the keymap descriptor from task 2-1.

**Solution**: Replace the three-column manual footer with a single-row condensed footer rendered from the task 2-1 Sessions keymap descriptor (filtering to `Core:true` entries, rendering the right-aligned `? help`). Token-colour the glyphs (`accent.blue`), labels (`text.detail`), and the `?` glyph (`accent.violet`). Recompute the single-row height budget. Implement §2.7 narrow truncation.

**Outcome**: The Sessions footer renders one row: `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` on the left + a right-aligned `? help`, with glyphs in `accent.blue`, labels in `text.detail`, `?` glyph in `accent.violet`, on the owned canvas above a 1px `border.footer` rule; `s switch view` and `x projects` show on ALL session views including Flat; `n`/`r`/`k`/`q`/paging are absent (help-only); the list height budget is recomputed for the now single-row footer; the capture matches the footer region of `Sessions — Modern Vivid v2` / `(Light)`.

**Do**:
- Add a condensed-footer render function in `internal/tui/` that takes the Sessions keymap descriptor (task 2-1), filters to `Core:true` entries, renders them as a single `·`-separated row (left cluster) with the `RightAligned` `? help` pinned right (flex spacer to terminal width). Render each entry's key glyph in `accent.blue` and its label in `text.detail`; render the `?` glyph specifically in `accent.violet` (the help-hint accent).
- Add a 1px top rule above the footer row in `border.footer` (the §3.4 single bottom row above a 1px top rule). Note: `border.separator` (header, 2px) and `border.footer` (footer, 1px) are two distinct tokens — use `border.footer` here.
- Retire the three-column path for Sessions: replace the `renderKeymapFooter(&m.sessionList, sessionFooterBindings(...))` call in `viewSessionList` (`internal/tui/model.go` ~line 2354) with the new condensed footer. Leave the Projects footer on its existing path for now (Projects footer is Phase 3) — but if `sessionFooterBindings`/`chunkBindingsIntoThreeColumns` become Sessions-unused, keep them only where Projects still needs them; do not break the Projects build.
- Recompute the single-row footer height budget: `applySessionListSize` (`internal/tui/model.go` ~line 891) currently subtracts `lipgloss.Height(renderKeymapFooter(...))`; update it to subtract the height of the new single-row condensed footer (which, combined with task 2-2's header height, must keep pagination exact). Verify every Sessions `SetSize` call site.
- Implement §2.7 narrow truncation: when the row exceeds the width, truncate gracefully (drop/`…` lower-priority entries) so the footer never wraps to a second line or overflows; the `? help` right anchor must survive as long as possible.
- All colours via Phase 1 §2.9 tokens; no literal hex. Leaf `.Background(canvas)`.
- Produce the `vhs` capture (this is also where task 2-1's descriptor becomes visible): drive the Sessions tape to the flat view (and confirm `s switch view`/`x projects` appear on Flat); `Screenshot` to the harness dir; compare the footer region against `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role.

**Acceptance Criteria**:
- [ ] The Sessions footer is a single row showing exactly `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` + right-aligned `? help`; `n`/`r`/`k`/`q`/paging are NOT in the footer.
- [ ] Key glyphs render in `accent.blue`, labels in `text.detail`, and the `?` glyph in `accent.violet`; a 1px `border.footer` top rule sits above the row.
- [ ] The footer is rendered from the task 2-1 keymap descriptor (single source of truth) — not a second hand-authored binding list.
- [ ] `s switch view` and `x projects` appear on ALL session views including Flat.
- [ ] The single-row footer height is folded into the list size budget so pagination stays exact (with task 2-2's header height); verified at every Sessions `SetSize` site.
- [ ] Narrow truncation: the row truncates gracefully below the width without wrapping/overflow; `? help` survives as long as possible.
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape captures the Sessions flat view footer to the harness dir; it matches `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper reference before handoff. (This capture also serves as task 2-1's deferred footer verification.)
- [ ] Behaviour parity: the footer is display-only — replacing the three-column footer changes no key behaviour; every action still dispatches as before (the keys it now omits, e.g. `k`/`n`/`r`/`q`/paging, still work, they are merely not shown).

**Tests**:
- `"it renders a single-row footer with exactly the Sessions core keys and a right-aligned ? help"`
- `"it renders key glyphs in accent.blue, labels in text.detail, and the ? glyph in accent.violet"`
- `"it omits n, r, k, q, and paging from the footer (help-only)"` (edge case: help-only keys excluded)
- `"it shows s switch view and x projects on the Flat view"` (edge case: on all session views incl. Flat)
- `"it sources the footer entries from the keymap descriptor"` (single source of truth)
- `"it subtracts the single-row footer height from the list size budget so pagination stays exact"` (edge case: single-row height-budget recompute)
- `"it truncates the footer row gracefully below the minimum width without wrapping"` (edge case: narrow truncation)
- `"it leaves every key action dispatchable after the footer swap (omitted keys still work)"` (parity)

**Edge Cases**:
- `s switch view` / `x projects` must appear on every session view (Flat, by Project, by Tag) — they are not grouping-conditional.
- The omitted keys (`n`/`r`/`k`/`q`/paging) remain fully functional; the footer just doesn't list them (they live in `?` help, §8.5, Phase 3).
- Single-row height budget: combined with task 2-2's header height, the list must still paginate one delegate line per row (the historic overflow bug guardrail).
- Narrow truncation must not wrap the footer to a second line (which would silently steal a list row from the height budget).
- `border.footer` (1px) ≠ `border.separator` (2px) — use the correct distinct token.

**Context**:
> §3.4 footer: a single bottom row above a 1px top rule (`border.footer`) showing the page's core keys — for Sessions exactly `↑↓ navigate · ⏎ attach · / filter · ␣ preview · s switch view · x projects` plus a right-aligned `? help`; `n` new, `r` rename, `k` kill, `q` quit, and paging are NOT in the footer (they live in `?` help, §8.5); `s switch view` and `x projects` appear on all session views (Flat included); key glyphs `accent.blue`, labels `text.detail`, `?` glyph `accent.violet`; the full keymap lives in `?` help. §14.2 calls out the manual three-column footer (→ condensed) as restyle-existing-render-code. Current footer plumbing: `renderKeymapFooter` (~line 827), `chunkBindingsIntoThreeColumns` (~line 799), `sessionFooterBindings` (~line 775), `keymapFooterColumnSize` (~line 744) in `internal/tui/model.go`; the footer is composed in `viewSessionList` (~line 2354) and its height subtracted in `applySessionListSize` (~line 891). This task consumes the task 2-1 descriptor (its first consumer) and supplies task 2-1's deferred footer `vhs` verification. The `?` binding itself is Phase 3 — the footer only shows the `? help` hint. §2.9 tokens are Phase 1.

**Spec Reference**: §3.4 (condensed footer + `? help`), §12.1 (Sessions keymap — which keys are core), §8.5 (full keymap in `?` help), §2.7 (narrow truncation), §14.2 (restyle the manual three-column footer), §2.9 (token table — `accent.blue`/`text.detail`/`accent.violet`/`border.footer`), §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-5 | approved

### Task spectrum-tui-design-2-5: Centred pagination dots — built-in paginator restyled (active violet / inactive faint)

**Problem**: `bubbles/list`'s built-in height-driven paginator renders dots with default styling. §3.5 requires the dots rendered as centred dots above the footer with the active page dot in `accent.violet` and inactive dots in `text.faint`; §3.6 forbids a full-screen frame. The pagination count and behaviour must stay exactly as `bubbles/list` computes them (parity) — only the dot styling and centring change.

**Solution**: Restyle the list's built-in paginator dot styles (active/inactive) to the §2.9 tokens and ensure the dot row renders centred above the footer, on the owned canvas, without introducing any full-screen frame. Leave the paginator's count/behaviour untouched.

**Outcome**: The Sessions list shows centred pagination dots above the footer — active dot `accent.violet`, inactive dots `text.faint` — with the page count and paging behaviour unchanged from the built-in paginator; a single page suppresses the dots (built-in behaviour preserved); no full-screen frame; the capture matches the pagination region of `Sessions — Modern Vivid v2` / `(Light)`.

**Do**:
- Restyle the `bubbles/list` paginator dot styles on the Sessions list (in `newSessionList`, `internal/tui/model.go` ~line 682, or wherever the list styles are configured post-Phase-1-v2-upgrade): set the active-dot glyph style to `accent.violet` and the inactive-dot glyph style to `text.faint` via the Phase 1 tokens (the v1 hooks are `Styles.ActivePaginationDot` / `Styles.InactivePaginationDot`, fed into `Paginator.ActiveDot`/`InactiveDot`; confirm the v2 equivalents after the Phase 1 upgrade). No literal hex.
- Ensure the dot row renders centred across the list width above the footer. If the built-in paginator already centres within the list frame, verify the centring holds with the new header/footer composition from tasks 2-2/2-4; if the dots are rendered as part of the list's own footer area, confirm they sit between the list body and the condensed footer (above the footer per §3.5).
- Do NOT change the paginator's page count, per-page computation, or paging keys — this is purely the dot styling + placement (parity). The height-driven page count is the same value `bubbles/list` computes from the (header/footer-reduced) height budget.
- Honour §3.6: no box/frame around the UI; the dots are a per-element treatment on the owned canvas (leaf `.Background(canvas)`), not a framed element.
- Produce the `vhs` capture: seed the fixture with enough sessions to span multiple pages so the dots render; drive the Sessions tape to the flat view; `Screenshot` to the harness dir; compare the dot row against `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role. Also capture a single-page fixture to confirm dots are suppressed.

**Acceptance Criteria**:
- [ ] The active page dot renders in `accent.violet` and inactive dots in `text.faint`, via tokens (no literal hex).
- [ ] The dots render centred above the footer; no full-screen frame is introduced (§3.6).
- [ ] A single-page list suppresses the dots (built-in behaviour preserved).
- [ ] The page count and paging behaviour are unchanged from the built-in paginator (parity) — same number of dots / same page for a given fixture as pre-task.
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape (multi-page fixture) captures the dot row to the harness dir and matches `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper reference before handoff.
- [ ] Behaviour parity: only the dot glyph styling/centring changed; the paginator's count, per-page, and paging keys are byte-identical in behaviour.

**Tests**:
- `"it renders the active pagination dot in accent.violet and inactive dots in text.faint"`
- `"it centres the pagination dots above the footer"`
- `"it suppresses the dots when the list fits on a single page"` (edge case: single-page suppresses dots)
- `"it leaves the paginator page count and paging behaviour unchanged"` (edge case: page count unchanged / parity)
- `"it introduces no full-screen frame around the UI"` (edge case: no-full-frame rule)

**Edge Cases**:
- Single-page lists: dots suppressed (do not force-render a single dot) — preserve `bubbles/list`'s default.
- The page count is height-driven and must match what `bubbles/list` computes from the reduced height budget (header + footer subtracted by tasks 2-2/2-4) — do not recompute or override it.
- No full-screen frame (§3.6): the dots are a per-element treatment, not a boxed component.
- Centred across the list width, not the terminal width if those differ (match the frame).

**Context**:
> §3.5: `bubbles/list`'s built-in height-driven paginator renders as centred dots above the footer — active page dot `accent.violet`, inactive dots `text.faint`. §3.6: no full-screen frame; structure is the two rules + per-element treatments; the owned canvas is a flat fill, not a frame. §14.1 keeps `bubbles/list`'s pagination (the dots) as the engine — restyle only. Current list config: `newSessionList` (`internal/tui/model.go` ~line 682). The paginator dot style hooks in bubbles v1 are `Styles.ActivePaginationDot`/`Styles.InactivePaginationDot` → `Paginator.ActiveDot`/`InactiveDot`; after the Phase 1 Bubble Tea v2 / Lipgloss v2 upgrade (task 1-2) confirm the equivalent v2 hooks. §2.9 tokens (`accent.violet`, `text.faint`) are Phase 1; `text.faint` is decorative-only (the inactive dots are exactly the decorative case it is reserved for).

**Spec Reference**: §3.5 (pagination dots — active violet / inactive faint), §3.6 (no full-screen frame), §14.1 (keep `bubbles/list` paginator as engine), §2.9 (token table — `accent.violet`/`text.faint`), §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-6 | approved

### Task spectrum-tui-design-2-6: Sessions Flat row anatomy + violet left-bar selection

**Problem**: `SessionDelegate.Render` (`internal/tui/session_item.go`) renders rows with literal colours — `cursorStyle` (`212`), `attachedStyle` (`76`), `detailStyle` (`#777777`) — and a `> ` cursor prefix, with the window count and attached marker concatenated as free text (not fixed-width trailing slots). §4.1/§3.3 require: name as a full-width left flex column (`text.primary`; selected: `text.on-selection` over `bg.selection` + a thick `▌` violet 2-cell left bar); a fixed-width trailing window-count slot (`text.detail`; selected: `text.strong`) reading `N window`/`N windows`; a fixed-width trailing `● attached` slot in `state.green` (empty slot of same width when not attached so bullets/counts column-align); flat row is name-only; over-long names truncate with `…`. On the selected row the `● attached` marker KEEPS `state.green` (green-on-`bg.selection` must clear the floor).

**Solution**: Restyle `SessionDelegate.Render` to the MV row anatomy on Phase 1 tokens: a 2-cell violet left-bar column (`▌` `accent.violet`) on the selected row over a `bg.selection` row tint; the name in a flex left column; fixed-width right-pinned trailing slots for the window count and the `● attached` marker (with an empty same-width slot when unattached). Truncate over-long names with `…`. Keep the attached marker `state.green` even on the selected row (verified against the tint).

**Outcome**: Sessions flat rows render the MV anatomy — name flex column, fixed-width window-count and attached-marker trailing slots aligned regardless of name length, thick violet left-bar + `bg.selection` tint + `text.on-selection` name on the selected row, `state.green` `● attached` (on selected too), `…`-truncated long names — all token-backed on the owned canvas; the capture matches the row region of `Sessions — Modern Vivid v2` / `(Light)`; selection/attach behaviour is unchanged.

**Do**:
- Restyle `SessionDelegate.Render` (`internal/tui/session_item.go` ~line 153): replace `cursorStyle`/`nameStyle`/`detailStyle`/`attachedStyle` (the `212`/`76`/`#777777`/bold literals ~lines 13-17) with Phase 1 §2.9 tokens. No literal hex at the call site.
- Selection (§3.3): on the selected row (`index == m.Index()`), render a thick block `▌` in `accent.violet` pinned at the far left as a full 2-cell column, and tint the row with `bg.selection`; render the name in `text.on-selection`. Unselected rows have no bar and no tint (replace the `> ` / `  ` cursor prefix with the 2-cell bar column — a selected row shows the violet bar, an unselected row shows 2 blank cells, keeping column alignment).
- Row layout (§4.1): name = full-width left flex column in `text.primary` (selected: `text.on-selection`); window count = a fixed-width trailing slot, left-aligned within its slot, in `text.detail` (selected: `text.strong`), reading `N window`/`N windows` (reuse `windowLabel`, ~line 42); attached marker = a fixed-width trailing slot to the right of the count: `● attached` in `state.green` when attached, an empty slot of the SAME width when not, so the bullets line up vertically and the counts stay column-aligned. Trailing slots are fixed-width and right-pinned; the name flexes to fill the remainder (use Lipgloss width-constrained styles / `flexShrink`-equivalent fixed slots — a fixed-width trailing column with the name column taking the remaining width). The flat row is name-only — no project/path column.
- On the SELECTED row keep `● attached` in `state.green` (do not recolour it to `text.on-selection`): the attached-only rule holds and green-on-`bg.selection` clears the floor (§4.1, §2.9 foreground-on-tint pairing). Verify name/count/bullet all stay above the floor against `bg.selection` (the foreground-on-tint pairings — these were tuned in Phase 1; reference them).
- Over-long names truncate with `…` (§2.7) so the trailing slots never get pushed off-row — truncate the flex name column to its available width with an ellipsis.
- Keep the row exactly one delegate line (`Height()==1` unchanged) — the pagination invariant.
- Note: this delegate is shared by grouped views (the indent is added in task 2-7); this task restyles the row anatomy itself (Flat). Do not change the grouping/indent logic here beyond leaving the existing `indent` hook intact for task 2-7.
- Produce the `vhs` capture: drive the Sessions tape to the flat view with a fixture mixing attached/unattached sessions and at least one over-long name; move the cursor to show a selected row; `Screenshot` to the harness dir; compare against `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role (column alignment, violet bar, green bullet on selection).

**Acceptance Criteria**:
- [ ] The name renders as a flex left column in `text.primary` (selected: `text.on-selection`); the window count is a fixed-width trailing slot in `text.detail` (selected: `text.strong`) reading `N window`/`N windows`; the attached marker is a fixed-width trailing slot — `● attached` in `state.green` when attached, an empty same-width slot when not.
- [ ] The selected row shows a thick `▌` `accent.violet` 2-cell left-bar column + `bg.selection` tint; unselected rows have no bar and no tint.
- [ ] The `● attached` bullets and the window counts are vertically column-aligned regardless of name length; over-long names truncate with `…` without pushing the trailing slots off-row.
- [ ] On the selected row the `● attached` marker keeps `state.green` (not recoloured); all selected-row foregrounds (name, count, bullet) clear the contrast floor against `bg.selection`.
- [ ] The flat row is name-only (no project/path column); the row is exactly one delegate line.
- [ ] All colours via tokens (no `212`/`76`/`#777777` literals survive in the delegate).
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape captures the flat view (mixed attached/unattached, a long name, a selected row) to the harness dir and matches `Sessions — Modern Vivid v2` / `(Light)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper reference before handoff.
- [ ] Behaviour parity: selection, attach-target resolution (keys on `Session.Name`), and the window-count/attached semantics are unchanged vs the pre-reskin delegate; only the rendering differs.

**Tests**:
- `"it renders the name in a flex left column with fixed-width window-count and attached-marker trailing slots"`
- `"it column-aligns the attached bullets and window counts regardless of name length"` (edge case: columns aligned regardless of name length)
- `"it renders an empty same-width attached slot for unattached sessions to preserve alignment"` (edge case: empty attached slot preserves alignment)
- `"it renders the selected row with a violet ▌ left bar, bg.selection tint, and text.on-selection name"`
- `"it keeps the ● attached marker in state.green on the selected row"` (edge case: attached marker keeps state.green on selection — fg-on-tint floor)
- `"it renders the selected-row window count in text.strong"` (edge case: selected-row count text.strong)
- `"it truncates an over-long name with … without pushing the trailing slots off-row"` (edge case: over-long name truncation)
- `"it renders the flat row name-only with no project/path column"`
- `"it uses no literal 212/76/#777777 hex in the delegate"`

**Edge Cases**:
- Attached marker keeps `state.green` even on the selected row (green-on-`bg.selection` foreground-on-tint pairing — must clear the floor, tuned in Phase 1).
- Columns (counts + bullets) stay aligned regardless of name length; the unattached attached-slot is an empty slot of the same width (not omitted) so the column doesn't collapse.
- Over-long names truncate with `…`; the trailing fixed-width slots are right-pinned and never pushed off-row.
- Selected-row foregrounds (name `text.on-selection`, count `text.strong`, bullet `state.green`) are each verified against `bg.selection`, in addition to the §2.3 canvas gate.
- Row stays exactly one delegate line (`Height()==1`) — pagination invariant.

**Context**:
> §4.1 row anatomy: name full-width left flex column `text.primary` (selected: `text.on-selection` over `bg.selection` + violet bar); window-count fixed-width trailing slot left-aligned `text.detail` (selected: `text.strong`) reading `N window`/`N windows`; attached marker fixed-width trailing slot `● attached` `state.green` when attached, empty same-width slot when not, so bullets+counts column-align; flat row name-only, no project/path column; over-long names truncate `…`; on the selected row `● attached` KEEPS `state.green` (green-on-`bg.selection` clears the floor — §2.9 foreground-on-tint pairing); selected-row foregrounds verified against the tints in addition to the §2.3 canvas gate. §3.3 selection: thick `▌` `accent.violet` 2-cell left-bar column + `bg.selection` tint; selected name `text.on-selection`; unselected no bar/tint. §2.7 truncation. Current delegate: `internal/tui/session_item.go` — `SessionDelegate.Render` (~line 153) with literal `cursorStyle`=`212`, `attachedStyle`=`76`, `detailStyle`=`#777777`, `> ` cursor prefix; `windowLabel` (~line 42); `Height()==1` (~line 139). §2.9 tokens (`text.primary`/`text.on-selection`/`text.detail`/`text.strong`/`accent.violet`/`state.green`/`bg.selection`) are Phase 1. The grouped-row indent hook (`it.GroupKey != ""` → `groupRowIndent`) stays for task 2-7.

**Spec Reference**: §4.1 (row anatomy), §3.3 (selection — thick violet left-bar), §2.7 (truncation), §2.9 (token table + foreground-on-tint pairings), §13.6, §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-7 | approved

### Task spectrum-tui-design-2-7: Sessions grouped reskin — `heading ··· N` + indented rows for By Project & By Tag

**Problem**: The grouped views render the heading via `headingStyle` (`lipgloss.NewStyle().Faint(true)`, `internal/tui/session_item.go`) — a uniform faint that does not distinguish the heading text from its count, and the row indent (`groupHeaderIndent`/`groupRowIndent`) uses literal 2-space strings rather than the §5.1 indent treatment. §5.1 requires: the heading `text.detail` with the `··· N` count in `text.dim` (dimmer); grouped session rows nest one indent level further than flat (cursor col 2, name col 4) while flat rows sit flush at col 2; cursor skips header rows. The grouping MACHINERY already exists and must stay behaviourally identical — only the `HeaderItem` render and row indent change.

**Solution**: Restyle the `HeaderItem` render so the heading text is `text.detail` and the `··· N` count is `text.dim` (the two are separately styled, not one faint run), and adjust the row indent so grouped session rows nest one level further than flat (cursor col 2 / name col 4) with flat rows flush at col 2 — all pure Lipgloss in the existing delegate (no `lipgloss/tree`). Preserve the grouping machinery (HeaderItem model, Pattern A/B, catch-alls, cursor-skip, directory resolution, mode persistence) byte-for-byte behaviourally.

**Outcome**: By Project and By Tag render `heading ··· N` headings (heading `text.detail`, `··· N` `text.dim`) with grouped session rows indented one level further than flat (cursor col 2, name col 4) and flat rows flush at col 2, on the owned canvas; the cursor never lands on a header; catch-all (Unknown/Untagged) headings use the same heading style; pagination stays exact; the captures match `Sessions — by Project (MV)` and `Sessions — by Tag (MV)`.

**Do**:
- Restyle the `HeaderItem` render in `SessionDelegate.Render` (`internal/tui/session_item.go` ~line 155) and `HeaderItem.label()` (~line 129): split the `Heading` from the `··· N` count so the heading renders in `text.detail` and the `··· N` count renders in `text.dim` (dimmer). Replace `headingStyle` (`Faint(true)`, ~line 21) with the token-backed two-part render. Keep the `···` separator glyph (`groupSeparator`, ~line 27) and the `Heading ··· N` shape.
- Adjust the indent constants/treatment (`groupHeaderIndent`/`groupRowIndent`, ~lines 29-39): group headers indent to align with the title box's left edge (col 2); grouped session rows nest one level further in — cursor at col 2, name at col 4 — when `GroupKey != ""`; flat rows render flush at col 2 (cursor col 2, name col 2-ish per the flat anatomy from task 2-6). Keep the indent logic pure Lipgloss styling layered into the existing `SessionDelegate` — NOT routed through `lipgloss/tree` (§14.1 build constraint).
- Preserve the grouping machinery exactly: do NOT change `grouping.go` (`buildByProject`/`buildByTag`/`assembleGroups`/`injectGroupHeaders`/`orderedSessionItems`/`resolveSessionTags`/`unknownItem`/`untaggedItem`), `HeaderItem`'s model (FilterValue=="" flatten-on-filter), the cursor-skip (`ensureSessionRowSelected`/`skipHeaderRow` in `model.go`), directory resolution (`resolveSessionDirs`), Pattern A/B, the Unknown/Untagged catch-alls, or mode persistence. This is a render-only reskin of the heading + indent (§5 reskin banner).
- Catch-all headings (Unknown / Untagged) render with the same `heading ··· N` style as resolvable groups (they are `HeaderItem`s too).
- Leave the "No tags yet" signpost (§11.3, `byTagSignpost`/`renderByTagSignpostRow` in `model.go`) behaviourally intact — its reskin is Phase 4 and OUT of scope here. Do not restyle the signpost row; only restyle the by-Tag *grouped rows* (the path taken when tags exist).
- All colours via Phase 1 tokens (`text.detail`, `text.dim`); no literal hex/`Faint(true)` at the call site.
- Produce the `vhs` captures: drive the Sessions tape to by-Project and by-Tag views from a fixture with multiple projects/tags (including a multi-tag session that repeats under each tag — Pattern B — and at least one Unknown/Untagged catch-all member); `Screenshot` both to the harness dir; compare against `Sessions — by Project (MV)` and `Sessions — by Tag (MV)` for layout/structure/colour-role (heading dimming, count-dimmer, indent levels, cursor on a session row).

**Acceptance Criteria**:
- [ ] Heading rows render `heading ··· N` with the heading in `text.detail` and the `··· N` count in `text.dim` (dimmer) — two separately styled runs, not one faint run; no literal hex / `Faint(true)` at the call site.
- [ ] Grouped session rows nest one indent level further than flat (cursor col 2 / name col 4); flat rows remain flush at col 2.
- [ ] The cursor never lands on a header row on initial selection or any navigation (the existing cursor-skip is preserved, not reimplemented).
- [ ] Catch-all (Unknown / Untagged) headings use the same heading style as resolvable groups.
- [ ] Grouping is pure Lipgloss in the delegate — no `lipgloss/tree` (§14.1).
- [ ] The grouping machinery (HeaderItem model, Pattern A/B, catch-alls, directory resolution, mode persistence, flatten-on-filter) is behaviourally identical to pre-task; pagination stays exact (one delegate line per row, header or session).
- [ ] The "No tags yet" signpost path is behaviourally intact and NOT restyled here (Phase 4).
- [ ] VISUAL VERIFICATION (mandatory): `vhs` tapes capture by-Project and by-Tag views to the harness dir and match `Sessions — by Project (MV)` / `Sessions — by Tag (MV)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper references before handoff.
- [ ] Behaviour parity: traced against the pre-reskin grouping render — same items, same order, same catch-alls, same cursor behaviour; only heading colour and row indent changed.

**Tests**:
- `"it renders the group heading in text.detail and the ··· N count in text.dim"`
- `"it indents grouped session rows one level further than flat (cursor col 2 / name col 4)"`
- `"it keeps flat rows flush at col 2"` (edge case: flat rows still flush col 2)
- `"it never lands the cursor on a header row on initial selection or navigation"` (edge case: cursor never lands on header — skip preserved)
- `"it renders Unknown and Untagged catch-all headings with the same heading style"` (edge case: catch-all headings same style)
- `"it routes grouping through the existing pure-Lipgloss delegate, not lipgloss/tree"` (edge case: no lipgloss/tree)
- `"it keeps pagination exact with header and session rows each one delegate line"` (edge case: pagination exactness preserved)
- `"it leaves the no-tags signpost path behaviourally unchanged"` (edge case: no-tags signpost reskin out of scope — Phase 4)
- `"it preserves the grouping machinery output (items/order/catch-alls) byte-for-byte"` (parity)

**Edge Cases**:
- Cursor never lands on a header (initial selection + every navigation, including crossing a group boundary and paging) — preserve `ensureSessionRowSelected`/`skipHeaderRow`, do not reimplement.
- Catch-all (Unknown / Untagged) headings use the same `heading ··· N` style as resolvable groups.
- Flat rows stay flush at col 2 (the indent is grouped-only, gated on `GroupKey != ""`).
- No `lipgloss/tree` (§14.1 build constraint).
- Pagination exactness: header and session rows are each exactly one delegate line (the historic in-delegate-heading overflow bug guardrail).
- The "No tags yet" signpost (§11.3) is Phase 4 — leave its path intact; restyle only the by-Tag rows that appear when tags exist.

**Context**:
> §5.1 render-layer grouping: heading row `heading ··· N` — heading `text.detail`, `··· N` count `text.dim` (dimmer); non-selectable (FilterValue=="" → flatten-on-filter); session rows nest one indent level further than flat (cursor col 2, name col 4); flat rows flush col 2; cursor skips header rows on initial selection + every navigation. §5.2 By Project Pattern A (one row per session under its project; key = canonical path; Unknown catch-all). §5.3 By Tag Pattern B (one row per (session, tag) pair — multi-tag repeats; Untagged catch-all; zero-tags-anywhere → signpost, Phase 4). §5 reskin banner: the grouping machinery is already implemented and must stay behaviourally identical — only the MV visual treatment of headings + rows changes. §14.1 build constraint: grouping stays pure Lipgloss in the delegate, NOT `lipgloss/tree`. Current code: `internal/tui/session_item.go` — `HeaderItem.label()` (~line 129), `headingStyle` (~line 21, `Faint(true)`), `groupHeaderIndent`/`groupRowIndent` (~lines 29-39), `SessionDelegate.Render` HeaderItem arm (~line 155) + grouped-row indent (~line 172). Machinery to preserve: `internal/tui/grouping.go` (all builders) + `ensureSessionRowSelected`/`skipHeaderRow` in `model.go` (~lines 1100-1123). The signpost is `byTagSignpost`/`renderByTagSignpostRow`/`byTagSignpostStyle` in `model.go` (~lines 2371-2389) — leave intact. §2.9 tokens are Phase 1.

**Spec Reference**: §5.1 (render-layer grouping — heading style + indent), §5.2 (By Project Pattern A), §5.3 (By Tag Pattern B), §5.4 (directory resolution — preserve), §5.5 (tags directory-anchored — preserve), §14.1 (build constraint: no `lipgloss/tree`), §11.3 (no-tags signpost — out of scope, Phase 4), §2.9 (token table — `text.detail`/`text.dim`), §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-8 | approved

### Task spectrum-tui-design-2-8: Filtering input-active + list-active reskin — accent.orange query + contextual footers + flatten-on-filter

**Problem**: Filtering today is `bubbles/list`'s built-in filter with default styling and no clear two-mode distinction. §7/§7.1/§7.2 require the MV treatment: `/` opens an inline filter input in the section-header row; the query renders in `accent.orange` with an `accent.orange` `/` prefix; two mutually-exclusive modes — input-active (typing, cursor at end, NO row selected, footer `type to filter · ↵/↓ browse results · esc clear`) and list-active (locked `accent.orange` query with no cursor, arrows move selection, `↵` attaches, footer `↵ attach · ↑↓ navigate · esc clear filter`, no background tint); the `↵`/`↓` boundary commits input→list, `Esc` clears from either mode; typing flattens grouped views (headings vanish); no match-count shown. The filter ENGINE is unchanged (parity) — restyle the query + the two footers + pin the two-mode boundary clarity.

**Solution**: Restyle the `bubbles/list` filter input (the `/` prefix + query in `accent.orange`, cursor in input-active, locked no-cursor query in list-active) where it renders in the section-header row, and render the two contextual footers driven by the filter mode (input-active vs list-active). Rely on `bubbles/list`'s existing filter-state machine (`Filtering`/`FilterApplied`) for the actual engine + the `↵`/`↓`→commit and `Esc`→clear boundary (parity); flatten-on-filter is already free via `HeaderItem.FilterValue()==""`.

**Outcome**: Pressing `/` shows an `accent.orange` `/`-prefixed query in the section-header row with a cursor while typing (input-active) and a locked cursor-less `accent.orange` query while browsing (list-active); the two contextual footers render per mode; never both an input cursor and a selected row at once; `↵`/`↓` commits input→list, `Esc` clears from either mode; grouped headings vanish the instant a query is typed; no match-count is shown; the captures match `Filtering — input active (MV)` and `Filtering — list-active (MV)`; the filter engine behaviour is unchanged.

**Do**:
- Restyle the filter input where `bubbles/list` renders it in the title/section-header row: set the filter prompt to an `accent.orange` `/` prefix and the query text to `accent.orange` (the v1 hooks are `FilterInput.PromptStyle`/`TextStyle`/`Cursor` and `Styles.FilterPrompt`/`FilterCursor`; confirm the v2 equivalents after the Phase 1 upgrade). No literal hex. In input-active mode the cursor sits at the end of the typed text; in list-active mode the query is locked `accent.orange` with no cursor (the cursor-less locked query is the signal the list is filtered).
- Render the two contextual footers in `viewSessionList` (or the footer renderer from task 2-4) driven by the filter mode: input-active (`m.sessionList.SettingFilter()` / `FilterState()==Filtering`) → `type to filter · ↵/↓ browse results · esc clear`; list-active (`FilterState()==FilterApplied`) → `↵ attach · ↑↓ navigate · esc clear filter`. These replace the standard condensed footer (task 2-4) while a filter mode is active. Token-colour the same way as the standard footer (glyphs `accent.blue`, labels `text.detail`).
- Pin the two-mode boundary clarity (§7.1/§7.2) WITHOUT changing the engine: input-active never shows a selected list row (no row tint/bar while typing); list-active shows a selected row but no background tint on the filter input. `↵` or `↓` commits input-active→list-active; `Esc` clears the filter from either mode (returns to unfiltered). These transitions are `bubbles/list`'s built-in behaviour — verify they hold and the styling reflects the mode; do not reimplement the state machine.
- The `/ to filter` hint (task 2-3) is replaced by the live query while filtering; ensure the hint↔query swap reads consistently (the query occupies the same section-header position).
- Confirm flatten-on-filter is preserved: typing a query makes grouped headings vanish for free because `HeaderItem.FilterValue()==""` (do not change this). No match-count is shown anywhere (the visible results suffice).
- Confirm `s` stays a literal filter character while input-active (the dispatch case is below the `SettingFilter()` guard — task 2-1 preserved this).
- All colours via Phase 1 tokens; leaf `.Background(canvas)`; the list-active query has no background tint (§7.1).
- Produce the `vhs` captures: drive the Sessions tape to (a) input-active — press `/`, type a partial query, `Screenshot` (matches `Filtering — input active (MV)`); (b) list-active — press `/`, type, then `↵`/`↓` to commit, `Screenshot` (matches `Filtering — list-active (MV)`). Compare both for layout/structure/colour-role (orange query, correct footer, no-row-selected in input-active vs selected-row-no-tint in list-active).

**Acceptance Criteria**:
- [ ] `/` opens an inline filter input in the section-header row; the query renders in `accent.orange` with an `accent.orange` `/` prefix; the list filters live as you type.
- [ ] Input-active: cursor at the end of the typed text, NO list row selected; footer reads `type to filter · ↵/↓ browse results · esc clear`.
- [ ] List-active: locked `accent.orange` query with no cursor, arrows move the selection, `↵` attaches, no background tint; footer reads `↵ attach · ↑↓ navigate · esc clear filter`.
- [ ] Never both an input cursor AND a selected row simultaneously (§7.1).
- [ ] `↵` or `↓` commits input-active→list-active; `Esc` clears the filter from either mode (parity with the engine).
- [ ] Typing a query flattens grouped views (headings vanish via `HeaderItem.FilterValue()==""` — preserved, not changed); no match-count is shown.
- [ ] `s` remains a literal filter character while input-active.
- [ ] VISUAL VERIFICATION (mandatory): `vhs` tapes capture input-active and list-active to the harness dir and match `Filtering — input active (MV)` / `Filtering — list-active (MV)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper references before handoff.
- [ ] Behaviour parity: the `bubbles/list` filter engine (matching, live update, commit/clear transitions, relevance ordering) is unchanged; only the query styling, the two footers, and the mode-clarity rendering differ.

**Tests**:
- `"it renders the filter query and / prefix in accent.orange while typing (input-active)"`
- `"it shows the cursor at end-of-text with no list row selected in input-active mode"` (edge case: never both cursor + selection)
- `"it renders the input-active footer: type to filter · ↵/↓ browse results · esc clear"`
- `"it renders a locked cursor-less accent.orange query with a selected row and no background tint in list-active mode"`
- `"it renders the list-active footer: ↵ attach · ↑↓ navigate · esc clear filter"`
- `"it commits input-active to list-active on ↵ or ↓"` (edge case: §7.2 boundary)
- `"it clears the filter from either mode on Esc"` (edge case: Esc clears from either mode)
- `"it flattens grouped views (headings vanish) the instant a query is typed"` (edge case: flatten-on-filter via empty HeaderItem FilterValue — parity)
- `"it treats s as a literal filter character while input-active"` (edge case: s literal while input-active)
- `"it shows no match-count"` (edge case: no match-count shown)

**Edge Cases**:
- Never both an active input cursor AND a selected row at once (§7.1) — input-active has no selected row; list-active has a locked cursor-less query.
- `↵`/`↓` commits input-active→list-active; `Esc` clears from either mode (the engine's transitions — verify, don't reimplement).
- Flatten-on-filter is free via `HeaderItem.FilterValue()==""` — do not change it.
- `s` is a literal filter character while the input is focused (dispatch below the `SettingFilter()` guard).
- No match-count shown (the no-matches centred state is task 2-9).
- List-active filter input has no background tint (§7.1).

**Context**:
> §7 filtering: `/` opens an inline filter input in the section-header row (where the `/ to filter` hint sits); query in `accent.orange` with an `accent.orange` `/` prefix; list filters live; the `/ to filter` hint shows top-right consistently; no match-count shown; typing flattens grouped views. §7.1 two mutually-exclusive modes — input-active (keystrokes to query, cursor at end, NO list row selected, footer `type to filter · ↵/↓ browse results · esc clear`); list-active (locked `accent.orange` query no cursor, arrows move selection, `↵` attaches, footer `↵ attach · ↑↓ navigate · esc clear filter`, no background tint). §7.2 boundary — `↵`/`↓` commits input→list; `Esc` clears from either mode. §5.1 flatten-on-filter for free (`HeaderItem.FilterValue()==""`). §7 reskin banner: live filtering is `bubbles/list`'s built-in; the change is the styling + boundary clarity, the engine is unchanged. Current filter handling: `m.sessionList.SettingFilter()` guard + the `Esc`/`FilterApplied` progressive-back logic in `updateSessionList` (`internal/tui/model.go` ~lines 1971-2004); filter-state helpers `SessionListFilterState`/`SetSessionListFilter` (~lines 318-337). The `bubbles/list` filter style hooks (`FilterInput`, `Styles.FilterPrompt`/`FilterCursor`) — confirm v2 equivalents after Phase 1 task 1-2. The footer renderer is task 2-4. §2.9 tokens (`accent.orange`/`accent.blue`/`text.detail`) are Phase 1.

**Spec Reference**: §7 (filtering), §7.1 (two mutually-exclusive modes + footers), §7.2 (boundary), §5.1 (flatten-on-filter), §2.9 (token table — `accent.orange`), §14.1 (filter engine kept as-is), §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-2-9 | approved

### Task spectrum-tui-design-2-9: Filtering no-matches state — centred `⌀` + `No sessions match "<query>"` + widen-search hint

**Problem**: When a filter query matches zero sessions, `bubbles/list` shows an empty body with no guidance. §7.3 requires a centred empty state: a dim `⌀` glyph (`text.faint`), `No sessions match "<query>"` (`text.primary`), and the hint `⌫ to widen the search · esc to clear the filter` (`text.detail`); the footer stays in input-active form. This is distinct from the empty-*sessions* state (§11.1, Phase 4) — that is "no sessions exist," this is "the filter matched nothing."

**Solution**: Render a centred no-matches empty state in the Sessions body when an active filter has zero visible results — the `⌀` glyph, the query-interpolated `No sessions match "<query>"` message, and the widen-search hint, all token-coloured — while keeping the footer in the input-active form (task 2-8). Detect the condition from the live filter state + zero visible items.

**Outcome**: With an active query matching zero sessions, the Sessions body shows a centred dim `⌀` (`text.faint`) over `No sessions match "<query>"` (`text.primary`) over `⌫ to widen the search · esc to clear the filter` (`text.detail`), on the owned canvas, with the footer staying in input-active form; the message interpolates the current query; the state renders only when the query matches zero (not when results exist, not when no sessions exist at all); the capture matches `Filtering — no matches (MV)`.

**Do**:
- Detect the no-matches condition in `viewSessionList` (`internal/tui/model.go` ~line 2329): an active filter (`m.sessionList.FilterState()` is `Filtering` or `FilterApplied` AND `m.sessionList.FilterValue() != ""`) with zero visible items (`len(m.sessionList.VisibleItems()) == 0`). Only then render the centred empty state in place of the list body.
- Render the centred empty state (`lipgloss.Place` centred in the list body area): a dim `⌀` glyph in `text.faint`; below it `No sessions match "<query>"` in `text.primary` with the current query interpolated (literal double-quote bytes around the query, byte-exact — mirror the `formatSessionGoneFlash` literal-quote pattern in `sessions_flash.go`, NOT `%q`, so a query with quotes/unicode renders verbatim); below that the hint `⌫ to widen the search · esc to clear the filter` in `text.detail`.
- Keep the footer in input-active form (`type to filter · ↵/↓ browse results · esc clear`, from task 2-8) — NOT the list-active footer — because there are no results to browse (§7.3).
- Render only when the query matches zero. Do NOT render it when results exist, and do NOT conflate it with the empty-*sessions* state (§11.1, Phase 4 — "no sessions exist at all"): this state requires an active non-empty query. Flag this distinction in the implementation comment.
- All colours via Phase 1 tokens (`text.faint`/`text.primary`/`text.detail`); leaf `.Background(canvas)`; no literal hex.
- Produce the `vhs` capture: drive the Sessions tape — press `/`, type a query that matches nothing in the seeded fixture; `Screenshot` to the harness dir; compare against `Filtering — no matches (MV)` for layout/structure/colour-role (centred `⌀`, the interpolated message, the hint, the input-active footer).

**Acceptance Criteria**:
- [ ] When an active non-empty query matches zero sessions, the body renders a centred dim `⌀` (`text.faint`), `No sessions match "<query>"` (`text.primary`) with the current query interpolated, and `⌫ to widen the search · esc to clear the filter` (`text.detail`).
- [ ] The footer stays in the input-active form (not list-active).
- [ ] The state renders ONLY when the query matches zero — not when results exist, and distinct from the empty-sessions state (§11.1, Phase 4) which requires no active query.
- [ ] The query is interpolated with byte-exact literal quotes (verbatim, like `formatSessionGoneFlash`), not `%q`.
- [ ] All colours via tokens; rendered on the owned canvas.
- [ ] VISUAL VERIFICATION (mandatory): a `vhs` tape captures the no-matches state to the harness dir and matches `Filtering — no matches (MV)` for layout/structure/colour-role (agent/user-judged, §15.2); implementer self-checks against the committed Paper reference before handoff.
- [ ] Behaviour parity: this adds a display-only empty state; the filter engine, the commit/clear transitions, and `⌫`/`Esc` behaviour are unchanged (`⌫` widens by deleting a query char, `Esc` clears — both are the engine's existing behaviour; the hint just documents them).

**Tests**:
- `"it renders the centred ⌀ / No sessions match \"<query>\" / widen-search hint when the query matches zero sessions"`
- `"it interpolates the current query verbatim into the message with literal quotes"` (edge case: query interpolated into message)
- `"it keeps the footer in input-active form on the no-matches state"` (edge case: footer stays input-active, not list-active)
- `"it renders the no-matches state only when an active non-empty query matches zero"` (edge case: renders only when query matches zero)
- `"it does not render the no-matches state when results exist"`
- `"it does not conflate the no-matches state with the empty-sessions state (no active query)"` (edge case: distinct from empty-sessions state — Phase 4)
- `"it colours the glyph text.faint, the message text.primary, and the hint text.detail"`

**Edge Cases**:
- Renders only when there is an active, non-empty query AND zero visible items — never when results exist.
- Distinct from the empty-*sessions* state (§11.1, Phase 4): that is "no sessions exist at all" (no active query); this is "the filter matched nothing." Do not merge the two paths.
- The query is interpolated byte-exact with literal quotes (handles spaces/dashes/unicode/embedded quotes) — use the literal-quote pattern, not `%q`.
- The footer stays input-active (no results to browse), not list-active (§7.3).
- A query whittled down to empty (all chars deleted) exits this state and returns to the normal filtered/unfiltered view.

**Context**:
> §7.3 over-filtered (no matches): when the query matches nothing — a centred empty state: a dim `⌀` glyph (`text.faint`), `No sessions match "<query>"` (`text.primary`), hint `⌫ to widen the search · esc to clear the filter` (`text.detail`); footer stays in input-active form. Distinct from §11.1 empty-sessions (Phase 4 — "No sessions yet" with a different glyph `▌ ▌ ▌` and a different footer swap; that state has no active query). The byte-exact literal-quote interpolation pattern exists in `internal/tui/sessions_flash.go` `formatSessionGoneFlash` (`fmt.Sprintf(\`session "%s" no longer exists\`, name)`) — mirror it (literal quotes, not `%q`). Current `viewSessionList` is `internal/tui/model.go` ~line 2329; filter-state accessors `SessionListFilterState`/`SessionListVisibleItems`/`SessionListFilterValue` (~lines 318-331). The input-active footer is task 2-8. §2.9 tokens (`text.faint`/`text.primary`/`text.detail`) are Phase 1.

**Spec Reference**: §7.3 (over-filtered no-matches state), §7.1 (input-active footer form), §11.1 (empty-sessions state — distinct, Phase 4), §2.9 (token table — `text.faint`/`text.primary`/`text.detail`), §15.1/§15.2. `.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`
