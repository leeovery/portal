# Plan: Preview Visual Distinction

## Phases

### Phase 1: Painted preview frame with chrome cascade
status: draft

**Goal**: `pagePreview`'s `View()` renders a Portal-painted rounded blue frame around the viewport whose top edge carries a width-cascading chrome line (window/pane indicators, optional window name, glyph keymap), with SGR-reset injection protecting the right border from unterminated scrollback SGR sequences, repainting every tick on resize and navigation.

**Why this order**: Sole phase. The specification is a single cohesive change-set confined to `internal/tui/pagepreview.go` (production) plus four sibling test files. Every subsystem — the rounded frame, the manually-composed top edge, the four-tier width cascade, the display-cell-aware truncation primitive, the SGR-reset injection, the resize repaint, the keymap glyph constants, and the `previewChromeHeight` → `previewFrameOverhead` rename — is mutually dependent and has no independent user value if shipped alone. Splitting would produce horizontal primitives-then-integration phases that defer the user-visible "this is preview" signal to the final phase, contradicting the vertical-slice rule. The `model.go:1421` call site is already plumbed, so no cross-file integration phase is required.

**Acceptance**:
- [ ] `pagePreview.View()` output contains rounded corners (`╭ ╮ ╰ ╯`) and all four edges are coloured via `previewBorderColor`.
- [ ] `composeChromeLine` exists as a pure function in `internal/tui/pagepreview.go`, returns a single-row top-edge string (no embedded newlines) of display-cell width `width + 2` for every `width >= 2`, and returns the empty string for `width < 0`.
- [ ] `composeChromeLine` selects the correct cascade tier — tier 1 (truncate with `…`), tier 2 (drop `· win: {name}` at the 8-cell minimum boundary), tier 3 (swap to `compactKeymap`), tier 4 (corners + `─` filler only) — verified by table-driven tests at threshold widths.
- [ ] Window-name truncation operates in display cells (not bytes or runes), appends `…` only when truncation actually occurred, never produces mid-rune cuts, and is verified across ASCII, CJK, emoji (including ZWJ sequences), and combining-mark glyph classes.
- [ ] Package-level constants `verboseKeymap = "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"` and `compactKeymap = "] [ ⇥ ⏎ ⎋"` exist with the exact spec-defined byte content; tests assert against these constants by literal bytes.
- [ ] `injectSGRResets` helper appends `\x1b[0m` to every non-empty viewport row, ignores empty lines and trailing-newline empty elements, and is applied to `viewport.View()` output before frame composition on every render.
- [ ] The `tea.WindowSizeMsg` case in `pagepreview.go`'s `Update` records `m.width` / `m.height` and calls `viewport.SetSize(max(0, msg.Width − 2), max(0, msg.Height − 2))`; `View()` recomputes the chrome line every tick with no cached chrome field.
- [ ] The constant rename `previewChromeHeight` → `previewFrameOverhead = 2` lands with its doc comment, and all references in `pagepreview_layout_test.go`, `pagepreview_precedence_test.go`, and `pagepreview_scroll_test.go` are updated (arithmetic adjusted because the value changes from 1 to 2).
- [ ] The previous `chromeLine()` method on `previewModel` is deleted; callers in `View()` invoke `composeChromeLine` directly.
- [ ] The manually-composed top-edge corner and edge glyphs are sourced from `lipgloss.RoundedBorder()` (not hardcoded); border parts are wrapped in `lipgloss.NewStyle().Foreground(previewBorderColor).Render(…)`; chrome content renders with no explicit foreground.
- [ ] `previewBorderColor` is declared as a package-level `lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}` and is consumed by both the `lipgloss` `BorderForeground` call (three rendered edges) and the hand-composed top edge's border-part styling.
- [ ] An end-to-end `Update + View` cascade-tier test with fixture window name `"nvim-editor"` asserts the expected tier signature at widths 200, 60, 40, 25, and 15, and asserts SGR-reset bytes are present on each viewport content row in every case.
- [ ] A chrome-row invariant test asserts `strings.Count(composeChromeLine(w, …), "\n") == 0` across the cascade-tier width thresholds.
- [ ] No production code outside `internal/tui/pagepreview.go` is modified; `internal/tui/model.go:1421` remains unchanged (already passes `m.termWidth, m.termHeight` to `NewPreviewModel`).
- [ ] No tests use `t.Parallel()`; no test imports the `tmuxtest` package; all tests use the existing constructor-injected `TmuxEnumerator` and `ScrollbackReader` mock seams.
- [ ] `go build ./...` and `go test ./...` pass; `pageSessions`'s `View()` is unchanged and renders no frame.
