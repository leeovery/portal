TASK: spectrum-tui-design-2-5 — Centred pagination dots: built-in bubbles/list paginator restyled (active accent.violet / inactive text.faint), centred above the footer, no full-screen frame, page-count parity.

ACCEPTANCE CRITERIA:
- Active page dot in accent.violet, inactive dots in text.faint, via tokens (no literal hex).
- Dots render centred above the footer; no full-screen frame introduced (§3.6).
- Single-page list suppresses the dots (built-in behaviour preserved).
- Page count / paging behaviour unchanged (parity) — same dot count / page for a fixture as pre-task.
- VISUAL VERIFICATION: multi-page vhs capture matches Sessions — Modern Vivid v2 / (Light) for layout/structure/colour-role.
- Behaviour parity: only dot glyph styling/centring changed; count, per-page, paging keys byte-identical in behaviour.

STATUS: Complete

SPEC CONTEXT:
§3.5 — bubbles/list's built-in height-driven paginator renders as centred dots above the footer: active page dot accent.violet, inactive dots text.faint. §3.6 — no full-screen frame; structure is the two horizontal rules + per-element treatments; the owned canvas is a flat fill, not a frame. §14.1 — bubbles/list pagination is kept as the engine (restyle only). §2.9 — text.faint is decorative-only ("inactive dots" is literally the reserved case); accent.violet carries the active dot. §1/§2.5 — leaf .Background(canvas) on every painted cell; NO_COLOR suppresses the canvas.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/model.go:910-923 `canvasPaginationDots` — re-points Styles.ActivePaginationDot → accent.violet, Styles.InactivePaginationDot → text.faint (both via `theme.MV.*.ColorFor(mode)`), each .Background(canvas) + SetString(paginationDotGlyph); re-feeds the rendered strings into Paginator.ActiveDot/InactiveDot (the v2 equivalent of the v1 hooks the task named), then centres the row.
  - internal/tui/model.go:931-937 `colourlessPaginationDots` — NO_COLOR carve-out: bare dot styles (no hue, no canvas bg), centring preserved.
  - internal/tui/model.go:947-949 `centrePaginationRow` — re-points Styles.PaginationStyle to base.Width(l.Width()).Align(Center), replacing the engine default's PaddingLeft(2); centred across the LIST width (l.Width()), per §3.5.
  - internal/tui/model.go:1278-1285 `applyListSize` — re-runs centrePaginationRow after every SetSize (SetSize resets the wrapper width), so the centring tracks resizes; canvas vs bare base chosen by m.colourless.
  - Wiring: applyCanvasMode (1160 sessions / 1198 projects) installs the dot styles; NO_COLOR branch (1134/1191). paginationDotGlyph = "•" (model.go:43).
- Notes:
  - The page count is NOT touched — only glyph styling + the PaginationStyle wrapper. The engine's TotalPages/per-page/paging keys are untouched (parity holds). Verified by reading: no SetSize height override beyond the existing header/footer/band reserve (applySessionListSize:1299-1313), which is owned by 2-2/2-4, not this task.
  - SetString keeps the engine's bullet glyph; only Foreground/Background change. The dot styles are correctly re-read into Paginator.ActiveDot/InactiveDot (list.New reads them once at construction, so the explicit re-feed is necessary and correct).
  - Centring is correctly applied at two layers — inside canvasPaginationDots/colourlessPaginationDots (construction) AND re-applied after each SetSize in applyListSize (resize). No clobber risk: SetSize resets the wrapper, applyListSize re-centres immediately after.

TESTS:
- Status: Adequate
- Coverage: internal/tui/pagination_dots_test.go covers all five required micro-acceptance tests, each well-targeted:
  - ActiveVioletInactiveFaint (dark+light) — asserts both role FG sequences present in the dot row.
  - ActiveDotIsViolet — pins the SGR run preceding the FIRST (page-0 active) dot to accent.violet, distinguishing it from the following inactive text.faint runs. This is the strong assertion that prevents a swapped/both-same regression.
  - CentredAboveFooter — asserts dotIdx is strictly above the footer lines AND leading pad > 0 AND |leading-trailing| ≤ 1 (centred, not left/right-aligned). Good, non-trivial centring check.
  - SuppressedOnSinglePage — 3-session fixture, asserts TotalPages==1 then no dot row in the composed view.
  - PageCountAndPagingUnchanged — dot count == Paginator.TotalPages AND Ctrl+↓/↑ advance/retreat the page. Direct parity assertion.
  - NoFullScreenFrame — no box-drawing glyphs in the composed view (§3.6).
  - Plus two non-required but spec-justified bonus tests: PaintsCanvasNoEdgeBleed (§1 leaf canvas) and ColourlessDropsHueAndCanvas (§2.5 NO_COLOR). These are warranted (real spec requirements), not over-testing.
- Notes:
  - The multi-page setup builds 60 sessions through the production applySessions path and asserts TotalPages>=2 in setup — robust against silent single-page degeneration.
  - Tests would genuinely fail if the feature broke: token swap, missing centring, count drift, or a frame would each trip a specific assertion. Not under-tested; not bloated.

CODE QUALITY:
- Project conventions: Followed. Tokens via theme.MV.*.ColorFor(mode) — no literal hex (enforced by the colour_literal_guard glob test which now covers model.go). No t.Parallel() in tests (cmd-package mutable-state rule; this is internal/tui but the repo-wide convention holds and is respected). dark-default-before-options construction pattern matches the rest of the file.
- SOLID principles: Good. canvasPaginationDots / colourlessPaginationDots / centrePaginationRow are small single-responsibility helpers; the canvas-vs-colourless split mirrors the established help-styles pair (canvasHelpStyles/colourlessHelpStyles), so the pattern is consistent across the file.
- Complexity: Low. Linear style assignment; no branching beyond the colourless carve-out.
- Modern idioms: Yes — lipgloss v2 builder chaining; the v1→v2 hook confirmation the task asked for is done (Styles.ActivePaginationDot still exists in v2 and is re-fed into Paginator.ActiveDot).
- Readability: Good. Comments are thorough and explain the load-bearing details (the SetString-once-at-construction re-feed, the SetSize re-centre coupling, why centring is list-width not terminal-width).
- Issues: None blocking.

VISUAL VERIFICATION:
- testdata/vhs/sessions-paged.tape drives the deterministic `sessions-paged` fixture (multi-page) through the offline capturetool harness (no tmux, no ~/.config) and screenshots testdata/vhs/sessions-paged.png.
- Compared sessions-paged.png against testdata/vhs/reference/sessions-modern-vivid-v2.png: the centred dot row sits between the list body and the condensed footer in both; the active (first) dot is the brighter violet, the inactive dots are faint grey; no full-screen frame. Layout / structure / colour-role match. PASS (agent-judged, per §15.2).
- Single-page suppression is covered by the unit test (SuppressedOnSinglePage) rather than a second committed PNG; acceptable — the suppression is bubbles/list built-in behaviour and the unit test is the stronger, deterministic check. The task's "also capture a single-page fixture" is a soft do-step, not an acceptance criterion; the suppression behaviour is verified.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] testdata/vhs/ — there is no committed single-page vhs capture to visually confirm dot suppression (the task's "also capture a single-page fixture" do-step). The unit test SuppressedOnSinglePage covers the behaviour deterministically, so this is optional; decide whether a committed single-page PNG adds enough reviewer value to be worth a tape. No behavioural gap.
