TASK: Turn the directory column on for a search-opened picker and nowhere else (open-with-forced-filter-3-3, tick-aec481)

ACCEPTANCE CRITERIA:
- A model built with a search form renders each session row with its recorded directory, in Flat, By Project and By Tag
- The term-less search form carries the column — the gate is the form, not the term
- A `-f` picker, a no-argument picker and a command-pending picker render their session rows byte-identically to today (existing suites green unmodified)
- The column survives an `s` regroup, a `Space` preview and back, a `SessionsMsg` refresh, a marked-set mutation and a live theme swap
- The column survives a hand edit of the committed filter text, including clearing it
- A group header row renders no directory in either grouped mode
- A session whose directory is known only to grouping (recorded empty, derived present) shows no directory, and the column issues no pane read on any path
- A width narrow enough to left-truncate or drop the directory changes no row's membership in the list
- `go test ./...` passes

STATUS: complete

SPEC CONTEXT: §6.1 requires every row in a sigil-opened list to show its recorded directory beside the session name, uniformly, in every grouping mode, and only the recorded value — never a derived one, and never at the cost of a pane read. §6.3 scopes that to the picker a sigil opened (term-less form included) for as long as it is open, with a hand edit of the filter text explicitly not taking the column away, and leaves rendering untouched for a picker reached any other way. §6.2 (placement, muted token, left truncation, drop floor) was delivered by task 3-2; this task is only the switch.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/model.go:938 (`ShowDir: m.searchForm` inside `sessionDelegate()`, model.go:933-941); flag declared at model.go:188-190 and set only by `WithSearchForm` (model.go:565-570, reached from internal/tui/build.go:132-134).
- Notes: The gate is exactly the one the plan prescribed — the invocation-scoped `searchForm`, never the term and never the filter's committed text. I enumerated every delegate construction and every `SetDelegate` in the package: `sessionDelegate()` (model.go:933) and the zero-value seed in `newSessionList` (model.go:827) are the only two `SessionDelegate{}` literals outside test files, and `refreshSessionDelegate` (model.go:947) plus `applyListCanvasMode` (model.go:975, reached from `applyCanvasMode` at model.go:967) are the only two `SetDelegate` call sites — both route through `sessionDelegate()`, so every rebuild re-reads the gate. First frame is covered: `New` ends with `syncResolvedMode()` (model.go:896) → `applyCanvasMode()` → `SetDelegate`, after the option loop has run, so the mid-loop delegate refresh `WithInitialMultiSelect` performs is superseded exactly as the plan's edge-case note predicts. `Build` ends with `armAppearanceDetection()` (build.go:189), which repeats that path after the post-`New` value copies (`WithCommand`, `WithInitialFilter`, `WithInsideTmux`), so those copies cannot strand a delegate without the gate. `rebuildSessionList` (model.go:1231-1266) mutates items via `SetItems` and never replaces the list, so an `s` regroup keeps the delegate. Recorded-vs-derived separation holds at the render: the delegate reads `it.Session.Dir` (internal/tui/session_item.go:267, :275) and `m.derivedDirs` is written only by `resolveSessionDirs` (model.go:1209-1228), never into `Session.Dir`. `installSearchFilter` (internal/tui/search_filter.go:85-91) touches only `sessionList.Filter`, so the term-less form (which it returns early for) still carries the column. Nothing outside the one line changed — the commit (cb3eb0623) is that line plus the new test file.

TESTS:
- Status: Adequate
- Coverage: internal/tui/search_dir_column_test.go (`TestSearchDirColumn`, 14 subtests) covers each criterion: Flat/By Project/By Tag with a term, the term-less form, `-f` and no-argument pickers asserting the directory is absent, the `s` regroup (with the mode asserted at each press after task 3-5's correction), `Space`-preview-and-back, a second `SessionsMsg`, an `m` marked-set mutation, `ApplyTheme`, a hand edit of the committed filter text and its clearing (both with the resulting `FilterValue` asserted as a precondition), a group-header row rendering no directory in both grouped modes, a recorded-empty/derived-present session showing nothing (with `m.derivedDirs` asserted non-empty first, so the fixture is proven to be the case it claims), and identical visible-row membership at width 220 vs 34.
- Notes: Assertions read the real rendered rows (`ansi.Strip(m.sessionList.View())`) through the model's own delegate rather than a hand-built one, so the test observes the gate rather than the field. The "it issues no pane read for the column" subtest is the weakest of the set — its session carries a recorded `Dir`, so `resolveSessionDirs` (model.go:1213) would skip the read regardless of the column — but the companion recorded-empty/derived-present subtest observes the same property where it can actually be violated (a column reading the derived value would render it and fail `assertDirAbsent`), so the property is covered. No over-testing: no subtest repeats another's assertion, and the fixtures are three fields and a temp dir. No existing suite was modified by the commit, which is the form the third criterion asks for.

CODE QUALITY:
- Project conventions: Followed — no `t.Parallel()`, table-free subtests named as behaviour sentences, helpers take `*testing.T` and call `t.Helper()`, seams injected through `Deps` rather than package state.
- SOLID principles: Good — the change adds no branch and no new state; the gate is read at the single delegate construction point the model already owns.
- Complexity: Low — one field assignment.
- Modern idioms: Yes (`strings.SplitSeq`, `for i, want := range` over a mode slice).
- Readability: Good — the `ShowDir` field carries an accurate doc comment (session_item.go:118-123) explaining both the default-off and the survives-a-filter-edit scope, and the `searchForm` field comment (model.go:188-189) states the invocation-scope property the gate depends on. No comment in the changed code makes a claim the code falsifies, and none references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes" — requires a unit-lane run; settled by reading only to the extent that every symbol the new test file references resolves to exactly one declaration in `package tui` (`ingestLanding`, `sessionNames` in search_landing_test.go; `stepListerStub` in pagepreview_refetch_test.go; `stubProjectStore` in background_restore_test.go; `fakeStamper`/`fakeDirRunner` in rebuild_dir_resolution_test.go; `pressM` in multi_select_test.go; `keymapParityEnumerator`/`keymapParityReader` in sessions_keymap_dispatch_test.go; `testLightTheme` in theme_testing_test.go) and the same-named `runCmd` in command_pending_staged_mint_test.go is in `package tui_test`, so it does not collide.
- "A `-f` picker, a no-argument picker and a command-pending picker render their session rows byte-identically to today (existing suites green unmodified)" — the substance is settled by reading (the gate is `m.searchForm`, set only by `WithSearchForm`, which only a non-nil `Deps.Search` supplies at build.go:132; the `-f` and no-argument cases are asserted directly), but "existing suites green" needs the suite run — no existing test file was modified by the commit.
- "The column survives an `s` regroup, a `Space` preview and back, a `SessionsMsg` refresh, a marked-set mutation and a live theme swap" and "The column survives a hand edit of the committed filter text, including clearing it" — each has a subtest driving the real `Update` path, but confirming they pass (notably that the temp-dir path renders whole at the chosen width 220, and is dropped at width 34) requires executing `go test ./internal/tui -run TestSearchDirColumn`.
