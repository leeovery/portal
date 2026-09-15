TASK: Capture the search-opened list as a fixture and check the rendered frame (open-with-forced-filter-3-4 / tick-400ee8)

ACCEPTANCE CRITERIA:
1. `sessions-search-results` is enumerated by `capture.FixtureNames()`, resolves through `FixtureByName`, and is covered by the swap-and-diff guard and every registry assertion with no exemption anywhere
2. The fixture declares no render size, so `TestFixtureRenderSize_DeclaredOnlyByTheGeometryFixtures` passes over it unchanged
3. The model it builds lands on the Sessions page with the term committed, and its visible rows are the containment set — the non-matching session is absent from the frame
4. With `HOME=/home/user` the rendered frame carries, in one screen: a `~/code/…` abbreviated path, a row whose name does not contain the term while its directory does, a row showing a name with an empty directory slot, an unabbreviated `/opt/…` path, and a left-truncated tail
5. The directory on every row begins exactly one column after its session name, and the count and attached slots stay aligned down the frame
6. `s` reaches By Project and By Tag live on the same fixture, with the column present in both — no second fixture is added for them
7. A `NO_COLOR` capture of the same fixture shows the directory with no bracket, glyph or separator standing in for the weight
8. The tape(s) and PNG(s) are committed with this task; `testdata/vhs/reference/*.png` is untouched
9. The human visual check is recorded before sign-off, with the still capture and the live-view command presented
10. `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
§6.1–6.3 of `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`. Every row in a sigil-opened list carries its recorded directory beside the name, in every grouping mode, for as long as that picker is open; a session with no recorded directory shows an empty slot; the directory begins one space after the name, takes the window count's muted token (one rung brighter on the selected row), is shortened from the LEFT so its tail survives, is dropped entirely below a floor, and is always home-abbreviated to `~/`. Where colour is off the row is unchanged — no bracket, glyph or separator stands in for the weight. The harness context (CLAUDE.md, "Visual capture harness"): fixtures are permanent because the swap-and-diff completeness guard enumerates whatever exists, while `testdata/vhs/*.tape` and `*.png` are scaffolding cleared at the feature's sign-off, with `testdata/vhs/reference/*.png` the kept carve-out.

IMPLEMENTATION:
- Status: Implemented (with one deliberate, sound divergence from the task text — see Notes)
- Location:
  - `internal/capture/fixtures.go:38` — the `search *tui.SearchForm` field on `Fixture`
  - `internal/capture/fixtures.go:94` — `Search: f.search` passed through `Deps`, nil for every other fixture (only `sessionsSearchResultsFixture` sets it)
  - `internal/capture/fixtures.go:157` — registration in `fixtureBuilders()`
  - `internal/capture/fixtures.go:467-506` — `sessionsSearchResultsFixture()`, its explanatory comment, and the six seeded sessions + four projects
  - `internal/capture/fixtures.go:459-465` — `fixtureHome()`, the running process's home with a `/home/user` fallback
  - `internal/capture/swap_harness_test.go:65` — the fixture's row in the `capturedStates()` coverage table
  - `testdata/vhs/sessions-search-results.tape`, `testdata/vhs/sessions-search-results.png`, `testdata/vhs/sessions-search-results-nocolor.tape`, `testdata/vhs/sessions-search-results-nocolor.png`
- Notes:
  - Criterion 1 holds with no exemption anywhere. The fixture is registered in the single registry both lookups derive from; it sets no `noColor`, so `excludeColourless` (`internal/capture/theme_swap_guard_test.go:93`) keeps it under the swap-and-diff guard; `TestThemeSwapGuard_EnumeratesRegistry` requires the guarded set to equal `FixtureNames()` minus the contrast swatch; `TestModelAt_ReachesCapturedState`'s first subtest requires the captured-state table to cover every build-backed fixture, and the table carries it; `cmd/capturetool/theme_persister_test.go:14` enumerates it too and it wires neither persister.
  - Criterion 2 holds structurally: the struct literal declares no `width`/`height`, so `TestFixtureRenderSize_DeclaredOnlyByTheGeometryFixtures` (`internal/capture/fixture_render_size_test.go:72`) — which skips only the two named geometry fixtures and the swatch — ranges over it unchanged.
  - The fixture exercises the production gate rather than a capture-only flag: `Deps.Search` non-nil → `WithSearchForm` (`internal/tui/build.go:132`) → `m.searchForm` → `ShowDir: m.searchForm` (`internal/tui/model.go:938`). `Search.Decide` is nil and the seam is documented nil-tolerant (`internal/tui/search_decision.go:8`), so no decision is left pending in a one-shot render.
  - Divergence from the task's "Do" text, introduced by the later task 10-5 (`3403de3bf`) and sound: the fixture's home paths are built from `os.UserHomeDir()` rather than hardcoded `/home/user`, and the tapes correspondingly run `go run ./cmd/capturetool …` with no `HOME` pin instead of the build-then-run-under-pinned-HOME pair. This serves the determinism intent better rather than worse — the home half of every path is abbreviated away before it reaches the frame, so the rendered screen is identical on any machine, and the criterion-4 shapes (`~/code/…`, `/opt/portal-tools`, the empty slot, the truncated tail) are all home-length-independent. Nothing the intent needs is lost.
  - Both PNGs read correctly against §6.2: `portal-a1b2 ~/code/portal` (abbreviated), `api-work ~/code/portal-gateway` (matched on its directory alone), `legacy-port-shim` with an empty slot, `portal-design-exports-review …/testdata/reference/design-exports/frames` (left-truncated, tail surviving), `portal-notes /opt/portal-tools` (unabbreviated), `evvi-sync-engine` absent. Directory one space after the name, in the count's muted rung, one step brighter on the selected row; count and attached slots right-aligned down the frame. The NO_COLOR frame shows the same single space with no bracket, glyph or separator.
  - The PNGs were rendered under the pre-10-5 tape and have not been re-rendered since the tapes changed, but they are not stale as artifacts: the captures are full-screen TUI frames with no shell line visible, and both tape forms produce the same frame content (see the divergence note above).
  - `testdata/vhs/reference/` is untouched by this feature (last touched by `theming-system` commits), and `testdata/vhs/README.md`'s retention table is generic (`<fixture>.tape` / `<fixture>.png`), so it needed no edit.
  - `gofmt -l internal/capture/` reports nothing; no `t.Parallel()` anywhere in the package.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/capture/capture_test.go:1026` `TestFixtureNamesIncludesSessionsSearchResults` — enumeration plus `FixtureByName` resolution and name identity.
  - `internal/capture/capture_test.go:1039` `TestSessionsSearchResultsFixture` — the search-opened landing (`Deps().Search.Term == "port"`, `ActivePage() == PageSessions`, Flat title), narrowing to the containment set with `evvi-sync-engine` absent, the abbreviated path under an arbitrary `$HOME` and under the developer's, the directory-only match, the empty slot (the `legacy-port-shim` row carries no `/` at all), the unabbreviated `/opt/portal-tools`, the `…/` left-truncated tail, and the column surviving `s` into By Project and By Tag.
  - The By-Project/By-Tag subtest is stronger than it looks: it asserts `SessionListTitle()` changes on each `s`, which can only happen if the search term landed as a *committed* filter — under `list.Filtering` the key would be a literal filter character. That is what pins "the term committed" (criterion 3) beyond the frame text.
  - `searchResultsRow` narrows an assertion to the one frame line carrying a name, so a row-level claim cannot be satisfied by another row's text; `settleCommands` drives returned commands back through `Update` so an async regroup is settled before the rows are read.
  - The column's own behaviour (one-space placement, trailing-slot alignment, left truncation never touching the name, the drop floor, the column-off row) is covered separately and thoroughly by `internal/tui/session_row_anatomy_test.go:308-451`, so this task's fixture tests are correctly scoped to the fixture rather than restating that.
- Notes:
  - Mild redundancy: the subtests at `internal/capture/capture_test.go:1075` and `:1083` assert the identical substring `"portal-a1b2 ~/code/portal"`, differing only in whether `HOME` is re-pointed first. Since the fixture derives its paths from whatever home is set, the arbitrary-home subtest strictly subsumes the other — there is no break one catches and the other misses. Harmless duplication, not a defect; flagged for the record only.
  - Two `present` entries assert `~/code/portal`, which is a prefix of `~/code/portal-gateway` and so could be satisfied by the gateway row (`swap_harness_test.go:65`, and the By-Project/By-Tag subtest). Both places also assert something the gateway row cannot satisfy, so nothing is left unobserved.

CODE QUALITY:
- Project conventions: Followed. Fixture registered in the single registry both lookups derive from; no render size; no `noColor` flag (which would have excluded it from the swap guard); persisters left nil; the `.tape` files follow the existing `Output …/.gifcache/*.gif` + `Set FontFamily/FontSize/Width/Height/Shell` shape, carry the SCAFFOLDING-NOT-AN-ASSET header, record the resulting 105×27 geometry, and — unlike the older tapes — carry no task-id reference in their comments.
- SOLID principles: Good. The fixture declares data only; the model is assembled by the production `tui.Build` path.
- Complexity: Low.
- Modern idioms: Yes (`filepath.Join` composition, `slices.Contains`, `strings.SplitSeq` in the row helper).
- Readability: Good. The fixture's doc comment enumerates exactly which branch each seeded session covers and justifies the one session that breaks the file's stamped-Dir convention; that convention claim checks out — `legacy-port-shim` (`fixtures.go:487`) is the only `tmux.Session` in the file without a `Dir`.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — settled only by running both lanes. Reading finds nothing that should fail, but the deep path's `…/` truncation at the 120-column harness width and the arbitrary-`$HOME` abbreviation subtest are measurements, not readings.
- "The human visual check is recorded before sign-off, with the still capture and the live-view command presented" — nothing in the repository records a gate; the committed PNGs are its only artifact. Settled by the run's own gate record. The live-view command for that check is `go run ./cmd/capturetool --fixture sessions-search-results` (add `--theme tokyo-night` to match the committed capture).
