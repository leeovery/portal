TASK: spectrum-tui-design-4-2 — Inline flash band reskin (warning + success) routed through the single-slot arbiter (§11.2 MV)

ACCEPTANCE CRITERIA:
- Warning flash = accent.orange bar + ⚠ glyph + message on bg.warning tint in text.on-warning; no literal hex (§2.9 tokens).
- Success flash = state.green bar + ✓ glyph + message; warning/success glyph-distinct, not colour-only (§2.2).
- Auto-clears on next actionable keypress AND after the short timeout, generation guard intact — parity with formatSessionGoneFlash/flashGen lifecycle.
- Takes the single notice slot over any persistent band for its duration, then the persistent band returns (via task-4-1 arbiter).
- text.on-warning on bg.warning clears the contrast floor (Phase-1 co-tuned pair).
- List viewport height recomputes on flash appear and clear (F10 via task 4-1); pagination never overflows.
- Under NO_COLOR: keeps bar + position + warning/success glyph + bold/dim, drops tint + bar colour.
- vhs capture produced + compared to "Sessions — inline flash (MV)"; behaviour parity (lifecycle + wording) confirmed.

STATUS: Complete

SPEC CONTEXT:
§11.2 (inline flash chrome band): transient accent.orange ▌ left-bar + ⚠ + message on bg.warning tint with text.on-warning; auto-clears on next keypress or short timeout; success variant uses state.green + ✓ so success is glyph-distinct (§2.2 — state never carried by hue alone); F10 — flash is chrome, list viewport height recomputed on appear/clear. §11 single-slot rule: a transient flash wins over any persistent band while shown, then the persistent band returns. §2.5 NO_COLOR carve-out: bands drop tint + bar colour but keep ▌ bar, position, ⚠/✓ glyph, bold/dim. §2.9: bg.warning (#241B10 dark / #E8D6A8 light) + text.on-warning (#E8C9A0 / #7A4B12) are the co-tuned pair; accent.orange / state.green are the bar tokens.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/sessions_flash.go:31-44 (flashKind enum, flashWarning zero-value default + flashSuccess), :94-104 (formatSessionGoneFlash wording preserved verbatim, literal quotes not %q).
  - internal/tui/model.go:404-413 (flashKind field on Model), :1736-1745 (setFlash → kind resets to flashWarning, resyncs layout), :1747-1758 (setSuccessFlash → flashSuccess, identical lifecycle), :1766-1769 (clearFlash → resync), :2150-2151 (bail calls setFlash + flashTickCmd via tea.Batch), :2182-2194 (flashTickMsg generation guard), :2942-2950 (actionable-key clear), :1342-1348 (sessionBandHeight F10 reserve), :1299-1314 (applySessionListSize reserves band), :4103-4106 (single-slot insertion above section header).
  - internal/tui/notice_band.go:38-41 (⚠/✓ glyph constants), :86-95 (barToken: bandWarning→accent.orange, bandSuccess→state.green), :107-114 (tintToken: both flashes on bg.warning — single co-tuned tint, no invented success tint), :120-129 (statusGlyph), :213-270 (renderNoticeBand: bar+glyph+message, NO_COLOR carve-out, multi-line wrap), :334-365 (activeNoticeBand arbiter + flashBandRole), :372-377 (noticeBandOnBandText → text.on-warning for flashes).
  - internal/tui/theme/theme.go:158/161/173/183 (state.green / accent.orange / bg.warning / text.on-warning tokens — §2.9 values, no literal hex at call sites).
- Notes: No literal hex anywhere in the flash render path — all colour sourced from the §2.9 closed vocabulary via theme tokens. The reskin consumes the task-4-1 primitive (renderNoticeBand / activeNoticeBand / sessionBandHeight) exactly as the task mandates — no re-implemented insertion. flashKind correctly defaults to flashWarning so the unparameterised setFlash (the externally-killed bail) stays a warning band. setSuccessFlash has no production caller yet — it is the success seam the spec anticipates ("the success variant follows the same pattern, not separately mocked"); this is deliberate, not dead code, and is exercised by unit tests.

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/sessions_flash_reskin_test.go — the dedicated 4-2 suite: warning band orange-bar/⚠/text.on-warning/bg.warning (TestWarningFlash_OrangeBarWarningGlyphOnWarningTint), success band green-bar/✓ (TestSuccessFlash_GreenBarSuccessGlyph), glyph-distinctness asserting each band carries ONLY its own glyph (TestFlash_WarningVsSuccessGlyphDistinct), kind default + reset (TestFlashKind_DefaultsToWarning), arbiter role mapping (TestActiveNoticeBand_FlashKindSelectsRole), NO_COLOR keeps bar+glyph and emits zero SGR (TestFlashReskin_NoColor), F10 recompute reserves/releases exactly 2 rows (TestFlashReskin_RecomputesListHeight), lifecycle parity for both kinds incl. generation guard (TestFlashReskin_AutoClearLifecyclePreserved), Build/InitialFlash capture seam (TestBuild_InitialFlash*), full-width single line (TestFlashReskin_BandFullWidthSingleLine).
  - internal/tui/notice_band_test.go:160-237 — slot-over-persistent: transient wins (TestNoticeSlot_SingleBand_TransientFlashWins), persistent returns after clear (TestNoticeSlot_PersistentReturnsAfterFlashClear), never both at once (TestNoticeSlot_NeverBothBandsSimultaneously), placement under separator above section header.
  - internal/tui/theme/contrast_test.go:219-287 — co-tuned pair gate: TestBgWarningPairRule (3 legs incl. text-on-tint floor + accent.orange bar 3:1) AND a §11.2-scoped TestInlineFlashWarningPairClearsFloor asserting text.on-warning on bg.warning clears 4.5:1 in both modes.
  - Wording parity: preview_attach_bail_flash_test.go (formatSessionGoneFlash exact wording across simple/empty/dashes/spaces/unicode), sessions_flash_replacement_test.go (replacement-bail wording across rapid bails), sessions_flash_render/clear/tick_test.go (the pre-reskin lifecycle, untouched).
- Notes: Coverage maps 1:1 to the task's named test list — every listed test exists. Not over-tested: each test targets a distinct invariant (no redundant duplication). Not under-tested: the success variant is fully exercised (glyph, role, height, lifecycle, NO_COLOR) despite having no production caller, and the contrast pair has both a general and a §11.2-scoped assertion so a flash-specific regression fails with a scoped message. The vhs capture (testdata/vhs/sessions-inline-flash.png) matches the reference frame (testdata/vhs/reference/sessions-inline-flash-mv.png) on layout/structure/colour-role: orange ▌ bar + ⚠ + message on dark-amber tint, under the separator, above "Sessions 4", list shifted down.

CODE QUALITY:
- Project conventions: Followed. Token-based colour (no literal hex at call sites), small-interface DI seams, no t.Parallel, component-bound rendering. Idiomatic Go; comments explain the load-bearing "why" (generation guard, single-slot rule, F10 reserve, co-tuned tint) without noise.
- SOLID principles: Good. activeNoticeBand is the single arbiter chokepoint; renderNoticeBand is the single shared band render base (the two info bands cannot drift); flashBandRole / noticeBandOnBandText are focused mappers. flashKind cleanly separates styling variant from lifecycle (the kind never touches the clear path).
- Complexity: Low. The renderNoticeBand wrap loop is the only non-trivial branch and it is well-isolated and commented.
- Modern idioms: Yes — lipgloss v2 / bubbletea v2 KeyPressMsg (Code/Text) handled correctly in isActionableKey.
- Readability: Good. Self-documenting names, intent-revealing comments.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
