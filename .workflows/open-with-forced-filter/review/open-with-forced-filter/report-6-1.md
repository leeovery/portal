TASK: Single-source the session set the sigil counts and the picker lists (open-with-forced-filter-6-1 / tick-6a5d97)

ACCEPTANCE CRITERIA:
1. `PickerSessions` returns every session whose name differs from `currentSession`, in enumeration order, and returns the whole set for an empty `currentSession` or a name no session carries
2. `PickerSessions` leaves the slice it was handed untouched — same length, same elements, no zeroed tail
3. `searchCandidates` and `Model.filteredSessions` each reach the exclusion only through `PickerSessions`: `grep -n 'DeleteFunc' cmd/open_search.go` returns nothing, and neither file carries a remaining hand-written loop or delete over `s.Name`/`currentSession`
4. Behaviour unchanged on both routes: `TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount`, `TestOpenCommand_SearchForm_AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches`, `TestOpenCommand_SearchForm_CountsNothingOutWhenTheCurrentSessionReadFails`, `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux` and `internal/tui`'s `"excludes the current session when inside tmux"` all pass with no edit to any of them
5. `go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean

STATUS: complete

SPEC CONTEXT: The specification states the searched set is the set the picker lists (§3.2), which omits the session the caller is attached to, and its 2026-09-14 corrigendum (specification.md:488) corrects §8.1's completion claim by naming `internal/tui/picker_sessions.go:13-23` as the declaration of that set — so the spec's own record already points at this helper as the single home of the rule. The degrade-rather-than-drop policy (a failed or empty attached-session read holds nothing back) stays on the caller's side, which is exactly where the implementation leaves it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/picker_sessions.go:13-23` — the new exported helper: early `return sessions` for an empty `currentSession`, otherwise a fresh `make([]tmux.Session, 0, len(sessions))` appended in enumeration order.
  - `internal/tui/model.go:1140-1145` — `filteredSessions` is now `if !m.insideTmux { return m.sessions }` then `return PickerSessions(m.sessions, m.currentSession)`; the hand-written loop the commit replaced is visible in `git show e16d3692c -- internal/tui/model.go`.
  - `cmd/open_search.go:177-183` — `searchCandidates` is `ListSessionsProbe` then `return tui.PickerSessions(sessions, currentPickerSession(src)), nil`; the `slices` import is gone from the file (commit diff shows the import removal).
- Notes: The `m.insideTmux` guard was kept at the model call site exactly as the task prescribed, so a hand-built model carrying a name without the flag still returns the whole set. The task's "keep `searchCandidates`'s own `tmux.InsideTmux()` / `CurrentSessionName()` read and its error policy" was satisfied at the time of the commit; a later task in the same plan (tick-e7336e) lifted that read into the shared `currentPickerSession` (`cmd/open_search.go:161-171`), which preserves the same policy ("" outside tmux, "" on a failed read) and is a deliberate, recorded advance rather than drift. The single-source property has since strengthened rather than eroded: `PickerSessions` now has three production callers — `cmd/open_search.go:182`, `cmd/completion.go:74` and `internal/tui/model.go:1144` — and no other production site restates the exclusion (`grep -n currentSession internal/tui/*.go` shows the remaining uses are title/section-header rendering only).

TESTS:
- Status: Adequate
- Coverage: `internal/tui/picker_sessions_test.go` carries the four prescribed subtests under `TestPickerSessions` — drops the named session keeping enumeration order, drops nothing for an empty name, drops nothing for a name no session carries, and leaves the caller's slice unmodified (asserting the full original three names after the call, which is what catches a zeroed tail). `cmd/open_search_test.go:689-708` adds `TestSearchCandidates_LeavesTheEnumerationItWasHandedUnmodified`, driving the real `searchCandidates` through a `verbatimSearchSource` that hands back the very slice it holds, with `t.Setenv("TMUX", …)` forcing the inside-tmux branch so the filtering path is the one exercised. The pre-existing behaviour tests named in criterion 4 all still exist and were untouched by the commit (its diff to `cmd/open_search_test.go` is purely additive at line 635+, and `internal/tui/rebuild_session_list_test.go:202` is not in the commit's file list).
- Notes: Not over-tested. The two "drops nothing" subtests cover genuinely different code paths (the `currentSession == ""` early return versus the loop finding no match), and the non-mutation property is asserted once at the helper and once at the caller that used to mutate — which is the pair the refactor actually put at risk. No mocking beyond the two-method fake source the seam already requires. Every test asserts observable behaviour (returned names, caller's slice contents), not internals.

CODE QUALITY:
- Project conventions: Followed. Exported helper carries a doc comment opening with its own name; the file is single-purpose and named for it; `internal/tui` already imports `internal/tmux` and `cmd` already imports `internal/tui`, so no new package edge was taken (which is the justification the task gave for this home over `internal/tmux`). No `t.Parallel()`, no real tmux touched — the new `cmd` test drives a fake source and only reads `TMUX` from the environment.
- SOLID principles: Good. The helper is a pure function of its two arguments with one responsibility; the inside/outside-tmux branch stays with the callers that know it, and the presentation policy stays out of the tmux CLI wrapper.
- Complexity: Low. One branch, one loop, no error path.
- Modern idioms: Yes. Pre-sized `make(…, 0, len(sessions))` with `append`, range-over-value.
- Readability: Good. The doc comment states the ownership contract the refactor depends on ("A filtered result is a fresh slice; an unfiltered one is the caller's own") rather than restating the code.
- Issues: None. Comments hold against the code: the "never deletes in place" claim is what the loop does, and the "callers go on reading" rationale is true of `m.sessions`, which `rebuildSessionList` reads on every rebuild. No process-artifact references (task ids, phases, spec sections) appear in any of the changed source.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Behaviour is unchanged on both routes: `TestOpenCommand_SearchForm_ExcludesTheCurrentSessionFromTheCount`, `TestOpenCommand_SearchForm_AttachesTheOtherMatchWhenTheCurrentSessionAlsoMatches`, `TestOpenCommand_SearchForm_CountsNothingOutWhenTheCurrentSessionReadFails`, `TestOpenCommand_SearchForm_ExcludesNothingOutsideTmux` and `internal/tui`'s `"excludes the current session when inside tmux"` all pass with no edit to any of them" — the "no edit" half is settled by reading (all five still exist at `cmd/open_search_test.go:612/627/642/657` and `internal/tui/rebuild_session_list_test.go:202`, and the commit's diff touched none of them); the "pass" half needs `go test ./cmd ./internal/tui` run.
- "`go test ./...` is green, `gofmt -l` reports nothing and `go vet ./...` is clean" — needs the unit lane, `gofmt -l .` and `go vet ./...` executed.
