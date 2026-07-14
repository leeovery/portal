TASK: restore-host-terminal-windows-9-1 (tick-44fa9c) — Extract the section-header line-0 splice into one shared helper

ACCEPTANCE CRITERIA:
- The `idx := strings.IndexByte(listView, '\n')` + degenerate-guard + splice appears exactly once (inside `replaceHeaderLine`); no `applySectionHeader` / `applyProjectsSectionHeader` branch hand-rolls it.
- All eight branches preserve their existing claimant conditions, precedence order, and render arguments — output byte-identical.
- `replaceListBodyWithNoMatches` is unchanged.
- The one-row-per-delegate pagination invariant holds (grouped/filter/banner render tests remain green).

STATUS: Complete

SPEC CONTEXT: A duplication/architecture analysis-cycle (Phase 9) finding. This work unit added four new section-header claimants (Opening band 6-5, pre-flight abort 6-7, multi-select banner 5-3, proactive unsupported banner 6-2), each re-copying the four-line "swap line 0, keep tail from first newline" idiom rather than reusing the two pre-existing copies — crossing Rule-of-Three three times over. The fix single-sources the swap-don't-insert contract (the one-row-per-delegate pagination invariant §3.5) so a future tweak is a single edit. Explicitly out of scope: `replaceListBodyWithNoMatches` (a different keep-first-line/replace-body op).

IMPLEMENTATION:
- Status: Implemented
- Location: helper at internal/tui/model.go:4678-4684; eight call sites at model.go:4441, 4448 (applyProjectsSectionHeader), 4717, 4738, 4751, 4766, 4775, 4782 (applySectionHeader).
- Notes:
  - Helper reproduces the idiom EXACTLY per the Do instruction: `idx := strings.IndexByte(listView, '\n'); if idx < 0 { return header }; return header + listView[idx:]`. No behavioural change, no premature `\r\n` handling (correctly deferred — the doc comment notes "Any future tweak to the contract lives here, once").
  - Splice `header + listView[idx:]` now appears exactly once in the whole tui package (grep `listView[idx` → single hit at model.go:4683). Verified no stray copies remain.
  - All eight branches collapse to `return replaceHeaderLine(listView, render…(…))`. Precedence preserved in applySectionHeader: Filtering (returns listView untouched) → burstPending (Opening) → abortBannerText (pre-flight abort) → multiSelectMode → unsupportedBannerActive → FilterApplied → standard. In applyProjectsSectionHeader: Filtering (untouched) → FilterApplied → standard.
  - Render arguments cross-checked against each render function's signature in section_header.go — every arg list matches (renderOpeningBand/renderPreflightAbortHeader/renderMultiSelectHeader/renderUnsupportedHeader/renderFilterQueryHeader/renderSectionHeader/renderProjectsSectionHeader). No arg drift.
  - `replaceListBodyWithNoMatches` (model.go:4827-4838) left untouched — it hand-rolls its own `strings.IndexByte` at 4832 for the distinct keep-first-line/replace-body operation, correctly excluded from this cluster.
  - Helper placed alongside applySectionHeader in model.go (one of the two Do-permitted homes).

TESTS:
- Status: Adequate
- Coverage: internal/tui/replace_header_line_test.go — table-driven TestReplaceHeaderLine covers all three spec-required cases: multi-line (splices header + tail from first newline), single-line no-`\n` (returns header bare), empty listView (returns header bare), plus a well-chosen bonus "trailing newline only" case pinning that the empty tail row is kept (`"old title\n"` → `"NEW HEADER\n"`). Regression render assertions for the eight branches live in burst_preflight_abort_test.go, burst_input_lock_test.go, unsupported_banner_test.go, multi_select_banner_test.go, sessions_flash_render_test.go — these guard the byte-identical output requirement.
- Notes: Well-scoped, not over-tested — one assertion per distinct edge, no redundant variations, no unnecessary mocking. Testing a package-private helper directly is borderline implementation-detail, but it is exactly what the task Tests section requested and the contract it pins (the pagination invariant) is genuine behaviour every branch depends on. Byte-identical output is structurally guaranteed since the helper reproduces the idiom char-for-char.

CODE QUALITY:
- Project conventions: Followed. Internal test package, table-driven, no t.Parallel() (per CLAUDE.md's cmd/tui package-level mutable-state rule). Small package-private helper with intent-revealing doc comment.
- SOLID principles: Good. Single responsibility — the helper owns exactly the line-0 splice contract; callers own their claimant guards.
- Complexity: Low. Helper is three lines of logic; each branch is now render call + one shared splice.
- Modern idioms: Yes. Idiomatic `strings.IndexByte` guard.
- Readability: Good. Doc comment states the invariant (§3.5) and the single-home rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
