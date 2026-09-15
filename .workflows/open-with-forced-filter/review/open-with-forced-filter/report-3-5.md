TASK: Make the s-regroup assertions read the mode they claim to reach (tick-28aca8 / open-with-forced-filter-3-5)

ACCEPTANCE CRITERIA:
1. "it keeps the column after an s regroup" asserts `m.sessionListMode` is `ModeByProject`, `ModeByTag`, `ModeFlat` after presses one, two and three respectively, and still asserts the directory renders after each.
2. "it renders the column in By Project and By Tag" asserts the list title is `"Sessions — by project"` and `"Sessions — by tag"` before its respective substring checks.
3. `searchResultsFrame` keeps its `(t, keys ...tea.KeyPressMsg) string` signature and delegates to the new sibling; the fixture drive loop is declared exactly once, and the seven call sites other than the By Project / By Tag subtest are unedited.
4. With the sessions-page key guard temporarily widened to swallow `s` under an applied filter, both subtests fail; with it reverted, both pass.
5. The task's diff touches only `internal/tui/search_dir_column_test.go` and `internal/capture/capture_test.go`; `go test ./...` is green.

STATUS: complete

SPEC CONTEXT: The specification names the `s` regroup explicitly as one of the re-renders a sigil-opened picker's containment set must survive ("a `Space` preview and back, an `s` regroup, a refresh after a session is killed elsewhere all re-render it — and every one of those reproduces the containment set", specification.md:176), and the directory column is scoped to a search-opened picker across every grouping mode that list can be in (specification.md:276). The spec also independently states the fact this task's problem statement turns on: under a committed filter "the grouping survives a committed filter but the headings do not, because a header row's filter value is empty" (specification.md:250) — so a grouped, filtered frame carries neither headings nor a mode title, and a frame-text assertion cannot discriminate grouped from Flat. The task therefore closes a coverage hole rather than fixing a defect, exactly as its description claims.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/search_dir_column_test.go:134-140` — the `for range 3` loop is now `for i, want := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag, prefs.ModeFlat}`, with `m.sessionListMode != want` checked (Fatalf naming the 1-based press index and both modes) before the retained `assertDirRendered(t, m, dir)`.
  - `internal/capture/capture_test.go:965-969` — `searchResultsFrame(t *testing.T, keys ...tea.KeyPressMsg) string` retained verbatim as a `t.Helper()` delegation discarding the model.
  - `internal/capture/capture_test.go:971-989` — new `searchResultsFrameModel(t *testing.T, keys ...tea.KeyPressMsg) (string, tui.Model)` holds the whole drive body and returns the settled model beside the stripped frame.
  - `internal/capture/capture_test.go:1118-1134` — the By Project / By Tag subtest takes frame+model from the new sibling and asserts `SessionListTitle()` before each `strings.Contains` check.
  - Commit `b50e80786`, `2 files changed, 23 insertions(+), 4 deletions(-)` — both files are `_test.go`; no production file touched.
- Notes:
  - Criterion 3's drive-loop uniqueness holds at HEAD: `for _, key := range keys` appears exactly once in `internal/capture/capture_test.go` (line 983). The other two `fx.ModelAt(` sites (lines 948, 1054) build a model with no keystrokes and pre-date this task; neither restates the drive.
  - The seven other `searchResultsFrame(t)` call sites at lines 1064, 1077, 1084, 1091, 1098, 1105, 1112 are unedited by the commit — counted from the current file and confirmed against the commit diff, which touches no line in any of them.
  - `searchResultsHome` / the `t.Setenv("HOME", …)` that were in the helper body at the time of this commit are gone at HEAD. That is a later task's change (Topen-with-forced-filter-10-5, "Build the search-results fixture's home paths from the running process's home"), not drift from this one, and the const is not left dangling (no reference remains anywhere under `internal/`).
  - The claim in the new helper's doc comment — that the frame "under a committed filter carries no mode indicator" — is true against the code: `applySectionHeader` (`internal/tui/model.go:3578-3585`) replaces the title row with `renderFilterQueryHeader` whenever `FilterState() == list.FilterApplied`, and `containmentFilter` (`internal/tui/search_filter.go:63-79`) ranks only targets present in `searchItemSource`, which `set` populates from `SessionItem` rows alone — a `HeaderItem`'s empty filter value contributes no entry, so group headings never survive into the frame either.

TESTS:
- Status: Adequate
- Coverage: Both named subtests now read a value that only a real regroup can produce. `internal/tui`'s site reads `m.sessionListMode` directly across all three presses, so it pins the full cycle including the wrap back to Flat; `internal/capture`'s site reads `SessionListTitle()`, which is `m.sessionList.Title` (`internal/tui/model.go:386-388`) and is written only by `rebuildSessionList` — a strictly stronger signal than the mode field, since it observes that the rebuild ran rather than only that the field moved.
- Notes:
  - The regression the task names is genuinely observable now. The sessions-page guard is `if m.sessionList.SettingFilter() { break }` (`internal/tui/model.go:2547`), sited before the rune switch that reaches `handleSwitchViewKey` (`internal/tui/model.go:2638-2648`). Widening it to also break under `list.FilterApplied` would skip the handler entirely: `sessionListMode` would stay `ModeFlat` and the title would stay `"Sessions"`, failing both new assertions. Previously both subtests asserted only that a directory string was on screen, which the Flat frame satisfies identically.
  - Both fixtures really are in the `FilterApplied` state the regression needs: the capture fixture opens through `search: &tui.SearchForm{Term: "port"}` (`internal/capture/fixtures.go:505`) and the tui fixture through `searchColumnModel(…, "portal", …)` (`internal/tui/search_dir_column_test.go:32-35`); `internal/tui/coldboot_session_refetch_test.go:296` independently pins that landing as `list.FilterApplied`. So the assertions are not vacuously green on a picker that never committed its filter.
  - Not over-tested: no new test file, no new subtest, and the pre-existing sibling assertion that the fixture opens Flat (`internal/capture/capture_test.go:1058-1060`, `SessionListTitle() == "Sessions"`) now reads as the baseline of the same cycle rather than a duplicate.
  - Reading `m.sessionListMode` is an unexported-field read, which the community `golang-testing` skill discourages in the abstract. It is the package's own established convention for this exact property (`internal/tui/switch_view_key_test.go:75,86,103,146,159,171`; `initial_mode_option_test.go:19,34`; `rebuild_session_list_test.go:190`), the test is in-package, and the plan prescribed the field by name — so it is consistent rather than a violation, and the capture-side title read covers the rebuild the field alone cannot.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (forbidden by CLAUDE.md). `t.Helper()` on both the wrapper and the new sibling. Test-only change, both files in the unit lane, no build-tag question raised. `prefs.SessionListMode` carries a `String()` (`internal/prefs/store.go:34-43`), so the `%v` verbs in the failure message render `by-project`/`by-tag`/`flat` rather than bare ints.
- SOLID principles: Good — the extraction is a single-responsibility split (drive once, project two different views of the result), and the wrapper keeps every existing caller on the narrowest return it needs.
- Complexity: Low. One extraction, one loop rewrite, four new comparisons.
- Modern idioms: Yes. `for i, want := range []prefs.SessionListMode{…}` replaces `for range 3`; the `if got, want := …; got != want` comparison form matches the surrounding file.
- Readability: Good. Every new failure message names the concrete regression ("s no longer regroups from a committed filter", "the press did not regroup") rather than only reporting a mismatch.
- Issues: None. The one comment added (`internal/capture/capture_test.go:971-973`) states why the model is returned and is accurate against the code; it references no task id, phase or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "With the sessions-page key guard temporarily widened to swallow `s` under an applied filter, both subtests fail; with it reverted, both pass." — settling this requires running the experiment: widen `internal/tui/model.go:2547` to `if m.sessionList.SettingFilter() || m.sessionList.FilterState() == list.FilterApplied { break }`, run `go test ./internal/tui -run TestSearchDirColumn` and `go test ./internal/capture -run TestSessionsSearchResultsFixture` and confirm both fail, then revert and confirm both pass. Reading the dispatch supports the failing half firmly (the widened guard `break`s before the rune switch, so `handleSwitchViewKey` never runs and neither `sessionListMode` nor `sessionList.Title` moves); the passing half is a plain suite run.
- "`go test ./...` is green." — a suite run; nothing in the diff is reachable from production code, so the only packages at risk are `internal/tui` and `internal/capture`.
