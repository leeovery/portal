TASK: spectrum-tui-design-4-1 — Notice-band primitive (`▌` left-bar, orange/green/violet role variants, under title separator above section header) + single-slot arbiter (persistent owns slot, transient flash temporarily wins then persistent returns) + band appears/clears viewport-height recompute (F10)

ACCEPTANCE CRITERIA (from tick-fa0f20):
- A single shared left-bar-band render primitive exists, parameterised by three role variants (orange/warning, green/success, violet/info), full-width single-line band with far-left `▌`; no literal hex — colours from §2.9 tokens.
- The notice slot holds at most one band: with a persistent band condition AND a transient flash active, only the flash renders; when the flash clears, the persistent band returns; the two never render at once.
- The band sits directly under the title separator, above the section header, full-width; section header + list shift down.
- On band appear and clear the list viewport height is recomputed; one-row-per-delegate pagination invariant holds — no overflow, no miscount.
- Flash auto-clear lifecycle preserved: generation guard drops superseded ticks; actionable keypress clears active flash (parity).
- Under NO_COLOR the band drops tint + bar colour but stays present via bar + position + message.
- vhs: no standalone Paper frame — visible effect verified through downstream consumers 4-2/4-3/4-4.

STATUS: Complete

SPEC CONTEXT:
§11 intro pins the shared left-bar notice convention: `▌` accent line directly under the title separator, above the section header, full-width, with section header + list shifting down. Role colours: accent.orange = transient/warning, state.green = transient/success, accent.violet = mode/info. The single-slot rule (§11 intro): the slot holds AT MOST ONE band; persistent mode notices (no-tags signpost §11.3, command-pending §11.4) own the slot while their mode is active; a transient flash (§11.2) takes the slot temporarily, replacing any persistent band, then the persistent band returns. §11.2 F10: the flash band is chrome — on appear/clear the list viewport height is recomputed so the list never overflows/miscounts. §2.5: NO_COLOR drops tint + bar colour, band stays present via `▌` + position + message (+ glyph/bold-dim on flashes). §1 outer-fill: the band drives the list height recompute UNDERNEATH the outer canvas fill, which re-pads to termH.

IMPLEMENTATION:
- Status: Implemented (evolved beyond the literal task wording in two deliberate, spec-consistent ways — see Notes)
- Location:
  - internal/tui/notice_band.go — the whole primitive: role enum (bandWarning/bandSuccess/bandInfo/bandCommand, :56-71), token mappings (barToken :86, tintToken :107, statusGlyph :120 — all §2.9 tokens, zero literal hex), shared bandBase (:164) + newBandBase (:173) so every band derives bar+tint from ONE place, renderNoticeBand (:213), renderCommandBand (:294, the 4-4 base reuse), the arbiter activeNoticeBand (:347), renderActiveNoticeBand (:385), renderSessionBandSlot (:408).
  - internal/tui/model.go — viewSessionList composes header → slot → listView → footer (:4103-4106), placing the band ABOVE the section header per §11; sessionBandHeight (:1342) measured off the SAME renderSessionBandSlot block; applySessionListSize reserves it (:1312); resyncSessionLayout (:1776) re-applies on every setFlash/clearFlash; the flashTickMsg generation guard (`msg.Gen == m.flashGen`, :2193) and actionable-key clear (:2949) preserved verbatim.
  - internal/tui/header.go — padRightWithStyle (:225) is the shared right-pad geometry extracted by task 10-1; noticeBandPadRight (notice_band.go:330) binds the band tint and delegates to it, headerPadRight binds the canvas. Verified against CURRENT code — no drift.
- Notes:
  - The legacy dual-insert is fully gone: grep finds NO remaining insertRowBelowTitle, byTagSignpostStyle, flashRowStyle, or #888888 literal anywhere in internal/tui. The two independent inserts collapsed to one arbitrated slot exactly as the task required.
  - DELIBERATE EVOLUTION 1 (band + blank breathing row): the slot is band + ONE canvas-painted blank row (renderSessionBandSlot :408-415), so the section header shifts down by TWO rows, not one. Height reserve (sessionBandHeight) is measured off the same slot, so budget and render cannot drift. This is a refinement of the task's "shift down one row" wording, not a regression — it is internally consistent and tested.
  - DELIBERATE EVOLUTION 2 (multi-line wrap): renderNoticeBand wraps the message at narrow widths (:240, ansi.Wrap) with the `▌` bar repeated on every line and continuation indent under line 1's message. The task/§11.2 describe a "single-line band"; the wrap is a narrow-terminal overflow fix that preserves the one-row-per-delegate invariant by measuring the wrapped height off the same slot. Both the band-message render doc (:182-212) and the file-header (:11-26) describe this accurately; consistent and well-tested.

TESTS:
- Status: Adequate (all 10 task-mandated tests present + 6 well-targeted extras for the wrap refinement; no over-testing)
- Coverage (task test list → file):
  - 3 role variants, far-left bar in role colour, message in on-band token → TestRenderNoticeBand_LeftBarInRoleColour (asserts real per-token SGR sequences via tokenFgSeq, so a broken role→token map fails)
  - at-most-one, flash wins → TestNoticeSlot_SingleBand_TransientFlashWins
  - persistent returns after clear → TestNoticeSlot_PersistentReturnsAfterFlashClear
  - never both simultaneously → TestNoticeSlot_NeverBothBandsSimultaneously
  - placed under separator above section header, shift down → TestNoticeBand_PlacedUnderSeparatorAboveSectionHeader (asserts the +2 shift + the blank-row position)
  - viewport-height recompute on appear/clear → TestNoticeBand_RecomputesViewportHeight (asserts baseHeight-2 / restored) + TestNoticeBand_FrameHeightConstant (frame stays termH)
  - generation guard preserved → TestNoticeBand_FlashGenerationGuardPreserved
  - actionable key clears → TestNoticeBand_ActionableKeyClearsFlash
  - short-timeout clears → TestNoticeBand_TimeoutClearsFlash
  - NO_COLOR keeps bar+position, drops tint+colour → TestRenderNoticeBand_NoColor (asserts band == ansi.Strip(band): zero SGR)
  - Extras (wrap refinement, all justified): TestNoticeBand_WrapsLongMessage, TestNoticeBand_BarOnEveryWrappedLine (coloured + NO_COLOR), TestNoticeBand_ContinuationLinesAlignUnderMessage, TestNoticeBand_FlashTintSpansEveryWrappedLine, TestNoticeBand_ShortMessageSingleLine, TestSessionBandHeight_TracksWrappedLineCount, TestNoticeBand_WrappedFrameHeightStaysTermH.
- Notes:
  - The success on-band token differs between the standalone primitive test (passes TextStrong at :96) and the live arbiter (noticeBandOnBandText returns TextOnWarning for bandSuccess, notice_band.go:372-377). This is NOT a drift: the primitive is parameterised on onBandText and the test exercises that parameter; the arbiter correctly selects text.on-warning because the success flash sits on the bg.warning tint. Consistent.
  - Tests assert SGR sequences derived from the live theme tokens (tokenFgSeq/tokenBgSeq), not hardcoded escapes — strong coupling to the no-literal-hex requirement.
  - Would fail if the feature broke: yes for every criterion (role colour, single-slot arbitration, placement, the ±2 height recompute, gen guard, NO_COLOR).

CODE QUALITY:
- Project conventions: Followed. Pure-function render primitive + small Model-method arbiter; tokens-only (no literal hex); NO_COLOR carve-out threaded through every style helper; no t.Parallel; mock-free unit tests over rendered output. Matches the established tui render/budget-symmetry pattern (the slot is measured off the same block it composes — the documented anti-drift idiom).
- SOLID principles: Good. Single source of truth for bar+tint (newBandBase) prevents the two info bands diverging; the arbiter is one chokepoint; padRightWithStyle is a clean shared-geometry extraction (10-1) consumed by two thin wrappers.
- Complexity: Low/Acceptable. renderNoticeBand carries the wrap/continuation-indent loop but it is linear and well-commented; cyclomatic load is modest.
- Modern idioms: Yes. ansi.Wrap for word-boundary wrap, lipgloss.JoinHorizontal/Vertical composition, iota role enum.
- Readability: Good. Doc comments are thorough and tie each decision back to a spec section; the band→blank→listView composition is explicit.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/notice_band.go:294-304 renderCommandBand (4-4's path) does NOT wrap or clamp its row to width — noticeBandPadRight returns an over-width row unchanged (header.go:226-228), so a long pending command would overflow the band's right edge at a narrow terminal, unlike renderNoticeBand which wraps. Out of 4-1's strict scope (it is the 4-4 banner) but it consumes this task's primitive; add a clamp/wrap or a width-aware chip truncation. Concrete location known.
- [do-now] internal/tui/notice_band.go:182-191 the renderNoticeBand doc lead-in still reads "emits a full-width single-line band" before the following paragraph (:196) corrects it to the multi-line wrap; tighten the opening sentence to say "single-line when it fits, wrapping to multi-line otherwise" so the first sentence is not contradicted two lines later.
- [do-now] internal/tui/notice_band.go:354 the arbiter's no-band return value `bandWarning, "", false` returns a meaningless role alongside ok=false; harmless (callers gate on ok) but a zero-value comment or a named `noActiveBand` sentinel would document that the role is don't-care when ok is false.
