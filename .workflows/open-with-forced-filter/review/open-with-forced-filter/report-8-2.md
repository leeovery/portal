TASK: open-with-forced-filter-8-2 — "Corrections" (tick-11e361): one edit to README.md:175 so the Tab-completion sentence states the plain completer's offered set and the search completer's offered set separately, instead of equating them with "those same names".

ACCEPTANCE CRITERIA:
1. `README.md` no longer contains the phrase "those same names", nor any other wording equating the two completers' offered sets.
2. The Tab-completion sentence names the two sets separately, and gives the search completer's set as every live session except the one the reader is attached to.
3. The sentence still records that the slash stays in place on an accepted search candidate.
4. The ``### `x` (open)`` section still contains the tokens `` `/<term>` ``, `-p /tmp`, `portal init` and `-f, --filter`, and its first bash block still carries the commented `x /port` and `x /` lines.
5. `git status --short` lists `README.md` and nothing else — no `.go` file modified, no test added, removed or reworded.

STATUS: complete

SPEC CONTEXT: §8.1 ("Completion looks past the sigil") states that what the search completer offers is "the set the sigil can reach, not the whole live enumeration" — the searched set of §3.2, which omits the session the caller is attached to, degrading to holding nothing back outside tmux or when the current-session read fails. §9.2 requires the README to carry that the form completes against live session names after the slash, plus the completion correction and its `portal init` rollout consequence. §9.3 confirms no CHANGELOG entry. The code matches: `cmd/completion.go:74` routes the search arm through `tui.PickerSessions(completionSessions(), completionCurrentSession())`, while `completeSessionNames` (`cmd/completion.go:32-40`) enumerates `completionSessions()` unfiltered — the asymmetry the README previously denied.

IMPLEMENTATION:
- Status: Implemented
- Location: `README.md:175` (single line); delivered as commit `936f540cb`, whose diffstat is `README.md | 2 +-` — one line changed, no other file.
- Notes:
  - The new clause reads "`x <TAB>` offers your live session names, and `x /po<TAB>` completes the term after the slash against the sessions a search can reach — every live session but the one you are attached to, since a search never lands you back where you already are — leaving the slash in place." That is accurate against `cmd/completion.go:74` + `internal/tui/picker_sessions.go:13-23` (the enumeration less the attached session; `""` drops nothing, which is the outside-tmux case) and against `currentPickerSession` (`cmd/open_search.go:164-173`), which returns `""` outside tmux and on a failed read.
  - The rest of line 175 is byte-intact: the bolded `portal init` clause, the "`/term` form is `portal open`'s own parsing" clause, and the bash 3.2 filename-completion parenthetical all survive unchanged in the diff.
  - Considered and deliberately not reported: the completer also holds back a session whose name contains `/` (`cmd/completion.go:75-77`), which §8.1 records as still *reachable* by a containment term though never offered — so the README's appositive is a shade broader than the offered set in that edge case. The task explicitly directs the README's own one-sentence register rather than the spec's long form, Portal never generates such a name (`SanitiseProjectName` works on a path basename), and the remedy would be documentation text only. It is a precision preference, not a defect, so it is not raised as a finding.

TESTS:
- Status: Adequate (no new tests required, and none were added — correct for a documentation-only correction)
- Coverage: `TestReadmeDocumentsSearchForm` (`cmd/open_docs_test.go:68-92`) is the guard that reads this section. Verified by reading the README against the helper's own extraction rule (`readmeOpenSection` cuts from ``### `x` (open)`` to the next `\n### `; `readmeOpenExamples` takes the first ```bash fence): the section contains `` `/<term>` ``, `-p /tmp`, `portal init` and `-f, --filter`, and the first bash block carries `x /port  # search live sessions for "port"` and `x /  # open the picker with the filter ready to type`. `portal init` occurs exactly once in the section — on the edited line — so the task's stated hazard (dropping it) was avoided and the guard still has its token.
- Notes: The tests pinning the behaviour the README now documents are untouched by this commit and still assert the asymmetry: `cmd/completion_test.go:52-61` ("it still offers the attached session on the plain session-name completer") and `:413-425` ("it does not offer the session the caller is attached to"), plus the slash-bearing hold-back at `:390-411`. No test was reworded, so no test semantics moved.

CODE QUALITY:
- Project conventions: N/A (no Go source touched; the project's no-CHANGELOG rule is respected — no CHANGELOG edit in the commit)
- SOLID principles: N/A
- Complexity: N/A
- Modern idioms: N/A
- Readability: Good — the two sets are stated in one sentence each, in the README's user-facing register, with the reason ("a search never lands you back where you already are") rather than a spec cross-reference.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None (all five acceptance criteria are settled by reading: criteria 1-4 against the README's current bytes and the guard's extraction rule, criterion 5 against commit `936f540cb`'s diffstat).
