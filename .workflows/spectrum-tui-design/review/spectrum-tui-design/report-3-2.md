TASK: spectrum-tui-design-3-2 — Projects page reskin (MV): two-line rows + full-height violet left-bar + green header + condensed footer

ACCEPTANCE CRITERIA:
- Two-line row: name text.primary (heavy) line 1, path text.detail (dim) line 2; uniform two-line height keeps pagination exact.
- Selected row: full-height accent.violet ▌ left bar spanning BOTH lines + bg.selection tint; name text.on-selection, path text.muted-bright; unselected rows have neither bar nor tint.
- Section header: `Projects` in state.green + text.detail count at the SAME cap-height as the label (dim, not smaller), right-aligned `/ to filter` hint.
- Footer: condensed Projects keymap `⏎ new session · x sessions · e edit · / filter · ? help`.
- No literal hex at restyled call sites — every colour a §2.9 token.
- Project CRUD identical (enter/e/d/n/x + navigation).
- Empty-projects state left intact (Phase 4).
- Selected name/path clear the fg-on-tint contrast floor against bg.selection.
- Visual: vhs PNG matches Projects (MV) frame.

STATUS: Complete

SPEC CONTEXT:
§6 retargets the existing Projects page (data + keymap + CRUD already exist) to Modern Vivid — a pure reskin, behaviour preserved. §6.1 header (state.green label + text.detail count + `/ to filter`), §6.2 two-line rows with full-height violet bar over bg.selection on selection (name text.on-selection, path text.muted-bright), §6.3 condensed footer. §3.3 establishes the thick `▌` violet left-bar as the single selection signal across all pages, with Projects using a full-height bar spanning its two-line row. §13.6 fixes counts beside labels at the same font size, distinguished by dim colour only (not smaller/superscript). §15 makes Paper authoritative for layout/structure/colour-role, not pixel spacing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/project_item.go:98-167 — ProjectDelegate.Render + renderRowLine (two-line rows, full-height bar, tint, truncation, width clamp, NO_COLOR carve-out, zero-width fallback). Height()==2 / Spacing()==0 preserved (:64,:67).
  - internal/tui/section_header.go:74-116 — renderProjectsSectionHeader + projectsLeftCluster (state.green label, text.detail count at same cap-height via a plain run, shared right-aligned hint via renderSectionHeaderRow).
  - internal/tui/footer.go:75-77 — renderProjectsFooter routes through the shared renderCondensedFooter driven by projectsKeymap (legacy three-column footer retired).
  - internal/tui/keymap.go:126-140 — projectsKeymap descriptor; Core order ⏎·x·e·/·? matches §6.3.
  - internal/tui/model.go:3819-3888 — viewProjectList composes header + (band) + listView (section-header-swapped) + condensed footer; modal cleared-canvas returns preserved.
- Notes:
  - Selection suppression while filtering (project_item.go:108-110) mirrors SessionDelegate §7.1 — bar/tint dropped when the filter input is focused. Correct, beyond the literal AC but spec-consistent.
  - CRUD parity verified: model.go:2290-2303 dispatch (n→handleNewInCWD, d→handleDeleteProjectKey, e→handleEditProjectKey, Enter→handleProjectEnter), x→Sessions transition (model.go:2475+ / :2282) all untouched by the reskin commit (65bbc893 changed rendering paths; handler bodies are behavioural and unchanged). Reskin changed only rendering.
  - Shared row-style helpers (rowBgStyle / rowTokenStyle / renderLeftBarColumn in session_item.go:292-336) are the CURRENT post-6-3 DRY form; project_item.go delegates to them via thin rowBg/rowToken wrappers. Verified against current code as instructed.

TESTS:
- Status: Adequate
- Coverage:
  - project_row_anatomy_test.go — two-line name.primary/path.detail (exact mode-resolved SGR, both Dark+Light, bold-run check), full-height bar + bg.selection tint on both lines, selected name text.on-selection / path text.muted-bright, unselected no-bar/no-tint + canvas paint, uniform two-line height (Height/Spacing + newline count parity across sel/idx), over-long truncation with ellipsis, no-overflow at pathological widths (1..80), no-legacy-cursor/colour-literal render cross-check, NO_COLOR carve-out.
  - projects_header_test.go — label green + count text.detail (exact count run, same cap-height), right-aligned hint + exact width, wordmark alignment, §2.7 narrow degrade drops hint, NO_COLOR.
  - projects_footer_test.go — exact §6.3 copy as left cluster + right-aligned `? help`, no Sessions-copy leak, per-glyph token colours (accent.blue keys / text.detail labels / accent.violet `?` / border.footer rule), NO_COLOR, list-active footer says `new session` not `attach`, narrow degrade.
  - projects_view_reskin_test.go — composed page (wordmark + green section header + count + hint + condensed footer, no legacy copy), three-way left-edge alignment (wordmark/Projects/bar), modal-clears-to-canvas.
  - projects_keymap_test.go — descriptor enumeration, Core classification, single right-aligned `?`, complete help set, footer order parity, no uppercase/vim/page-jump keys.
  - project_item_test.go — interface, Height/Spacing/Update, name+path render, selected full-height bar (not cursor), truncation, identical-name distinct paths.
  - theme/contrast_test.go:452,456 — both selected-row fg-on-tint pairings (text.on-selection + text.muted-bright on bg.selection) gated at the normal floor, Dark+Light.
  - colour_literal_guard_test.go — AST source guard: no raw lipgloss.Color literal at any internal/tui render site (covers project_item.go), complementing the render-level legacy-literal cross-check.
  - Visual: testdata/vhs/projects.{tape,png} present; reference/projects-mv.png present.
- Notes:
  - Coverage is focused, not over-tested. The exact-SGR assertions catch token swaps (not mere glyph presence), matching the rigour of the Sessions-row tests. The two test files (project_item_test.go behavioural + project_row_anatomy_test.go SGR-exact) divide cleanly with no redundant duplication.
  - Would fail if the feature broke: a token swap, a dropped bar, a non-uniform row height, a hex literal, or a CRUD footer leak are all individually pinned.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel() (documented per the shared-canvas-helper constraint). Tokens-only at call sites. Delegates to shared free functions (session_item.go) so the selection/canvas colour role lives in one place.
- SOLID principles: Good. ProjectDelegate is a thin renderer; colour-role composition (rowTokenStyle), background role (rowBgStyle), and the left-bar column (renderLeftBarColumn) are single-responsibility shared helpers. The keymap descriptor is the single source for footer + help.
- Complexity: Low. Render/renderRowLine are short and linear; the zero-width fallback + width clamp are clearly commented guards.
- Modern idioms: Yes. ansi.Truncate for width-safe ellipsis, lipgloss.Width for cell counting, JoinHorizontal/JoinVertical composition.
- Readability: Good. Section-referenced doc comments throughout; intent (why the bar spans both lines, why the title row is swapped not inserted) is explicit.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
