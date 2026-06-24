TASK: spectrum-tui-design-1-9 — In-terminal contrast-floor VALIDATION & lock-in/bail gate: pin + eyeball the four light surface tints (bg.selection #D0C6F0, bg.warning, bg.track, light borders #C9CDDB) against #e1e2e7, confirm every foreground-on-tint pairing clears the floor, record the lock-in/bail decision.

ACCEPTANCE CRITERIA (from tick-6b0f62 / plan):
- (VISUAL) Four light surface tints pinned to concrete hexes and eyeballed against #e1e2e7 in a real terminal — each a distinct surface, not a wash-out (numeric pass insufficient).
- Every foreground-on-tint pairing (selected-row name/count/attached on bg.selection; text.on-warning on bg.warning) clears the floor in-terminal; any remedy is the more-contrast direction, never a lowered floor, co-tuned to clear simultaneously.
- Each pinned tint derived from its dark anchor + the surface it renders on (recorded), not invented.
- Lock-in (or bail) decision recorded explicitly with the final pinned hexes (lock-in) or the failing tint/pairing + rationale (bail).
- vhs captures of the foundation/validation surface in dark AND light, compared to the named frames for layout/structure/colour-role.
- Note (§12.3): confirm Ctrl+up/Ctrl+down paging chords are delivered, not swallowed by terminal/tmux; record fallback if intercepted.

STATUS: Complete

SPEC CONTEXT:
§1 + §16.5 make the colour direction a hypothesis until prototyped in a real terminal; this is the anti-sunk-cost lock-in gate where bail is a legitimate recorded outcome. §2.3 sets the contrast floor (4.5 normal / 3.0 large-UI) measured against the exact owned canvas (light #e1e2e7). §2.9 finalises the light SURFACE tints at this §15 gate because the recurring failure class — a light tint on a light canvas — is numerically insufficient and must be eyeballed; the remedy rule is more contrast, never a lowered floor, with the two-knob co-tune. §15.6 makes light-mode coverage per-token and names the pin+eyeball of each light surface tint as an explicit implementation task. §4.1 requires every foreground-on-tint pairing verified against the tint.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/theme/theme.go:164-184 — the four light tints PINNED to concrete hexes with inline derivation comments (dark anchor + light canvas): bg.selection #D0C6F0 (anchor #28243a), bg.warning #E8D6A8 (anchor #241B10), bg.track #D2D4DE (anchor #26283A), border.separator/border.footer #C9CDDB (shared). Matches the captured/labelled hexes and the LOCK-IN record exactly.
  - testdata/vhs/LOCK-IN.md — committed lock-in/bail record: DECISION = LOCK-IN (line 146-167) with final pinned hexes, per-tint derivations (lines 40-45), numeric ratio tables (fills-vs-canvas, §4.1 fg-on-tint pairings, accent bars), the human-eyeball wash-out finding on green-on-bg.selection + its remedy, and the Ctrl+↑/↓ chord finding (delivered, no fallback needed).
  - internal/capture/swatch.go — the contrast-validation swatch tea.Model (pure renderSwatch + mode resolution) that renders the four labelled tint bands with on-tint foreground pairings on the owned canvas for the pinned mode. Correctly NOT the production Sessions surface (those tint surfaces land in later phases) — the anti-sunk-cost token gate.
  - cmd/capturetool/main.go:79-90 + internal/capture/fixtures.go:163 — fixture wired with --appearance pin.
  - testdata/vhs/contrast-validation-{dark,light}.{tape,png} — captures present in both modes.
- Notes: I read both PNGs. The LIGHT capture renders the four light tints against #e1e2e7 (labelled #D0C6F0 / #E8D6A8 / #D2D4DE / #C9CDDB) with the name (text.on-selection) + "3 windows" (text.strong) + green "● attached" on the selection band, the amber "⚠ ..." warning band (text.on-warning), the violet loading bar over the grey empty track, and the separator/footer rule. The DARK capture mirrors it on #0b0c14 with the dark hexes. The labelled hexes match theme.go byte-for-byte. The lock-in artefact records LOCK-IN, not bail — consistent with a foundation that cleared the bar.
  - Phase-2 supersession is correctly reflected: the former dedicated state.green-on-selection override was folded into the global state.green (light darkened #456E1C → #3B5E18), which clears 4.5 on BOTH the canvas (>4.64) and bg.selection (4.65). theme.go:152-158 and the LOCK-IN.md supersession block (lines 88-101) document this cleanly; the swatch renders the single token (#3B5E18 = rgb 59,94,24, asserted in swatch_test.go).

TESTS:
- Status: Adequate
- Coverage:
  - internal/tui/theme/contrast_test.go — TestLightSurfaceTintsPinned (pins all four light hexes to their 1-9-locked values), TestLightTintFillsArePerceptible (every light tint ≥1.1 vs #e1e2e7 — the light-tint-on-light-canvas numeric leg), TestForegroundOnTintPairings (name/count/attached on bg.selection + text.on-warning on bg.warning, both modes, vs the tint), TestStateGreenClearsCanvasAndSelection (the single-token wash-out remedy: #3B5E18 ≥4.5 on canvas AND bg.selection, with a regression guard on the exact hex), TestBgWarningPairRule / TestBgTrackPairRule / TestBgSelectionPairRule (three-leg approved pair rule), TestContrastMath (anchors the WCAG math against the black/white 21:1 reference so no floor assertion passes vacuously).
  - internal/capture/swatch_test.go — TestSwatchBandsCoverEveryLightTint, TestSwatchCoversForegroundOnTintPairings, TestSwatchAttachedMarkerUsesStateGreen (asserts the marker renders in #3B5E18 = rgb 59,94,24), TestSwatchRendersBothModes (mode actually drives the resolved variant), TestSwatchModeFromAppearance.
  - cmd/capturetool/swatch_test.go — resolveProgram wires the contrast-validation fixture for light/dark and errors on an invalid appearance.
- Notes: Test balance is good — not over-tested. The TestContrastMath anchor is exactly right (prevents vacuous passes). The numeric floor tests are the necessary-but-insufficient layer; the visual eyeball is correctly carried by the human in LOCK-IN.md (the captures + recorded DECISION are the evidence the harness cannot self-assert). The "more contrast never lower the floor" remedy is encoded as a regression guard (TestStateGreenClearsCanvasAndSelection pins #3B5E18; a revert to #456E1C re-introduces the 3.72 wash-out and fails the bg.selection leg). No redundant assertions; the per-section duplication (e.g. inline-flash pair vs general pair rule) is deliberate and carries section-scoped failure messages.

CODE QUALITY:
- Project conventions: Followed. No raw hex at render call sites — the swatch references theme tokens via ColorFor(mode) throughout (the §2.8/§2.9 token-discipline rule). Tests use table-driven subtests (golang-testing convention), no t.Parallel() (correct for this repo). renderSwatch is kept pure (mode-in, string-out) so it is unit-testable without a tea.Program — idiomatic seam.
- SOLID principles: Good. swatchModel is a thin tea.Model wrapper; the render is a free function; mode resolution (modeFromAppearance) is isolated and mirrors the production WithCanvasMode pin path.
- Complexity: Low. Small pure helpers (tintLabel, pairCaption, selectionBand, warningBand, trackBand, borderRule, padBand), each one responsibility.
- Modern idioms: Yes (strings.Builder, lipgloss styling, image/color).
- Readability: Good. Comments tie each band/derivation back to the spec section and the dark anchor; the LOCK-IN.md supersession block preserves the historical Phase-1 finding while flagging the current state.
- Issues: None blocking.

BLOCKING ISSUES:
- None. The task is complete: tints pinned + derived (recorded), every foreground-on-tint pairing clears the floor numerically and is rendered for eyeball, captures exist in both modes, the lock-in DECISION is recorded explicitly (LOCK-IN, with final hexes and the green-on-selection wash-out finding + remedy), and the Ctrl+↑/↓ chord finding is recorded (deliverable; tmux-passthrough re-confirm correctly deferred to 2-1).

NON-BLOCKING NOTES:
- [do-now] testdata/vhs/contrast-validation-light.tape:9-12 — the tape header comment still describes the on-selection foreground as "state.green-on-selection — the §2.8 darker on-selection override, the 1-9 wash-out remedy". That override was superseded in Phase 2 (folded into the global state.green #3B5E18); the swatch now renders the single state.green token. Update the comment to say "state.green (darkened light #3B5E18, the folded-in wash-out remedy)" to match theme.go:152-158, LOCK-IN.md, and the dark tape (which already reads only "on-tint foreground text"). Documentation-only; no logic.
- [do-now] testdata/vhs/LOCK-IN.md:158-163 — the DECISION block's "On-selection green remedy" bullet still states the remedy as "a dedicated state.green-on-selection token (light #3B5E18 ... light-only) — the global state.green is UNCHANGED" and instructs "Phase 2 task 771c41 MUST use state.green-on-selection". The supersession block at lines 88-101 already corrects this (the override was removed, #3B5E18 folded into the global token), but the DECISION bullet at the bottom was not re-synced and now contradicts it. Add a one-line "(superseded — see the Phase-2 note above; folded into global state.green)" pointer to the DECISION bullet so the authoritative bottom-of-file record isn't self-contradictory. Documentation-only.
