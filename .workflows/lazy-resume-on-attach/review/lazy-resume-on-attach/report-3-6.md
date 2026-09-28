TASK: Both Screens on Demand for the Visual Gate (lazy-resume-on-attach-3-6) — six standalone named capture surfaces rendering the production resume panel / discard confirmation, reachable through capturetool, with the fixture-guard skip declared once and a palette-diff guard of their own.

ACCEPTANCE CRITERIA:
- All six names are listed by capture.FixtureNames(), resolve through SurfaceByName, and are rejected by FixtureByName with its existing unknown-fixture error.
- resolveProgram returns a model for each of the six at a built-in slug and at an explicit .theme path, and the model's view content is byte-identical to calling the production tui.RenderResumePanel / tui.RenderResumeDiscardConfirm with the same seed, theme and pinned size.
- The two degraded surfaces render the plain stack at their pinned size with no terminal resize involved, and the two report surfaces render their report row.
- A surface's tea.View carries AltScreen and leaves BackgroundColor unset, and the surface satisfies neither branch of run's restore type switch.
- NO_COLOR set to a non-empty value renders every surface colourless with no canvas painted, by the same env read resolveModel uses.
- The widened skip assertion fails if a surface name is enumerated but absent from the skip set, and fails if a name in the skip set resolves through FixtureByName.
- The surface palette-diff guard fails when a token painted under palette A is not painted under palette B on the same surface, and fails when a surface paints no token at all.
- TestPortalBinaryDoesNotImportCapture still passes — nothing added here reaches the production binary.
- Every existing fixture guard still covers exactly what it covered; the registry, colourless, render-size, swap-harness and capturetool theme-persister suites each skip the six surface names through the shared skip set rather than fataling.

STATUS: complete

SPEC CONTEXT: Section 5.2-5.4 define the waiting panel and discard confirmation as full-pane canvases with a centred card that degrade to a plain stack below the card's size, carry a single report row only when there is something to say, and honour NO_COLOR as a glyph-backed colourless render. Section 5.5 names the committed Nord reference frames the screens are held against at the visual gate. The CLAUDE.md "Visual capture harness" rules make capturetool the only pre-release route to seeing a screen and make the swap-and-diff guard's coverage enumerate the registry, which is why the new surfaces need their own guard in exchange for being skipped.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/capture/resume_surfaces.go:44-103 — Surface type (seed, draw func, pinned size via ResumeScreen.Width/Height), WithTheme, Update (records size, quits on q/ctrl+c/esc), View (production draw at the pinned size, AltScreen set, no BackgroundColor), PinRenderSize
  - internal/capture/resume_surfaces.go:108-118 — six registrations, each a single variation (report string or degraded 40x10 size) from its 80x24 base, all carrying a directory-plus-identifier command
  - internal/capture/resume_surfaces.go:122-148 — SurfaceNames, SurfaceByName, and the one shared skip declaration StandaloneNames / IsStandalone
  - internal/capture/fixtures.go:201-211 — FixtureNames appends StandaloneNames() before the sort; FixtureByName (189-196) still resolves *Fixture only
  - internal/capture/harness.go:79-87 — substituteDeclaredSize shared between Fixture.renderSize and the surfaces
  - cmd/capturetool/main.go:83-92 — renderSizeFilter pins a surface's size before the fixture lookup
  - cmd/capturetool/main.go:108-110 — resolveProgram routes a surface name before resolveModel
  - cmd/capturetool/main.go:196, 202-205 — the single noColourRequested() env read used by both the surface branch and resolveModel
  - Enumerating sites moved onto IsStandalone: internal/capture/theme_swap_guard_test.go:102 (registryFixtures) and :148 (first sub-test), internal/capture/fixture_registry_test.go:29-39 and :44, internal/capture/fixture_colourless_test.go:32, internal/capture/fixture_render_size_test.go:77, internal/capture/swap_harness_test.go:24 (buildBackedFixtureNames), cmd/capturetool/theme_persister_test.go:21
  - CLAUDE.md "Visual capture harness" paragraph carries the standalone-surfaces clause
- Notes: The surfaces call tui.RenderResumePanel / tui.RenderResumeDiscardConfirm directly rather than a copy of the layout. *Surface has neither tui.Model's type nor the OriginalBackground/PaintedCanvasHex pair, so run's restore switch (cmd/capturetool/main.go:65-73) sets nothing back. The first "surface absent from the skip set" check cannot fire today because StandaloneNames is built from SurfaceNames; it pins that composition against a later edit, which is sound. No production file gained an import of internal/capture: the only new importer is the separate cmd/capturetool main, and the resume draw path (cmd/state_resume_draw.go) imports tui/theme/prefs only.

TESTS:
- Status: Adequate
- Coverage:
  - Registry: SurfaceNames order, FixtureNames lists all six, FixtureByName rejects each with "unknown fixture" (internal/capture/resume_surfaces_test.go:80-113)
  - Byte-identity to the production render per surface, with seeds restated independently (internal/capture/resume_surfaces_test.go:118-133); through resolveProgram at a built-in slug and at an explicit .theme path (cmd/capturetool/resume_surface_test.go:67-92)
  - Pinned dimensions, frame present on roomy and absent on degraded, report row present only on the report pair (internal/capture/resume_surfaces_test.go:135-166)
  - AltScreen set / BackgroundColor nil, and neither restore-switch branch satisfied (internal/capture/resume_surfaces_test.go:182-202; cmd/capturetool/resume_surface_test.go:106-132)
  - NO_COLOR through the env read byte-identical to the colourless production render (cmd/capturetool/resume_surface_test.go:94-104), the single-read guard (:151-159), and no token run under colourless (internal/capture/resume_surfaces_test.go:168-179)
  - renderSizeFilter pins each surface (cmd/capturetool/resume_surface_test.go:135-147)
  - Widened skip assertion (internal/capture/theme_swap_guard_test.go:155-195) plus the exact skip-set pin (internal/capture/resume_surfaces_test.go:223-248)
  - Palette-diff guard per surface (internal/capture/resume_surface_swap_guard_test.go:18-34), with its failing paths driven through harnesstest.Recorder for set mismatch, no token painted, and an A-run surviving (:73-116)
- Notes: The two packages each test byte-identity and the no-background rule, but at different seams (the Surface directly vs. the program path through resolveProgram and renderSizeFilter), which the criteria require separately. The existing enumerating suites run unchanged against the registered surfaces, which is the substance of "it keeps every enumerating guard over its own set".

CODE QUALITY:
- Project conventions: Followed — no t.Parallel, shared themetest/harnesstest helpers, the swap guard's tokenForms/carriesRun/observedTokens reused rather than restated, surface names unexported, no process-artifact references in comments
- SOLID principles: Good — the skip is declared once (StandaloneNames/IsStandalone) and every enumerating site reads through it; the size rule is shared through substituteDeclaredSize
- Complexity: Low
- Modern idioms: Yes — slices.Contains/DeleteFunc/Sorted, per-iteration loop variables
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
