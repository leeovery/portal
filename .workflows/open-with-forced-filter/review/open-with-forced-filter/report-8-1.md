TASK: Route every search surface's session set through one attached-session read (tick-e7336e / open-with-forced-filter-8-1)

ACCEPTANCE CRITERIA: [from plan]
1. `rg -n 'CurrentSessionName\(\)' --glob '*.go' --glob '!*_test.go' cmd/` shows exactly one call, inside `currentPickerSession`, alongside the interface method declarations.
2. Neither `openTUI` nor `searchCandidates` carries a `tmux.InsideTmux()` gate of its own; the package's other two uses (connector selection) are untouched.
3. Inside tmux, `completeSearchTerm` never returns the attached session's name — for a typed term and for the bare `/` alike.
4. Outside tmux, and whenever the current-session read fails or answers empty, `completeSearchTerm` and `searchCandidates` each drop nothing.
5. `completeSessionNames` offers the same set as today, the attached session included.
6. The existing search-count suites pass unchanged, including `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux`'s zero-read assertion.
7. The new seam is staged in tests only through `withFuncSeam` (a `withCompletionCurrentSession` wrapper beside its neighbour), so `cmd/seam_guard_test.go`'s derived function-var arm stays green.
8. `go build ./... && go test ./...` green; `golangci-lint run` clean.

STATUS: complete

SPEC CONTEXT:
Specification §3.2 (line 86) fixes the searched set as "the set the picker lists". §8.1's completion rule (line 328) and the 2026-09-14 corrigendum (line 488) carry the consequence explicitly: "the offered set is the searched set, which §3.2 defines as the set the picker lists and which omits the session the caller is attached to"; and "Outside tmux, and whenever the attached-session read fails or answers nothing, nothing is held back on that account — the same degrade-rather-than-drop rule the count follows, and the only one available here, since completion carries no channel for a failure." The task collapses the *read* producing that exclusion argument to one function and puts all three surfaces (count, picker, completer) on it.

IMPLEMENTATION:
- Status: Implemented (delivered in b795eb1a9; the completer arm was subsequently strengthened by 2fdf38b20 / task 10-2, which is consistent with this task's intent — see Notes)
- Location:
  - `cmd/open_search.go:156-159` — `currentSessionReader`, the one-method reader (`CurrentSessionName() (string, error)`).
  - `cmd/open_search.go:161-173` — `currentPickerSession(src)`: returns "" outside tmux with **no** read taken, "" on a failed read, and the read's own answer otherwise (so an empty answer returns "").
  - `cmd/open_search.go:175-183` — `searchCandidates` tail is now `return tui.PickerSessions(sessions, currentPickerSession(src)), nil`, with only the `ListSessionsProbe` error return ahead of it.
  - `cmd/open.go:733-736` — `openTUI`'s inline block replaced by `if sessionName := currentPickerSession(client); sessionName != "" { cfg.insideTmux = true; cfg.currentSession = sessionName }`.
  - `cmd/completion.go:23-27` — the `completionCurrentSession` package-level function-var seam, defaulting to `currentPickerSession(tmux.DefaultClient())`.
  - `cmd/completion.go:70-83` — `completeSearchTerm` now iterates `tui.PickerSessions(completionSessions(), completionCurrentSession())`.
- Notes:
  - AC1 verified by re-running the grep: `cmd/open_search.go:146` and `:158` are the two interface method declarations (`SearchSessionSource`, `currentSessionReader`) and `cmd/open_search.go:168` is the single call. No other non-test call exists in `cmd/`.
  - AC2 verified: the only non-test `tmux.InsideTmux()` uses in `cmd/` are `cmd/open.go:106` (`buildSessionConnector`), `cmd/open.go:460` (`openPath`) and `cmd/open_search.go:165` (inside `currentPickerSession`). The plan cited the second connector-selection site as `cmd/open.go:449`; it now reads 460 after intervening tasks moved the file. Substance holds: two connector-selection uses, both untouched.
  - **Drift, in the task's favour**: the plan prescribed a `name == current` skip in `completeSearchTerm` because the completer then worked over a `[]string` seam (`completionSessionNames`). Task 10-2 (`2fdf38b20`, "Derive the search completer's offered set from the rules that own it") reshaped that seam to `completionSessions() []tmux.Session`, so the completer now routes the exclusion through `tui.PickerSessions` itself — one declaration of the set rule for all three surfaces rather than two. The delivered code is strictly closer to the task's stated outcome than its prescribed edit; nothing the task required in substance was lost.
  - The three surfaces all now answer from one place: the picker at `internal/tui/model.go:1144`, the count at `cmd/open_search.go:182`, the completer at `cmd/completion.go:74` — all three `tui.PickerSessions`, all fed by `currentPickerSession` (or the model field `openTUI` sets from it).
  - `tui.PickerSessions(sessions, "")` returns the caller's own slice unmodified (`internal/tui/picker_sessions.go:14-16`), so routing the empty case through it rather than short-circuiting is behaviour-identical to the removed branch — confirmed by `TestSearchCandidates_LeavesTheEnumerationItWasHandedUnmodified` (`cmd/open_search_test.go:689-709`).
  - README's completion paragraph (`README.md:175`) already states the exclusion in user words ("every live session but the one you are attached to"), so the documented surface matches the delivered behaviour.

TESTS:
- Status: Adequate
- Coverage: all seven prescribed tests exist and read the criteria they name.
  - `cmd/open_search_test.go:747-789` — `TestCurrentPickerSession`: "it returns the attached session's name inside tmux" (also asserting exactly one read), "it takes no current-session read outside tmux" (asserts `currentCalls == 0`), "...when the current-session read fails", "...when the current-session read answers empty". Four branches, one assertion each; no redundancy.
  - `cmd/completion_test.go:413-425` — "it does not offer the session the caller is attached to" (`completeSearchTerm("/po")` with the attached session among the live names).
  - `cmd/completion_test.go:428-437` — "it holds back the attached session for a bare slash too".
  - `cmd/completion_test.go:439-448` — "it offers every live name when the current-session read answers nothing".
  - `cmd/completion_test.go:53-62` — "it still offers the attached session on the plain session-name completer" (AC5's regression guard).
  - `cmd/open_search_test.go:657-672` — the pre-existing `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux` still asserts `CurrentSessionName` is read zero times outside tmux (line 666), which is the criterion AC6 names; the assertion is unchanged and the new code satisfies it by `currentPickerSession`'s short-circuit.
- Notes:
  - Each new test would fail if the behaviour broke: dropping the `!tmux.InsideTmux()` short-circuit fails the zero-read assertions at `cmd/open_search_test.go:768` and `cmd/open_search_test.go:667`; dropping the completer's exclusion fails `cmd/completion_test.go:423`; excluding on an empty read fails `cmd/completion_test.go:446`.
  - Not over-tested: no assertion duplicates another, and the only seams staged are the two function vars the code actually reads.
  - AC7 holds: `withCompletionCurrentSession` (`cmd/completion_test.go:22-25`) is a `withFuncSeam` wrapper, and no test file assigns `completionCurrentSession` directly — the seam guard (`cmd/seam_guard_test.go:39-44`) derives its seam set from the production sources, and `var completionCurrentSession = func() string {…}` is a function-literal initialiser, so it is in that derived set the day it appeared.

CODE QUALITY:
- Project conventions: Followed. The new seam is a package-level function var in `cmd` (the package's second seam family, per CLAUDE.md's DI section), staged only through `withFuncSeam`; the reader is a one-method interface, matching the project's "small interfaces (1-3 methods)" rule; `internal/tui` keeps the set rule and `cmd` keeps the read.
- SOLID principles: Good. `currentSessionReader` is the narrowest possible interface for the job and is satisfied by `*tmux.Client` and the existing `SearchSessionSource` seam without either being changed — the callers pass what they already hold.
- Complexity: Low. `currentPickerSession` is two branches; `searchCandidates` lost three.
- Modern idioms: Yes — `if sessionName := currentPickerSession(client); sessionName != ""` at `cmd/open.go:733` is the idiomatic scoped-assignment form and replaces a four-line nested block.
- Readability: Good.
- Issues: None. Comments checked against the code they describe: `cmd/open_search.go:161-163` ("the empty string outside tmux or when the read fails or answers empty") holds — the function returns `current`, which is "" on an empty answer; `cmd/completion.go:23-24` ("builds its own client for the same reason as its neighbour") holds against `completionSessions` at `cmd/completion.go:12-21`; `cmd/completion.go:63-69` ("The searched set comes from tui.PickerSessions") holds against line 74. No comment references a task id, phase or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "The existing search-count suites pass unchanged, including `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux`'s assertion that `CurrentSessionName` is read zero times outside tmux" — the assertion's presence and its consistency with the delivered code were settled by reading (`cmd/open_search_test.go:666`); that the suite *passes* needs `go test ./cmd -run TestOpenCommand_SearchForm` run.
- "`go build ./... && go test ./...` green; `golangci-lint run` clean" — needs the unit lane and the linter run.
