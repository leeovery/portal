TASK: lazy-resume-on-attach-6-8 — The Row on Demand for the Visual Check (pending-resume capture fixtures, coloured + colourless)

ACCEPTANCE CRITERIA:
- Both fixture names are listed by `capture.FixtureNames()` and resolve through `FixtureByName`.
- The coloured fixture's frame carries all four combinations — neither, attached alone, pending alone, both — in one render, so the packing is seen rather than asserted.
- The rendered rows are produced by the production `SessionDelegate` through `tui.Build`: no layout is restated in the fixture.
- The colourless sibling renders `A`, `P` and `AP` and reports `Colourless() == true`, matching its `Deps().NoColor`.
- The palette-swap completeness guard enumerates the coloured fixture and passes over it; the colourless one is excluded by the existing `Colourless()` flag and not by a name list.
- The pending state reaches the rows through the production seam — the fixture's reader is wired as `Deps.PendingReader` and nothing seeds the delegate directly.
- No fixture string names a tool, and the fixtures declare state rather than text.
- `TestPortalBinaryDoesNotImportCapture` still passes — nothing added here reaches the production binary.
- Every existing fixture is unchanged: a fixture declaring no pending sessions wires a nil reader and renders exactly as before.

STATUS: complete

SPEC CONTEXT: The picker's session row drops the word `attached` and gains a second indicator: a green attached dot and an `accent.attention` pending dot, packed hard right in fixed order (attached, then pending), no reserved lanes. A row carries the pending dot when any pane in the session is waiting. Under NO_COLOR each indicator becomes a letter in the same cell — `A`, `P`, or `AP` — with no second row geometry. Every string the feature's surfaces render is tool-agnostic. The design reference is the committed Nord frame `testdata/vhs/reference/sessions-pending-resume-dot-nord.png`; these fixtures exist so the real row can be viewed live against it (the gate itself is not this task's criterion).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/capture/fakes.go:54-66 — `fakePendingReader` beside `fakeLister`, building a `tmux.PendingResumeView` (non-nil `Sessions`, `Panes` counted) from declared session names
  - internal/capture/fixtures.go:31-32 — `pendingSessions []string` field on `Fixture`
  - internal/capture/fixtures.go:83 — wired into `tui.Deps` as `PendingReader`
  - internal/capture/fixtures.go:128-134 — `pendingReader()` returns nil when none declared (Build wires any non-nil reader, internal/tui/build.go:138-140)
  - internal/capture/fixtures.go:169-170 — both builders registered in `fixtureBuilders()`, so `FixtureNames()`/`FixtureByName` derive them
  - internal/capture/fixtures.go:524-549 — `sessionsPendingResumeFixture` (flat; `fabric-lk26UG` neither, `agentic-workflows-codify` attached only, `evvi-sync-engine` pending only, `folio-Jiz4el` both; every session has a stamped `Dir`) and `sessionsPendingResumeColourlessFixture` derived from it with its own name and `noColor: true`
  - internal/capture/harness.go:34-40, 57-68 — `ModelAt` now reads the fixture's `Deps.PendingReader` and carries the set on `tui.SessionsMsg.Pending`, the same message production's `fetchSessionsCmd` (internal/tui/model.go:1491-1496) emits via `readSessionList`; the live capturetool route instead reaches the reader through `Build` → `WithPendingResumeReader` → Init's fetch
  - cmd/capturetool/main.go:196 — `deps.NoColor = deps.NoColor || noColourRequested()`, so the fixture's own NO_COLOR declaration is no longer overwritten by the environment check on the live route (necessary: this is the first registry fixture declaring `noColor`)
- Notes: Fixtures declare session names, attached flags and pending names only; the indicator glyph/letter comes from `SessionDelegate.indicatorCluster`/`indicator` (internal/tui/session_item.go:341-358). No row layout exists in `internal/capture`. Fixtures with no `pendingSessions` get a nil reader, and `pendingSessionsOf(nil)` returns nil, so their `SessionsMsg` is identical to what was sent before (the `Pending` field was absent/nil). Only `cmd/capturetool/main.go` imports `internal/capture` among non-test sources, so the production binary gains no edge. Under NO_COLOR, `fillPaneCanvas` (internal/tui/canvas_fill.go:20-23) still pads to w×h, so the render-size suite's caller-size assertion holds for the colourless fixture without exemption.

TESTS:
- Status: Adequate
- Coverage:
  - internal/capture/pending_resume_fixture_test.go:114-128 — both names enumerated, not standalone, resolve by name
  - :130-167 — coloured frame: per-row trailing glyphs (none / ● / ● / ●●), the SGR painting each dot (state.positive vs accent.attention, attached before pending), and lone attached/pending dots sharing the rightmost column
  - :169-201 — the fixture frame equals `tui.Build` over its own Deps; withdrawing the reader leaves non-pending rows unchanged and strips the dot from pending rows (the both-row keeps only the attached dot); no flash/command/multi-select seeded beside the state
  - :203-224 — colourless frame renders "", A, P, AP and no ●
  - :226-236 — colourless fixture reports Colourless() and Deps().NoColor true; coloured one does not
  - :238-256 — seeded session names name no tool; every session carries a Dir
  - :258-269 — every other build-backed fixture wires a nil reader
  - internal/capture/swap_harness_test.go:65-66 — both fixtures enrolled in `capturedStates`, reaching the Sessions page
  - cmd/capturetool/main_test.go:425-454 — the colourless fixture renders colourless on the live resolveModel route without NO_COLOR, and its coloured sibling still paints the pinned canvas
  - Existing enumerating guards (theme_swap_guard_test.go `registryFixtures`/`guardedFixtures`, fixture_colourless_test.go, fixture_render_size_test.go, fixture_registry_test.go) take both fixtures as ordinary registry members with no name-based exemption
- Notes: Tests would fail if the dots stopped coming through the reader, if the packing order or colours changed, or if the colourless letters regressed. Proportionate — no redundant variants.

CODE QUALITY:
- Project conventions: Followed — fixtures declare state not text, registry-derived lookups, stamped Dir convention, no tool names, `internal/capture` stays test-harness-only
- SOLID principles: Good
- Complexity: Low
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The palette-swap completeness guard enumerates the coloured fixture and passes over it" — enumeration and the Colourless()-based exclusion are settled by reading (internal/capture/theme_swap_guard_test.go:93-121); that the guard passes over `sessions-pending-resume` (TestThemeSwapGuard_NoStaleValueSurvives, TestThemeSwapGuard_EveryBValuePresentInUnion, TestThemeSwapGuard_RenderIsTruecolor and the TestTokenCoverage_* exclusivity rows) needs `go test ./internal/capture -run 'ThemeSwapGuard|TokenCoverage'` to be run.
