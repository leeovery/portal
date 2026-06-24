---
topic: spectrum-tui-design
cycle: 1
total_proposed: 4
---
# Analysis Tasks: spectrum-tui-design (Cycle 1)

## Task 1: Extract one shared footer key-hint helper and collapse the parallel footer-group types
status: approved
severity: high
sources: duplication, architecture, standards

**Problem**: The `<key/glyph> <label>` footer-hint primitive — a key glyph in `accent.blue`, a one-cell canvas-painted gap, and a label in `text.detail`, joined horizontally — is independently re-authored across modal and footer render sites. The byte-identical body (`headerStyle(theme.MV.AccentBlue, mode, colourless).Render(key)` + `headerCanvasBg(mode, colourless).Render(" ")` + `headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)` joined via `lipgloss.JoinHorizontal`) appears in `killModalKeyHint` (kill_modal.go:138), `deleteModalKeyHint` (delete_modal.go:144), `renameModalKeyHint` (rename_modal.go:183), `previewFooterHint` (pagepreview.go:318), and `editFooterGroup` (edit_modal.go:561), with `renderFooterEntry` (footer.go:262) implementing the same shape parameterised by token. The in-source comments admit the copy ("mirrors killModalKeyHint", "the shared footer key-hint shape"). The same `{glyph,label}` concept is also modelled by two parallel struct types, `footerGroup` (edit_modal.go:518) and `previewFooterGroup` (pagepreview.go:240). This is well past the Rule of Three: the `accent.blue` key / `text.detail` label / single-gap convention is the §3.4/§8.x footer contract, and any change to it (key-glyph colour, gap width) must be hand-applied in five-to-six places or the modals silently diverge from the footer. The three modal footer-row functions (`killModalFooterRow` kill_modal.go:129, `deleteModalFooterRow` delete_modal.go:134, `renameModalFooterRow` rename_modal.go:174) compound this — each hand-assembles confirm-hint + fixed canvas gap + cancel-hint + horizontal join, differing only in the key/label/gap constants.

**Solution**: Extract a single shared `renderKeyHint(key, label string, keyTok theme.Token, mode theme.Mode, colourless bool) string` helper into a shared file (e.g. a new `internal/tui/modal_footer.go`, or `footer.go`) that renders the key-segment + one-cell canvas gap + label, honouring an empty-key label-only fast path (so `editFooterGroup`'s "empty on save = delete" label-only group collapses onto it). Re-point `killModalKeyHint`, `deleteModalKeyHint`, `renameModalKeyHint`, `previewFooterHint`, and `editFooterGroup` (and, where natural, `renderFooterEntry`) through it. Collapse `footerGroup` and `previewFooterGroup` into one `{Key/Glyph, Label}` type. Add a companion `renderConfirmCancelFooter(confirmKey, confirmLabel, cancelKey, cancelLabel string, mode theme.Mode, colourless bool) string` that renders the two-hint footer (confirm-hint + fixed canvas gap + cancel-hint) via the new key-hint helper, and route the three modal footer-row functions through it so their per-modal constants (y/kill, y/delete, ⏎/rename, all paired with esc/cancel) become arguments.

**Outcome**: One canonical implementation of the footer key-hint shape and one of the confirm/cancel footer row; the `accent.blue` key / `text.detail` label / single-canvas-gap convention is defined in exactly one place. A future change to the footer-hint colour role or gap width is a one-line edit. The duplicate `footerGroup`/`previewFooterGroup` value types are unified. The rendered output of every footer and modal footer is byte-identical to before.

**Do**:
1. Create `internal/tui/modal_footer.go` (or extend `internal/tui/footer.go`) with `renderKeyHint(key, label string, keyTok theme.Token, mode theme.Mode, colourless bool) string` that renders `headerStyle(keyTok, mode, colourless).Render(key)` + `headerCanvasBg(mode, colourless).Render(" ")` + `headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)` joined with `lipgloss.JoinHorizontal`, defaulting `keyTok` callers to `theme.MV.AccentBlue`. Add an empty-key early-return that renders the label-only group (matching `editFooterGroup`'s current empty-key fast path).
2. Replace the bodies of `killModalKeyHint`, `deleteModalKeyHint`, `renameModalKeyHint`, and `previewFooterHint` with a call to `renderKeyHint`, preserving each call site's existing `key`/`label`/`mode`/`colourless` arguments.
3. Replace `editFooterGroup`'s body with a call to `renderKeyHint`, confirming the empty-key label-only path produces identical output.
4. Collapse `footerGroup` (edit_modal.go) and `previewFooterGroup` (pagepreview.go) into a single shared `{Key/Glyph, Label}` struct type in the shared file; update both consumers.
5. Add `renderConfirmCancelFooter(confirmKey, confirmLabel, cancelKey, cancelLabel string, mode theme.Mode, colourless bool) string` that renders confirm-hint + the fixed-gap canvas spacer (the existing `*FooterGap` const, `"   "`) + cancel-hint, joined horizontally, via `renderKeyHint`.
6. Replace the bodies of `killModalFooterRow`, `deleteModalFooterRow`, and `renameModalFooterRow` with calls to `renderConfirmCancelFooter`, passing each modal's existing key/label/gap constants as arguments.
7. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Acceptance Criteria**:
- Exactly one function renders the `<key/glyph> <label>` footer-hint shape; `killModalKeyHint`, `deleteModalKeyHint`, `renameModalKeyHint`, `previewFooterHint`, and `editFooterGroup` all route through it (or are removed in favour of direct calls).
- Exactly one struct type models the `{Key/Glyph, Label}` footer-group concept; `footerGroup` and `previewFooterGroup` are unified.
- Exactly one function renders the confirm/cancel two-hint footer row; the three modal footer-row functions route through it.
- The empty-key (label-only) case is handled inside the shared key-hint helper and produces output identical to the prior `editFooterGroup` behaviour.
- `go build` succeeds and the full `internal/tui` test package passes.
- The rendered footer/modal-footer output (key colour `accent.blue`, label colour `text.detail`, single canvas-painted gap, confirm/cancel gap) is unchanged for every call site in both light and dark modes and under the `colourless`/NO_COLOR carve-out.

**Tests**:
- A table-driven test asserting `renderKeyHint` produces the expected rendered string for a normal key+label pair and for the empty-key label-only case, in both `mode` values and with `colourless` true/false.
- A test asserting `renderConfirmCancelFooter` matches the prior hand-assembled output for the kill (y/esc), delete (y/esc), and rename (⏎/esc) constant sets.
- Regression: assert each refactored modal/footer render function's output is byte-identical to a captured golden of the pre-refactor output for representative inputs.

## Task 2: Consolidate the kill / delete-project destructive-confirm modals behind one parameterised builder
status: approved
severity: high
sources: duplication, architecture

**Problem**: `internal/tui/delete_modal.go` is a near-verbatim clone of `internal/tui/kill_modal.go` — the comments throughout delete_modal.go say so ("Mirrors killModalHeaderRow", "mirrors the kill modal's session-name row", "Mirrors killModalConsequenceRows", "Mirrors killModalFooterRow", "Mirrors killModalKeyHint"). Both are the same domain element — a "destructive confirm" panel: a `state.red` `▲ <Title>` header (identical render: `▲` glyph + gap + title, both `state.red`+bold via the same `headerStyle` calls), a body of `state.red`+bold target name followed by a single canvas-painted blank separator row and a word-wrapped `text.detail` consequence line at body-width 52 (identical `ansi.Wordwrap(<text>, <width>, "")` → `strings.Split` → per-line `headerStyle(TextDetail).Render` loops), and a `y <verb> · esc cancel` footer. The deltas are pure data: the title string, the confirm verb, the consequence copy, and one extra body row (the project path) for delete. The per-compartment render logic — `killModalHeaderRow`/`deleteModalHeaderRow`, `killModalConsequenceRows`/`deleteModalConsequenceRows`, the body-assembly pattern, and the footer rows — is duplicated wholesale (kill_modal.go:62-143, delete_modal.go:69-149). A colour-role, glyph, spacing, or body-width change must be applied in two places or the two modals silently diverge.

**Solution**: Introduce a shared `renderDestructiveConfirm` renderer (parameterised directly or via a `destructiveConfirmSpec` struct) that owns the destructive treatment once: the `state.red ▲ <Title>` header, the `state.red`+bold target name row, optional extra body rows (the delete modal's project path), the single canvas blank separator, the word-wrapped `text.detail` consequence at the shared body-width const (52), and the `y <verb> · esc cancel` footer (reusing the confirm/cancel footer helper from Task 1 if present, but not depending on Task 1 — fall back to the existing footer-row functions if the helper does not yet exist). Reduce both `renderKillModalContent` and `renderDeleteModalContent` to supplying only their distinct data (title/glyph, target name, the optional path row for delete, consequence copy, footer verb). Keep the modal update/state logic (`updateKillConfirmModal` / `updateDeleteProjectModal`) separate and untouched — this is render-layer consolidation only.

**Outcome**: The destructive-confirm panel grammar (red `▲` title, red target name, canvas separator, `text.detail` consequence at body-width 52, `y verb · esc cancel` footer) is defined once. delete_modal.go collapses to its distinct strings (`deleteTitle`, `deleteConsequence`, the path row) plus a call; kill_modal.go likewise supplies only its data. Both modals render byte-identically to before; a future change to the destructive treatment is a single edit. The modal-specific update/state logic remains separate.

**Do**:
1. Define a `destructiveConfirmSpec` struct (or an equivalent parameter set) capturing: title string, header glyph (`▲`), target name (red-emphasised), optional `extraBodyRows []string` (the delete path row), word-wrapped consequence text, body-width const, and footer confirm key/verb (cancel is always esc/cancel).
2. Implement `renderDestructiveConfirm(spec destructiveConfirmSpec, mode theme.Mode, colourless bool) string` that builds: the `state.red ▲ <Title>` header (factored from the identical `killModalHeaderRow`/`deleteModalHeaderRow` logic), the red+bold target name row, any extra body rows, the single canvas-painted blank separator, the word-wrapped `text.detail` consequence (factored from the identical `killModalConsequenceRows`/`deleteModalConsequenceRows` word-wrap loops at width 52), and the `y <verb> · esc cancel` footer. Compose through the existing `renderJoinedPanel`.
3. Rewrite `renderKillModalContent` to construct a `destructiveConfirmSpec` from the kill title/verb/consequence/target and call `renderDestructiveConfirm`.
4. Rewrite `renderDeleteModalContent` to construct a `destructiveConfirmSpec` from the delete title/verb/consequence/target plus the project-path `extraBodyRows`, and call `renderDestructiveConfirm`.
5. Remove the now-dead per-modal helpers (`killModalHeaderRow`, `deleteModalHeaderRow`, `killModalConsequenceRows`, `deleteModalConsequenceRows`, and the duplicated body-assembly/footer logic) whose behaviour now lives in the shared renderer.
6. Leave `updateKillConfirmModal` and `updateDeleteProjectModal` unchanged.
7. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Acceptance Criteria**:
- A single `renderDestructiveConfirm` (or `destructiveConfirmSpec` + one renderer) owns the destructive-confirm panel render; both kill and delete content functions supply only their distinct data.
- The delete modal's extra project-path body row is expressed as data passed to the shared renderer, not as a forked render path.
- The body-width const (52), the `▲` glyph, the `state.red` title/target colour role, the `text.detail` consequence colour, and the `y verb · esc cancel` footer shape are each defined in exactly one place.
- The modal update/state logic (`updateKillConfirmModal`, `updateDeleteProjectModal`) is unchanged.
- `go build` succeeds and the `internal/tui` test package passes.
- The rendered kill and delete modal content is byte-identical to the pre-refactor output (including header, target name, separator, consequence wrap, path row for delete, and footer) in both light and dark modes and under the colourless carve-out.

**Tests**:
- A test asserting `renderDestructiveConfirm` produces the expected output for a kill spec (no extra body rows) and a delete spec (with the path extra-body row), in both modes and with `colourless` true/false.
- Regression: assert `renderKillModalContent` and `renderDeleteModalContent` outputs are byte-identical to captured goldens of the pre-refactor render for representative session/project names and consequence copy.
- A test asserting the consequence word-wrap at body-width 52 matches the prior `killModalConsequenceRows`/`deleteModalConsequenceRows` line-splitting for a multi-line consequence string.

## Task 3: Extract shared row-style and left-bar-column helpers for the Session/Project list delegates
status: approved
severity: medium
sources: duplication

**Problem**: `ProjectDelegate.rowBg` and `ProjectDelegate.rowToken` (project_item.go:83-106) are byte-identical to `SessionDelegate.rowBg` and `SessionDelegate.rowToken` (session_item.go:303-327) — the comments say so ("Mirrors SessionDelegate.rowBg" / "Mirrors SessionDelegate.rowToken"): same `colourless` guard, same `bg.selection`-vs-`canvas` branch, same Foreground/Background composition. Separately, the §3.3 left-bar column rendering is duplicated verbatim in `renderSessionRow` (session_item.go:370-374) and `renderRowLine` (project_item.go:158-162): `bg.Render(padTo("", leftBarColumnWidth))` for the unselected case, and `rowToken(..., AccentViolet, true).Render(selectorBar) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(selectorBar)))` for the selected case. The two delegates were written independently and converged on the same selection grammar, so the row-style helpers and the 2-cell selector-bar column logic now live in two places — a colour-role or selector-width change must be applied in both or they diverge.

**Solution**: Extract the row-background and row-token style logic into shared free functions (e.g. `rowBgStyle(mode theme.Mode, selected, colourless bool)` and `rowTokenStyle(base lipgloss.Style, fg theme.Token, mode theme.Mode, selected, colourless bool)`) and a shared `renderLeftBarColumn(styler, selected)` helper for the §3.3 2-cell selector-bar column, all homed in session_item.go (both delegates already depend on the shared `selectorBar`/`leftBarColumnWidth`/`padTo` primitives there). Re-point both delegates' `rowBg`/`rowToken` methods and both left-bar render sites at the shared helpers.

**Outcome**: The row-background/selection-token style logic and the §3.3 left-bar selector-column rendering are each defined once and called by both the Session and Project delegates. A future change to the selection background role, the accent.violet selector colour, or the 2-cell column width is a single edit. Both delegates render byte-identically to before.

**Do**:
1. In session_item.go, add free functions `rowBgStyle(mode theme.Mode, selected, colourless bool) lipgloss.Style` and `rowTokenStyle(base lipgloss.Style, fg theme.Token, mode theme.Mode, selected, colourless bool) lipgloss.Style` carrying the existing `colourless` guard and `bg.selection`-vs-`canvas` branch logic.
2. Re-point `SessionDelegate.rowBg`/`rowToken` and `ProjectDelegate.rowBg`/`rowToken` to delegate to these free functions (or remove the methods if call sites can call the free functions directly).
3. Add `renderLeftBarColumn(styler ..., selected bool) string` in session_item.go that renders the unselected case (`bg.Render(padTo("", leftBarColumnWidth))`) and the selected case (`rowToken(..., AccentViolet, true).Render(selectorBar) + bg.Render(padTo("", leftBarColumnWidth-lipgloss.Width(selectorBar)))`) using the shared `selectorBar`/`leftBarColumnWidth`/`padTo` primitives.
4. Replace the duplicated left-bar blocks in `renderSessionRow` (session_item.go:370-374) and `renderRowLine` (project_item.go:158-162) with calls to `renderLeftBarColumn`.
5. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Acceptance Criteria**:
- `rowBg`/`rowToken` style logic exists in exactly one place; both delegates route through the shared free functions.
- The §3.3 left-bar selector-column render (both selected and unselected cases) exists in exactly one place; both `renderSessionRow` and `renderRowLine` call it.
- `go build` succeeds and the `internal/tui` test package passes.
- The rendered Session and Project list rows (selection background, accent.violet selector bar, 2-cell column width, indentation) are byte-identical to before in both light and dark modes and under the colourless carve-out, for selected and unselected rows.

**Tests**:
- A test asserting `rowBgStyle` and `rowTokenStyle` produce the expected styles for selected/unselected × both modes × colourless true/false.
- A test asserting `renderLeftBarColumn` produces the expected output for the selected (selector bar + padding) and unselected (full-width padding) cases.
- Regression: assert `renderSessionRow` and `renderRowLine` outputs are byte-identical to captured goldens of the pre-refactor render for a selected and an unselected row.

## Task 4: Remove the stale post-detection documentation and the dead dark-pinned cursorStyle var
status: approved
severity: low
sources: standards

**Problem**: Two artifacts in the theme/session-item layer misstate the now-shipped light/dark detection (§2.6, which landed in 1-7 via appearance_gate.go + model.canvasMode + syncResolvedMode). (1) The `internal/tui/theme/theme.go` package doc (lines 1-12) states "Detection (§2.6) lands in 1-7; until then ColorFor(Dark) is what every Color() call resolves to" and "Resolution currently defaults to the DARK variant ... Detection (§2.6) lands in 1-7", and the `Token.Color()` method comment (lines 57-64) repeats "Until light/dark detection lands (1-7) this is always the DARK variant." Detection HAS landed — every live renderer resolves per-mode via `ColorFor(mode)` — so these narrative comments are stale and would mislead a future reader into thinking the resolver still hard-pins dark. (2) The package-level `cursorStyle = lipgloss.NewStyle().Foreground(theme.MV.AccentViolet.Color())` in session_item.go:15-21 is unreferenced in production; its comment claims it is "Retained for the projects page (project_item.go), which still resolves the dark default", but project_item.go fully resolves per-mode through `ProjectDelegate.rowToken → AccentViolet.ColorFor(d.Mode)` and never reads `cursorStyle` (the only live `cursorStyle` is a shadowing local in edit_modal.go:170). This is dead code (YAGNI / code-quality.md) plus a comment that misstates the actual project-page implementation, and it is the last surviving dark-pinned `.Color()` call at a would-be render site — leaving it invites a future contributor to wire it back in and silently break light mode. No behavioural impact today; the risk is future regression and misdirection.

**Solution**: Update the theme.go package doc and the `Token.Color()` doc to state that the resolved mode flows from the appearance gate (`canvasMode`) via `ColorFor(mode)`, and that `Color()` is a dark-pinned convenience retained only for not-yet-mode-resolved call sites — removing the "lands in 1-7 / until then" phrasing now that 1-7 has shipped. Delete the unused `cursorStyle` var and its misleading comment from session_item.go.

**Outcome**: The theme-layer documentation accurately describes the shipped detection flow; no comment claims detection is still pending. The dead dark-pinned `cursorStyle` var is gone, removing the last would-be render-site `.Color()` call that could be re-wired to break light mode. No behavioural change.

**Do**:
1. In `internal/tui/theme/theme.go`, rewrite the package doc (lines 1-12) to describe the shipped flow: the resolved mode flows from the appearance gate (`canvasMode`) into renderers via `ColorFor(mode)`. Remove the "Detection (§2.6) lands in 1-7; until then ColorFor(Dark)..." and "Resolution currently defaults to the DARK variant ... lands in 1-7" phrasing.
2. In the same file, update the `Token.Color()` method comment (lines 57-64) to state that `Color()` is a dark-pinned convenience retained only for not-yet-mode-resolved call sites, removing "Until light/dark detection lands (1-7) this is always the DARK variant."
3. In `internal/tui/session_item.go`, delete the package-level `cursorStyle` var and its "Retained for the projects page..." comment (lines 15-21). Confirm there are no production references (the only live `cursorStyle` is the shadowing local in edit_modal.go:170, which is unaffected).
4. Run `go build -o portal .` and `go test ./internal/tui/...`.

**Acceptance Criteria**:
- The theme.go package doc and `Token.Color()` comment no longer claim detection "lands in 1-7" / hard-pins dark, and accurately describe the `canvasMode → ColorFor(mode)` flow.
- The package-level `cursorStyle` var and its comment are removed from session_item.go.
- The edit_modal.go:170 shadowing local `cursorStyle` is untouched and still compiles.
- `go build` succeeds and the `internal/tui` test package passes with no unused-variable or unreferenced-symbol errors.
- No behavioural change to any rendered output in either mode.

**Tests**:
- Compilation/build is the primary guard (removing an unused var and editing comments cannot regress runtime behaviour); confirm `go build ./...` and `go test ./internal/tui/...` pass.
- If a doc-accuracy or dead-code guard test exists in the suite, confirm it still passes; otherwise no new test is required since the change is documentation + dead-code removal with no behavioural surface.
