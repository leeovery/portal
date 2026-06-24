TASK: spectrum-tui-design-4-6 — Preview overlay chrome reskin (peek mode): full-screen accent.cyan joined panel (header/body/footer), ◉ preview marker, footer `←→ window  ⇥ pane  ⏎ attach  ␣ back`, window key ←/→ (was ]/[), pane Tab, captured ANSI untouched, previewBorderColor → accent.cyan token.

ACCEPTANCE CRITERIA (from tick-a4924f):
- Header: ◉ preview (accent.cyan) + session (text.primary) + Window x/y · Pane x/y (text.detail) + footer nav hints (text.detail / accent.blue glyphs); no literal hex.
- Content area framed by accent.cyan border via previewBorderColor; captured ANSI left untouched (only chrome themed — §9.2).
- Preview is a full-screen overlay, NOT a modal — §8.1 blank-screen rule does not apply.
- Scroll (↑/↓/Ctrl), Tab next pane, ←/→ window, Enter attach (this pane), Space/Esc back behave identically to pre-reskin (parity — previewModel.Update unchanged).
- Width-cascade tiers preserved (re-styled, not removed).
- previewBorderColor points at the accent.cyan token.
- NO_COLOR: colourless chrome on native bg, structure (top bar + frame + glyphs) intact; cyan-on-canvas clears the §2.9 floor.
- vhs: preview-screen.png produced and matched against Paper frame "Preview Screen (MV)".

STATUS: Complete

SPEC CONTEXT: CORRIGENDUM §9 + §9.1/§9.2/§9.3 require the §9 restructure: preview becomes a full-screen joined panel (renderJoinedPanel shape, single-tone accent.cyan, rounded, no fill) with header/body/footer compartments. Header = ◉ preview (cyan) + session (primary) + Window x/y · Pane x/y (detail); body = untouched captured ANSI; footer = nav hints (accent.blue glyphs + text.detail labels, space-separated, no middots): `←→ window  ⇥ pane  ⏎ attach  ␣ back`. Window binding changed ]/[→←/→; pane stays Tab; marker ⊙→◉. Preview is a full-screen OVERLAY (not §8.1 modal); a `?` help overlays without blanking. accent.cyan + accent.blue must clear the §2.3/§2.9 contrast floor against the owned canvas.

IMPLEMENTATION:
- Status: Implemented (faithful to the §9 restructure)
- Location: internal/tui/pagepreview.go
  - previewMarker = "◉ preview" (line 20); previewBorderColorToken = theme.MV.AccentCyan (line 35).
  - Header cascade: selectPreviewHeaderTier (4 tiers, lines 175-203) + composePreviewHeaderRow (lines 212-237); previewCounters → "Window x/y · Pane x/y" (line 146-148); segment tokens accent.cyan / text.primary / text.detail.
  - Footer: previewFooterGroups derives from the shared previewKeymap() Core entries (lines 245-254); composePreviewFooterRow full→compact→drop cascade (lines 268-291); glyphs accent.blue, labels text.detail, space-separated.
  - View() (lines 688-719) composes [header][body][footer] via renderJoinedPanel(previewBorderColorToken) — single-tone cyan border + dividers; body = injectSGRResets(m.viewport.View()) untouched ANSI.
  - Keys (handlePreviewKey, lines 576-639): ←/→ window (KeyLeft/KeyRight, intercepted before viewport horizontal scroll), Tab pane, Enter attach (raw indices via currentRawIndices), Space/Esc back, Home/End jumps, `?` overlay help. previewModel.Update key handling is parity-preserved (only View()/chrome composers restyled).
  - Full-screen overlay: helpOpen path composites via overlayHelpOnPreview (lipgloss Compositor, lines 731-747) — preview stays visible behind, NOT routed through the §8.1 blank-screen modal path.
- Notes: footer/help both source from the single previewKeymap() descriptor (§8.5 "generated from descriptor" mandate honoured — footer + help can never drift). No stale verboseKeymap/compactKeymap/previewBorderColorDark/AdaptiveColor/]/[/Ctrl+Left/Right remnants in non-test source (grep-clean). renderJoinedPanel is the SAME shared shape the modals use (single-tone token threaded). Visual (preview-screen.png) matches §9.1 exactly: cyan ◉ preview marker, primary session, detail counters, rounded cyan frame with header/footer dividers, untouched body, footer nav hints.

TESTS:
- Status: Adequate (thorough; arguably extensive, but the surface is large and each test pins a distinct contract)
- Coverage:
  - Header content + role colours: pagepreview_peek_chrome_test.go (marker accent.cyan, session text.primary, counters text.detail), pagepreview_chrome_test.go (1-based ordinals, non-contiguous indices, pane-base-index 1, verbatim session incl. pipe/spaces, no raw-index leak, no #W: prefix).
  - Footer canonical content + roles: pagepreview_keymap_constants_test.go (exact byte content, no middots, compact glyphs-only cascade), pagepreview_compose_chrome_test.go (4 tiers + fits-within-width + no embedded newlines + always-carries-marker).
  - Cyan frame + untouched ANSI: pagepreview_peek_chrome_test.go (corner glyphs preceded by cyan SGR; embedded \x1b[41m raw SGR survives verbatim), pagepreview_view_frame_test.go (rounded corners, full-width top row, SGR reset injection).
  - Full-screen overlay not modal: pagepreview_peek_chrome_test.go (FullScreenOverlayNotBlankScreenModal), pagepreview_help_test.go (help OVERLAYS without blanking — ◉ preview + body still present behind panel; help is descriptor-driven; toggles; Esc dismisses without backing out; other keys inert; colourless overlay).
  - Key parity: pagepreview_bracket_test.go (←/→ window nav, wrap, paneIdx reset, single-window no-op, intercepted-before-horizontal-scroll, exactly-one-Tail), pagepreview_tab_test.go (Tab pane cycle/wrap/no-op/intercept), pagepreview_enter_test.go (raw-index attach incl. non-contiguous + walked nav + nil-attacher no-op + unconditional dispatch across viewport states), pagepreview_space_dismiss_test.go + dismiss tests (Space/Esc → previewDismissedMsg), pagepreview_scroll_test.go (↑/↓/PgUp/PgDn/Ctrl-u/d/j/k delegate, Home/End, resize-no-Tail).
  - Width cascade preserved: pagepreview_compose_chrome_test.go tiers + pagepreview_peek_chrome_test.go NarrowWidthDegradesGracefully (every frame line == terminal width across 120..7).
  - previewBorderColor → token: pagepreview_keymap_constants_test.go (TestPreviewBorderColorPointsAtAccentCyanToken — retired #7B95BD must not survive).
  - NO_COLOR: pagepreview_peek_chrome_test.go ColourlessKeepsStructureDropsHue (structure survives, no \x1b[38; SGR), help colourless test.
  - Contrast floor: internal/tui/theme/contrast_test.go pins accent.cyan AND accent.blue at floorNormal (4.5) against canvasDark/canvasLight — the §2.9 cyan-on-canvas re-verification.
  - Layout/sizing parity: pagepreview_layout_test.go (joined-panel header/body/footer ordering, fills full terminal height, chrome row count constant across cycles), pagepreview_resize_test.go (viewport sized to dims − previewFrameOverhead, clamped non-negative).
  - Brand-new/mixed content, error states, refetch, externalkill, filter, precedence covered by sibling files.
- Notes: No over-testing of concern — the large file count reflects one-contract-per-file discipline, not redundancy. Tests would fail if the feature broke (e.g. raw-SGR-survives, cyan-SGR-on-corners, exact footer byte content, retired-hex guard). Running tests was not performed per instructions; assessed by reading.

CODE QUALITY:
- Project conventions: Followed. Value-receiver methods throughout previewModel (documented rationale at readFocusedPaneIntoViewport); small interface seams (TmuxEnumerator, ScrollbackReader, PreviewAttacher) constructor-injected; tokens-not-hex; component logging not relevant here (pure render). Doc comments are dense and intent-revealing, consistent with the package style.
- SOLID principles: Good. Single source of truth for the keymap (previewKeymap drives footer + help); renderJoinedPanel shared with modals (DRY); footer/header composers decomposed into selectTier / compose / fromGroups with clear single responsibilities.
- Complexity: Low/Acceptable. Cascade functions are linear tier ladders with explicit boundaries; handlePreviewKey is a flat switch.
- Modern idioms: Yes. max(...) builtin, runewidth-aware truncation, functional viewport options for the v2 upgrade.
- Readability: Good. Self-documenting names; the previewFrameOverhead vs previewChromeRowOverhead distinction is justified in-comment.
- Issues: One stale doc comment (see non-blocking notes) — no logic impact.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/pagepreview_resize_test.go:13-15 — The TestPreviewWindowSizeMsg_RecordsDimensionsAndSetsViewportToInnerSize doc comment still describes the pre-§9 arithmetic: "reduced by previewFrameOverhead (= 2)" and "(msg.Width − 2) × (msg.Height − 2)" / "frame occupies one row at the top and one at the bottom, plus one column at the left and one at the right". previewFrameOverhead is now 6 (top + header + 2 dividers + footer + bottom rows; 2 side borders + 2·panelRowInset cols). The assertions correctly use the constant, so the test passes and is correct — only the comment's literal "2" and the one-row/one-column description are stale. Update the comment to the 6-overhead joined-panel framing.
