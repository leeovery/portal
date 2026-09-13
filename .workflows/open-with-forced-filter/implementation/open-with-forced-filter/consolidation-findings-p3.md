# Consolidation Findings: open-with-forced-filter (Phase 3)

## Findings

### F1: both `s`-regroup assertions pass whether or not the list regrouped
- **Class**: behaviour
- **Failure**: A search-opened picker is the one picker that replaces the list's filter func and lands in `list.FilterApplied` (`internal/tui/search_filter.go:56-62`, `internal/tui/model.go:1293-1308`), so "`s` still regroups from there" is a property of this feature rather than of the pre-existing grouping. Two subtests were written to pin it — task 3-3's and task 3-4's — and neither can observe it. Both assert only that a directory string is still on screen after the press, which a list that never regrouped satisfies identically: the Flat frame carries the same directory text, and under a committed filter the header row is the filter query rather than the mode title (`internal/tui/model.go:3454-3462`), so the rendered frame carries no mode indicator for the capture site to discriminate on. A later change that swallowed `s` while a filter is applied — the same shape as the existing `SettingFilter()` guard at `internal/tui/model.go:2424`, one state over — leaves both tests green and the whole grouped-mode arm of the feature's acceptance untested. It reaches the user as a search-opened picker whose `s` key silently does nothing, noticed only by pressing `s` in a live picker or in `capturetool`.
- **Evidence**:
  - `internal/tui/search_dir_column_test.go:130-138` — "it keeps the column after an s regroup": three `s` presses, each followed by `assertDirRendered`, with no read of the mode
  - `internal/capture/capture_test.go:1106-1116` — "it renders the column in By Project and By Tag": `strings.Contains(frame, "~/code/portal")` after one and two `s` presses; the Flat frame of the same fixture contains that substring (`testdata/vhs/sessions-search-results.png`, row 1)
  - `internal/tui/model.go:2484` — `case isRuneKey(msg, "s"): return m.handleSwitchViewKey()`, the dispatch both tests exercise
  - `internal/tui/model.go:3454-3462` — the `FilterApplied` branch replaces the section header with the filter-query header, so the mode title is absent from a filtered frame
  - `internal/tui/model.go:1231` — `m.sessionList.Title = sessionListTitleForMode(...)`, the mode signal a test can read off the model
- **Proposed shape**: Make each site read the mode it claims to have reached, before asserting the column. In `internal/tui/search_dir_column_test.go:130`, assert `m.sessionListMode` advances `ModeFlat → ModeByProject → ModeByTag → ModeFlat` across the loop's three presses. In `internal/capture/capture_test.go:1106`, the frame alone cannot carry the signal, so have `searchResultsFrame` return the settled `tui.Model` beside the frame (or add a sibling that does) and assert `SessionListTitle()` is `"Sessions — by project"` and `"Sessions — by tag"` respectively before the substring check. Test-side only; no production change.

## Comment Corrections

- `internal/capture/fixtures.go:87` — the parenthetical states the registry-wide invariant that every fixture session carries a stamped `Dir`; task 3-4's `sessionsSearchResultsFixture` seeds `legacy-port-shim` with none (`internal/capture/fixtures.go:465`), deliberately and by the plan. What actually keeps the harness off a pane read is the nil seams themselves — `resolveDerivedDirs` returns before reading anything when either is nil (`internal/tui/model.go:1183-1184`) — so the surviving claim is worth keeping and its stated reason is not.
  OLD:
  ```
		// DirReader/DirRunner stay nil (sessions are pre-stamped, so the lazy
		// pane-read fallback never fires); ModePersister stays nil so an
		// `s`-toggle writes nowhere.
  ```
  NEW:
  ```
		// DirReader/DirRunner stay nil, so the lazy pane-read fallback returns
		// before reading anything and a fixture session carrying no recorded
		// directory groups into the catch-all; ModePersister stays nil so an
		// `s`-toggle writes nowhere.
  ```
