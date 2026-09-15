TASK: open-with-forced-filter-5-5 (tick-aceea8) — Keep the user's cursor position through the bash completion shim

ACCEPTANCE CRITERIA:
1. The measurement is recorded in the task's implementation record: the bash and `bash-completion` versions, the `cur` cobra saw for a mid-line Tab before the edit, the request the stub recorded, `COMPREPLY`, and whether file completion supplied filenames in its place.
2. A measurement contradicting the derivation closes the task with `cmd/init.go` unedited and that recorded; no further criterion applies in that case.
3. The driver's `_init_completion` stand-in derives `cur` from `COMP_LINE`/`COMP_POINT`, and every existing end-of-line case in `cmd/init_completion_shell_test.go` still passes unchanged.
4. The mid-line case fails against the pre-edit shim and passes after it, with both runs recorded.
5. The emitted bash script carries no `COMP_POINT=${#COMP_LINE}`: the offset is the user's own plus the length difference between `portal open` and the typed name.
6. The rewritten `COMP_LINE` preserves the user's spacing — `x   api` becomes `portal open   api`, not `portal open api`.
7. A line with leading whitespace either completes correctly or has its limit stated in one comment line beside the shim.
8. `zshOpenCompletionShim`, the fish emission, and both `__start_portal` registrations (`portal`, `<name>ctl`) are byte-unchanged.
9. `go test ./cmd` is green, including the verbatim shim expectation in `cmd/init_test.go`.

STATUS: issues_found

SPEC CONTEXT:
The specification (§2.2, restated at §8.3, and the Corrigendum 2026-09-14 at specification.md:484) fixes the configurations in which Portal answers a bash Tab at all: `compopt` must be a builtin (bash 4.0+) and the `bash-completion` package must be present, because cobra's generated function reaches `_get_comp_words_by_ref` from that package before any completion runs. Those are exactly the configurations in which this task's defect — a cursor pinned to end-of-line by the shim, so `cur` becomes the tail of the line rather than the word under the cursor — is visible. The spec does not itself legislate cursor handling; it states that `x /tm<TAB>` must behave as `portal open /tm<TAB>` does once the corrected `portal init` output is live, which is the property this task restores for a cursor left mid-line.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/init.go:65-72` (`bashOpenCompletionShim`), specifically `:69` (`COMP_LINE=${COMP_LINE/"$typed"/$expansion}`) and `:70` (`(( COMP_POINT += ${#expansion} - ${#typed} ))`); `cmd/init.go:66` captures `typed` from `COMP_WORDS[0]` before the array is rewritten at `:67`. Comment at `cmd/init.go:59-64`.
- Notes:
  - The edit does exactly what the task prescribed: substitute into the typed line rather than re-join the word array, and shift the offset rather than assign it. Traced by hand against the emitted script and the driver:
    - mid-line (`x /po extra`, cursor at end of `/po`, `COMP_POINT=5`): line becomes `portal open /po extra`, point becomes 5 + 11 − 1 = 15, the walk lands the current word at offset 12, so `cur` = `/po`.
    - pre-edit shim on the same input: line `portal open /po extra`, point pinned to 21, `cur` = `/po extra` — so the new suite case genuinely discriminates (criterion 4's behavioural half holds).
    - spacing (`x   api`): `${COMP_LINE/"$typed"/$expansion}` touches only the function name, so the line is `portal open   api` (criterion 6).
    - leading whitespace (`   x /po`): the replacement is unanchored, so it lands on the first occurrence of the typed name wherever it sits; point 8 + 10 = 18, walk start 15, `cur` = `/po`. Handled rather than merely stated (criterion 7 satisfied by handling, with the limit-free behaviour also stated at `cmd/init.go:63`).
    - end-of-line inputs are byte-identical to the pre-edit result for every existing case (`x /po`, `x ` with an empty trailing word, `x -s `, `p /po`), so no regression in the common path.
  - Criterion 5 holds: `COMP_POINT=${#COMP_LINE}` is gone from the shim, and cobra's generated body assigns `COMP_POINT` nowhere.
  - Criterion 8 holds: `git show 4067537e1` touches `bashOpenCompletionShim` alone in `cmd/init.go`; `zshOpenCompletionShim` (`cmd/init.go:75-80`), the fish emission and both `complete -o default -F __start_portal …` registrations are untouched.
  - The later `openFunctionExpansion` single-sourcing (`cmd/init.go:54`, `%[1]s` at `:66-67`) post-dates this task and emits the same bytes; the verbatim expectation in `cmd/init_test.go:148` still matches.
  - Criteria 1, 2 and the recording half of 4 are unmet — see FINDINGS.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/init_completion_shell_test.go:36-48` replaces the stand-in's `cur=${COMP_WORDS[COMP_CWORD]}` with the cursor walk (`:43`), which is the half of `bash-completion` the old driver modelled away; `:52-55` takes `COMP_CWORD`/`COMP_LINE`/`COMP_POINT` from the Go side rather than pinning them, and `:60-61` reports the line and offset the shim left behind.
  - `completionLine` + the `cursorAfterWord` / `typedLine` options (`:118-168`) default to a single-space line with the cursor at its end, so all six pre-existing call sites are unchanged — I re-derived `cur` for each under the new stand-in and each still resolves to the word it asserts on (criterion 3).
  - Four new bash-only cases: `:430` (request is `__complete|open|/po` and the stub candidate is offered for a mid-line cursor), `:442` (`COMP_POINT` = 15, i.e. shifted not pinned), `:452` (`x   api` → `portal open   api`), `:461` (leading whitespace).
  - The mid-line case would fail against the pre-edit shim (derived above), so the guard can fail for the reason it names — the explicit obligation the task carried.
- Notes:
  - Not over-tested: `:430` and `:442` drive the same line but assert different observables (the request Portal received vs. the offset the shell was left holding), and a single combined case would report only the first failure.
  - `cursorAfterWord` is a no-op for the zsh and fish drivers (`CURRENT=$#` at `:88`, and fish's driver reads only the line), but no zsh/fish case uses it and the comment at `:427-429` states why the cursor cases are bash's alone — nothing vacuous exists today.
  - The stand-in skips only literal spaces (`:39`) where the real package skips `[[:space:]]`, and it performs no `COMP_WORDBREAKS` reassembly. Both are deliberate simplifications of a model the suite documents as a stand-in, and neither is reachable from the cases driven.

CODE QUALITY:
- Project conventions: Followed. The shell test stays unit-lane-legal (no portal binary built or run, no tmux contacted, `--noprofile --norc`, temp dirs only), which CLAUDE.md's lane rule requires. Test helper options follow the package's existing functional-option shape; `t.Parallel()` is correctly absent.
- SOLID principles: Good. `completionLine` isolates "what line is this Tab taken on" from `driveCompletion`'s staging, so the cursor and the spacing became one concept rather than two parameters threaded through every call site.
- Complexity: Low. The shim is two added lines plus one `local`; `point()` is a single forward walk.
- Modern idioms: Yes.
- Readability: Good. The `cmd/init.go:59-64` comment states the three reasons the rewrite takes the form it does (matching words, spacing, cursor) without restating the code, and `cmd/init_completion_shell_test.go:33-35` names what the stand-in now models and why.
- Comment accuracy: Verified. `cmd/init.go:63`'s "the replacement is unanchored so it also lands on a line that begins with whitespace" holds against `${COMP_LINE/"$typed"/$expansion}`; `cmd/init_completion_shell_test.go:444`'s arithmetic ("`x /po` is 5 characters and `portal open /po` is 15") is correct; the `completionRun` doc's "bash alone rewrites either, so the other shells report both empty" matches the three drivers.
- Security: N/A — no user input reaches an interpreter that did not already have it; the pattern half of the substitution is quoted, so a glob character in a `--cmd` name stays literal.
- Performance: N/A.
- Issues: None beyond the finding below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] cmd/init.go:65-72 — the task gated this edit on a measurement against a real `bash-completion` install ("Measure before changing… If the measurement contradicts the derivation, the finding is void and the task closes with `cmd/init.go` unedited"), and no such measurement is recorded anywhere: `tick show tick-aceea8` carries zero notes, both of the task's commits (`4da790b0f` record, `4067537e1` implementation) have single-line messages and no body, `.workflows/open-with-forced-filter/implementation/open-with-forced-filter/` has a `fix-tracking-*` file for 5-2, 5-3 and 5-4 but none for 5-5, and no file under `.workflows/open-with-forced-filter/` names a `bash-completion` version, a measured `cur`, a `COMPREPLY` or a filename fallback for this task. The machine still lacks the package (`/usr/share/bash-completion` absent; `/opt/homebrew/share/bash-completion/` holds only a `completions` drop directory, with no `bash_completion` script defining `_init_completion`), so the precondition the task stated has not changed. The fix: run the staging the task prescribes on a bash that has the package — the emitted `portal init bash` script over the recording stub, **no** `_init_completion` declared, `COMP_WORDS=(x /po extra)` / `COMP_CWORD=1` / `COMP_LINE='x /po extra'` with `COMP_POINT` at the end of `/po`, calling `__start_portal_open` — and record the versions, the traced `cur`, the recorded request, `COMPREPLY` and whether filenames appeared, against both the pre-edit and post-edit shim; if the measured `cur` is the word under the cursor rather than the tail of the line, revert `cmd/init.go:66,69-70` to the pinned form per criterion 2. — FAILS: the only evidence that the shipped shell integration needed changing is a reading of cobra's and bash-completion's sources, and the guard that now protects it (`cmd/init_completion_shell_test.go:36-48`) was authored from that same reading — it models the derivation in, so it confirms the shim agrees with the stand-in, never that either agrees with the package a user actually has. Nothing in the tree can detect the derivation being wrong, the task's own void-condition can no longer be triggered, and a later reader cannot tell whether the gate was passed or skipped.

UNSETTLED:
- "A measurement contradicting the derivation closes the task with `cmd/init.go` unedited and that recorded; no further criterion applies in that case." — Settling this needs a bash with the `bash-completion` package installed (a container or a local install; absent on this machine), driving the emitted `portal init bash` script with no `_init_completion` declared, and reading back the `cur` cobra traces to `BASH_COMP_DEBUG_FILE` for a mid-line cursor. Reading alone cannot decide whether the real `__get_cword_at_cursor_by_ref` truncates at `COMP_POINT` as derived.
- "`go test ./cmd` is green, including the verbatim shim expectation in `cmd/init_test.go`." — Needs `go test ./cmd` executed; the four new shell-driven cases in particular run real `bash`/`zsh`/`fish` subprocesses, which reading cannot stand in for.
