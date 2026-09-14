# Consolidation Tasks: Open With Forced Filter (Phase 3)

## Task 1: Make the s-regroup assertions read the mode they claim to reach
placement: phase 3
severity: drift

**Problem**: A search-opened picker is the one picker that replaces the sessions list's filter func and lands with the filter committed rather than focused, so "the `s` key still regroups from there" is a property this feature introduced rather than one the pre-existing grouping already carried. Two subtests were written to pin it — one in `internal/tui/search_dir_column_test.go:130` ("it keeps the column after an s regroup") and one in `internal/capture/capture_test.go:1106` ("it renders the column in By Project and By Tag") — and neither can observe whether the press did anything. Both assert only that a directory string is still on screen afterwards, which a list that never regrouped satisfies identically: the Flat frame carries the same text, and under a committed filter the section header is the filter query rather than the mode title, so the rendered frame carries no mode indicator to discriminate on. A later change that swallowed `s` while a filter is applied — the same shape as the existing guard one state over — would leave both tests green while the grouped-mode arm of this feature goes untested, reaching the user as a search-opened picker whose `s` key silently does nothing.

**Solution**: Have each site read the mode it claims to have reached, before asserting the column. In `internal/tui/search_dir_column_test.go`, assert the model's session-list mode advances Flat → By Project → By Tag → Flat across the loop's three presses. In `internal/capture/capture_test.go` the frame alone cannot carry the signal, so have the frame helper return the settled model beside the frame (or add a sibling that does) and assert the list title is the by-project and by-tag form respectively before the substring check. Test-side only; no production change. The regroup works today — verified against the dispatch, which gates on the focused-filter state rather than the committed one — so this closes a coverage hole rather than fixing a defect.

**Outcome**: Both subtests fail if `s` stops reaching the grouping switch from a committed filter, which is the regression neither can currently see.

**Do**:
- In `internal/tui/search_dir_column_test.go`, rewrite the `for range 3` loop in "it keeps the column after an s regroup" to walk `[]prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag, prefs.ModeFlat}`: press `s`, assert `m.sessionListMode` equals that press's expected mode (failure naming the press index and the mode read), then keep the existing `assertDirRendered(t, m, dir)` — the three presses then pin the full cycle including the wrap back to Flat.
- In `internal/capture/capture_test.go`, add `searchResultsFrameModel(t *testing.T, keys ...tea.KeyPressMsg) (string, tui.Model)` holding the whole of the current `searchResultsFrame` body (the `HOME` setenv, `FixtureByName`, `ModelAt`, the per-key `Update`+`settleCommands` drive, `ansi.Strip(...View().Content)`) and returning the settled model beside the frame; reduce `searchResultsFrame` to a delegation that discards the model, so the drive loop exists once and the seven other call sites stay byte-unchanged.
- In "it renders the column in By Project and By Tag", take frame and model from `searchResultsFrameModel` and assert `SessionListTitle()` is `"Sessions — by project"` after one `s` and `"Sessions — by tag"` after two, each before its existing `strings.Contains(frame, "~/code/portal")` check.
- Verify both strengthened subtests can see the regression: temporarily widen the sessions-page key guard at `internal/tui/model.go:2424` so it also breaks while the filter is merely applied, confirm each subtest fails, then revert the widening — it is a check, not a change, and must not reach the commit.
- Run `go test ./internal/tui ./internal/capture` and then `go test ./...`; no production file is edited by this task.

**Acceptance Criteria**:
- [ ] "it keeps the column after an s regroup" asserts `m.sessionListMode` is `ModeByProject`, `ModeByTag`, `ModeFlat` after presses one, two and three respectively, and still asserts the directory renders after each.
- [ ] "it renders the column in By Project and By Tag" asserts the list title is `"Sessions — by project"` and `"Sessions — by tag"` before its respective substring checks.
- [ ] `searchResultsFrame` keeps its `(t, keys ...tea.KeyPressMsg) string` signature and delegates to the new sibling; the fixture drive loop is declared exactly once, and the seven call sites other than the By Project / By Tag subtest are unedited.
- [ ] With the sessions-page key guard temporarily widened to swallow `s` under an applied filter, both subtests fail; with it reverted, both pass.
- [ ] The task's diff touches only `internal/tui/search_dir_column_test.go` and `internal/capture/capture_test.go`; `go test ./...` is green.

**Tests**:
- `"it keeps the column after an s regroup"` (`internal/tui/search_dir_column_test.go`) — same name and same fixture, now reading the mode each press reached before asserting the column; the third press covers the wrap back to Flat
- `"it renders the column in By Project and By Tag"` (`internal/capture/capture_test.go`) — same name, now reading the settled model's list title for each grouped mode before the frame substring check
- No new test file and no new subtest: both sites are strengthened in place, and every other subtest in the two files is left as it stands
