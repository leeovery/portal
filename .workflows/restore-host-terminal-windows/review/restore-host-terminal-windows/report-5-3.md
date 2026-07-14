TASK: restore-host-terminal-windows-5-3 — Multi-select banner + notice-band single-slot precedence

ACCEPTANCE CRITERIA:
1. In multi-select mode with N distinct marked sessions the section-header row reads `N selected` (violet) with a right-aligned `esc cancel` (dim); the `Sessions ··· N` header is not shown.
2. A multi-tag session marked via several rows contributes exactly 1 (len(m.selectedSessions)).
3. N=0 in mode renders `0 selected`.
4. Count updates live: each `m` toggle changes the number by exactly 1.
5. Filter input focused (Filtering) → banner steps aside, orange filter line owns the row.
6. In multi-select mode the By-Tag "No tags yet" signpost is suppressed.
7. A transient flash still renders in the notice band while in mode (flash outranks the banner).
8. Banner is exactly one row, does not perturb the list height budget (replaces the header row).

STATUS: Complete

SPEC CONTEXT: Spec "Multi-Select Mode → Mode affordance (visual)" pins the mode as a distinct mode modelled on filter mode: a violet `N selected` banner (filter-line analogue) at the section-header row, counting distinct sessions, plus the notice-band precedence ladder (filter line > flash > multi-select banner > no-tags signpost for the Phase-5 subset). The delivered frame (design/sessions-multi-select-active.png) governs placement: a bare `N selected` / `esc cancel` section-header analogue with NO `▌` left bar, replacing `Sessions ··· N`.

IMPLEMENTATION:
- Status: Implemented (matches acceptance + spec; no drift)
- Location:
  - internal/tui/section_header.go:127-131 — renderMultiSelectHeader: `N selected` in theme.MV.AccentViolet via headerStyle, `esc cancel` (const multiSelectCancelHint, section_header.go:54) in theme.MV.TextDetail, composed through the shared renderRightAnchoredSectionRow right-anchor core (section_header.go:257) — same geometry/flex-spacer/§2.7 narrow-degrade as the standard section headers.
  - internal/tui/model.go:4750-4757 — applySectionHeader multi-select branch: swaps line 0 via replaceHeaderLine (model.go:4678, first-newline split — the same mechanism the FilterApplied branch uses) with renderMultiSelectHeader(len(m.selectedSessions), ...). Precedence: Filtering (4706) returns listView untouched; multi-select (4750) precedes both the unsupported banner (4765) and FilterApplied (4774), so a committed filter in-mode shows the banner, not the query header — exactly the planned order.
  - internal/tui/notice_band.go:371 — activeNoticeBand byTagSignpost arm gated `&& !m.multiSelectMode` (signpost suppressed in mode); flash arm (361) stays first so a flash still wins the slot. Banner is NOT routed through the `▌` notice band (renderMultiSelectHeader is a section-header variant), per the frame.
- Notes:
  - Two deviations from the literal "Do" text, both benign and intentional given later phases:
    (a) count formatted via `strconv.Itoa(count) + " selected"` rather than `fmt.Sprintf("%d selected", count)` — equivalent, and the modernize-linter-preferred form; strconv already imported.
    (b) applySectionHeader interleaves Phase-6 branches (Opening band 4716, pre-flight abort 4737, unsupported 4765) and the signpost arm carries an extra `&& !m.unsupportedBannerActive()` term — these belong to sibling Phase-6 tasks (this review is at cycle-5-complete). They do not alter the 5-3 contract: multi-select still outranks FilterApplied and the standard header, and still steps aside for Filtering. The ordering is documented in-source.

TESTS:
- Status: Adequate
- Coverage (internal/tui/multi_select_banner_test.go):
  - AC1 → TestApplySectionHeader_MultiSelectShowsBanner (asserts `3 selected`, absence of `Sessions`, violet fg seq) + TestMultiSelectHeader_CountVioletCancelDetail (dark+light, exact styled runs) + TestMultiSelectHeader_RightAlignedCancelHint (hint after count, width == content width).
  - AC2 → TestApplySectionHeader_ByTagMultiMembershipCountsOnce (real By-Tag 2-row session, marked once → `1 selected`).
  - AC3 → TestMultiSelectHeader_ZeroSelected + TestApplySectionHeader_MultiSelectZero.
  - AC4 → TestApplySectionHeader_CountUpdatesLive (drives `pressM` enter/toggle/toggle, asserts 0→1→2).
  - AC5 → TestApplySectionHeader_FilteringOwnsRowInMultiSelect (applySectionHeader returns listView unchanged; banner absent).
  - AC6 → TestActiveNoticeBand_SuppressesSignpostInMultiSelect (ok flips false when multiSelectMode set).
  - AC7 → TestActiveNoticeBand_FlashOutranksBannerInMultiSelect (flash owns slot in mode; role/message correct).
  - AC8 → TestMultiSelectHeader_ExactlyOneRow (counts 0/1/42 → height 1).
  - Extra edge cases (spec, not over-testing): TestApplySectionHeader_FilterAppliedInMultiSelectShowsBanner (branch order — banner beats locked query), TestMultiSelectHeader_NarrowDegradeDropsHint (§2.7), TestMultiSelectHeader_ColourlessDropsHueAndCanvas (NO_COLOR carve-out), TestMultiSelectHeader_PaintsCanvasNoEdgeBleed (canvas paint).
- Notes:
  - Not over-tested: the render-level tests (renderMultiSelectHeader in isolation) and the integration-level tests (through applySectionHeader) verify distinct layers (unit render vs. model wiring), not the same assertion twice.
  - Tests would fail if the feature broke (colour role, width, precedence, suppression are all pinned directly). Would fail if someone reverted the byTagSignpost gate (AC6) or reordered the multi-select branch below FilterApplied (FilterAppliedInMultiSelectShowsBanner).
  - Minor completeness gap: AC7's underlying design is a two-row CO-render (flash in the `▌` band + banner still at the section-header row); no single test pins that both render simultaneously. It is covered by composition — applySectionHeader has no flash gate, so the banner shows whenever multiSelectMode is true regardless of flash — but the emergent co-render is not asserted in one place. Non-blocking (the co-render seam is heavily commented in notice_band.go:348-360 and the strict single-row collapse is explicitly a Phase-6 concern).

CODE QUALITY:
- Project conventions: Followed. No raw hex (tokens only — AccentViolet/TextDetail/Canvas via headerStyle); NO_COLOR carve-out honoured; single-source wording const (multiSelectCancelHint); shared right-anchor core reused rather than duplicated; no t.Parallel(); tests assert rendered behaviour, not internals.
- SOLID principles: Good. renderMultiSelectHeader is single-responsibility and delegates geometry to the shared assembler; applySectionHeader remains the single section-header chokepoint.
- Complexity: Low. Small pure render function; a linear, well-commented if-chain for precedence.
- Modern idioms: Yes (strconv.Itoa for the count; max/ansi helpers in the shared core).
- Readability: Good. Extensive, accurate doc comments explain the precedence rationale and the flash co-render seam.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/multi_select_banner_test.go:342 — In TestActiveNoticeBand_FlashOutranksBannerInMultiSelect, add an assertion that the `N selected` banner STILL renders at the section-header row (e.g. bannerFirstLine(m) contains `<N> selected`) while the flash owns the notice band, so the intended two-row co-render (documented at notice_band.go:348-360) is pinned in one place rather than only inferred from two independent tests.
