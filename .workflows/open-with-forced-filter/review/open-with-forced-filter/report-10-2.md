TASK: Derive the search completer's offered set from the rules that own it (tick-430d00 / open-with-forced-filter-10-2)

ACCEPTANCE CRITERIA:
- `cmd/completion.go` carries no `strings.Contains` test and no `name == current` test — both hold-backs are function calls into `resolver` and `tui`
- `completeSearchTerm` reaches its candidates only through `tui.PickerSessions`
- The offered set is unchanged on every input the suite covers: `/po`, `/`, `/ort`, a slash-bearing name, the attached session, an empty current-session read, and a failed session read
- `completeSessionNames` still offers the attached session, so the bare positional and `-s` completions are untouched
- `TestCompletionExcludesInternalSessions` still excludes `_portal-x` from both the plain and the search completers
- No new package edge — `cmd` already imports `internal/tmux` and `internal/tui`

STATUS: complete

SPEC CONTEXT: §8.1 (tab completion for the sigil) states the offered words as "the searched set's session names the typed term prefixes", carrying the slash on the candidate. Two corrigenda bear directly on this task. The 2026-09-14 corrigendum (specification.md:488) corrects §8.1's original "live session names" to the *searched* set — the set the picker lists, which omits the attached session — and names `internal/tui/picker_sessions.go:13-23` as its home, because offering a name the search cannot reach completes to a term landing on a picker filtered to zero rows. The 2026-09-15 corrigenda (specification.md:492, :494, :496) fix the `--` bound and the post-separator directive; they touch `completeOpenPositional`, not the offered set, and were delivered by other tasks. This task is the structural follow-through on the 2026-09-14 one: the completer had restated the exclusion inline rather than taking it from the declaring function, so the rule and its copy could drift apart silently.

IMPLEMENTATION:
- Status: Implemented (commit 2fdf38b20), matching every step of the task's Do list.
- Location:
  - `cmd/completion.go:12-21` — the seam is now `var completionSessions = func() []tmux.Session` over `tmux.DefaultClient().ListSessions()`, nil on error exactly as the name-based seam did, and the doc comment keeps the stated reason for building its own client (bootstrap-exempt `__complete` path carries no context client).
  - `cmd/completion.go:32-40` — `completeSessionNames` maps `s.Name` at its own call site; it never consults `completionCurrentSession`, so the attached session is still offered on the plain completer.
  - `cmd/completion.go:74` — the loop is built over `tui.PickerSessions(completionSessions(), completionCurrentSession())`; the inline `if current != "" && name == current { continue }` and the `current` local are gone.
  - `cmd/completion.go:75-77` — the shape hold-back is `if !resolver.IsSearchSigil("/" + s.Name) { continue }`.
  - `cmd/completion.go:63-69` — the doc comment now names `tui.PickerSessions` and `resolver.IsSearchSigil` as the sources of the two hold-backs rather than restating the rules.
  - `cmd/completion.go:8` — `internal/tui` import added; `cmd` already took that edge in `cmd/open_search.go:182`, so no new package edge appears (verified: `internal/tui` imports no `cmd`, so no cycle).
- Notes: behaviour equivalence holds exactly on both substitutions. `IsSearchSigil("/"+name)` is `HasPrefix("/"+name, "/") && Count("/"+name, "/") == 1`; the prefix is true by construction, so the test reduces to `Count(name, "/") == 0` — the precise complement of the deleted `strings.Contains(name, "/")`. `PickerSessions(sessions, current)` returns `sessions` unchanged when `current == ""` and otherwise a fresh slice less the named session, in enumeration order — the precise behaviour of the deleted two-line guard, including its empty-current degrade. The seam widening is likewise exact: `Client.ListSessionNames` (`internal/tmux/tmux.go:200-209`) is itself defined over `ListSessions`, so the `_`-prefix filter (`tmux.go:188-197`) and the error shape are the same read, not a comparable one — no extra tmux work and no new failure mode. `grep` confirms no remaining reference to `completionSessionNames` anywhere in the tree, and `ListSessionNames` retains other production callers (`internal/state/capture.go:46`, `internal/restore/restore.go:99`), so nothing was orphaned. All three completer registrations (`cmd/open.go:785-787`, `cmd/kill.go:57`) route through `completeSessionNames` or `completeOpenPositional` and are untouched.

TESTS:
- Status: Adequate.
- Coverage: `cmd/completion_test.go` reseeds every case with `tmux.Session` values and renames the staging helper to `withCompletionSessions` (`cmd/completion_test.go:17-20`), leaving assertions and failure text byte-identical — verified by reading the commit diff, which is a pure seed-and-helper substitution with no assertion edit anywhere. All nine `TestCompleteSearchTerm` subtests survive and cover exactly the criterion's enumerated inputs: `/po` (:332), bare `/` (:345), `/ort` (:358), the slash-bearing name at both a term and the empty term (:371, :383), the attached session at both (:393, :407), the empty current-session read (:419), and the failed session read (:429). `TestCompleteSessionNames`'s "it still offers the attached session on the plain session-name completer" (:53) pins the untouched `-s`/bare-positional arm. `TestCompletionExcludesInternalSessions` (:252) now seeds from a real `tmuxtest` socket's `client.ListSessions()` and asserts `_portal-x` absent from both the plain (`my-work`) and search (`/my-work`) answers. `TestCompleteOpenPositional` and `TestSearchCompletionWiring` exercise the routing end to end through `__complete`.
- Notes: no new test is owed — this is a behaviour-preserving refactor and the attached-session and slash-bearing-name subtests would fail outright if either hold-back were dropped rather than delegated. The structural claim (candidates reached *only* through `tui.PickerSessions`) is not itself asserted by a test, which is inherent to a delegation refactor and is settled by reading: line 74 is the sole enumeration in the function. Not over-tested — no assertion was added, and the seed change is the minimum the type move required.

CODE QUALITY:
- Project conventions: Followed. The seam stays a package-level function var staged through `withFuncSeam` (`cmd/completion_test.go:19`), which `cmd/seam_guard_test.go` derives rather than lists, so the renamed var is covered without an edit. No new package edge, and the leaf/import rules in CLAUDE.md are respected.
- SOLID principles: Good — the change is precisely a dependency-inversion correction: the completer now asks the declaring functions rather than holding private copies of their rules.
- Complexity: Low. The function loses one local and one branch.
- Modern idioms: Yes.
- Readability: Good. `resolver.IsSearchSigil("/" + s.Name)` reads as a question put to the recogniser rather than a shape test, and the doc comment explains why the composed string is the right question to ask.
- Comment accuracy: The rewritten doc comment on `completeSearchTerm` (`cmd/completion.go:63-69`) holds against the code — `tui.PickerSessions` supplies the set (line 74), `resolver.IsSearchSigil` supplies the shape hold-back (line 75), and "a name that fails it would compose a second slash" is the only way the composed word can fail the recogniser. The `completionSessions` comment (`:12-14`) still states a true reason for the self-built client. No process-artifact references anywhere in the changed code.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None — every criterion was settled by reading. The two behavioural substitutions are exact by construction (the `IsSearchSigil` reduction and `PickerSessions`' documented empty-current degrade), the reseeded suite's assertions are unchanged in the commit diff, and the seam widening routes through the same `ListSessions` read the old seam already used. The suites themselves (`TestCompleteSearchTerm`, `TestCompleteSessionNames`, `TestCompleteOpenPositional`, `TestSearchCompletionWiring`, `TestCompletionExcludesInternalSessions`) are ordinary unit-lane tests and will be exercised by the change-set pass.
