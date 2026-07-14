TASK: restore-host-terminal-windows-6-11 (Task 6.11) — Visual gates: capture + wire the remaining Phase-6 frames (unsupported-terminal, multi-select pre-flight-abort, in-burst Opening band)

ACCEPTANCE CRITERIA:
- capture.FixtureByName returns each of sessions-unsupported-terminal, sessions-multi-select-preflight-abort, sessions-burst-opening; FixtureNames() includes all three (sorted).
- --fixture sessions-unsupported-terminal renders the amber ⚠ unsupported terminal — Apple Terminal · com.apple.Terminal + blue see docs over the normal list — matches reference.
- --fixture sessions-multi-select-preflight-abort renders the red ⚠ '<session>' is gone — nothing opened + esc dismiss, gone row flagged red, surviving ● intact, multi-select footer — matches reference.
- --fixture sessions-burst-opening renders the Opening 2/3… band (new residual, no delivered reference).
- NO_COLOR variant of each renders glyph-backed (⚠/●/text/see docs/esc) on native bg without crashing (no hue, no canvas).
- Dark appearance only — no light-mode fixtures/tapes.
- capturetool import guard + fixture-list tests pass with the three new fixtures.

STATUS: Complete

SPEC CONTEXT: Phase 6 wires the in-picker N≥2 burst. This task is the visual-gate deliverable: three capture-only fixtures seed the otherwise-async detection/abort/burst state through the shared tui.Build constructor so the delivered Paper frames (§6.2 unsupported banner, §6.7 pre-flight abort) can be verified, and the §6.5 Opening n/N… band — a design residual with no Paper oracle — is captured fresh. Detection resolves via the resolution-based DetectUnsupported() test (a non-NULL, recognised-but-undriven Apple Terminal resolves unsupported), not IsNull().

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/capture/fixtures.go:447-523 — sessionsUnsupportedTerminalFixture / sessionsMultiSelectPreflightAbortFixture / sessionsBurstOpeningFixture, all reusing sessionsFlatFixture()'s 12-session set verbatim.
  - internal/capture/fixtures.go:171-175 (FixtureByName cases) + :200 (FixtureNames slice, sorted via sort.Strings).
  - internal/capture/fixtures.go:99-131 — Deps() maps InitialDetection / InitialGoneFlagged / InitialBurstOpening onto tui.Deps.
  - internal/tui/spawn_detect.go:47-70 — WithInitialDetection: resolves via zero-config spawn.ResolveAdapter, seeds detectAdapter + detectResolution + detectResolved (not just IsNull), so DetectUnsupported() is true for Apple Terminal.
  - internal/tui/burst_preflight_abort.go:62-85 — WithInitialGoneFlagged: seeds goneFlagged + abortBannerText via the SAME spawn.GoneMessage the live handlePreflightAbort uses; refreshes delegate.
  - internal/tui/burst_progress.go:270-286 — WithInitialBurstOpening: seeds burstPending + burstDone/burstTotal; [0,0] no-op.
  - internal/tui/build.go:240-242 — options applied in correct order (multi-select before gone-flagged so survivors keep their ●).
  - testdata/vhs/ — sessions-{unsupported-terminal,multi-select-preflight-abort,burst-opening}.tape/.png + -nocolor.tape/.png (6 tapes, 6 PNGs).
  - testdata/vhs/reference/sessions-unsupported-terminal-mv.png + sessions-multi-select-preflight-abort-mv.png — byte-identical (SHA1-matched) copies of the delivered design/ frames.
- Notes: WithInitialDetection intentionally uses the zero-config spawn.ResolveAdapter rather than the production config-aware m.resolve — an explicitly-approved plan CORRECTION (sufficient because Apple Terminal is undriven regardless of config: only Ghostty is a native adapter, resolver.go:32-37, so com.apple.Terminal → ResolutionUnsupported). No burst-opening reference frame exists — correct, it is a design residual per the task; the fresh capture is the reference. Work is committed (43ec57f2).

TESTS:
- Status: Adequate
- Coverage (internal/capture/capture_test.go):
  - TestSessionsUnsupportedTerminalFixture:883 — asserts flat set, seeded Apple Terminal identity (Name/BundleID), NOT multi-select, DetectUnsupported()==true, MultiSelectActive()==false.
  - TestSessionsMultiSelectPreflightAbortFixture:930 — asserts flat set, three marked, cursor on gone row, InitialGoneFlagged={fab-flowx-explore}, MultiSelectActive()==true (survivors stay marked).
  - TestSessionsBurstOpeningFixture:977 — asserts flat set, three marked, InitialBurstOpening=={2,3}, BurstPending()==true, BurstDone()==2, BurstTotal()==3.
  - TestFixtureNamesIncludes{UnsupportedTerminal,MultiSelectPreflightAbort,BurstOpening} — membership pins via shared assertFixtureNameListed helper.
  - Shared flatFixtureWants()/assertFlatFixtureSet:826-868 helper dedupes the 12-session drift guard across the three fixtures.
  - Import guard cmd/capturetool/import_guard_test.go is structural (transitive-dep exclusion of internal/capture) — correctly needs no per-fixture count bump; no hardcoded fixture-count assertion exists anywhere, so the membership tests are the right seam.
- Notes: Render assertions (banner hue/glyph, NO_COLOR glyph-backing) live in internal/tui per tasks 6-2/6-5/6-7 and the PNG visual gate — the fixture tests are correctly scoped to state-wiring, not render. Not over-tested: each full-set assertion guards a distinct fixture; membership checks are one-liners. Visual gate verified by direct PNG read below.

VISUAL GATE (PNG read):
- sessions-unsupported-terminal.png vs reference: MATCH — amber ⚠ unsupported terminal — Apple Terminal · com.apple.Terminal, right-anchored blue see docs, normal list + footer. (Window counts differ from the Paper mock because the fixture uses the canonical sessions-flat set — expected; the banner under test matches exactly.)
- sessions-multi-select-preflight-abort.png vs reference: MATCH — red ⚠ 'fab-flowx-explore' is gone — nothing opened, dim esc dismiss, gone row shows red ⚠ + session gone badge, two survivors keep violet ●, multi-select footer.
- sessions-burst-opening.png: violet Opening 2/3… band at section-header row (highest claimant), three marked rows keep violet ●, multi-select footer — correct residual capture.
- All three -nocolor.png: glyph-backed on native bg, no hue, no canvas — ⚠/●/session gone/see docs/esc all survive.

CODE QUALITY:
- Project conventions: Followed — fixtures reuse sessionsFlatFixture() verbatim (only seed fields + name differ), matching the established fixture pattern (sessions-multi-select-active). Seed seams mirror WithInitialMultiSelect/WithInitialFlash nil/zero-tolerant convention. Tapes mirror sessions-flat.tape structure with the documented VHS slashed-path quoting + 4s cold-compile pad.
- SOLID principles: Good — each seam is a single-purpose Option; the capture harness stays out of the production binary (import guard).
- Complexity: Low.
- Modern idioms: Yes — [2]int seed for the (done,total) pair, slices.Contains in the test helper.
- Readability: Good — every seam and fixture carries a load-bearing doc comment explaining why the state must be seeded (async/Enter-driven origin) and the correction rationale (resolution-based DetectUnsupported, not IsNull).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
