# Review Tracking: Open With Forced Filter - Integrity

## Findings

### 1. What `portal init fish` emits is left undecided behind a check that cannot be run

**Severity**: Important
**Plan Reference**: Phase 5, task 5-2 (`open-with-forced-filter-5-2`) — Do, the `emitFishInit` step
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Move**: settled
**Change Type**: update-task

**Problem**:
The fish half of the completion correction has no decided output. The step says to emit a second registration line only if a live fish check shows the filename fallback does not survive the wrap — and the same task records that fish is not installed on the machine the work happens on, so the check cannot be run and the implementer ships whichever shape they guess. Guessing wrong is not cosmetic: without that line, `x /tm<TAB>` in fish still completes to `/tmp/`, which turns a search into a mint at the filesystem root — the exact failure the correction exists to prevent, and the one the task's own acceptance criterion ("filename fallback stays off on that word in every shell") promises is gone.

**Proposal**:
Emit `complete -c <cmdName> -f` unconditionally beside the wrap. The line is inert where the wrap already carries the no-files property across and is the requirement where it does not, so the emitted output stops depending on a verification nobody can perform. It is determined rather than chosen: the task already requires filename fallback off on that word in every shell, and the specification requires the corrected completion to stop falling through to filenames for a path argument. The Do's existing instruction to extend the emitted-text tables with "the new registration lines for the configured name" already covers asserting both lines, so no Tests change is owed.

**Current**:
```
- In `emitFishInit` (`cmd/init.go:81`), change the `cmdName` registration from `complete -c %s -w portal` (`:101`) to `complete -c %s -w 'portal open'`, leaving the `ctlName` wrap at bare `portal`. Verify in fish where it is installed that the no-file behaviour survives the wrap (`complete -C 'x /tm'` offers no filesystem path); where it does not, emit `complete -c %s -f` for `cmdName` beside the wrap and assert that line too.
```

**Proposed Text**:
```
- In `emitFishInit` (`cmd/init.go:81`), replace the `cmdName` registration `complete -c %s -w portal` (`:101`) with two lines for `cmdName` — `complete -c %s -f` followed by `complete -c %s -w 'portal open'` — leaving the `ctlName` wrap at bare `portal` with no `-f` line of its own. The `-f` is unconditional rather than contingent on a live fish check: it is what holds the no-filename contract on that word whether or not the wrap carries the property across, and it is inert where the wrap already does.
```

**Resolution**: Fixed
**Notes**: Applied to task open-with-forced-filter-5-2 (detail file and tick record) under auto mode; the fish edge-case bullet now states the `-f` line is unconditional so nothing rests on the unrunnable check.

---

### 2. Task 1-6 breaks a Phase 1 test it never names

**Severity**: Minor
**Plan Reference**: Phase 1, task 1-6 (`open-with-forced-filter-1-6`) — Do
**Category**: Task Self-Containment
**Move**: settled
**Change Type**: add-to-task

**Problem**:
Task 1-3 pins `"it applies no filter for an empty search term"` — the term-less form reaching the landing inert. Task 1-6 makes that same landing open a focused empty filter, so that pin goes red the moment 1-6's change lands, with nothing in 1-6 saying so. The implementer meets a failing suite mid-task and has to decide for themselves whether the new behaviour or the old pin is the correct one — and the plan already decided: task 2-2 re-points task 1-5's superseded expectation by name, so the convention exists and only this one supersession is missing it.

**Proposal**:
Add a Do step to task 1-6 re-pointing that subtest by name, matching what task 2-2 does for the expectation task 1-5 pinned the other way. Placed last, where the other tasks in this phase put their test-repointing steps.

**Current**:
```
**Do**:
- Extend `applySearchLanding()` (`internal/tui/model.go`): when the search form carries an empty term, call `m.sessionList.SetFilterText("")`, then `m.sessionList.SetFilterState(list.Filtering)`, then `m.ensureSessionRowSelected()`. Add one comment on the ordering — the text call is what populates the filtered set, so the state flip must not come first.
- Add nothing to the render layer: the focused-filter footer, the untouched section-header row and the `SettingFilter` key guard already cover the `Filtering` state.
- Leave the `-f` empty-value refusal (`cmd/open.go:161`) exactly as it is.
```

**Proposed Text**:
```
**Do**:
- Extend `applySearchLanding()` (`internal/tui/model.go`): when the search form carries an empty term, call `m.sessionList.SetFilterText("")`, then `m.sessionList.SetFilterState(list.Filtering)`, then `m.ensureSessionRowSelected()`. Add one comment on the ordering — the text call is what populates the filtered set, so the state flip must not come first.
- Add nothing to the render layer: the focused-filter footer, the untouched section-header row and the `SettingFilter` key guard already cover the `Filtering` state.
- Leave the `-f` empty-value refusal (`cmd/open.go:161`) exactly as it is.
- Re-point task 1-3's `"it applies no filter for an empty search term"` subtest to the superseding expectation: the term-less landing now leaves the session list in `list.Filtering` with an empty filter value and every live session visible.
```

**Resolution**: Fixed
**Notes**: Applied to task open-with-forced-filter-1-6 (detail file and tick record) under auto mode.

---

### 3. The warm single-match warning delivery is pinned by a test that cannot fail

**Severity**: Important
**Plan Reference**: Phase 4, task 4-3 (`open-with-forced-filter-4-3`) — Do, the test-file step
**Category**: Task Template Compliance (Tests) / Scope and Granularity
**Move**: settled
**Change Type**: update-task

**Problem**:
The task's headline behaviour — a warm single-match attach printing its soft bootstrap warnings before handing the terminal to tmux — is asserted through a route that produces those lines whether or not the code under test exists. Task 4-3 runs before task 4-4, so at that point a `/term` line is still classified as a CLI line and `PersistentPreRunE` writes the warnings to stderr and drains the sink before `RunE` is reached; the new write then finds an empty sink and adds nothing, and every assertion in the subtest passes over output the task did not produce. An implementer working the TDD cycle writes the test, sees green, and has no way to drive the change — and a saver-down warning silently dropped on an attach is precisely the defect this task exists to prevent, so the verification has to be able to fail.

**Proposal**:
Drive the warm route by calling `runSearchForm` directly, with the sink seeded after the reset and stderr pointed at a buffer, so the write under test is the only thing that can put a line in that buffer whichever way the invocation classifies at this point in the phase. The seams the task already relies on make this available with no new machinery: `buildSearchSessionSource` returns the injected `OpenDeps.SearchSessions` without touching tmux, and `openSessionFunc` is already stubbed for the call-order assertion.

**Current**:
```
- Add `cmd/open_search_warnings_test.go`: drive the warm route through `rootCmd.Execute()` with `rootCmd.SetErr(&buf)`, `resetBootstrapWarnings(t)`, a stubbed `openSessionFunc` recording call order; drive the teardown route by building a model with `tui.Build` carrying a decision closure, stepping it through `LoadingMinElapsedMsg` and `BootstrapCompleteMsg{Warnings: …}`, and calling `emitSearchTeardownWarnings` over a buffer.
```

**Proposed Text**:
```
- Add `cmd/open_search_warnings_test.go`: drive the warm route by calling `runSearchForm` directly — `resetBootstrapWarnings(t)`, one warning added to the sink after that reset, a `*cobra.Command` whose `SetErr` is a buffer, the session source injected through `withOpenDeps` and a stubbed `openSessionFunc` recording call order — so the only thing that can put a line in that buffer is the write under test, whichever way the invocation classifies at this point in the phase; drive the teardown route by building a model with `tui.Build` carrying a decision closure, stepping it through `LoadingMinElapsedMsg` and `BootstrapCompleteMsg{Warnings: …}`, and calling `emitSearchTeardownWarnings` over a buffer.
```

**Resolution**: Fixed
**Notes**: Applied to task open-with-forced-filter-4-3 (detail file and tick record) under auto mode; an edge-case bullet now records why the end-to-end shape cannot drive this task. The end-to-end `rootCmd.Execute()` shape is not lost — task 4-4 already pins the stderr side of the flipped classification with `"it holds warnings out of stderr for a search-form line"`.
