TASK: open-with-forced-filter-11-5 (tick-2cd16f) — "Decide how far the completer's separator bound reaches": record the completer's boundary-word exception in `completeOpenPositional`'s doc comment and pin it at both arities in `TestCompleteOpenPositionalSeparatorBound`.

ACCEPTANCE CRITERIA:
1. `completeOpenPositional`'s doc comment no longer claims a post-separator word takes the session-name arm however it is spelled, and states the boundary-word exception together with the reason the flag layer cannot see it.
2. `TestCompleteOpenPositionalSeparatorBound` drives `open -- /po` and `open api -- /po`, each asserting candidates `["/portal-a1b2"]` — the sigil arm — rather than the session-name arm's `["portal-a1b2", "web-9"]`.
3. The four existing subtests are unedited and still pass: `-- ls /po` matches `completeSessionNames("/po")` in both candidates and directive, the two no-separator lines still offer `/portal-a1b2`, and the probe-parse subtest still pins the `<=`.
4. No production behaviour changes — `completingPreDashPositional`, `preDashPositionals`, `completeOpenPositional`'s body and `completeSearchTerm` are all untouched.
5. No specification edit is made; §8.1 already records the exception.
6. `go test ./cmd -run TestCompleteOpenPositional` passes.

STATUS: complete

SPEC CONTEXT: §8.1 (specification.md:332) states the completion bound and its one exemption verbatim: "Completion is bounded by the `--` separator, as recognition is (§2.3), and by nothing else… One position is outside the bound and stays as it is: the word immediately after the separator is, at the flag layer the completer reads, indistinguishable from the next ordinary positional, so it keeps the sigil arm." The 2026-09-15 corrigenda (specification.md:492, :494, :496) record the three passes that produced that bound: the separator bound itself and the boundary-word exemption (:492), the "separator is the only bound" clarification (:494), and the later switch of the post-separator answer to no candidates + `ShellCompDirectiveDefault` with "the boundary word immediately after the separator" explicitly unchanged (:496). The task's settlement — keep the bound where it is, state the exception, test it — matches §8.1 exactly.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/completion.go:85-103` — `completeOpenPositional`'s doc comment. The clause the task named as the overstatement ("belongs to that command, so it takes the session-name arm however it is spelled.") is gone; `cmd/completion.go:90-93` now reads "…with one exception at the boundary: the word immediately after the separator sits at the same index as the next pre-dash positional would, so the flag layer completingPreDashPositional reads cannot tell the two apart and it keeps the two pre-separator arms."
  - `cmd/completion_test.go:657-675` — the two new subtests in `TestCompleteOpenPositionalSeparatorBound`.
  - Delivering commit: `6446076c2`, touching exactly `cmd/completion.go` (+7/-2, comment only) and `cmd/completion_test.go` (+20, additions only). The paired record commit `5486561ce` touches only `.tick/tasks.jsonl` and the workflow manifest.
- Notes:
  - Criterion 1 holds at HEAD. The comment's wording was subsequently reworded by tasks 12-4 (`0af96d2ca`) and 12-6 (`756d46a23`) — which changed the post-separator answer to `nil, ShellCompDirectiveDefault` — but the exception clause and its reason survived intact and now correctly describe the two-arm boundary behaviour rather than the sigil arm alone.
  - The comment's central claim is true against the code. Verified against cobra v1.10.2 (`completions.go:369-373`, `:397-400`): cobra probe-parses `append(finalArgs, "--")` and then the real line, so for `open -- /po` the completer sees `args=[]`, `ArgsLenAtDash()==0`, and for `open api -- /po` it sees `args=["api"]`, `ArgsLenAtDash()==1` — the same `(len(args), dash)` pair a no-separator line of the same arity leaves after the probe parse. `completingPreDashPositional` (`cmd/open_search.go:45-48`) returns true for both, so the boundary word does take the pre-separator arms, and the flag layer genuinely cannot discriminate.
  - Criterion 4 holds for this task's commit: the `6446076c2` diff changes no statement in `cmd/completion.go` and does not touch `cmd/open_search.go`. `completingPreDashPositional`'s body at HEAD (`cmd/open_search.go:46-47`) still reads the flag set alone with `<=`, and no completion call site consults the raw `__complete` argv. `completeOpenPositional`'s body was later restructured by task 12-6, which is that task's delivered scope, not drift here.
  - Criterion 5 holds: neither commit touches the specification.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/completion_test.go:657-665` — "it keeps the sigil arm for the word immediately after the separator": `completionCandidates(t, "__complete", "open", "--", "/po")` under the suite's `twoSessions` seed, asserting `[]string{"/portal-a1b2"}` via `slices.Equal`.
  - `cmd/completion_test.go:667-675` — "it keeps the sigil arm for the boundary word beside a target": the `open api -- /po` arity, same seed, same expectation. This is the arity the original residual record missed, so the pair is what makes the exemption a boundary-word rule rather than a zero-target one.
  - Both drive the real completion flow end to end through `completionCandidates` → `completionResult` (`cmd/completion_test.go:288-324`), which resets the root command and executes `__complete`, so the dash index under assertion comes from a genuine cobra probe-then-real parse rather than a hand-staged `cobra.Command` — the right route given the pflag carry-over the sibling probe-parse subtest (`cmd/completion_test.go:677-694`) pins.
- Notes:
  - The tests would fail if the behaviour broke. Tightening `completingPreDashPositional`'s `<=` to `<` makes `0 < 0` and `1 < 1` both false, sending each line down `completeOpenPositional`'s `!completingPreDashPositional` arm (`cmd/completion.go:104-106`) to `nil, ShellCompDirectiveDefault` — an empty candidate slice against the expected `["/portal-a1b2"]`.
  - Not redundant with the probe-parse subtest: that one asserts the predicate directly on a hand-staged command; these assert the answer the shell actually receives at the boundary.
  - Not over-tested: two subtests for the two arities the task's settlement names, no extra mocking beyond the shared `twoSessions` seed already used by every sibling in the table.
  - The subtests assert candidates only, not the directive — matching the three pre-existing sigil-arm subtests beside them (`cmd/completion_test.go:627-655`) and the criterion's own wording. The sigil arm's `ShellCompDirectiveNoFileComp` is separately pinned by `TestCompleteOpenPositional`'s "it routes a search word through the search branch" (`cmd/completion_test.go:464-475`), so the directive is not an unguarded gap.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; subtests use the file's `t.Run(<behaviour sentence>)` naming; the shared `twoSessions` closure and `withCompletionSessions` seam staging (which routes through `withFuncSeam`, satisfying `cmd/seam_guard_test.go`) are used rather than a direct seam assignment.
- SOLID principles: N/A — a doc comment and two subtests; no structure changed.
- Complexity: Low.
- Modern idioms: Yes — `slices.Equal` for the candidate comparison, consistent with the rest of the file.
- Readability: Good. The comment names the exception, the index reason, and the predicate that cannot see it, which is what the task required of the wording.
- Comment accuracy: The doc comment holds against the code at HEAD, including the later 12-6 restructure; it cites no task id, phase or spec section. The subtest names describe the behaviour, not the mechanism.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./cmd -run TestCompleteOpenPositional` passes." — reading settles the logic but not the run; execute `go test ./cmd -run TestCompleteOpenPositional` to settle it.
- "The four existing subtests are unedited and still pass: `-- ls /po` matches `completeSessionNames(\"/po\")` in both candidates and directive, the two no-separator lines still offer `/portal-a1b2`, and the probe-parse subtest still pins the `<=`." — the "unedited" half is settled by reading (`6446076c2` is additions-only to `cmd/completion_test.go`); the "still pass" half needs the same run. Note for the change-set pass: the `-- ls /po` subtest's expectation was deliberately changed afterwards by task 12-6 (`756d46a23`) from "matches `completeSessionNames`" to no candidates + `ShellCompDirectiveDefault`, so HEAD must be measured against 12-6's contract, not this criterion's wording.
