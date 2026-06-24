TASK: spectrum-tui-design-1-4 — Light token variants + independent contrast-floor numeric verification

ACCEPTANCE CRITERIA:
- Every §2.9 token carries a light variant matching the §2.9 light column (surface tints provisional).
- The numeric contrast-floor test passes: each foreground token clears its floor against its OWN mode-canvas, measured independently (dark vs #0b0c14, light vs #e1e2e7).
- text.dim asserted to the 3:1 floor; text.faint exempt (asserted only to stay in the decorative band, never promoted to functional).
- Text-carrying tints verified as a pair (tint-vs-canvas AND text-vs-tint) both clearing simultaneously for bg.selection+on-band and bg.warning+text.on-warning.
- Light surface tints flagged provisional-pending the in-terminal eyeball (1-9); any remedy is the more-contrast direction (never a lowered floor) and recorded.

STATUS: Complete

SPEC CONTEXT:
§2.3 mandates a hard contrast gate before taste: 4.5:1 normal text, 3:1 large/bold/UI, each variant measured against its EXACT owned canvas (dark #0b0c14, light #e1e2e7), resolving INDEPENDENTLY (no single value need hold on both). §2.9 pins the closed ~20-token vocabulary, requires the canvas re-verification pass, the co-tuned text-carrying-tint pair rule (tint-vs-canvas AND text-vs-tint clear together; two knobs — move the text token if the tint can't satisfy both), the remedy rule (adjust toward MORE contrast, never lower the floor), and flags the four light surface tints as finalised/eyeballed at the §15 validation gate (1-9). text.dim is held to 3:1 (de-emphasised); text.faint is decorative-only and exempt.

IMPLEMENTATION:
- Status: Implemented (with documented, user-approved drift from the §2.9-published light hexes — see Notes)
- Location: internal/tui/theme/theme.go:132-184 (light variants on every token); ColorFor at :49-54 resolves the two variants independently.
- Notes: All 20 tokens carry a non-empty, valid light hex. Six light foreground hexes were corrected (darkened, hue-preserved) from the §2.9 light column because §2.9's published light RATIO column was computed vs #FFFFFF, not the mandated canvas #e1e2e7 — under #e1e2e7 the original values were under-floor. The corrections (text.detail #5A6296→#586093, text.dim #7C84AA→#767DA2, accent.blue #2E5FD0→#2D5CCA, accent.cyan #0E7490→#0D6C87, state.green #4C7A1F→#456E1C→#3B5E18, state.red #C32647→#BD2545) each carry an inline erratum comment with the corrected ratio. text.muted-bright was further darkened #515A80→#4C5478 so the selected-row path clears 4.5 on bg.selection. state.green was folded to a single token (#3B5E18) clearing BOTH canvas and bg.selection, removing a per-context on-selection override. All drift is recorded in the tick notes as user-approved and is the more-contrast direction (spec-compliant remedy rule). I independently recomputed WCAG ratios with go-colorful for every token against its own canvas and against each tint — every value clears its floor.

TESTS:
- Status: Adequate
- Coverage: internal/tui/theme/contrast_test.go is a thorough, behaviour-focused numeric gate:
  * TestContrastMath anchors the WCAG math against black/white=21.00 and self=1.00 (prevents vacuous passes).
  * TestForegroundFloorAgainstOwnCanvas measures every foreground token against ONLY its own canvas (dark and light subtests), per-token floor.
  * TestTextDimHeldToThreeToOneFloor pins text.dim to 3:1 AND asserts it stays below the 4.5 normal floor (both bounds — guards against silent tightening or loosening).
  * TestTextFaintDecorativeBand asserts text.faint is >1.0 (visible) and strictly <3.0 (structurally decorative, can never be promoted to functional) in both modes — exactly the "exempt but guarded" requirement.
  * TestBgSelectionPairRule / TestBgWarningPairRule encode the approved three-leg pair rule (text-on-tint ≥ floor, accent bar ≥3:1 vs canvas, fill perceptible ≥1.1) in both modes.
  * TestForegroundOnTintPairings covers every §4.1 selected-row foreground (on-selection, strong, muted-bright path, state.green attached marker) on bg.selection plus on-warning on bg.warning.
  * TestStateGreenClearsCanvasAndSelection justifies the single-token green (clears canvas AND selection in both modes) and pins #3B5E18 against regression to the washing-out #456E1C.
  * TestEveryTokenHasLightVariant proves the light column is fully populated and each hex parses.
  * TestLightSurfaceTintsPinned / TestLightTintFillsArePerceptible / TestBgTrackPairRule cover the surface-tint provisional/now-pinned story.
- Notes: I re-derived all ratios independently — every assertion passes against the implemented hexes (e.g. text.detail L 4.63, text.dim L 3.11 and <4.5, accent.orange L 4.53, bg.warning fill 1.11 vs 1.1, state.green L vs bg.selection 4.65). The independent-resolution requirement (asserting each variant only against its own canvas) is correctly honoured — no test cross-measures a variant against the other mode's canvas. text.dim has light overlap between the main table test and the dedicated TestTextDimHeldToThreeToOneFloor, but the dedicated test adds the load-bearing upper-bound (<4.5) assertion, so it is not pure redundancy.

CODE QUALITY:
- Project conventions: Followed. Table-driven subtests, t.Helper() on the math helpers, no t.Parallel() (per CLAUDE.md), idiomatic Go. go-colorful (already a dep) used for the WCAG sRGB linearization — correct, exact WCAG math (validated by the 21:1 anchor). go-colorful was promoted from indirect to a direct require in go.mod:11 — correct and expected, since the test now imports it directly (go mod tidy promotes it). No scope creep.
- SOLID principles: Good. The Token{Dark,Light} + ColorFor(mode) seam keeps the two variants independent; the test layer is pure and isolated.
- Complexity: Low. Test helpers are trivial; the token table is declarative.
- Modern idioms: Yes.
- Readability: Strong. The erratum comments record original→corrected hex + measured ratio beside each adjusted token; the test doc-comments tie each gate to its spec section.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/theme/theme.go:147 — accent.violet light #8A3FD1 measures 4.37 vs #e1e2e7, not the §2.9-published 5.7 (same #FFFFFF-vs-#e1e2e7 erratum as the six corrected tokens). It clears its 3.0 floor unchanged so no remedy was needed, but unlike the six corrected tokens it carries no note of the published-ratio discrepancy. Add a one-line comment recording "§2.9 published 5.7 vs #FFFFFF; 4.37 vs #e1e2e7, clears the 3.0 floor unremedied" so the table-vs-actual gap is documented consistently across all affected tokens.
- [do-now] internal/tui/theme/contrast_test.go:331 — TestEveryTokenHasLightVariant and theme_test.go's TestEachTokenCarriesLightVariant overlap in intent ("light variant exists"). They are not identical (the latter proves the resolver seam, the former proves every token is populated + parseable), but a one-line cross-reference comment on each pointing at the other would prevent a future contributor deleting one as a perceived duplicate.
