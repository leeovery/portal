---
topic: spectrum-tui-design
cycle: 4
total_proposed: 4
---
# Analysis Tasks: Spectrum TUI Design (Cycle 4)

## Task 1: Delete dead modal-box scaffolding and scrub misleading "not-yet-reskinned" comments
status: approved
severity: medium
sources: architecture

**Problem**: The reskin migrated all five modals (kill, rename, delete, edit, help) onto hand-drawn `renderJoinedPanel` content builders, each routed through its own `render*ModalOnClearedCanvas` wrapper from the two page composers (model.go:4064-4082 Sessions, model.go:3872-3890 Projects). As a result `renderModalOnClearedCanvas` (internal/tui/modal.go:67) has no production caller — its only references are its own definition and doc-comment mentions — and `modalBorderStyle` (internal/tui/modal.go:32) is transitively dead, its sole caller being `renderModalOnClearedCanvas`. The `Padding(1,2)` box `modalBorderStyle` builds is never composited into any rendered frame. Worse, five surviving comment blocks (modal.go:94, :105, :118, :130, :143) claim the box is "left intact for the OTHER (not-yet-reskinned) modals" — but no un-reskinned modals remain; every modal hand-draws its own joined panel. A reader adding a new modal would reasonably follow these comments into the dead path.

**Solution**: Delete the dead `renderModalOnClearedCanvas` and `modalBorderStyle` functions and remove the two now-orphaned tests that exercise the dead path. Scrub the "not-yet-reskinned modals" framing from the five wrapper comments so the documentation matches the live single-path-per-modal reality.

**Outcome**: `internal/tui/modal.go` carries no unreachable rendering scaffolding, no comment claims a non-existent set of un-reskinned consumers, and a future contributor adding a modal is pointed only at the live `renderJoinedPanel` content-builder + `render*ModalOnClearedCanvas` wrapper pattern. `go build` and `go test ./...` pass.

**Do**:
1. Confirm `renderModalOnClearedCanvas` (internal/tui/modal.go:67) has no production caller — grep the `internal/tui` package (and the whole repo) for the symbol; its only references should be its own definition/doc-comment.
2. Confirm `modalBorderStyle` (internal/tui/modal.go:32) is referenced only by `renderModalOnClearedCanvas`.
3. Delete both `renderModalOnClearedCanvas` and `modalBorderStyle` and any now-unused imports/helpers they alone pulled in.
4. Locate and delete the two orphaned tests that exercise the dead path in `internal/tui/help_modal_frame_test.go` and `internal/tui/edit_modal_test.go` (the cases that drive `renderModalOnClearedCanvas` / `modalBorderStyle`). Do not delete tests that still exercise the live joined-panel path.
5. Edit the five wrapper comment blocks at modal.go:94, :105, :118, :130, :143 to remove the "left intact for the OTHER (not-yet-reskinned) modals" framing; reword to describe the actual live per-modal hand-drawn-panel pattern.
6. Run `go build -o portal .` and `go test ./internal/tui/...` to confirm no compile or test breakage.

**Acceptance Criteria**:
- `renderModalOnClearedCanvas` and `modalBorderStyle` no longer exist in `internal/tui/modal.go`.
- A repo-wide grep for `renderModalOnClearedCanvas` and `modalBorderStyle` returns no matches.
- The two orphaned tests in help_modal_frame_test.go / edit_modal_test.go that drove the dead path are removed; all remaining `internal/tui` tests still exercise the live joined-panel modal path.
- No comment in modal.go references "not-yet-reskinned" modals or implies un-reskinned consumers exist.
- `go build` and `go test ./...` pass with no unused-symbol or unused-import errors.

**Tests**:
- `go test ./internal/tui/...` passes — confirms the live modal render paths (kill/rename/delete/edit/help) are intact after the dead-code excision.
- `go build -o portal .` succeeds — confirms no dangling references to the deleted symbols and no orphaned imports.

## Task 2: Remove the unconsumed bubbles/list help-styling layer and correct its stale doc comment
status: approved
severity: medium
sources: architecture

**Problem**: The condensed footer is fully descriptor-driven: `renderSessionsFooter`/`renderProjectsFooter` → `renderCondensedFooter` over the `keymapEntry` descriptors (footer.go:64-123), and nothing in the live render path consumes the bubbles/list `help.Model` (the built-in help is disabled via `SetShowHelp(false)` at model.go:1037/1075; a grep for FullHelpView/ShortHelpView/`l.Help` in the render path returns nothing). Yet `brightenHelpStyles` (model.go:883-893) still populates `l.Help.Styles.*` at list construction with DARK-PINNED colours via the dark-only `Token.Color()` convenience, and both `canvasHelpStyles` (model.go:912-918) and `colourlessHelpStyles` (model.go:931-936) re-populate those same `l.Help.Styles.*` fields on every restyle. All these writes target a struct that is never rendered. The doc comment at model.go:1023-1029 compounds the problem by asserting the manual footer is "rendered ... via the list's own help.Model.FullHelpView" and that "brightenHelpStyles still populates l.Help.Styles.* so the manual render keeps the same colour ... palette" — both claims are stale; the live render is `renderCondensedFooter`, which sources its own tokens. The dead layer also masks a latent light-mode contradiction: the dark-pinned `Token.Color()` usage would be a real bug if these styles were consumed.

**Solution**: Drop the `l.Help.Styles.*` writes from `brightenHelpStyles`, `canvasHelpStyles`, and `colourlessHelpStyles`, retaining in the latter two only the still-load-bearing `l.Styles.HelpStyle` background and the delegate/pagination styling they also drive. If `brightenHelpStyles`'s only remaining concern is the now-removed help styling (its pagination-dots concern being handled separately by `canvasPaginationDots`/`colourlessPaginationDots`), remove the function entirely. Correct the model.go:1023-1029 comment to describe the descriptor-driven `renderCondensedFooter` path. With the last dark-pinned `Token.Color()` call sites gone, retire the dark-only `Token.Color()` convenience method (theme.go:60-62) if it has no other callers.

**Outcome**: No code writes to `l.Help.Styles.*` anywhere in `internal/tui`; the only list-help-related styling retained is the load-bearing `l.Styles.HelpStyle` background and pagination/delegate styling. The model.go:1023-1029 comment accurately names `renderCondensedFooter` as the live footer path. The dark-pinned `Token.Color()` call sites are eliminated, and the dark-only convenience method is retired if otherwise unused. `go build` and `go test ./...` pass; footer rendering (Sessions and Projects, canvas and colourless) is unchanged.

**Do**:
1. Inspect `brightenHelpStyles` (model.go:883-893), `canvasHelpStyles` (model.go:912-918), and `colourlessHelpStyles` (model.go:931-936); identify every `l.Help.Styles.*` assignment.
2. Remove all `l.Help.Styles.*` assignments from the three functions. In `canvasHelpStyles`/`colourlessHelpStyles`, retain the `l.Styles.HelpStyle` background write and any delegate/pagination styling they drive.
3. If `brightenHelpStyles` is left with no remaining responsibility (pagination dots are handled by `canvasPaginationDots`/`colourlessPaginationDots`), delete the function and its call site(s); otherwise keep only the still-needed body.
4. Rewrite the stale doc comment at model.go:1023-1029 to state the footer is rendered manually via `renderCondensedFooter` over the `keymapEntry` descriptors, and remove the claim that it renders through `help.Model.FullHelpView` / that `brightenHelpStyles` keeps the palette in sync.
5. Grep the `internal/tui` package for remaining callers of the dark-only `Token.Color()` convenience (theme.go:60-62). If the removed writes were the last callers, delete the `Token.Color()` method; if other callers remain, leave it.
6. Run `go build -o portal .` and `go test ./internal/tui/...`; visually confirm via the existing footer/golden tests that canvas and colourless footer output is byte-identical to before.

**Acceptance Criteria**:
- No `l.Help.Styles.*` assignment exists anywhere in `internal/tui` (repo-wide grep returns no matches in the render/construction path).
- `canvasHelpStyles`/`colourlessHelpStyles` retain only the load-bearing `l.Styles.HelpStyle` background and pagination/delegate styling.
- `brightenHelpStyles` is either removed (if fully emptied) or contains only still-needed work; no dead body remains.
- The model.go:1023-1029 comment names `renderCondensedFooter` and contains no `help.Model.FullHelpView` claim.
- The dark-only `Token.Color()` method (theme.go:60-62) is deleted if it has no remaining callers; otherwise it is retained with documented remaining callers.
- Existing footer/golden tests pass with unchanged canvas and colourless footer output; `go build` and `go test ./...` pass.

**Tests**:
- `go test ./internal/tui/...` passes, including the existing footer-render and golden-output tests — confirms Sessions and Projects condensed footers render identically in canvas and colourless modes after the dead-styling removal.
- `go build -o portal .` succeeds with no unused-symbol/unused-import/unused-method errors — confirms the `Token.Color()` retirement (if applied) and the removed writes leave no dangling references.

## Task 3: Make SessionDelegate.canvasBg / tokenStyle delegate to the canonical header leaf-style helpers
status: approved
severity: low
sources: duplication

**Problem**: Two foundational leaf-style rules exist as canonical helpers in header.go: `headerStyle` (role-token foreground over `Background(canvas)` for the mode, bare under colourless; header.go:87-94) and `headerCanvasBg` (canvas background only, bare under colourless; header.go:99-104). `SessionDelegate.canvasBg` (session_item.go:197-202) is a byte-for-byte re-statement of the `headerCanvasBg` body (the same `if d.Colourless { return NewStyle() } ... Background(theme.MV.Canvas.ColorFor(mode))`), and `SessionDelegate.tokenStyle` (session_item.go:214-221) is the same rule as `headerStyle` plus a caller-supplied base style. The codebase already demonstrates the intended fix elsewhere: `loading_view.go`'s `loadingStyle` (line 251) and `loadingFg` (line 259) deliberately delegate to `headerCanvasBg`/`headerStyle` so the leaf canvas-paint carve-out lives in exactly one place, and `SessionDelegate`'s own `rowBg`/`rowToken` (session_item.go:337,344) delegate to shared `rowBgStyle`/`rowTokenStyle` free functions. `canvasBg`/`tokenStyle` are the one pair that re-derive the rule, duplicating the colourless-fallback and canvas-resolution logic across the delegate and the header layer — a drift risk if the NO_COLOR carve-out or canvas resolution ever changes.

**Solution**: Make `SessionDelegate.canvasBg` delegate to `headerCanvasBg(d.Mode, d.Colourless)` and `SessionDelegate.tokenStyle` delegate to `headerStyle(fg, d.Mode, d.Colourless).Inherit(base)` (or thread the base through), mirroring how `loadingStyle`/`loadingFg` and `rowBg`/`rowToken` already delegate to their shared sources. The bodies are already identical, so this is a no-behaviour-change collapse onto the canonical helpers.

**Outcome**: The canvas-paint colourless carve-out and canvas-resolution logic live in exactly one place (the header leaf helpers); `SessionDelegate.canvasBg`/`tokenStyle` are thin delegations matching the established sibling-helper pattern. Rendered session-row output (canvas and colourless modes) is byte-identical to before. `go build` and `go test ./...` pass.

**Do**:
1. Read `headerCanvasBg`/`headerStyle` (header.go:87-104) and `SessionDelegate.canvasBg`/`tokenStyle` (session_item.go:197-221) to confirm the bodies are identical (modulo the caller-supplied base in `tokenStyle`).
2. Rewrite `SessionDelegate.canvasBg` to `return headerCanvasBg(d.Mode, d.Colourless)`.
3. Rewrite `SessionDelegate.tokenStyle` to delegate to `headerStyle(fg, d.Mode, d.Colourless)` and apply the caller-supplied base via `.Inherit(base)` (or by threading the base through), preserving the exact resulting style.
4. Confirm the delegation matches the reference pattern in `loadingStyle`/`loadingFg` (loading_view.go:251,259) and `rowBg`/`rowToken` (session_item.go:337,344).
5. Run `go build -o portal .` and `go test ./internal/tui/...`; confirm session-row golden/render tests are unchanged.

**Acceptance Criteria**:
- `SessionDelegate.canvasBg` is a single delegation to `headerCanvasBg(d.Mode, d.Colourless)` with no re-stated colourless/canvas-resolution branch.
- `SessionDelegate.tokenStyle` delegates to `headerStyle(...)` with the caller-supplied base composited, with no re-stated colourless branch.
- The colourless-fallback and canvas-resolution logic for these two rules exists only in the header leaf helpers.
- Existing session-row render/golden tests pass with byte-identical canvas and colourless output; `go build` and `go test ./...` pass.

**Tests**:
- `go test ./internal/tui/...` passes, including session-delegate render/golden tests — confirms grouped and flat session rows render identically in canvas and colourless modes after the delegation collapse.
- `go build -o portal .` succeeds — confirms the delegations compile against the existing `headerCanvasBg`/`headerStyle` signatures.

## Task 4: Move and rename the shared joined-panel frame primitives off the use-site `help*` prefix
status: approved
severity: low
sources: architecture

**Problem**: `renderJoinedPanel` (panel.go:34-61) is the single shared chrome for the help modal, the kill/rename/delete/edit modals, AND the full-screen preview overlay (panel.go documents this), yet it is built entirely from `help*`-prefixed primitives — `helpFrameStyle`, `helpFrameTop`/`Bottom`/`Divider`/`ContentLine`, `helpInsetRow`, `helpRowInset`, `helpRuleGlyph`, and the `helpFrame*` consts (help_modal.go:56-78, 124-166) — which physically live in help_modal.go. The naming is a use-site label from the surface where these were first written; they are now the universal joined-panel framework. A maintainer reading `helpFrameStyle(theme.MV.AccentCyan, ...)` in the preview, or `helpInsetRow` inside the edit modal, must mentally override the prefix to understand it is the shared frame, not help-specific. This is a naming/locality smell, not a correctness or composition defect — the abstraction is sound and well-shared.

**Solution**: Move the shared frame primitives from help_modal.go into panel.go beside `renderJoinedPanel`, and rename the role-agnostic `help*` prefix to a panel/frame-neutral one (e.g. `panelFrameStyle`, `panelInsetRow`, `panelRowInset`, `panelFrameTop`/`Bottom`/`Divider`/`ContentLine`, `panelRuleGlyph`, `panelRowInset` const, `panelFrame*` consts). Leave genuinely help-specific names (`helpModalHeader`, `helpModalRow`, `helpTitle`) in help_modal.go. This is a mechanical rename + relocation with no behaviour change.

**Outcome**: The shared joined-panel frame primitives live in panel.go beside their sole framework entry point `renderJoinedPanel` and carry frame-neutral names, so a maintainer reading them in the preview or any modal reads the role directly. Help-specific names remain in help_modal.go. Rendered output for every modal and the preview is byte-identical. `go build` and `go test ./...` pass.

**Do**:
1. Enumerate the shared frame primitives in help_modal.go: the consts (`helpRowInset` and the `helpFrame*` consts at help_modal.go:56-78) and the functions (`helpFrameStyle`, `helpFrameTop`, `helpFrameBottom`, `helpFrameDivider`, `helpFrameContentLine`, `helpInsetRow`, `helpRuleGlyph` at help_modal.go:124-166). Confirm each is consumed by `renderJoinedPanel` and/or by multiple surfaces (modals + preview), not solely by help-specific rendering.
2. Distinguish genuinely help-specific symbols (`helpModalHeader`, `helpModalRow`, `helpTitle`) — leave these in help_modal.go with their names unchanged.
3. Move the shared primitives into panel.go beside `renderJoinedPanel`.
4. Rename the `help*` prefix on the moved symbols to a frame-neutral prefix (e.g. `panelFrameStyle`, `panelFrameTop`/`Bottom`/`Divider`/`ContentLine`, `panelInsetRow`, `panelRuleGlyph`, `panelRowInset`, `panelFrame*` consts), updating every call site across the modals, the preview, and any tests in one pass.
5. Run `go build -o portal .` and `go test ./...`; confirm all modal/preview render and golden tests pass with unchanged output.

**Acceptance Criteria**:
- The shared joined-panel frame primitives (the `helpFrame*`/`helpInset*`/`helpRowInset`/`helpRuleGlyph` family backing `renderJoinedPanel`) reside in panel.go beside `renderJoinedPanel` and carry a frame-neutral (`panel*`) prefix.
- No `help*`-prefixed name remains for a primitive that is part of the shared frame; genuinely help-specific symbols (`helpModalHeader`, `helpModalRow`, `helpTitle`) remain in help_modal.go unchanged.
- Every call site (modals, preview, tests) references the renamed symbols; a repo-wide grep for the old shared-primitive names returns no matches.
- Rendered output for all five modals and the preview overlay is byte-identical to before; `go build` and `go test ./...` pass.

**Tests**:
- `go test ./internal/tui/...` passes, including the modal-frame and preview-overlay render/golden tests — confirms the rename/relocation is purely mechanical with no output change across every joined-panel surface.
- `go build -o portal .` succeeds — confirms all call sites were updated and no symbol references the old name.
