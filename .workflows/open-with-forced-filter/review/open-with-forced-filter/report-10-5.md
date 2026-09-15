TASK: open-with-forced-filter-10-5 (tick-d24700) — Build the search-results fixture's home paths from the running process's home

ACCEPTANCE CRITERIA:
1. `go run ./cmd/capturetool --fixture sessions-search-results` on a machine whose home is not `/home/user` renders `~/code/portal` beside `portal-a1b2`, `~/code/portal-gateway` beside `api-work`, `/opt/portal-tools` beside `portal-notes`, an empty slot beside `legacy-port-shim` and a `…/`-prefixed tail on the deepest path
2. No `HOME` pin for this fixture survives anywhere — `rg -n 'HOME=/home/user|searchResultsHome' testdata/vhs internal/capture` returns nothing
3. Each tape runs `capturetool` in one typed line, with no separate build step
4. The swap-and-diff entry for `sessions-search-results` asserts `~/code/portal` and `~/code/portal-gateway` alongside `/opt/portal-tools`, and the completeness assertion still covers every build-backed fixture
5. The rendered text is identical to today's pinned frame, so the committed PNGs stay accurate and are not re-captured
6. No other fixture's session directories or project paths change

STATUS: complete

SPEC CONTEXT: The specification fixes the rendered and searched form of a session's recorded directory as the home-abbreviated one — §4.1 ("the searched form is the displayed form": a directory under the user's home is searched as `~/Code/portal`, never as tmux recorded it) and §6.2 ("a path under the user's home is always displayed abbreviated to `~/`, at any width — the abbreviation is how the value is rendered rather than a rung of the truncation ladder"). `internal/tui/session_dir_column.go:28` implements that through `resolver.AbbreviateHome`, which folds only paths under the *process's* home (`internal/resolver/path.go:94-106`). The fixture therefore has to be built under the rendering process's home or the one frame the feature's directory column is signed off on shows no abbreviation at all. The search-narrowing side is unaffected by the change: `resolver.SearchFields` (`internal/resolver/search.go:11-13`) matches against the *abbreviated* directory, so no text from the running machine's home can leak into the containment set — `evvi-sync-engine` stays out of the frame under any home.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/capture/fixtures.go:451-465` (the `fixtureHome` helper), `internal/capture/fixtures.go:478-509` (the fixture built from it), `internal/capture/capture_test.go:966-976` + `:1075-1081` (pin removed, arbitrary-home subtest added), `internal/capture/swap_harness_test.go:65` (entry extended), `testdata/vhs/sessions-search-results.tape:22` and `testdata/vhs/sessions-search-results-nocolor.tape:23` (single typed line each).
- Notes:
  - `fixtureHome()` degrades on exactly `err != nil || home == ""`, which is `AbbreviateHome`'s own give-up condition (`internal/resolver/path.go:96`), so fixture and renderer cannot disagree: either both fold (`~/code/portal`) or neither does (`/home/user/code/portal` whole). The doc comment's claim to that effect holds against the code.
  - The builders are invoked per lookup (`fixtureBuilders()` → `FixtureByName`, `internal/capture/fixtures.go:140-182`), not cached at package init, so a `t.Setenv("HOME", …)` taken before the lookup reaches the fixture's own paths as well as the renderer's. That is what makes the new subtest a real probe rather than a tautology.
  - Index pairing survives: both `Session.Dir` and `Project.Path` are the same Go string, and `project.CanonicalDirKey` (`internal/project/pathkey.go:10-27`) reduces both through the same ladder with a stable fallback for a path that does not exist — so grouping and tags pair under any home, existent or not.
  - The three dir-bearing project paths and the four home-relative session dirs were converted; `/opt/portal-tools` stays a literal and `legacy-port-shim` stays `Dir`-less, so the frame keeps all five branches of the column.
  - Criterion 6 verified against the commit diff (`3403de3bf`): the 40-odd `/home/user` literals in the other fixtures are untouched, and they render nothing — the directory column is gated on `ShowDir: m.searchForm` (`internal/tui/model.go:938`), which only the search-opened fixture sets.
  - Criterion 5 verified against the committed frame: `testdata/vhs/sessions-search-results.png` carries `portal-a1b2 ~/code/portal`, `api-work ~/code/portal-gateway`, an empty slot on `legacy-port-shim`, `…/testdata/reference/design-exports/frames` and `portal-notes /opt/portal-tools` — the exact strings the unpinned path now produces. Neither PNG was re-captured in the commit, correctly.
  - Retention rules respected: both tapes and both PNGs are scaffolding under `testdata/vhs/README.md`'s table and are still present because the feature has not signed off; the permanent Go fixture is untouched as a registry member, so the swap-and-diff coverage list does not shrink.

TESTS:
- Status: Adequate
- Coverage: The rendered property is now asserted where it is produced. `internal/capture/capture_test.go:1075-1081` reads the frame under `t.Setenv("HOME", t.TempDir())` and still finds `portal-a1b2 ~/code/portal` — the property the deleted pin used to manufacture. The five branch cases (`:1083`, `:1090`, `:1097`, `:1104`, `:1111`) now run under the ambient home with no pin, and `:1118` covers the column in both grouped modes. `internal/capture/swap_harness_test.go:65` judges `~/code/portal` and `~/code/portal-gateway` alongside `/opt/portal-tools`, and the completeness subtest (`:88-97`) still compares the table against every build-backed fixture name, so the entry cannot be narrowed without failing.
- Notes:
  - `:1075` and `:1083` assert the identical string and differ only in the home they run under (arbitrary vs ambient). That is the plan's prescription and the two do probe different environments, so it is not redundancy worth removing.
  - `fixtureHome`'s degrade branch (`os.UserHomeDir` erroring) is not exercised. It is a two-line fallback in a harness-only fixture whose failure mode is a visual frame showing absolute paths; nothing depends on it, so the gap is not worth closing.

CODE QUALITY:
- Project conventions: Followed. `internal/capture` remains test/harness-only and out of the production binary (the `cmd/capturetool/import_guard_test.go` boundary is untouched); no new package edge is introduced — `os` and `path/filepath` are stdlib.
- SOLID principles: Good. One unexported helper owning one decision, consumed by one fixture.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The three derived dirs are named (`portalDir`, `gatewayDir`, `evviDir`) and the deep path composes off `portalDir`, so the "same checkout, deeper" relation is stated by the code rather than by a repeated literal.
- Issues: None. The comment changes are subtractive and each removed passage described machinery that no longer exists (the `$HOME`/build-cache split in both tapes, the swap-harness "asserted where HOME is pinned" note, the `searchResultsHome` rationale); the one added comment holds against the code it describes.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go run ./cmd/capturetool --fixture sessions-search-results` on a machine whose home is not `/home/user` renders `~/code/portal` beside `portal-a1b2`, `~/code/portal-gateway` beside `api-work`, `/opt/portal-tools` beside `portal-notes`, an empty slot beside `legacy-port-shim` and a `…/`-prefixed tail on the deepest path" — settled at the model layer by reading (the in-process subtests assert every one of those forms at the 120x40 harness width under an arbitrary home, and the rendered text is home-independent by construction), but the criterion names the live route at the tape's 105x27 geometry. Running `go run ./cmd/capturetool --fixture sessions-search-results --theme tokyo-night` on this machine and eyeballing the five column branches — in particular that `~/code/portal-gateway` still fits whole and the deepest path still reaches the `…/` rung at 105 columns — is what would settle it. The committed PNG at that geometry shows exactly those five forms, so the expected outcome is on record.
