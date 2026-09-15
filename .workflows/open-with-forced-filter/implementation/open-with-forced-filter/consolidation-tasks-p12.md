# Consolidation Tasks: Open With Forced Filter (Phase 12)

## Task 1: Owe the terminal every warning nobody surfaced, however the picker exits
placement: phase 12
severity: behaviour

**Problem**: Pressing Ctrl-C on the loading page throws away every soft bootstrap warning the run accumulated. Phase 12's task 1 made the bootstrap's terminal event move the staged set into the buffered one and empty the staged field (`internal/tui/model.go:1671-1677`); `WarningsOwedAtTeardown` (`model.go:481-493`) answers with the buffered set only when the search decision recorded an attach or a read failure, and with the staged set otherwise — which by then is nil. So a cancel after the bootstrap completed but before the picker painted writes nothing: the user is never told the saver is down or that restore degraded, and no surface records that a warning existed. Phase 12's task 3 made that window matter. Before it, the cancel window was the sliver between the bootstrap completing and the picker painting — bounded by the 1.2s loading pad. After it, cancelling on the loading page is the *designed* escape from a decision waiting on a slow tmux server, so the window is unbounded and the user most likely to hit it is the one whose server is misbehaving — exactly the user a saver-down warning is for.

**Solution**: Have the teardown owe the union of both sets rather than choosing between them. Each field is emptied by whoever consumes it (`internal/tui/bootstrap_warnings.go:41-66`), so the union already means "what nobody surfaced" on every path that exists: on a search attach or read failure the staged half is nil and the answer is the buffered set as today; on a painted picker the buffered half is nil and the answer is the staged set as today; on a cancelled loading page the buffered half now carries what the gate took, which is the hole. The discriminator goes with it, so the teardown stops asking which exit this was and asks only what is still unsurfaced. Behaviour-preserving on every route but the cancel.

**Outcome**: Whatever the picker's exit, a warning nobody put on screen reaches the terminal — including the cancel that phase 12 turned into the standard way out of a slow decision.

**Do**:
- Collapse `WarningsOwedAtTeardown` (`internal/tui/model.go:488-493`) to `slices.Concat(m.bufferedWarnings, m.pendingBootstrapWarnings)`, dropping the `searchAttached || searchErr != nil` branch entirely. `slices` is already imported for the `BootstrapCompleteMsg` arm at `:1674`.
- Replace its doc comment (`:481-487`) so it states the union rule instead of the discriminator: each field is emptied by whoever consumes it — the notice band and the stderr flush (`internal/tui/bootstrap_warnings.go:41-66`) and the loading gate's take (`internal/tui/model.go:1671-1677`) — so a set still sitting in either field at teardown is one nobody surfaced. The paragraph claiming a cancelled loading page is owed nothing goes with the branch.
- Invert `cmd/open_search_warnings_test.go`'s `"it owes nothing when the loading page was cancelled"` (`:270-291`): it drives the cancel and asserts nil today, and must now assert the buffered set through the suite's `assertOwed` helper, renamed and re-reasoned to what it pins. Keep its two pre-assertions (the buffer is non-empty; neither an attach nor an error was recorded).
- Add a sibling subtest for the in-flight cancel — the unbounded window phase 12's task 3 created: drive `tea.WindowSizeMsg`, `tui.LoadingMinElapsedMsg`, then `tui.BootstrapCompleteMsg{Warnings: …}`, leave the dispatched decision command unrun, send Ctrl-C, and assert the same warnings are owed.
- Change nothing else: `cmd/open.go:626` is the sole production caller and its call stays as it is, and the accessors at `:471-479` are untouched.

**Acceptance Criteria**:
- [ ] `WarningsOwedAtTeardown` returns the concatenation of `bufferedWarnings` and `pendingBootstrapWarnings` and reads neither `searchAttached` nor `searchErr`.
- [ ] Ctrl-C on the loading page after the bootstrap completed owes the warnings the gate took — both before the search decision is dispatched and while it is in flight.
- [ ] The four unchanged exits keep today's answer: a search attach owes the buffered set (including a warning staged before the concurrent launch), a failed session-list read owes the buffered set, a warm picker owes the staged set, a painted picker owes nothing.
- [ ] Every `TestWarningsOwedAtTeardown` subtest other than the cancel one passes with no edit.
- [ ] `go test ./...` is green and `golangci-lint run` reports nothing new.

**Tests**:
- `"it owes the buffered set when the loading page was cancelled"` — the inverted subtest; the cancel now writes what the gate took.
- `"it owes the buffered set when the loading page was cancelled with the decision still in flight"` — the escape from a slow tmux server that phase 12 made the designed one.
- `"it owes the buffered set when the loading gate quit on a named session"` — unchanged, must stay green.
- `"it still owes a staged warning at teardown when the gate quits on a search attach"` — unchanged, must stay green: the union must not reorder or drop the staged half the gate folded in.
- `"it owes nothing once a loading gate has surfaced the buffer"` — unchanged, must stay green: a surfaced buffer is an emptied one, so the union answers with nothing.

## Task 2: Let a word past the separator complete as the trailing command's own
placement: phase 12
severity: behaviour

**Problem**: Tab does nothing for a path argument to the trailing command. `portal open ~/Code/api -- ls src/<TAB>` offers no candidates at all: the word routes to the session-name completer, whose answer never prefix-matches a path fragment, and whose directive suppresses the shell's filename fallback (`cmd/completion.go:39`, the same suppression the search arm carries at `:82`). The user gets silence where every other command completes files.

The routing itself is phase 11's; what this phase authored is the reasoning for it, and that reasoning is false. `completeOpenPositional`'s doc comment (`cmd/completion.go:101-103`) and the specification twice (§8.1 at `:332` and `:334`) credit the separator bound with saving "the filenames the user actually wanted" — but the arm the bound *selects* suppresses them exactly as the arm it rejects would, so the bound preserves no filename. The 2026-09-15 corrigendum at `:494` states both halves in one sentence and draws the two-wrongs conclusion anyway.

**Solution**: Answer a post-separator word with the default directive, so the shell's own filename completion runs. The specification already settles which way this goes: §2.3 states the words after a separator are "the trailing command's own, passed to that command untouched", and §8.1 twice names the loss of those filenames as a cost worth avoiding. A word Portal passes untouched is one Portal should neither complete nor suppress — the session-name arm is the right *candidate* set to offer (nothing), and `NoFileComp` is the wrong *directive* to offer it with.

Scope is the post-separator word alone. Every pre-separator positional keeps `NoFileComp`: that is the stated contract for `portal open`'s own arguments (README's "a path argument after the function does not fall through to filename completion, which is the contract `portal open` has always had"), and this task does not touch it.

With the behaviour corrected, the reasoning becomes true and only needs its self-contradiction removed: the corrigendum at `:494` says the selected arm suppresses filenames, which stops being so.

**Outcome**: Tab completes paths for a trailing command the way it completes them everywhere else, and the reason the specification gives for the separator bound is one the code actually delivers.

**Do**:
- In `completeOpenPositional` (`cmd/completion.go:104-109`), answer a word `completingPreDashPositional` reports false for with no candidates and `cobra.ShellCompDirectiveDefault`, ahead of the session-name fallthrough. Leave both existing arms as they are: the sigil arm, the pre-dash session-name arm, and `completeSessionNames`/`completeSearchTerm`'s own `NoFileComp` (`:39`, `:82`) are untouched, and the boundary word immediately after the separator still reports pre-dash through `cmd/open_search.go:46-49` and keeps today's two arms.
- Rewrite the doc comment at `:85-103` to state what the code now does — past the separator the word is the trailing command's own, so Portal offers it nothing and leaves the shell's filename completion on; before the separator every positional keeps the session-name arm and its suppression. Keep the boundary-word exception and the "the separator is the only bound" paragraph.
- Replace `cmd/completion_test.go`'s `"it answers a post-dash word exactly as the plain session-name completer does"` (`:597-609`): it pins the post-dash directive and candidates as identical to `completeSessionNames`', which is exactly what stops holding. Pin the directive as `cobra.ShellCompDirectiveDefault` and the candidate list as empty, and add the reported failure — `__complete open ~/Code/api -- ls src/` — as its own subtest beside it.
- Correct the specification in place (`.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`): in §8.1's separator paragraph (`:332`), the clause "so it is completed against session names like any other word rather than offered sigil candidates" is falsified by this change — it must instead state that such a word is answered with no candidates and the shell's own filename completion left on, with the two-part reason after the dash kept verbatim. In the 2026-09-15 corrigendum at `:494`, remove the self-contradiction: the clause "offering **session names** there also suppresses the filenames the user wanted" stops being true, while the "corrects two wrongs" conclusion it introduces now holds and stays. Leave `:334`, `:340`, `:376` and every other corrigendum entry byte-untouched — `:334`'s conclusion becomes true under this change, and the earlier entries are dated records the new one supersedes.
- Append one dated corrigendum entry at the end of that file's `## Corrigenda` section, quoting the two corrected claims and stating what is true (a post-separator word is answered with no candidates and `cobra.ShellCompDirectiveDefault`, so the shell's own filename completion runs and the bound now preserves the filenames §8.1 twice names), in the form `> **Corrigendum {YYYY-MM-DD}** (from `implementation/open-with-forced-filter`): …` — the date read from `date` on this machine when the edit lands — then re-index with `node .claude/skills/workflow-knowledge/scripts/knowledge.cjs index .workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`.

**Acceptance Criteria**:
- [ ] `portal __complete open ~/Code/api -- ls src/` answers with no candidates and `cobra.ShellCompDirectiveDefault`, so the shell's filename completion runs.
- [ ] Every pre-separator positional is unchanged: session-name candidates with `ShellCompDirectiveNoFileComp` for a plain word, a word beside another target, and a word on a line carrying `-f`.
- [ ] The boundary word immediately after the separator is unchanged: `open -- /po` and `open api -- /po` still offer sigil candidates.
- [ ] `completeSessionNames` and `completeSearchTerm` are untouched and still return `NoFileComp`, and `completeOpenPositional`'s doc comment no longer claims a post-separator word is completed against session names.
- [ ] §8.1's separator paragraph states the post-separator answer as no candidates plus the shell's filename completion; the 2026-09-15 corrigendum no longer claims the arm the bound selects suppresses filenames; one new dated corrigendum entry records the correction; the knowledge re-index has run and its store changes are committed with the task rather than left dirty.
- [ ] `go test ./...` is green and `golangci-lint run` reports nothing new.

**Tests**:
- `"it leaves filename completion on for a path argument to the trailing command"` — `open ~/Code/api -- ls src/` answers with no candidates and `ShellCompDirectiveDefault`.
- `"it offers no session names for a word among a trailing command's arguments"` — the replacement for the identity-with-`completeSessionNames` subtest: `open ~/Code/api -- ls /po` answers empty, not with the two live session names.
- `"it offers no sigil completion for a word among a trailing command's arguments"` — unchanged, must stay green.
- `"it keeps the sigil arm for the word immediately after the separator"` and `"it keeps the sigil arm for the boundary word beside a target"` — unchanged, must stay green: the new arm must not swallow the boundary position.
- `"it still offers sigil completions for a pre-dash word beside another target"` and `"it leaves a second positional on today's behaviour"` — unchanged, must stay green: the pre-separator `NoFileComp` contract is not touched.
