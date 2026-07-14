TASK: 5.8 — Visual gate: `sessions-multi-select-active` capture fixture + reference (restore-host-terminal-windows-5-8 / tick-333179)

ACCEPTANCE CRITERIA:
- capture.FixtureByName("sessions-multi-select-active") returns a fixture and capture.FixtureNames() includes it (sorted).
- `go run ./cmd/capturetool --fixture sessions-multi-select-active` renders in multi-select mode: violet `3 selected` banner + right-aligned `esc cancel`, `●` on the three marked rows (incl. the banded cursor row fab-flowx-explore), multi-select footer copy.
- The captured `sessions-multi-select-active.png` matches the delivered Paper frame (`testdata/vhs/reference/sessions-multi-select-active-mv.png`) — banner, markers (cursor row also marked), footer, layout.
- The NO_COLOR variant renders glyph-backed (`●`/`▌`/`esc`) on the native bg without crashing (no violet hue, no canvas).
- Dark appearance only — no light-mode fixture/tape added.
- The capturetool import guard + fixture-list tests pass with the new fixture.

STATUS: Complete

SPEC CONTEXT: Spec §Design References → "Sessions — Multi-Select (active)" prescribes the violet `3 selected` banner (filter-line analogue), violet `●` markers on selected rows including the cursor row, `Space` still preview, and the footer `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel`. Tokens are dark-mode only (light deferred). The visual-gate process requires re-capturing fresh frames through the capturetool/vhs harness and moving the delivered `design/` frame into `testdata/vhs/reference/`. The harness never opens a real tmux server; it drives the exact production model via `tui.Build` with in-memory fakes, seeding otherwise-transient state through the same mechanism as `InitialFlash`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Seed seam: internal/tui/build.go:92-102 (`Deps.InitialMultiSelect`, `Deps.InitialCursor`), wired in Build at build.go:231-232 (before armAppearanceDetection) via the options.
  - internal/tui/model.go:1005-1017 `WithInitialMultiSelect` (mirrors handleMultiSelectToggle enter step: sets multiSelectMode, seeds selectedSessions keyed on name, refreshes delegate; nil/empty = no-op).
  - internal/tui/model.go:1025-1029 `WithInitialCursor` (stores name only; applied post-ingest); applyInitialCursor at model.go:1876-1884 runs from evaluateDefaultPage (model.go:1850) after applyInitialFilter, delegating to reanchorSessionCursor (model.go:1933) which selects the visible row by name and clamps on absence — so it survives the initial SetItems index-0 reset. Production never sets it.
  - Fixture: internal/capture/fixtures.go:435-445 `sessionsMultiSelectActiveFixture()` reuses sessionsFlatFixture() verbatim, sets name + initialMultiSelect {agentic-workflows-codify, fab-flowx-explore, designlab-web-r8suyU} + initialCursor "fab-flowx-explore". Struct fields mapped into Deps() at fixtures.go:115-116. Registered in FixtureByName (fixtures.go:169-170) and FixtureNames (fixtures.go:200, sorted at 201).
  - Tapes: testdata/vhs/sessions-multi-select-active.tape (dark, mirrors sessions-flat.tape) + testdata/vhs/sessions-multi-select-active-nocolor.tape (inline NO_COLOR=1). Committed PNGs: sessions-multi-select-active.png (143k) + -nocolor.png (106k). Reference moved to testdata/vhs/reference/sessions-multi-select-active-mv.png (205k, matches design/sessions-multi-select-active.png).
  - NO_COLOR wiring: cmd/capturetool/main.go:126-127 reads NO_COLOR env → deps.NoColor.
- Notes: Visual comparison of captured dark frame vs the Paper reference confirms all load-bearing dimensions match — violet `3 selected` banner, right-aligned `esc cancel`, `●` on the three marked rows, cursor on fab-flowx-explore rendered as a bold banded row that keeps its `●` (marker takes precedence over the `▌` selector), and the exact multi-select footer copy. Divergences from the Paper mock (per-row window counts, the spread of `● attached` markers, and the 3 paginator dots) are session-data/placeholder differences inherited from reusing the canonical sessions-flat set verbatim — explicitly mandated by the task ("reuse the sessions-flat session set verbatim") and not gate dimensions. NO_COLOR frame renders glyph-backed: monochrome `3 selected`, `●` markers retained, no violet hue, no canvas, native bg, no crash.

TESTS:
- Status: Adequate
- Coverage:
  - internal/capture/capture_test.go:733-809 TestSessionsMultiSelectActiveFixture — asserts the reused 12-session sessions-flat set (names/windows/attached, ordered), the seed-seam wiring through Deps (3 marked names + InitialCursor == fab-flowx-explore), and builds the production model asserting PageSessions + "Sessions" (Flat) title + MultiSelectActive()==true + SelectedSessionCount()==3 + IsSessionSelected() for each marked name.
  - internal/capture/capture_test.go:814-824 TestFixtureNamesIncludesMultiSelectActive — pins registration in the discoverable name list.
  - cmd/capturetool/import_guard_test.go — structural (portal binary excludes internal/capture); unchanged and still valid; carries no fixture-count assertion, so no update was required. No total-count assertion test exists anywhere, so nothing else needed updating.
- Notes: The render assertions (violet banner / `●` / footer styling) are correctly delegated to the visual gate and the tui-package render tests (tasks 5-1..5-7) rather than duplicated here — the test's own doc comment states this scope. The NO_COLOR render logic is covered by tui-package colourless tests; this task's NO_COLOR artifact is the tape+PNG verified at the visual gate. Not under-tested for this task's scope; not over-tested (no redundant assertions).

CODE QUALITY:
- Project conventions: Followed. The fixture reuses sessionsFlatFixture() verbatim (only name + seed fields differ) — the established pattern shared by the unsupported-terminal / preflight-abort / burst-opening fixtures. Seed seams mirror the documented InitialFlash pattern. Tapes mirror the sessions-flat / sessions-flat-nocolor pair. Reference-first convention honoured (design frame moved to testdata/vhs/reference/*-mv.png).
- SOLID principles: Good. `WithInitialMultiSelect` is a single-purpose option mirroring the live toggle's enter step; `WithInitialCursor` cleanly separates store (option) from apply (post-ingest), avoiding the SetItems reset trap.
- Complexity: Low. Fixture is a 10-line clone-and-override; options are small and guarded.
- Modern idioms: Yes (range-over-slice seeding, map-with-capacity).
- Readability: Good. Fixture and option doc comments precisely explain the marker-over-selector precedence and the capture-only nature.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/capture/capture_test.go:791-808 — TestSessionsMultiSelectActiveFixture asserts Deps().InitialCursor == "fab-flowx-explore" and builds the model, but never drives the load loop (WindowSizeMsg/SessionsMsg → evaluateDefaultPage) to assert the cursor actually resolves onto fab-flowx-explore's row index. The anchor's effect (applyInitialCursor → reanchorSessionCursor) is currently verified only by the manual visual gate. Concrete addition: extend the test (or add a sibling) that runs the Init/Update loop and asserts the selected session name is fab-flowx-explore, closing the seam→behaviour gap in an automated test.
