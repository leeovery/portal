TASK: open-with-forced-filter-5-2 (tick-a6b112) — Ask Portal for open's completions after the session-opening function

ACCEPTANCE CRITERIA: [from the plan task]
- Driven through the emitted bash script, Tab after the session-opening function records the request `portal __complete open …` — for `x /po`, `x ""` and `x -s ""` alike — never `portal open __complete …`
- The same holds through the emitted zsh script
- In fish, completion after the function asks for `open`'s completions and offers live session names
- The control function's request is unchanged (`portal __complete …`) in all three shells
- The correction follows `portal init --cmd <name>`: with `--cmd p`, `p` gets the corrected registration and `pctl` the unchanged one, and the literal `x` appears nowhere in the emitted registrations
- Candidates carrying a leading `/` survive the shell's own prefix filter into the completion reply
- Filename fallback stays off on that word in every shell: `x /tm<TAB>` never gains a trailing slash and never turns into `/tmp/`
- A path argument after the function (`x ~/Code/pro<TAB>`) no longer completes filenames — the deliberate loss
- The emitted function bodies are unchanged
- No portal binary is built or exec'd by the suite and no tmux server is contacted; the suite runs in `go test ./...`
- A shell binary absent from the machine skips its own subtest rather than failing the suite
- `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
§8.2 names the pre-existing defect: cobra's emitted script builds its completion request from the *typed word* (`requestComp="${words[0]} __complete ${args[*]}"` in bash, `${words[1]}` in zsh), and the typed word `x` expands to `portal open` — so Tab after `x` ran `portal open __complete …`, a real `open` invocation that never reached the completer. §8.3 requires `portal init` be corrected so the request resolves to `portal __complete open …` in bash, zsh and fish, following the `--cmd <name>` configured name rather than the literal `x`, leaving the control function (`xctl`) alone, and accepting the loss of filename fallback on that word (the same contract `portal open` already has). §8.4 explains the fold-in; §8.5 records the rollout consequence (an existing shell picks the fix up only on re-eval). The 2026-09-14 corrigendum qualifies the "fallback off" claim: the switch-off is the shell's, and bash 3.2 without `bash-completion` cannot perform it.

IMPLEMENTATION:
- Status: Implemented (with two approved downstream refinements folded in)
- Location:
  - `cmd/init.go:54` — `openFunctionExpansion = "portal open"`, the single declaration both the emitted function body and the shims read (landed by task 9-4, `b98692f50`)
  - `cmd/init.go:65-73` — `bashOpenCompletionShim`: rewrites `COMP_WORDS`, `COMP_CWORD`, `COMP_LINE` and `COMP_POINT` before delegating to `__start_portal`
  - `cmd/init.go:75-80` — `zshOpenCompletionShim`: rewrites `words` and `CURRENT` before delegating to `_portal`
  - `cmd/init.go:102-110` — bash: shim emitted after `GenBashCompletionV2`, then `complete -o default -F __start_portal_open <cmdName>`, with the `ctlName` line left at `__start_portal`
  - `cmd/init.go:135-143` — fish: `complete -c <cmdName> -f` then `complete -c <cmdName> -w 'portal open'`, `ctlName` left wrapping bare `portal`
  - `cmd/init.go:168-177` — zsh: shim emitted, then `compdef _portal_open <cmdName>`, `compdef _portal <ctlName>` unchanged
- Notes:
  - Verified the mechanism against the vendored generator rather than trusting the plan's prose. `bash_completionsV2.go:428-436` (cobra v1.10.2) shows `__start_portal` declaring `local cur prev words cword` and deriving them through `_init_completion`, which reads `COMP_WORDS`/`COMP_LINE`/`COMP_POINT` — so rewriting all four is the correct seam, and rewriting `COMP_WORDS` alone would have left `cur` derived from a line that disagreed with it. `zsh_completions.go:124-140` shows `_portal` truncating `words=("${=words[1,CURRENT]}")` and then composing `requestComp="${words[1]} __complete ${words[2,-1]}"` — so prepending two words and incrementing `CURRENT` by one (net +1 after the typed word is consumed) lands the request on `portal __complete open …`.
  - The bash shim diverges from the plan's literal text (`COMP_LINE="${COMP_WORDS[*]}"` / `COMP_POINT=${#COMP_LINE}`). That divergence is the deliberate product of task 5-5 (`4067537e1`, "Keep the user's cursor position through the bash completion shim"): rebuilding the line would normalise the user's own spacing and pinning the offset to the end would move a cursor left mid-line. Both replacements are pinned by tests, and `TestInitBash_EmitsNoPinnedCompletionCursor` (`cmd/init_test.go:494`) guards the pinned form from returning. Sound, and better than what was written.
  - `-o default` is kept on the new `cmdName` registration exactly as the plan required, and the `ctlName` lines in all three shells are byte-identical to what they were.
  - The emitted registration for a `--cmd portal` user lands after cobra's own `complete … portal` / `compdef _portal portal`, so it wins — which is what the edge case predicted; the emission order in `emitBashInit`/`emitZshInit` is what makes that true.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/init_completion_shell_test.go` drives the real `bash`/`zsh`/`fish` over the emitted script with a recording `#!/bin/sh` stub named `portal` first on `PATH` (`:35-39`), so no portal binary is built or exec'd and no tmux server is contacted — the criterion holds by construction. The driver shells run `--noprofile --norc` / `-f` / `--no-config` and write only into `t.TempDir()`, so the developer's environment is untouched.
  - `TestInitCompletion_AsksPortalForOpenCompletions` (`:317`) covers all three named inputs (`x /po`, `x ""`, `x -s ""`) across all three shells; the `|` field separator (`:22-26`) is what keeps the trailing empty word visible in the recorded request, which a space-joined record would have lost.
  - `TestInitCompletion_LeavesControlFunctionRequestUnchanged` (`:353`), `…_CarriesSlashPrefixedCandidateIntoReply` (`:366`), `…_FollowsTheConfiguredFunctionName` (`:403`), `…_SkipsAnAbsentShell` (`:416`) map one-to-one onto the remaining criteria.
  - `…_OffersNoFilenameForPartialAbsoluteDirectory` (`:378`) asserts the switch-off per shell in the shell's own terms — the reply carries nothing under `/tm`, the compsys stand-ins record no file-completion call, and cobra's debug channel carries "Activating no file completion" (bash) / "deactivating file completion" (zsh). I checked both strings against the generator (`bash_completionsV2.go:132-135`, `zsh_completions.go:281`); the bash proxy is necessary because `-o default` filenames are readline's and never appear in `COMPREPLY`, and the test says so in place.
  - The three cursor/spacing tests (`:430`, `:442`, `:452`) and `…_CompletesOnALineWithLeadingWhitespace` (`:461`) each name a property of the substitution form and would fail if it were replaced by the rebuild-and-pin form.
  - `cmd/init_test.go` carries the emitted-text tables for all six required cases (`TestInitBash` `:144-152`, `TestInitZsh` `:44-52`, `TestInitFish` `:247-251`, and the three `_CmdFlag` siblings `:200`, `:386`, `:247`), plus `TestInitCmdFlag_RegistrationsCarryNoDefaultName` (`:437`) for the "no literal `x`" criterion and the `--cmd portal` shadowing row (`:400-408`).
  - `resetRootCmd` (`cmd/root_test.go:36`) resets `--cmd` to `x`, so a `--cmd p` run cannot leak into a later default assertion.
- Notes:
  - The `len(run.fileComp) != 0` arm of the no-filename test only has teeth under zsh (bash and fish drivers declare no `_files`/`_arguments` stand-ins). It costs nothing and the per-shell arms beside it carry the assertion; not a gap worth acting on.
  - `driveCompletion` reports a driver failure with stdout only (`:277`); `cmd.Output()` puts the shell's stderr on the `*exec.ExitError`, so a failing run gives a thinner diagnostic than it could. Cosmetic.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`; the suite is unit-lane and untagged, which is correct — the lane rule bars building/exec'ing the portal binary and spawning daemons, none of which this does. Emission stays in `cmd/init.go` beside its siblings.
- SOLID principles: Good. The shims are package constants consumed by one emitter each; `openFunctionExpansion` gives the function body and its completion shim one declaration, so the two cannot drift into asking for a command line nothing runs.
- Complexity: Low. Each emitter is a straight sequence of `Fprintf`s with checked errors.
- Modern idioms: Yes — `strings.SplitSeq`, `strings.CutPrefix`, `slices.Contains` in the test; indexed format verbs (`%[1]s`) where the shim repeats its argument.
- Readability: Good. The test's named types (`completionRun`, `completionLine`, `completionOption`) turn a shell-harness into something readable, and the functional options spell the two departures from "words joined by a single space with the cursor at the end".
- Issues: None. I checked every comment in the changed code against the code it sits on: the `bashOpenCompletionShim` block's four claims (the typed-word request, line-must-match-words, the unanchored replacement, the format-string escape) each hold, and the `openFunctionExpansion` comment describes both of its consumers accurately. No comment references a task id, phase or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "In fish, completion after the function asks for `open`'s completions and offers live session names" — fish is not installed on this machine (`command -v fish` resolves nothing), so every fish subtest in `cmd/init_completion_shell_test.go` takes the `requireShell` skip. The emitted-text assertions (`TestInitFish`, `TestInitFish_CmdFlag`) do run and are correct by reading. Settling the live leg needs `go test ./cmd -run TestInitCompletion` on a machine with fish installed, or a manual `fish -c` run over the emitted script.
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — not settleable by reading. One detail rests on it specifically: for the zsh "an empty word" case to record `__complete|open|`, cobra's `words=("${=words[1,CURRENT]}")` truncation must preserve the trailing empty word so its `lastChar` test appends `""`. The test asserts exactly that, so a run of `go test ./cmd -run TestInitCompletion_AsksPortalForOpenCompletions` settles it either way.
