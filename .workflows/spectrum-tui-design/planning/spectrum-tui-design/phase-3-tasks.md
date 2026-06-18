---
phase: 3
phase_name: "Projects page + modal layer (kill · rename · delete · two-mode edit · ? help)"
total: 9
---

## spectrum-tui-design-3-1 | approved

### Task spectrum-tui-design-3-1: Blank-screen modal layer (shared) — clear the page behind an open modal to the owned canvas + centre the border-defined panel

**Problem**: Today every modal (kill, rename, delete-project, edit-project) renders as an overlay composited *on top of* the live list view — `renderModal`/`renderListWithModal` in `internal/tui/modal.go` splice the styled modal box over the visible session/project rows. §8.1/§13.5 change this: when a modal opens, the page behind must be **cleared to the owned canvas** (mode-matched per §1, not a dimmed overlay, not a literal black), and the centred border-defined panel sits on that blank canvas. This is a single shared modal-layer change, not a per-modal restyle — every modal in this phase inherits it.

**Solution**: Change the modal render shell so that when a modal is active the composed view is the owned canvas (full-terminal fill) with the centred modal panel drawn on it, instead of the list view with the modal spliced over it. The underlying confirm/input logic of every modal is untouched (parity) — only the surrounding render shell changes. Resolve the §14.6 open question (adapt the existing `renderModal`/`renderListWithModal` path vs a modal-system rework) against the code at implementation, defaulting to "adapt if small, rework only if forced."

**Outcome**: When any modal is open, the session/project rows behind it are gone — the panel sits on a flat, mode-matched canvas fill — and the modal's confirm/input behaviour is byte-for-byte identical to before; the one-row-per-delegate pagination invariant of the list underneath is never perturbed (the list simply is not rendered while the modal is up, then renders normally on dismissal).

**Do**:
- Read `internal/tui/modal.go` (`renderModal`, `renderListWithModal`, `modalStyle`) and the two callers in `internal/tui/model.go` (`viewSessionList` → kill/rename branch, `viewProjectList` → delete/edit branch) to trace exactly how the modal content is composited today.
- Assess §14.6: can the existing path be **adapted** (when `modalContent != ""`, replace the background `listView` with a full-terminal owned-canvas fill of size `termW × termH`, then centre the panel on it — likely a small change to `renderListWithModal` so it composites onto a canvas fill rather than `l.View()`) or does it need a **rework**? Default to adapt; only rework if the existing splice mechanic structurally cannot clear-to-canvas. Record the decision (one or two sentences) in a code comment at the changed site.
- Build the cleared canvas from the Phase 1 owned-canvas primitive (the `canvas` token + the outer full-terminal fill, e.g. `lipgloss.Place(termW, termH, …)` with `WithWhitespaceBackground(canvas)` / a `Width=termW·Height=termH·Background=canvas` container) so the blank backdrop is the same mode-matched fill the rest of the app uses. Do NOT re-derive canvas selection — consume Phase 1's resolved canvas.
- Centre the panel on the canvas (reuse the existing centre maths in `renderModal`, sized to `termW/termH` rather than the list's `w/h` fallback). The panel stays border-defined with no distinct fill (that styling is per-modal in 3-5/3-6/3-7/3-9; this task only changes the backdrop + centring shell).
- Ensure the modal footer always carries the dismiss key as `esc <verb>` is a per-modal concern (3-5…3-9); this task must not break that — keep the footer slot the per-modal renderers fill.
- Confirm modals stay key-exclusive: the existing `updateModal` dispatch already routes all input to the active modal handler and returns before page binds fire — verify this still holds (it is the §8.1 "key-exclusive, `Esc` resolves against the modal first" rule) and do not regress it.
- Honour `NO_COLOR` (§2.5): under `NO_COLOR` the cleared backdrop is the terminal's native bg (no painted canvas) — this is inherited from Phase 1's carve-out (canvas suppression), so the same code path that suppresses the canvas elsewhere must suppress it here; do not add a second NO_COLOR branch.

**Acceptance Criteria**:
- [ ] When a modal is open the page content behind it is cleared to the owned mode-matched canvas (dark `#0b0c14` / light `#e1e2e7`) — no session/project rows, no list, no dimmed overlay — and the panel is centred on that fill.
- [ ] The §14.6 adapt-vs-rework decision is recorded in a code comment at the changed site (defaulting to adapt-if-small).
- [ ] Confirm/input logic of every modal is unchanged — kill still confirms on `y`, rename still commits on `Enter`, delete-project still deletes on `y`, edit-project still routes keys to its handler — verified by the existing modal handler tests staying green.
- [ ] Modals remain key-exclusive: underlying page binds (`s`/`x`/`n`/`e`/`d`/clear-filter/quit) do not fire while a modal is open, and `Esc` resolves against the modal first.
- [ ] Under `NO_COLOR` the modal blank-screen clears to the terminal's native bg (no painted canvas), inherited from the Phase 1 carve-out — not a separate code path.
- [ ] No full-screen frame is drawn around the cleared canvas (§3.6) — the backdrop is a flat fill, the panel is border-defined.
- [ ] The list's one-row-per-delegate pagination invariant is intact on modal dismissal (the list renders identically to before once the modal closes).
- [ ] VISUAL VERIFICATION: a `vhs` tape (Phase 1 §15.2 harness) drives the TUI to open a kill-confirm modal (Sessions → `k`) and writes a PNG; the capture shows the modal centred on a cleared canvas with NO list rows visible behind it. The visible effect of this plumbing task is the cleared canvas — verify it on the kill modal (the first modal that lands), comparing against `Kill Confirm Modal (MV)` for the blank-canvas backdrop + centring (panel styling/copy is verified in 3-5; here the check is "is the backdrop a clean canvas, panel centred, no rows behind").

**Tests**:
- `"it clears the page behind an open modal to the owned canvas (no list rows in the composed view)"`
- `"it centres the modal panel on the canvas using terminal dimensions"`
- `"it preserves kill confirm logic — y confirms, esc cancels (parity with pre-change handler)"`
- `"it preserves rename/delete/edit confirm-input logic (parity)"`
- `"it keeps modals key-exclusive — a page bind (e.g. s/x) does not fire while a modal is open"`
- `"it clears to the terminal native bg under NO_COLOR (no painted canvas)"`
- `"it does not perturb the list pagination when the modal is dismissed"`
- `"it renders the modal centred on a cleared canvas in the vhs capture (blank-screen, no rows behind)"`

**Edge Cases**:
- Zero terminal dimensions (`termWidth`/`termHeight == 0`) — keep the existing 80×24 fallback so the canvas fill is never zero-sized (mirror the `renderListWithModal` fallback).
- Dynamic vertical changes underneath (a flash band that was present before the modal opened) must not leak into the cleared view — the backdrop is the canvas fill, not the previous composed body.
- `NO_COLOR`: native bg, no canvas — inherited, must not double-branch.
- Adapt-vs-rework: if the existing splice path genuinely cannot clear-to-canvas, a minimal rework is permitted — record why in the decision comment.

**Context**:
> §8.1: "Modals render on a blank screen (changed behaviour). When a modal opens, the page behind is cleared to the owned canvas (mode-matched — §1, not a literal black) and the modal is centred on it. This changes today's behaviour — existing modals render as an overlay on top of the page content." §13.5 restates: "cleared to the owned canvas … not a dimmed overlay … The Preview overlay is the exception."
> §14.6 (OPEN QUESTION): "Whether the existing modal render path can be adapted for the blank-screen treatment or needs a modal-system rework is not yet determined — assess against the code at implementation. The underlying confirm/input logic of each modal is preserved either way." Default to adapt-if-small, rework only if forced.
> Current code: `internal/tui/modal.go` `renderModal` composites the styled modal OVER `listView` (left bg + fg + right bg, ANSI-aware); `renderListWithModal` passes `l.View()` as the background. This task changes the background from the live list to the cleared canvas.
> Phase 1 dependency: consume the resolved `canvas` token + owned-canvas full-terminal fill primitive and the `NO_COLOR` canvas-suppression carve-out — do NOT re-derive detection or canvas selection.

**Spec Reference**: §8.1, §13.5, §14.6, §2.5, §1, §3.6 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-2 | approved

### Task spectrum-tui-design-3-2: Projects page reskin (MV) — two-line rows + full-height violet left-bar selection + green header + condensed footer

**Problem**: The Projects page renders with the legacy delegate (`ProjectDelegate` in `internal/tui/project_item.go`): a bold name, a `#777777` path (`projectPathStyle`), and a `"> "` cursor (`cursorStyle`, pink `212`). §6 retargets this to Modern Vivid: two-line rows on tokens, a full-height violet left-bar selection spanning both lines over a `bg.selection` tint, a `state.green` section header + `text.detail` count, and the condensed Projects footer. CRUD behaviour stays identical (reskin parity).

**Solution**: Restyle `ProjectDelegate.Render` to emit MV-tokenised two-line rows with the full-height violet bar + selection tint, and restyle the Projects section header (green `Projects` label + dim count + `/ to filter` hint) inside the shared chrome (Phase 2). All values come from the §2.9 token layer (Phase 1) — no literal hex survives at these call sites. The list model, pagination, and CRUD plumbing are untouched.

**Outcome**: The Projects page matches `Projects (MV)` for layout/structure/colour-role — two-line rows (name `text.primary` heavy on line 1, path `text.detail` dim on line 2), selected row carries a full-height `accent.violet` left bar across both lines + `bg.selection` tint with name in `text.on-selection` and path in `text.muted-bright`, header `Projects` in `state.green` with a `text.detail` count and a right-aligned `/ to filter` hint — and project CRUD (enter=new session, e=edit, d=delete, n=new-in-cwd, x=sessions) behaves exactly as before.

**Do**:
- Restyle `ProjectDelegate.Render` in `internal/tui/project_item.go`: replace `projectNameStyle`/`projectPathStyle`/the `cursorStyle` `"> "` cursor with MV tokens — name `text.primary` (heavy/bold), path `text.detail`. Keep `Height() == 2`, `Spacing() == 0` so two-line uniform-height rows keep `bubbles/list` pagination exact.
- For the selected row (`index == m.Index()`): render a **full-height `accent.violet` left bar** — a column of coloured cells spanning BOTH lines (not a single-line `> ` cursor) — plus a `bg.selection` row tint across both lines; the name becomes `text.on-selection`, the path becomes `text.muted-bright`. Unselected rows: no bar, no tint (mirror §3.3 / §6.2).
- Restyle the Projects section header (in `viewProjectList` / the shared section-header renderer from Phase 2 task 2-3): `Projects` label in `state.green`, count in `text.detail` at the SAME cap-height as the label (dim, not smaller — §3.2/§13.6), right-aligned `/ to filter` hint in `text.detail`.
- Use the condensed footer `⏎ new session · x sessions · e edit · / filter · ? help` — this is sourced from the Projects keymap descriptor in task 3-3; this task wires the Projects footer to render via the shared condensed-footer renderer (Phase 2) rather than the legacy three-column `renderKeymapFooter`. (If 3-3 lands the descriptor first, consume it; if authored before, leave a clear seam — but the footer copy must read exactly the §6.3 condensed string.)
- Leave the empty-projects state (§11.1) intact — it is Phase 4; do not restyle it here.
- Verify parity: read the current `ProjectDelegate.Render`, `viewProjectList`, and the project CRUD handlers (`handleProjectEnter`, `handleEditProjectKey`, `handleDeleteProjectKey`, `handleNewInCWD`, the `x`→Sessions transition); confirm the reskin changes only rendering, no behaviour.

**Acceptance Criteria**:
- [ ] Each project renders as a two-line row: name `text.primary` (heavy) on line 1, path `text.detail` (dim) on line 2; uniform two-line height preserved so pagination stays exact.
- [ ] The selected row shows a full-height `accent.violet` left bar spanning BOTH lines + a `bg.selection` tint; name becomes `text.on-selection`, path becomes `text.muted-bright`; unselected rows have neither bar nor tint.
- [ ] The section header reads `Projects` in `state.green` with a `text.detail` count rendered at the same cap-height as the label (dim, not smaller), and a right-aligned `/ to filter` hint.
- [ ] The footer is the condensed Projects keymap `⏎ new session · x sessions · e edit · / filter · ? help`.
- [ ] No literal hex survives at the restyled call sites — every colour is a §2.9 token.
- [ ] Project CRUD behaviour is identical to before (verified against the current handlers): enter=new session, e=edit, d=delete, n=new-in-cwd, x=sessions, navigation.
- [ ] The empty-projects state is left intact (Phase 4).
- [ ] The selected-row name `text.on-selection` and path `text.muted-bright` clear the contrast floor against `bg.selection` (the foreground-on-tint pairing, §2.9) — inherited from Phase 1 token tuning, verified here for these two pairings.
- [ ] VISUAL VERIFICATION: a `vhs` tape seeds a known projects fixture, opens the Projects page (Sessions → `x`), and writes a PNG; the capture is compared against `Projects (MV)` for layout/structure/colour-role (two-line rows, full-height violet bar on the selected row, green header, condensed footer).

**Tests**:
- `"it renders a project as two lines — name text.primary heavy, path text.detail dim"`
- `"it draws a full-height accent.violet left bar spanning both lines of the selected row over a bg.selection tint"`
- `"it renders the selected name in text.on-selection and the selected path in text.muted-bright"`
- `"it leaves unselected rows with no bar and no tint"`
- `"it renders the Projects header in state.green with a text.detail count at the same cap-height as the label"`
- `"it renders the condensed Projects footer copy exactly"`
- `"it keeps uniform two-line row height so pagination row counts are unchanged (parity)"`
- `"it preserves project CRUD behaviour — enter/e/d/n/x dispatch identically to the pre-reskin handlers"`
- `"it captures a Projects page matching the Projects (MV) frame (layout/structure/colour-role)"`

**Edge Cases**:
- Over-long project name or path — truncate with `…` (§2.7), keep the two-line height uniform so pagination row counts never drift.
- Selected row at a page boundary — the full-height bar + tint must render on both lines even when the row sits at the top/bottom of the visible page.
- foreground-on-tint floor for the selected name/path (verified against `bg.selection`, not just the canvas).
- Empty projects list — leave the existing empty state untouched (Phase 4 owns it).
- Count rendered at the same cap-height as the label (dim colour, not a smaller font).

**Context**:
> §6.2: "Each project is a two-line row (uniform height …): Line 1 — name in text.primary (heavy). Line 2 — path in text.detail (dim). Selected: a full-height accent.violet left bar (a column of coloured cells spanning both lines) + bg.selection tint; the name becomes text.on-selection, the path text.muted-bright."
> §6.1: "Projects (state.green) + count (text.detail) on the left; the / to filter hint on the right." §6.3: footer "⏎ new session · x sessions · e edit · / filter · ? help".
> Current code: `internal/tui/project_item.go` `ProjectDelegate` uses `projectNameStyle` (bold) + `projectPathStyle` (`#777777`) + `cursorStyle` (`"> "`, pink `212`); `viewProjectList` in `internal/tui/model.go` composes the list + the legacy three-column footer via `renderKeymapFooter`/`projectFooterBindings`.
> Reskin, not rebuild (§1/§6): CRUD behaviour identical; the change is provably cosmetic. Phase 1/2 dependencies: §2.9 tokens, owned canvas, shared chrome + section-header renderer, condensed-footer renderer.

**Spec Reference**: §6, §6.1, §6.2, §6.3, §3.2, §3.3, §13.6, §2.9, §2.7, §1 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-3 | approved

### Task spectrum-tui-design-3-3: Projects keymap descriptor + §12.2 Projects-side keymap revision (drop the `s`→Sessions alias; `x` toggles both directions)

**Problem**: The keymap descriptor (single source of truth driving footer + help, introduced in Phase 2 task 2-1 for Sessions) does not yet carry a Projects entry, so the Projects footer and the new `?` help (3-4) have nothing to generate from. Separately, §12.2 de-overloads the page-toggle keys: today `updateProjectsPage` handles BOTH `s` (→Sessions) and `x` (→Sessions) and `projectHelpKeys` advertises `s/x`; the revision drops the Projects-side `s` alias so `x` is the single both-directions toggle (the Sessions-side `x` toggle + the drop of the Sessions-side `p` alias were done in Phase 2 task 2-1 — do not duplicate them here).

**Solution**: Add a Projects keymap descriptor to the single-source descriptor type from Phase 2 task 2-1, with the §12.1 Projects keymap (arrows / `Ctrl+↑↓` page / `/` filter / `Enter` new-session / `x` → Sessions / `e` edit / `d` delete / `n` new-in-cwd / `q` quit / `Esc`), tagging which entries are footer-core vs help-only so it drives BOTH the condensed footer (3-2) AND the Projects `?` help (3-4) from one place. Then remove the Projects-side `s`→Sessions alias from `updateProjectsPage` so `x` is the only page toggle.

**Outcome**: A single Projects keymap descriptor exists that generates the condensed footer copy `⏎ new session · x sessions · e edit · / filter · ? help` and the complete Projects help list; pressing `s` on the Projects page no longer toggles to Sessions (only `x` does, in both directions); no uppercase bindings are introduced; every dispatched Projects action behaves identically (parity) apart from the removed `s` alias.

**Do**:
- Add a Projects descriptor to the keymap-descriptor type from Phase 2 task 2-1 (the type the Sessions descriptor already uses). Populate it from the §12.1 Projects keymap and mark footer-core entries (`Enter` new session, `x` sessions, `e` edit, `/` filter, `? help`) vs help-only entries (`d` delete, `n` new in cwd, navigation/paging, `q` quit, `Esc`). Use the descriptor field/shape established in 2-1; do NOT hand-author separate footer and help strings.
- In `internal/tui/model.go` `updateProjectsPage`, remove the `case isRuneKey(msg, "s")` arm that sets `m.activePage = PageSessions` (the §12.2 drop). Keep the `case isRuneKey(msg, "x")` arm exactly as-is (it already toggles to Sessions and dispatches `refreshSessionsAfterPreviewCmd("")`). After removal, `s` on the Projects page falls through to the list (`m.projectList.Update`) — confirm that is harmless (no Projects list bind on `s`; it becomes a no-op/literal, matching "single meaning").
- Migrate `projectHelpKeys` (the legacy `[]key.Binding` list, which advertises `s/x` for sessions and includes `d`/`n`/`q`) into the descriptor as the authoritative source — the descriptor replaces it as the help/footer source. If the legacy `projectHelpKeys`/`projectFooterBindings`/three-column `renderKeymapFooter` path is still wired, retire it for Projects in favour of the descriptor-driven condensed footer + help (coordinate with 3-2 which wires the Projects footer renderer).
- Do NOT touch the command-pending Projects keymap (§11.4 / `commandPendingHelpKeys`) — that is a different footer owned by Phase 4; leave it.
- No uppercase bindings (§12.2): the descriptor entries are lowercase / arrow / glyph keys only.
- Verify parity: trace every Projects action dispatch (`handleProjectEnter`, `handleEditProjectKey`, `handleDeleteProjectKey`, `handleNewInCWD`, the `x` toggle, `q`/`Esc`/`Ctrl+C`) and confirm only the `s` alias is removed; all other behaviour is identical.

**Acceptance Criteria**:
- [ ] A Projects keymap descriptor is added to the Phase 2 single-source descriptor type, carrying the §12.1 Projects keymap with footer-core vs help-only entries tagged.
- [ ] The descriptor drives BOTH the condensed Projects footer (`⏎ new session · x sessions · e edit · / filter · ? help`) AND the complete Projects help — neither is hand-authored.
- [ ] Pressing `s` on the Projects page no longer toggles to Sessions (the `s`→Sessions arm is removed); `x` toggles Projects⟷Sessions (both directions, unchanged from the Sessions-side work in 2-1).
- [ ] No uppercase bindings are introduced.
- [ ] The command-pending Projects keymap (§11.4) is untouched.
- [ ] Every other Projects action dispatches identically to before (parity): enter=new session, e=edit, d=delete, n=new-in-cwd, q/Esc/Ctrl+C, navigation.
- [ ] The Sessions-side `x` toggle and dropped `p` alias from 2-1 are not duplicated/re-touched here.
- [ ] VISUAL VERIFICATION: this is a data/plumbing task; its visible effect (the Projects footer + Projects help generated from the descriptor) is verified through the footer in 3-2's `Projects (MV)` capture and through the help modal in 3-4. State this — no standalone frame for 3-3.

**Tests**:
- `"it generates the condensed Projects footer copy from the descriptor"`
- `"it generates the complete Projects help list from the descriptor including help-only keys (d/n/nav/q)"`
- `"it no longer toggles to Sessions on s (the alias is removed)"`
- `"it still toggles Projects→Sessions on x (parity with the pre-change x arm)"`
- `"it dispatches enter/e/d/n/q/Esc identically to the pre-change handlers (parity)"`
- `"it introduces no uppercase bindings"`
- `"it leaves the command-pending keymap unchanged"`

**Edge Cases**:
- After removing the `s` arm, `s` must be a harmless no-op on the Projects page (no list bind), not a crash or an unexpected action.
- The `x` arm dispatches `refreshSessionsAfterPreviewCmd("")` — that side-effect must survive untouched (it makes tag edits visible on return to Sessions).
- Command-pending mode: `s`/`x`/`e`/`d` are already gated (`if m.commandPending { return m, nil }`) — the descriptor change must not alter command-pending gating.
- Descriptor footer-core vs help-only split must match §6.3 (footer) and §12.1 (complete help).

**Context**:
> §12.2: "Page ⟷ view keys de-overloaded. x toggles Sessions ⟷ Projects (both directions); s is Sessions-only (cycle views). The former p (Sessions → Projects) and s (Projects → Sessions) aliases are dropped, so each key has a single meaning." "No uppercase bindings anywhere."
> §8.5 / §14.4: "Content source: the help modal is generated from the page's keymap descriptor — the single source of truth that also drives the footer and §12.1 — not hand-authored per page."
> §12.1 Projects keymap: ↑/↓ move · Ctrl+↑/Ctrl+↓ page · / filter · Enter new-session-from-project · x → Sessions · e edit · d delete · n new-in-cwd · q quit · Esc.
> Current code: `internal/tui/model.go` `updateProjectsPage` has both `case isRuneKey(msg, "s")` and `case isRuneKey(msg, "x")` arms (identical body); `projectHelpKeys` advertises `s/x` for "sessions" and is consumed by `projectFooterBindings` → the legacy three-column `renderKeymapFooter`. The Phase 2 descriptor type (task 2-1) is the single source to extend.

**Spec Reference**: §12.2, §12.1, §8.5, §14.4, §6.3, §11.4 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-4 | approved

### Task spectrum-tui-design-3-4: `?` help modal (new) — help-modal type + generic descriptor-driven renderer + bind `?` on Sessions & Projects

**Problem**: There is no `?` help modal today; `?` is actively SWALLOWED on both pages (`if isRuneKey(msg, "?") { return m, nil }` in `updateSessionList` and `updateProjectsPage`) so `bubbles/list` doesn't toggle its own help. §8.5/§14.4 add a new per-page help modal: bind `?`, add a help-modal type, and build a generic two-column renderer over the per-page keymap descriptor (single source driving footer + help) — listing the page's COMPLETE keymap including the footer's own keys.

**Solution**: Add a `modalHelp` modal state + a generic help-modal renderer (~60–80 lines) that reads the active page's keymap descriptor (Sessions from Phase 2 task 2-1, Projects from task 3-3) and renders a two-column panel (key glyph `accent.blue` / action label `text.strong`) with header `? Keybindings` (`text.primary`) and a right-aligned `esc close` (`text.detail`) in the header — the documented help-modal exception to §8.1 (dismiss hint in the header, no contextual footer). Un-swallow `?` on Sessions + Projects so it opens this modal; close on `?` toggle or `Esc`; key-exclusive while open.

**Outcome**: Pressing `?` on Sessions or Projects opens a help modal generated from that page's descriptor, listing the complete page keymap (including the keys also shown in the footer); the header carries `? Keybindings` left and `esc close` right; `?` toggles it closed and `Esc` dismisses it without falling through to clear-filter/quit; the Sessions help matches `Sessions — Help Modal (?)`.

**Do**:
- Add a `modalHelp` value to the `modalState` enum in `internal/tui/modal.go`. Add help-open state to the model (the active page's descriptor or a page tag the renderer resolves to a descriptor).
- Un-swallow `?`: in `internal/tui/model.go` `updateSessionList` and `updateProjectsPage`, replace the `if isRuneKey(msg, "?") { return m, nil }` swallow with handlers that open the help modal (`m.modal = modalHelp`) for the current page. (The swallow exists so `bubbles/list` doesn't self-toggle its own help — opening our modal still consumes the key, so the list's help never fires.)
- Add a help-modal handler (route via `updateModal`): `?` (toggle) or `Esc` closes it (`m.modal = modalNone`); all other keys are consumed (key-exclusive — §8.1) so `Esc` dismisses the modal and does NOT fall through to the page's clear-filter / quit.
- Build the generic renderer (~60–80 lines) over the keymap descriptor: two columns — key-hint glyph in `accent.blue`, action label in `text.strong`; header row `? Keybindings` in `text.primary` LEFT with a right-aligned `esc close` in `text.detail` (the help-modal exception — dismiss hint in the HEADER, NOT a contextual footer); list the page's COMPLETE keymap (every descriptor entry, footer-core AND help-only — it is the full reference, not just the footer's overflow). Generated from the descriptor, not hand-authored.
- The help modal renders on the blank screen for Sessions/Projects (inherits 3-1's cleared-canvas shell), extending the existing rounded-border modal primitive (`modalStyle`). It is NOT a contextual-footer modal (the §8.1 exception).
- Bind `?` on Sessions + Projects here. Preview's `?` (which overlays without blanking — §8.5/§9.3) and the Preview keymap descriptor + help-from-Preview wiring are DEFERRED to Phase 4 — flag this explicitly in a code comment so the Preview arm is not forgotten; do not wire Preview help in this task.
- Only Sessions help is mocked (`Sessions — Help Modal (?)`); the Projects help follows its audited descriptor (3-3) with no separate frame.

**Acceptance Criteria**:
- [ ] `?` is bound on Sessions and Projects and opens the help modal (the prior swallow is replaced; `bubbles/list` still never self-toggles its own help).
- [ ] The help modal is GENERATED from the page's keymap descriptor (Sessions / Projects), not hand-authored, and lists the page's COMPLETE keymap including the footer's own keys.
- [ ] The renderer is two-column: key-hint glyph in `accent.blue`, action label in `text.strong`.
- [ ] The header reads `? Keybindings` (`text.primary`) left with a right-aligned `esc close` (`text.detail`); there is NO contextual footer (the §8.1 help-modal exception).
- [ ] The help modal closes on `?` (toggle) or `Esc`; while open it is key-exclusive — `Esc` dismisses it and does NOT fall through to clear-filter/quit.
- [ ] The help modal renders on the cleared blank screen (inherits 3-1) for Sessions/Projects.
- [ ] Preview's `?` (overlay-without-blanking) + the Preview descriptor + help-from-Preview wiring are flagged as deferred to Phase 4 (code comment), not wired here.
- [ ] No literal hex at the renderer call sites — every colour is a §2.9 token.
- [ ] VISUAL VERIFICATION: a `vhs` tape opens the Sessions help modal (Sessions → `?`) and writes a PNG; compared against `Sessions — Help Modal (?)` for layout/structure/colour-role (two columns, blue glyphs / strong labels, `? Keybindings` header + right `esc close`, complete keymap on a cleared canvas).
- [ ] LIGHT-MODE EYEBALL (§15.6): the `?` help modal is rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — the two-column glyph/label wiring (`accent.blue` glyph / `text.strong` action) and the header (`text.primary` / `text.detail` `esc close`) read correctly in light (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).

**Tests**:
- `"it opens the help modal on ? for Sessions"`
- `"it opens the help modal on ? for Projects"`
- `"it no longer lets bubbles/list self-toggle its own help on ? (the swallow is replaced by our modal)"`
- `"it generates the Sessions help content from the descriptor including footer-core keys"`
- `"it generates the Projects help content from the descriptor including help-only keys (d/n/nav/q)"`
- `"it renders key glyphs in accent.blue and action labels in text.strong"`
- `"it renders the header ? Keybindings left and esc close right with no contextual footer"`
- `"it closes on ? (toggle)"`
- `"it closes on Esc and does not fall through to clear-filter/quit (key-exclusive)"`
- `"it captures a Sessions help modal matching the Sessions — Help Modal (?) frame"`

**Edge Cases**:
- `Esc` while the help modal is open and a filter is applied — must dismiss the help modal only, NOT clear the filter (key-exclusive, §8.1).
- `?` while the help modal is open — toggles it closed (not a no-op, not a re-open).
- Help opened from Preview — out of scope this phase; flag as Phase 4 (Preview help overlays without blanking).
- The complete keymap must include keys that are ALSO in the footer (it is the full reference, not the footer's overflow) — assert a footer-core key appears in help.
- Long keymap exceeding panel height — the renderer must keep the panel within the cleared canvas (truncate/scroll per the existing modal primitive's behaviour); note thresholds are an implementation detail.

**Context**:
> §8.5: "A centred panel listing the current page's keymap (two columns: key-hint glyph in accent.blue / action label in text.strong), header ? Keybindings (text.primary), right-aligned esc close (text.detail) — the documented help-modal exception to §8.1. The help modal lists the page's complete keymap — including the keys also shown in the footer. Content source: the help modal is generated from the page's keymap descriptor — the single source of truth that also drives the footer and §12.1 — not hand-authored. … The help modal closes on ? (toggle) or Esc; while open it is key-exclusive (§8.1), so Esc dismisses it and does not fall through to the page's clear-filter / quit."
> §14.4: "The ? help modal — a new modal type + binding ? (currently swallowed) + a generic renderer over the per-page keymap descriptor … not hand-authored content per page (~60–80 lines for the modal type + renderer). Extends the existing rounded-border modal overlay primitive."
> §8.1: the help modal is the exception — dismiss hint in the header right-corner as `esc close` (no "to"), no contextual footer.
> Current code: `internal/tui/model.go` `updateSessionList` (line ~1968) and `updateProjectsPage` (line ~1561) both have `if isRuneKey(msg, "?") { return m, nil }` swallows. `modalStyle` is the rounded-border primitive in `internal/tui/modal.go`. Sessions descriptor from Phase 2 task 2-1; Projects descriptor from task 3-3.

**Spec Reference**: §8.5, §14.4, §8.1, §13.3, §12.1, §9.3 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-5 | approved

### Task spectrum-tui-design-3-5: Kill confirm modal reskin (MV) — destructive treatment + drop `n` + blank-screen

**Problem**: The kill confirm modal renders today as `fmt.Sprintf("Kill %s? (y/n)", m.pendingKillName)` (a plain `(y/n)` overlay over the list), and `updateKillConfirmModal` accepts `n` as cancel. §8.3 retargets it to the MV destructive treatment (`state.red` header `▲ Kill session?`, name in `state.red`, `· N window(s)` in `text.detail`, a consequence line, footer `y kill · esc cancel`) and §8.1 drops `n` (cancel is `Esc` only). The confirm action itself is preserved (parity).

**Solution**: Restyle the kill-modal content into the MV destructive panel and trim the keymap so cancel is `Esc`-only. The confirm path (`killAndRefresh`) is untouched. The modal inherits the blank-screen shell (3-1) and the `NO_COLOR` carve-out.

**Outcome**: The kill confirm modal matches `Kill Confirm Modal (MV)` and `Kill Confirm Modal (Light)` — `state.red` header `▲ Kill session?`, the session name in `state.red` with `· N window(s)` in `text.detail`, the consequence line "Ends the tmux session and all its panes. Can't be undone." in `text.detail`, and footer `y kill · esc cancel` — pressing `y` kills the session exactly as before, `Esc` cancels, and `n` no longer cancels.

**Do**:
- Build the kill-modal content (in `internal/tui/modal.go` and/or the kill branch of `viewSessionList`): header row `▲ Kill session?` with both `▲` and the title text in `state.red`; the session name in `state.red`; `· N window(s)` in `text.detail` (singular `window` for 1, plural `windows` otherwise); the consequence line "Ends the tmux session and all its panes. Can't be undone." in `text.detail`; a contextual footer `y kill · esc cancel` with key glyphs in `accent.blue` and labels in `text.detail`, the dismiss key as `esc cancel` (§8.1 — footer, never header).
- Capture the window count: today `handleKillKey` stores only `m.pendingKillName`. Add a `m.pendingKillWindows` field (from `si.Session.Windows`, available on `tmux.Session`) set in `handleKillKey`, and render it as `· N window(s)`. Clear it alongside `pendingKillName` on confirm/cancel.
- Drop `n` in `updateKillConfirmModal`: change `case isRuneKey(keyMsg, "n"), keyMsg.Type == tea.KeyEsc:` to `case keyMsg.Type == tea.KeyEsc:` so only `Esc` cancels. `y` still confirms (`killAndRefresh`).
- The modal renders on the cleared blank screen (inherits 3-1) and is border-defined with no distinct fill (§8.1). Under `NO_COLOR` the backdrop is native bg and the destructive emphasis carries via the `▲` glyph + text + bold (state never colour-only — §2.2/§2.5).
- Verify parity: read `updateKillConfirmModal` + `killAndRefresh`; confirm the only behaviour change is dropping `n`; the confirm action is byte-for-byte identical.

**Acceptance Criteria**:
- [ ] The kill modal renders a `state.red` header `▲ Kill session?` (glyph + title both red), the session name in `state.red`, `· N window(s)` in `text.detail` (correct singular/plural), and the consequence line "Ends the tmux session and all its panes. Can't be undone." in `text.detail`.
- [ ] The footer reads `y kill · esc cancel` (glyphs `accent.blue`, labels `text.detail`); the dismiss key is in the footer as `esc cancel`, never the header.
- [ ] `y` confirms the kill exactly as before (`killAndRefresh` path unchanged — parity); `Esc` cancels; `n` no longer cancels (it is ignored).
- [ ] The window count is captured at modal-open from `si.Session.Windows` and rendered correctly.
- [ ] The modal renders on the cleared blank screen (inherits 3-1), border-defined with no distinct fill; `state.red` is used for destructive emphasis only.
- [ ] Under `NO_COLOR` the modal clears to native bg and destructive state carries via the `▲` glyph + text + bold (not colour-only).
- [ ] No literal hex at the call sites — every colour is a §2.9 token.
- [ ] VISUAL VERIFICATION: `vhs` tapes drive the TUI to the kill modal (Sessions → `k`) in BOTH dark and light modes and write PNGs; compared against `Kill Confirm Modal (MV)` and `Kill Confirm Modal (Light)` for layout/structure/colour-role.

**Tests**:
- `"it renders a state.red ▲ Kill session? header"`
- `"it renders the session name in state.red and · N window(s) in text.detail"`
- `"it pluralises window/windows correctly (1 window vs N windows)"`
- `"it renders the consequence line in text.detail"`
- `"it renders the footer y kill · esc cancel"`
- `"it confirms the kill on y (parity with the pre-reskin killAndRefresh path)"`
- `"it cancels on Esc"`
- `"it ignores n (n no longer cancels)"`
- `"it captures the kill modal matching Kill Confirm Modal (MV) and (Light)"`

**Edge Cases**:
- Session with exactly 1 window → `· 1 window` (singular).
- Session with 0 windows (defensive) → `· 0 windows`.
- `n` keypress → ignored (no cancel, no confirm), consistent with §8.1 dropping `n`.
- `Esc` cancel must clear `pendingKillName` AND `pendingKillWindows` (no stale state).
- `NO_COLOR` → native bg, `▲` glyph + bold carries destructive emphasis.

**Context**:
> §8.3: "A centred panel with a state.red header ▲ Kill session?, the session name in state.red, · N window(s) (text.detail), a consequence line "Ends the tmux session and all its panes. Can't be undone." (text.detail), footer y kill · esc cancel. state.red is reserved for destructive actions. Keys: y (confirm) / Esc (cancel)."
> §8.1: destructive modals render the title + `▲` in `state.red`; the dismiss key always lives in the footer as `esc cancel`; drops `n`.
> Current code: `internal/tui/model.go` `viewSessionList` kill branch renders `fmt.Sprintf("Kill %s? (y/n)", m.pendingKillName)`; `updateKillConfirmModal` accepts `case isRuneKey(keyMsg, "n"), keyMsg.Type == tea.KeyEsc:` as cancel and `y` → `killAndRefresh`; `handleKillKey` stores `m.pendingKillName` only (window count must be added from `si.Session.Windows`). `tmux.Session.Windows` is an `int`.
> Reskin (§1): confirm action preserved; only rendering + the `n` drop change.

**Spec Reference**: §8.3, §8.1, §2.2, §2.5, §2.9, §1 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-6 | approved

### Task spectrum-tui-design-3-6: Rename modal reskin (MV) — labelled NEW NAME input + focus grammar + blank-screen

**Problem**: The rename modal renders today as a bare `m.renameInput.View()` (a `textinput` with `Prompt = "New name: "`) overlaid on the list. §8.4 retargets it to the MV treatment: header `Rename session` (`text.primary`), a labelled `NEW NAME` input (focused label `accent.violet`, value `text.primary` + a violet `▌` cursor), a `was: <old name>` context line (`text.detail`), and footer `↵ rename · esc cancel`. The rename flow is preserved (parity).

**Solution**: Restyle the rename-modal content into the MV labelled-input panel, applying the §13.1 focus-vs-edit grammar (the input is the live edit element — violet cursor), and keep the rename flow (`renameAndRefresh`, `Enter`/`Esc`) untouched. Inherits the blank-screen shell (3-1) and `NO_COLOR` carve-out.

**Outcome**: The rename modal matches `Rename Modal (MV)` — header `Rename session` in `text.primary`, a `NEW NAME` label in `accent.violet` over the input value in `text.primary` with a violet `▌` cursor, a `was: <old name>` line in `text.detail`, and footer `↵ rename · esc cancel` — pressing `Enter` renames exactly as before, `Esc` cancels.

**Do**:
- Build the rename-modal content (in the rename branch of `viewSessionList` / `internal/tui/modal.go`): header `Rename session` in `text.primary`; a labelled input — label `NEW NAME` in `accent.violet` (the focused label colour), the input value in `text.primary` with a violet `▌` cursor; a `was: <old name>` context line in `text.detail` (sourced from `m.renameTarget`); a contextual footer `↵ rename · esc cancel` (glyphs `accent.blue`, labels `text.detail`), dismiss key in the footer as `esc cancel`.
- Style the `textinput` to the MV palette: value `text.primary`, cursor `▌` in `accent.violet`. The input stays border-defined with a transparent fill (no recessed-input token — §8.1). Apply the §13.1 grammar: the live input is the editing element (violet cursor/fill treatment), not just a focus outline.
- The modal renders on the cleared blank screen (inherits 3-1). Under `NO_COLOR` the backdrop is native bg and the input renders colourless (cursor via the terminal default).
- Verify parity: read `handleRenameKey`, `updateRenameModal`, `renameAndRefresh`; confirm the rename flow (Enter commits a trimmed non-empty name → `renameAndRefresh`; empty trimmed name is a no-op; Esc cancels) is unchanged — only the rendering changes.

**Acceptance Criteria**:
- [ ] The rename modal renders header `Rename session` in `text.primary`.
- [ ] The input is labelled `NEW NAME` in `accent.violet`, value in `text.primary`, with a violet `▌` cursor.
- [ ] A `was: <old name>` context line renders in `text.detail` from `m.renameTarget`.
- [ ] The footer reads `↵ rename · esc cancel` (glyphs `accent.blue`, labels `text.detail`); dismiss key in the footer.
- [ ] `Enter` renames exactly as before (`renameAndRefresh` unchanged — parity); empty trimmed name is still a no-op; `Esc` cancels.
- [ ] The modal renders on the cleared blank screen (inherits 3-1), border-defined with transparent input fill.
- [ ] Under `NO_COLOR` the modal clears to native bg and renders colourless.
- [ ] No literal hex at the call sites — every colour is a §2.9 token.
- [ ] VISUAL VERIFICATION: a `vhs` tape drives the TUI to the rename modal (Sessions → `r`) and writes a PNG; compared against `Rename Modal (MV)` for layout/structure/colour-role (header, labelled NEW NAME input with violet cursor, `was:` line, footer).
- [ ] LIGHT-MODE EYEBALL (§15.6): the rename modal is rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — each light token reads correctly and the panel/input/`was:` line stay legible (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).

**Tests**:
- `"it renders the header Rename session in text.primary"`
- `"it renders a NEW NAME label in accent.violet over the input value in text.primary"`
- `"it renders a violet ▌ cursor in the input"`
- `"it renders the was: <old name> context line in text.detail"`
- `"it renders the footer ↵ rename · esc cancel"`
- `"it renames on Enter for a non-empty trimmed name (parity with renameAndRefresh)"`
- `"it is a no-op on Enter for an empty trimmed name (parity)"`
- `"it cancels on Esc"`
- `"it captures the rename modal matching the Rename Modal (MV) frame"`

**Edge Cases**:
- Empty / whitespace-only new name on `Enter` → no-op (existing behaviour preserved).
- Very long old name in the `was:` line → truncate with `…` so the panel does not overflow the canvas.
- `NO_COLOR` → native bg, colourless input.
- The `▌` cursor must follow the §13.1 edit grammar (the input is the editing element — violet), distinct from a navigate-mode outline.

**Context**:
> §8.4: "A header Rename session (text.primary), a labelled NEW NAME input (focused label accent.violet, value text.primary + violet ▌ cursor), a was: <old name> context line (text.detail), footer ↵ rename · esc cancel. Keys: Enter/Esc."
> §13.1: "Focused (navigate): outline only … Editing (cursor live): accent.violet fill + cursor … So: outline = focused, fill = editing." The rename input is the editing element.
> §8.1: inputs stay border-defined with a transparent fill (no recessed-input token); dismiss key in the footer as `esc cancel`.
> Current code: `internal/tui/model.go` `handleRenameKey` builds a `textinput` with `Prompt = "New name: "` seeded to `m.renameTarget`; `viewSessionList` rename branch renders `m.renameInput.View()`; `updateRenameModal` commits on `Enter` (trimmed non-empty → `renameAndRefresh`), cancels on `Esc`, delegates other keys to the textinput.
> Reskin (§1): rename flow preserved; only rendering changes.

**Spec Reference**: §8.4, §8.1, §13.1, §2.5, §2.9, §1 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-7 | approved

### Task spectrum-tui-design-3-7: Delete-project confirm modal reskin (MV) — mirror kill's destructive treatment with a record-only consequence line + drop `n` + blank-screen

**Problem**: The delete-project confirm modal renders today as `fmt.Sprintf("Delete %s? (y/n)", m.pendingDeleteName)` (a plain `(y/n)` overlay), and `updateDeleteProjectModal` accepts `n` as cancel. §8.6 retargets it to mirror the kill modal's destructive treatment but with a consequence line that DISAMBIGUATES deleting a project record from killing a session, and §8.1 drops `n` (cancel is `Esc` only). The confirm action is preserved (parity).

**Solution**: Restyle the delete-project-modal content into the MV destructive panel (mirroring kill — `state.red` header `▲ Delete project?`, project name in `state.red`, path in `text.detail`) with the record-only consequence line, and trim the keymap so cancel is `Esc`-only. The confirm path (`deleteAndRefreshProjects`) is untouched. Inherits the blank-screen shell (3-1) and `NO_COLOR` carve-out.

**Outcome**: The delete-project confirm modal mirrors `Kill Confirm Modal (MV)`'s destructive layout — `state.red` header `▲ Delete project?`, the project name in `state.red`, its path in `text.detail`, and the record-only consequence line "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." in `text.detail`, with footer `y delete · esc cancel` — pressing `y` deletes the project record exactly as before, `Esc` cancels, `n` no longer cancels. (No dedicated Paper frame exists; it is mocked at implementation mirroring `Kill Confirm Modal (MV)`.)

**Do**:
- Build the delete-project-modal content (in the delete branch of `viewProjectList` / `internal/tui/modal.go`): header `▲ Delete project?` with `▲` + title both in `state.red`; the project name (`m.pendingDeleteName`) in `state.red`; its path (`m.pendingDeletePath`) in `text.detail`; the consequence line "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." in `text.detail` (DISTINCT from kill's "Ends the tmux session…" line — this is the disambiguation requirement); footer `y delete · esc cancel` (glyphs `accent.blue`, labels `text.detail`), dismiss key in the footer.
- Drop `n` in `updateDeleteProjectModal`: change `case isRuneKey(keyMsg, "n"), keyMsg.Type == tea.KeyEsc:` to `case keyMsg.Type == tea.KeyEsc:` so only `Esc` cancels. `y` still confirms (`deleteAndRefreshProjects`).
- The modal renders on the cleared blank screen (inherits 3-1), border-defined with no distinct fill. Under `NO_COLOR` the backdrop is native bg and destructive emphasis carries via the `▲` glyph + text + bold (§2.2/§2.5).
- Verify parity: read `handleDeleteProjectKey`, `updateDeleteProjectModal`, `deleteAndRefreshProjects`; confirm the only behaviour change is dropping `n`; the confirm action is byte-for-byte identical.

**Acceptance Criteria**:
- [ ] The delete-project modal renders a `state.red` header `▲ Delete project?` (glyph + title both red), the project name in `state.red`, and its path in `text.detail`.
- [ ] The consequence line reads "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." in `text.detail` — DISTINCT from the kill modal's consequence line (record-only disambiguation).
- [ ] The footer reads `y delete · esc cancel` (glyphs `accent.blue`, labels `text.detail`); dismiss key in the footer.
- [ ] `y` deletes the project record exactly as before (`deleteAndRefreshProjects` unchanged — parity); `Esc` cancels; `n` no longer cancels (ignored).
- [ ] The modal renders on the cleared blank screen (inherits 3-1), border-defined with no distinct fill; `state.red` for destructive emphasis only.
- [ ] Under `NO_COLOR` the modal clears to native bg and destructive state carries via the `▲` glyph + text + bold.
- [ ] No literal hex at the call sites — every colour is a §2.9 token.
- [ ] VISUAL VERIFICATION: a `vhs` tape drives the TUI to the delete-project modal (Projects → `d`) and writes a PNG; because no dedicated Paper frame exists, it is mocked at implementation MIRRORING `Kill Confirm Modal (MV)` — compare for the same destructive layout/structure/colour-role, with the record-only consequence line substituted. State this in the capture review.

**Tests**:
- `"it renders a state.red ▲ Delete project? header"`
- `"it renders the project name in state.red and its path in text.detail"`
- `"it renders the record-only consequence line distinct from the kill consequence line"`
- `"it renders the footer y delete · esc cancel"`
- `"it deletes the project on y (parity with the pre-reskin deleteAndRefreshProjects path)"`
- `"it cancels on Esc"`
- `"it ignores n (n no longer cancels)"`
- `"it captures the delete-project modal mirroring Kill Confirm Modal (MV) with the record-only consequence line"`

**Edge Cases**:
- `n` keypress → ignored (no cancel, no confirm), consistent with §8.1 dropping `n`.
- `Esc` cancel must clear BOTH `pendingDeletePath` and `pendingDeleteName` (no stale state — preserve the existing clear logic).
- The consequence line must NOT read like kill's (no "Ends the tmux session…") — it must state the project record is removed but sessions/files are untouched.
- Very long project path → truncate with `…` so the panel does not overflow the canvas.
- `NO_COLOR` → native bg, `▲` glyph + bold carries destructive emphasis.

**Context**:
> §8.6: "A centred panel mirroring the kill modal's destructive treatment: a state.red header ▲ Delete project?, the project name in state.red, its path (text.detail), and a consequence line that disambiguates it from killing a session — it removes only the project record: "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched." (text.detail). Footer y delete · esc cancel. Keys: y (confirm) / Esc (cancel)." Header note: "(Mocked at implementation, mirroring Kill Confirm Modal (MV).)"
> §8.1: destructive modals render title + `▲` in `state.red`; drops `n`; dismiss key in the footer as `esc cancel`.
> Current code: `internal/tui/model.go` `viewProjectList` delete branch renders `fmt.Sprintf("Delete %s? (y/n)", m.pendingDeleteName)`; `updateDeleteProjectModal` accepts `case isRuneKey(keyMsg, "n"), keyMsg.Type == tea.KeyEsc:` as cancel and `y` → `deleteAndRefreshProjects`; `handleDeleteProjectKey` stores `m.pendingDeletePath` + `m.pendingDeleteName`.
> Reskin (§1): confirm action preserved; only rendering + the `n` drop change. No dedicated Paper frame — mirror `Kill Confirm Modal (MV)`.

**Spec Reference**: §8.6, §8.1, §2.2, §2.5, §2.9, §1 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-8 | approved

### Task spectrum-tui-design-3-8: Two-mode edit-project modal — interaction state machine + immediate-persist (⚠ behaviour change, parity does NOT apply)

**Problem**: The edit-project modal is asymmetric today: tags persist live (Enter adds via `AddTag`, `x` removes via `RemoveTag`) but Name and Aliases batch until the confirm `Enter` (`handleEditProjectConfirm`), and Esc discards the batched Name/Aliases edits. §8.2 replaces this with a UNIFORM two-mode (Navigate / Edit), immediate-persist model across NAME / ALIASES / TAGS. This is a DELIBERATE behaviour change — behaviour parity does NOT apply; implement exactly as specified.

**Solution**: Replace `handleEditProjectKey` / `updateEditProjectModal` / `handleEditProjectConfirm`'s asymmetric Name/Aliases-batch + tags-live logic with a single two-mode state machine that treats all three fields identically: Navigate mode (Tab/Shift+Tab across fields, ←/→ across chips + the trailing `+ add` slot, `x` deletes a focused chip, `Esc` closes) and Edit mode (one element live — type to edit, ←/→ move the text cursor, `Enter` commits & persists, `Esc` discards that element's edit). Persistence is immediate per item via the project store and alias store; there is no dirty state, no save key, no batch. This task is the interaction core + persistence; the render/chips/footers are task 3-9.

**Outcome**: The edit-project modal drives a uniform two-mode state machine: Tab/Shift+Tab/←/→ navigate, Enter/e/+ enter edit, Enter commits & persists each item immediately (Name via `Rename`, aliases via `SetAndSave`/`DeleteAndSave`, tags via `AddTag`/`RemoveTag`), Esc backs out one level (edit→discard element edit; navigate→close), and the falling-out rules (empty-on-commit = delete, empty Name reverts, duplicate-on-commit = silent no-op, brand-new empty chip vanishes on Esc) all hold — with no batch and no `handleEditProjectConfirm` save key.

**Do**:
- Read `internal/tui/model.go` `handleEditProjectKey`, `updateEditProjectModal`, `handleEditProjectConfirm`, the edit-field state (`editFieldName`/`editFieldAliases`/`editFieldTags`, `editName`, `editAliases`, `editAliasCursor`, `editNewAlias`, `editRemoved`, `editTags`, `editTagCursor`, `editNewTag`, `editTagsMutated`, `editError`), and `internal/project/tags.go` (`AddTag`/`RemoveTag`/`NormaliseTag` — case-sensitive). Confirm `handleEditProjectConfirm` already does NOT touch tags (it doesn't — tags persist live today); this task removes the batch entirely.
- Introduce an explicit mode state (navigate vs edit) plus a focused-element model: focus is (field, element-within-field), where a chip field's elements are its chips + a trailing `+ add` slot. Add the model fields needed (e.g. an `editMode` enum and a per-chip-field focus index) — wire alongside the existing `editFocus`/cursor fields, replacing the batch-specific ones (`editRemoved`, `editNewAlias`/`editNewTag` as batch buffers become per-edit live buffers).
- Navigate mode: `Tab`/`Shift+Tab` move between fields; `←`/`→` move across chips and the trailing `+ add` slot within a chip field. Entering a chip field via `Tab`/`Shift+Tab` lands on the trailing `+ add` slot (then `←` reaches the existing chips). `x` deletes a focused chip immediately (alias via `DeleteAndSave`, tag via `RemoveTag`). `Esc` closes the modal.
- Edit mode (one element live): entered by `Enter`/`e` on a chip, `Enter` on Name, or `Enter`/`+` on a focused `+ add` slot (which spawns a NEW empty chip already in edit mode — edit highlight + live cursor). Landing on `+ add` via `Tab`/`←/→` is navigate-mode focus only; it never auto-enters edit. Type to edit; `←`/`→` move the TEXT CURSOR within the value. `Enter` commits & persists → navigate; `Esc` discards that element's edit (a brand-new empty chip vanishes) → navigate.
- Persistence is immediate, per item, on exit-edit (`Enter`): Name → `m.projectEditor.Rename(path, name, "cli")`; new alias → `m.aliasEditor.SetAndSave(name, path, "cli")` (with the existing collision pre-check via `Load`); edited/removed alias → `DeleteAndSave` then `SetAndSave` as appropriate; tags → `m.projectEditor.AddTag(path, tag)` / `RemoveTag(path, tag)`. NO dirty state, NO save key, NO `handleEditProjectConfirm` batch.
- Falling-out rules: (1) empty-on-commit = delete (a chip committed empty is deleted — new or existing; deleting a focused chip via `x` is immediate); (2) empty Name can't persist → reverts to the prior value (no `Rename` call, no error modal that blocks); (3) duplicate-on-commit = silent no-op dedupe (committing a chip whose value already exists in the same field leaves the existing chip, adds nothing, shows no error — consistent with the store's per-field dedupe; tags case-sensitive via `NormaliseTag`); (4) `Esc` backs out one level (edit → discard element edit; navigate → close — all already saved).
- Because edits persist live, closing on `Esc` from navigate mode must NOT discard saved work and must refresh the cached project records + grouping index so changes are visible on return to Sessions (the existing `loadProjects()`/refresh-on-return path — preserve/extend it for ALL three fields now, not just tags).
- Remove `handleEditProjectConfirm` (or reduce it to nothing) and the batched Name/Aliases logic; remove the `editRemoved` batch buffer. Update `updateModal`'s `modalEditProject` dispatch if the handler signature changes.

**Acceptance Criteria**:
- [ ] Navigate mode: `Tab`/`Shift+Tab` move between NAME/ALIASES/TAGS; `←`/`→` move across chips + the trailing `+ add` slot within a chip field; entering a chip field via Tab/Shift+Tab lands on `+ add`; `x` deletes a focused chip immediately; `Esc` closes the modal.
- [ ] Edit mode: entered by `Enter`/`e` on a chip, `Enter` on Name, or `Enter`/`+` on a focused `+ add` slot (which spawns a new empty chip already in edit mode); landing on `+ add` via Tab/←→ is navigate focus only (no auto-edit); type edits, `←`/`→` move the text cursor; `Enter` commits & persists → navigate; `Esc` discards the element edit → navigate.
- [ ] Persistence is immediate per item — Name via `Rename`, aliases via `SetAndSave`/`DeleteAndSave`, tags via `AddTag`/`RemoveTag` — with NO dirty state, NO save key, NO batch; `Esc` never discards saved work.
- [ ] Empty-on-commit = delete (new or existing chip); empty Name reverts to prior (no persist); duplicate-on-commit = silent no-op dedupe (tags case-sensitive); a brand-new empty chip vanishes on `Esc`.
- [ ] Closing the modal refreshes the cached project records + grouping index so Name/Alias/Tag changes are visible on return to Sessions (extended to all three fields).
- [ ] The asymmetric batch logic (`handleEditProjectConfirm` save key, `editRemoved` batch buffer) is removed; `handleEditProjectConfirm` no longer touches tags (verified — it already doesn't).
- [ ] This is implemented as a deliberate behaviour change — parity vs the old asymmetric model does NOT apply; the acceptance is the new state machine + immediate-persist + falling-out rules.
- [ ] VISUAL VERIFICATION: this task is the interaction STATE MACHINE — its tests are state-transition + persistence + falling-out-rule tests, NOT a frame compare (the render/frames are task 3-9). State this; no `vhs` frame compare is required for 3-8.

**Tests**:
- `"it moves between NAME/ALIASES/TAGS on Tab and Shift+Tab in navigate mode"`
- `"it lands on the trailing + add slot when entering a chip field via Tab"`
- `"it moves across chips and the + add slot with ←/→ in navigate mode"`
- `"it deletes a focused chip immediately on x (alias via DeleteAndSave, tag via RemoveTag)"`
- `"it enters edit mode on Enter on Name and on Enter/e on a chip"`
- `"it spawns a new empty chip in edit mode on Enter/+ on a focused + add slot"`
- `"it treats landing on + add via Tab or ←/→ as navigate focus only (no auto-edit)"`
- `"it moves the text cursor within a value on ←/→ in edit mode"`
- `"it commits and persists Name on Enter via Rename"`
- `"it commits and persists a new alias on Enter via SetAndSave with the collision pre-check"`
- `"it commits and persists a new tag on Enter via AddTag"`
- `"it deletes an existing chip committed empty (empty-on-commit = delete)"`
- `"it reverts to the prior Name when Name is committed empty (no persist)"`
- `"it silently dedupes a duplicate-on-commit chip (no add, no error; tags case-sensitive)"`
- `"it vanishes a brand-new empty chip on Esc in edit mode"`
- `"it discards the current element edit on Esc in edit mode and returns to navigate"`
- `"it closes the modal on Esc in navigate mode without discarding saved work"`
- `"it refreshes cached project records + grouping index on close for Name/Alias/Tag changes"`

**Edge Cases**:
- Esc in edit mode on an EXISTING chip discards only that in-progress edit (the prior chip value remains) — distinct from Esc on a brand-new empty chip (which vanishes).
- Esc in navigate mode never discards saved work (everything is already persisted) — it just closes.
- Empty Name commit reverts to prior; it must NOT pop a blocking error modal (the old `editError = "Project name cannot be empty"` blocking flow is gone — the falling-out rule is a silent revert).
- Duplicate tag commit is case-sensitive (`NormaliseTag` preserves case): "Work" and "work" are distinct, so committing "work" when "Work" exists ADDS "work".
- New alias collision with ANOTHER project's path — preserve the existing collision pre-check (`Load` then reject if the alias maps to a different path); decide whether this surfaces as a revert/no-op or an inline error per §8.2 (note: §8.2's falling-out rules cover empty/duplicate/empty-name but not cross-project alias collision explicitly — see Context ambiguity note; default to a silent no-op revert consistent with the duplicate rule, leaving the existing mapping intact).
- `x` on a chip while in navigate mode deletes immediately; `x` typed while in edit mode is a literal character in the value.
- Tab/Shift+Tab while in edit mode — define behaviour: per §8.2, Tab/Shift+Tab are navigate-mode field moves; in edit mode `Enter` commits first. Treat Tab in edit mode as either ignored or commit-then-move; note the ambiguity in Context and pick commit-then-move only if the spec text supports it — default to ignore (edit mode is "one element live", field moves are a navigate-mode action).

**Context**:
> §8.2 (⚠ BEHAVIOUR CHANGE — NOT parity): "This replaces the current asymmetric model (tags persist live; Name/Aliases batch) with a uniform two-mode immediate-persist model across all three fields. Behaviour parity does not apply here." Navigate mode: Tab/Shift+Tab move fields; ←/→ across chips + the trailing + add slot; entering a chip field via Tab/Shift+Tab lands on + add; x deletes a focused chip immediately; Esc closes. Edit mode: entered by Enter/e on a chip, Enter on Name, or Enter/+ on a focused + add slot (spawns a new empty chip already in edit mode); type to edit; ←/→ move the text cursor; Enter commits & persists → navigate; Esc discards that element's edit (a brand-new empty chip vanishes) → navigate. "Persistence is immediate, per item … There is no dirty state, no save key, no batch; Esc never discards saved work … This extends the codebase's existing tags-persist-live behaviour to Name + Aliases."
> Falling-out rules: empty-on-commit = delete; empty Name can't persist → reverts; duplicate-on-commit = no-op silent dedupe (tags case-sensitive); Esc backs out one level.
> §13.1: focus vs edit grammar (outline = focused, fill = editing) — the render in 3-9, but the state machine here must distinguish the two modes per element.
> Current code: `internal/tui/model.go` `handleEditProjectKey` seeds `editName`/`editAliases`/`editTags` + cursors; `updateEditProjectModal` has the asymmetric Tab cycle, Enter-adds-tag-live + Enter-confirms-name/aliases-batch, Up/Down cursor, Runes-x-removes / Runes-types branches; `handleEditProjectConfirm` does the batched Name (`Rename`) + alias (`SetAndSave`/`DeleteAndSave`) saves and explicitly does NOT touch tags. `internal/project/tags.go`: `AddTag`/`RemoveTag` persist immediately + dedupe per-field; `NormaliseTag` trims + preserves case (case-sensitive). `ProjectEditor` = {Rename, AddTag, RemoveTag}; `AliasEditor` = {Load, SetAndSave, DeleteAndSave}.
> AMBIGUITY (note, do not invent silently): §8.2's falling-out rules enumerate empty/duplicate/empty-name but not (a) cross-project alias collision or (b) Tab/Shift+Tab pressed while in edit mode. Default cross-project alias collision to a silent no-op revert (consistent with the duplicate-dedupe rule, leaving the existing mapping); default Tab/Shift+Tab in edit mode to ignored (edit mode is one-element-live; field moves are a navigate action). Surface both in the task's Context for the implementer to confirm against the live spec/Paper frames.

**Spec Reference**: §8.2, §13.1, §5.5 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`

## spectrum-tui-design-3-9 | approved

### Task spectrum-tui-design-3-9: Two-mode edit-project modal — MV render + chip grammar + contextual footers (three frames)

**Problem**: The edit-project modal renders today with `renderEditProjectContent` / `renderEditListField` — a plain `> ` indicator, `[x] <entry>` rows, an `Add: <input>` row, and a `[Enter] Save [Esc] Cancel [Tab] Switch field` footer — none of it tokenised or chip-shaped. §8.2's visual states + §13.1's focus-vs-edit grammar retarget this to MV: neutral chips (never green) with three states, focused-field labels in `accent.violet`, a `+ add` inline slot in `text.faint`, a `◉ EDIT MODE` indicator shown ONLY while editing in place, and three per-mode contextual footers. This is the render layer over the 3-8 state machine.

**Solution**: Rewrite `renderEditProjectContent` (and `renderEditListField`) to render the MV edit-modal: header `Edit Project <name>` with a right-corner `◉ EDIT MODE` (only while editing in place, empty otherwise), labelled NAME / ALIASES / TAGS fields (focused label `accent.violet`, others `text.detail`), chips in one neutral style (`text.primary` on a subtle tint) with three states (normal / focused / editing), a `+ add` inline slot in `text.faint`, and a contextual footer that matches the current focus/mode. All tokens from §2.9; renders on the blank screen (3-1). Matches the three edit-modal Paper frames.

**Outcome**: The edit-project modal matches `Edit Modal — navigate (name)`, `Edit Modal — chip focused`, and `Edit Modal — edit in place` for layout/structure/colour-role — chips are one neutral style (never green) with normal / focused (violet outline + violet `✕`) / editing (violet fill + cursor, no `✕`) states, the focused field's label is `accent.violet` (others `text.detail`), `+ add` is a `text.faint` inline slot, `◉ EDIT MODE` shows only while editing in place, and the footer reads the correct per-mode string — all driven by the 3-8 state machine.

**Do**:
- Rewrite `renderEditProjectContent` in `internal/tui/model.go`: header `Edit Project <name>` (`text.primary`) with a right-corner `◉ EDIT MODE` (`accent.violet`) shown ONLY while editing in place (empty otherwise — no standing "navigate" label, per §8.1/§8.2). Read the 3-8 mode + focus state.
- Render NAME / ALIASES / TAGS field labels: the FOCUSED field's label in `accent.violet`, the others in `text.detail` (§8.2). The NAME value is an editable element following the §13.1 grammar (focused = outline; editing = violet fill + cursor).
- Replace `renderEditListField`'s `[x] <entry>` / `Add: <input>` rendering with chip elements (aliases AND tags use ONE neutral style — `text.primary` on a subtle tint, NEVER green — §2.9/§13.1):
  - normal: subtle tint, no `✕`;
  - focused (navigate): `accent.violet` outline + a violet `✕` (showing it's removable via `x`);
  - editing: `accent.violet` fill + cursor, no `✕`.
- Render the `+ add` trailing slot as an inline input slot (not a button/popup) in `text.faint`; when it spawns a chip-in-edit-mode (3-8), that new chip renders in the editing state.
- Render the contextual footer per focus/mode (§8.2 / §13.x):
  - Name focused (navigate): `↵ edit · ⇥ next field · esc close`;
  - Chip focused (navigate): `↵/e edit · x remove · ←→ move · ⇥ next field · esc close`;
  - Editing in place: `↵ save · esc discard · ←→ cursor · empty on save = delete`.
  Key glyphs in `accent.blue`, labels in `text.detail`; the dismiss verb differs by mode (`esc close` in navigate, `esc discard` in edit) — never "esc to …".
- The modal stays a SINGLE bundle for NAME + ALIASES + TAGS (not split). It renders on the cleared blank screen (inherits 3-1), border-defined with no distinct fill (§8.1). Under `NO_COLOR` it clears to native bg and the chip states carry via the `✕` glyph + outline/fill attributes + bold/dim (state never colour-only — §2.2).
- Remove the legacy `[Enter] Save [Esc] Cancel [Tab] Switch field` footer and the `[x] <entry>` / `Add:` rendering. No literal hex at the call sites — every colour a §2.9 token.

**Acceptance Criteria**:
- [ ] Chips (aliases AND tags) render in ONE neutral style — `text.primary` on a subtle tint, NEVER green — with three states: normal (subtle, no `✕`), focused (violet outline + violet `✕`), editing (violet fill + cursor, no `✕`).
- [ ] The focused field's label is `accent.violet`; the other field labels are `text.detail`.
- [ ] `+ add` renders as an inline input slot in `text.faint`.
- [ ] `◉ EDIT MODE` (`accent.violet`) shows in the header right-corner ONLY while editing in place; it is empty otherwise (no standing "navigate" label).
- [ ] The contextual footer matches the current focus/mode exactly: Name focused → `↵ edit · ⇥ next field · esc close`; Chip focused → `↵/e edit · x remove · ←→ move · ⇥ next field · esc close`; Editing in place → `↵ save · esc discard · ←→ cursor · empty on save = delete`.
- [ ] The header reads `Edit Project <name>` in `text.primary`.
- [ ] The modal is a single bundle (NAME + ALIASES + TAGS), renders on the cleared blank screen (inherits 3-1), border-defined with no distinct fill.
- [ ] Under `NO_COLOR` the modal clears to native bg and chip states carry via the `✕` glyph + outline/fill + bold/dim (not colour-only).
- [ ] No literal hex at the call sites — every colour is a §2.9 token; the legacy `[x]`/`Add:`/`[Enter] Save` rendering is removed.
- [ ] VISUAL VERIFICATION: `vhs` tapes drive the TUI to each of the three states — (a) navigate focus on Name, (b) a chip focused, (c) editing a chip in place — and write PNGs; compared against `Edit Modal — navigate (name)`, `Edit Modal — chip focused`, and `Edit Modal — edit in place` respectively for layout/structure/colour-role (chip states, violet field label, `+ add` faint slot, `◉ EDIT MODE` only-while-editing, per-mode footer).
- [ ] LIGHT-MODE EYEBALL (§15.6): all three edit-modal states are rendered in light mode against `#e1e2e7` and visually confirmed in a real terminal — chip tint/outline/fill, the violet field label, the `+ add` faint slot, and the `◉ EDIT MODE` indicator each read correctly in light (no further Paper mock required per §15.6; this is the deferred light eyeball task 1-9 punted to this surface, not a frame compare).

**Tests**:
- `"it renders chips in one neutral style (text.primary on a subtle tint, never green)"`
- `"it renders a normal chip with no ✕"`
- `"it renders a focused chip with a violet outline and a violet ✕"`
- `"it renders an editing chip with a violet fill and cursor and no ✕"`
- `"it renders the focused field label in accent.violet and others in text.detail"`
- `"it renders the + add slot in text.faint"`
- `"it shows ◉ EDIT MODE only while editing in place and empty otherwise"`
- `"it renders the Name-focused footer ↵ edit · ⇥ next field · esc close"`
- `"it renders the chip-focused footer ↵/e edit · x remove · ←→ move · ⇥ next field · esc close"`
- `"it renders the editing-in-place footer ↵ save · esc discard · ←→ cursor · empty on save = delete"`
- `"it renders the header Edit Project <name> in text.primary"`
- `"it keeps NAME + ALIASES + TAGS in a single bundled modal"`
- `"it carries chip state via the ✕ glyph + outline/fill under NO_COLOR (not colour-only)"`
- `"it captures the three edit-modal states matching the navigate-name / chip-focused / edit-in-place frames"`

**Edge Cases**:
- A field with zero chips — the `+ add` slot is the only element; focused-field label still violet, footer reads the `+ add`-focused (navigate) form (Name-focused-style minus chip actions, per the spec footer set; default to the chip-focused footer's navigable subset when `+ add` is the focused element — confirm against the Paper frames).
- The brand-new empty chip spawned in edit mode (3-8) renders in the editing state (violet fill + cursor, no `✕`).
- A green token must NEVER appear on a chip (§2.9/§13.1 — green is attached-only) — assert no `state.green` in chip rendering.
- `◉ EDIT MODE` must be absent (not "navigate" / not a placeholder) when in navigate mode.
- `NO_COLOR` → native bg; focused vs editing distinguished by the `✕` glyph + outline-vs-fill + bold/dim, not colour.
- Long chip value or many chips overflowing the panel width — wrap or truncate per the modal primitive so the panel stays within the canvas; thresholds are an implementation detail.

**Context**:
> §8.2 (Visual states — the focus-vs-edit grammar, §13): "Chips (aliases AND tags) are one neutral style — text.primary on a subtle tint; never green. Three states: normal (subtle, no ✕) · focused (accent.violet outline + a violet ✕ …) · editing (accent.violet fill + cursor, no ✕). Field labels: the focused field's label is accent.violet; the others are text.detail. + add is an inline input slot (not a button/popup) in text.faint; the mode indicator reads ◉ EDIT MODE (accent.violet) in edit mode, dim in navigate." Header: "the right-corner shows ◉ EDIT MODE only while editing in place, empty otherwise — no standing "navigate" label."
> Contextual footers: Name focused (navigate): `↵ edit · ⇥ next field · esc close`; Chip focused (navigate): `↵/e edit · x remove · ←→ move · ⇥ next field · esc close`; Editing in place: `↵ save · esc discard · ←→ cursor · empty on save = delete`. "The modal stays a single bundle for Name + Aliases + Tags (not split)."
> §13.1: outline = focused, fill = editing — unambiguous everywhere; the Name field in edit mode also turns violet-filled (same treatment as chips). §2.9: "chips are text.primary on a tint, never green" (green is attached/live-positive only).
> §8.1: the dismiss key/verb differs by mode (`esc close` navigate, `esc discard` edit) — never "esc to …".
> Current code: `internal/tui/model.go` `renderEditProjectContent` writes `Edit: <name>`, a `Name: <val>` line with a `> ` indicator, calls `renderEditListField` for Aliases + Tags (which emits `[x] <entry>` rows + `Add: <input>` + a `(none)` empty state + `> ` cursor markers), and a trailing `[Enter] Save [Esc] Cancel [Tab] Switch field` line. All of this is replaced. The render reads the 3-8 mode/focus state.
> Depends on 3-8 (state machine) for the mode/focus state it renders, and on 3-1 (blank-screen shell). Three named Paper frames.

**Spec Reference**: §8.2, §13.1, §13.x, §8.1, §2.9, §2.2, §2.5, §1 — `/Users/leeovery/Code/portal/.workflows/spectrum-tui-design/specification/spectrum-tui-design/specification.md`
