TASK: restore-host-terminal-windows-6-7 — Pre-flight abort UI: gone flash + prune keeping survivors

ACCEPTANCE CRITERIA:
1. A pre-flight gone session aborts atomically: zero adapter calls, zero connector calls, no tea.Quit, no m.selected.
2. The section-header row renders ⚠ '<session>' is gone — nothing opened (red) with a right-aligned dim esc dismiss.
3. The gone session's row is flagged with a red ⚠ marker + red `session gone` badge; surviving marked rows keep their violet ●.
4. The gone session(s) are pruned from m.selectedSessions; every survivor stays marked (a second Enter proceeds with survivors, not a re-abort).
5. Multiple gone sessions are all named in the one-line message.
6. The picker stays in multi-select mode with the multi-select footer; esc dismisses the abort banner and the gone flags without exiting mode.
7. Zero windows opened → no leave-what-opened flash renders.

STATUS: Complete

SPEC CONTEXT:
Spec §"Stance: pre-flight + all-or-nothing" (L154-162): all-or-nothing at the pre-flight gate — if any marked session is gone, nothing opens; show a clean one-line error naming the gone session(s) (design copy `⚠ '<session>' is gone — nothing opened`), prune the gone session(s) keeping surviving marks intact so a second Enter proceeds with survivors, stay in multi-select mode, same prune-what's-gone rule as the sticky-selection preview round-trip (§"Sticky selection" L119-121). §"Notice-band precedence" (L138): transient error/guidance flash (pre-flight abort) outranks the multi-select banner. Design ref §"Sessions — Multi-Select (pre-flight abort)" (L504) governs placement. The delivered frame confirms: red ⚠ banner at section-header row (no ▌ left bar), right-aligned dim `esc dismiss`, gone row flagged with red ⚠ + `session gone` badge on the banded/cursor row, survivors keeping violet ●, multi-select footer unchanged.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/burst_preflight_abort.go:38-60 (handlePreflightAbort), :73-85 (WithInitialGoneFlagged harness seed), :92-96 (clearAbortBanner)
  - internal/tui/model.go:2529-2535 (spawnAbortMsg case → handlePreflightAbort), :3311-3319 (Esc/actionable-key abort-dismiss precedence), :4725-4744 (section-header abort-banner render), :1284-1288 (GoneFlagged propagated through the single sessionDelegate() chokepoint), :449-450 (abortBannerText/goneFlagged fields)
  - internal/tui/section_header.go:209-214 (renderPreflightAbortHeader), :81 (preflightAbortDismissHint = "esc dismiss")
  - internal/tui/session_item.go:415-417 (renderGoneLeftBarColumn), :482-492 (left-bar precedence: gone > marked > selector), :540-543 (red `session gone` badge trailing region), :65-71 (goneBadge const)
  - internal/spawn/message.go:40-42 (GoneMessage — shared renderer, count-aware verb)
- Notes: Behaviour matches the design frame byte-for-byte and the spec. Precedence in applySectionHeader is correct: Filtering → Opening band (burstPending) → abort banner → multi-select banner (model.go:4706/4716/4737/4750), matching §138. The abort banner is a section-header claimant (not the §11 notice band) per the delivered frame and the Task 5.3/6.2 golden-frame-governs-placement decision; the deviation from its notice-band spawn-failure/permission siblings is documented in-source (model.go:4732-4735). No adapter/connector/tea.Quit on this path — resetBurstState clears burst lifecycle, no m.selected set. Positive drift from the plan's literal instruction: the banner uses the higher-level spawn.GoneMessage rather than assembling spawn.QuoteJoin + spawn.GoneVerb at the call site — a DRY improvement (single copy-edit location, message.go:35 documents the picker banner as a caller) and it avoids the duplicate-declaration trap the plan's CORRECTION warned about. Width invariance is preserved because `used` (name-flex budget, session_item.go:513) reserves attachedSlotWidth+rowRightMargin unconditionally and the gone badge exactly fills that region (goneBadge width 12 == attachedSlotWidth 10 + rowRightMargin 2), so no downstream column shifts.

TESTS:
- Status: Adequate
- Coverage (internal/tui/burst_preflight_abort_test.go):
  - AC1 → TestBurstPreflightAbort_AbortsAtomicallyNoAdapterNoSelfAttach (end-to-end N≥2 Enter with one vanished session: adapter.Calls==0, Selected()=="", follow cmd nil = no tea.Quit, burst-pending cleared, stays in mode, flashText empty, banner set, gone flagged).
  - AC2 → TestBurstPreflightAbort_BannerNamesGoneSessionWithEscDismiss (exact copy byte-match + ⚠ glyph + `esc dismiss` + outranks `N selected`), TestPreflightAbortHeader_RedGlyphMessageDimHint (state.red glyph/message, text.detail hint, dark+light), TestPreflightAbortHeader_RightAlignedOneRow (right-anchored, exactly 1 row, exact content width).
  - AC3 → TestSessionRow_GoneFlaggedShowsRedWarningAndBadge (red ⚠ at col 0, no ● on gone row, red `session gone` badge, no attached badge, state.red run; survivor keeps violet ● and no ⚠).
  - AC4 → TestBurstPreflightAbort_PrunesGoneKeepsSurvivorsMarked (gone pruned, survivors marked, count==2, stays in mode, rendered list shows badge).
  - AC5 → TestBurstPreflightAbort_MultipleGoneAllNamed (both named, plural verb `are`, both flagged+pruned, survivors kept).
  - AC6 → TestBurstPreflightAbort_EscDismissesWithoutExitingMode (first Esc clears banner+flags stays in mode, survivor stays marked, second Esc exits mode).
  - AC7 → asserted in the atomic test (flashText=="").
  - Invariant/carve-out coverage: TestSessionRow_GoneFlaggedWidthByteUnchanged (§3.5 one-delegate-line width byte-unchanged), TestPreflightAbortHeader_ColourlessDropsHueAndCanvas + TestSessionRow_GoneFlaggedColourlessSurvives (§2.5 NO_COLOR carve-out for banner and row), TestSessionRow_HeaderNeverGoneFlagged (HeaderItem never gone-flagged).
- Notes: Well-balanced — not over-tested (each test pins a distinct invariant; the colourless/width/header-guard tests defend real spec contracts, not implementation detail). Two small completeness gaps, both non-blocking: (a) AC4's "a second Enter PROCEEDS with the survivors" is verified only via its constituents (prune keeps survivors marked + the non-Esc-key fall-through at model.go:3317 that re-runs the handler); there is no end-to-end test that presses Enter a second time and asserts a fresh burst dispatches over the survivor set. (b) The "multi-select FOOTER unchanged" clause is verified transitively via MultiSelectActive() rather than by asserting the footer text; the footer is a pure function of multiSelectMode and is covered by sibling multi-select tasks.

CODE QUALITY:
- Project conventions: Followed. Small-interface DI (spawn.Adapter/AckChannel seams), package-level *Deps-style wiring, no t.Parallel, white-box tui tests consistent with the surface. GoneFlagged propagated through the single sessionDelegate() chokepoint (model.go:1281) so applyCanvasMode/refreshSessionDelegate can't drift — mirrors the Selected propagation. Delegate glyph column folded into renderLeftBarGlyphColumn (DRY across ▌/●/⚠). Copy centralised in spawn.GoneMessage.
- SOLID principles: Good. handlePreflightAbort has one clear responsibility; render, prune, and lifecycle-reset are each delegated to named helpers.
- Complexity: Low. Linear map builds over small slices; clear switch-based left-bar precedence.
- Modern idioms: Yes (map[string]struct{} sets, slices helpers in tests, value-receiver Model with explicit (&m) mutation for the in-place helpers).
- Readability: Excellent. Doc comments cross-reference spec sections and sibling tasks; the section-header-vs-notice-band precedence deviation is explicitly justified in-source.
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/burst_preflight_abort_test.go:318 — add an end-to-end assertion for AC4's second-Enter path: after the prune, press Enter again and assert a fresh burst dispatches over the survivor set (not a re-abort). Currently only the prune + stay-marked constituents are tested; the composed "second Enter proceeds" behaviour is inferred.
- [do-now] internal/tui/burst_preflight_abort_test.go:374 — in TestBurstPreflightAbort_EscDismissesWithoutExitingMode, add a single assertion that the rendered footer is the multi-select footer after dismissal, making AC6's "multi-select footer unchanged" clause explicit rather than transitive via MultiSelectActive().
