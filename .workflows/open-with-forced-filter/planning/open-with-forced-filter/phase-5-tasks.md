---
phase: 5
phase_name: Completion and Documentation
total: 4
---

# Phase 5: Completion and Documentation — 4 tasks

## open-with-forced-filter-5-1

### Task 5-1: Complete the search term after the slash against live session names

**Problem**: `openCmd.ValidArgsFunction` (`cmd/open.go:730`) hands every positional word to `completeSessionNames` (`cmd/completion.go:24`), which keeps the live session names the typed word prefixes. A word beginning with `/` prefixes no session name, so the form completes to nothing — probed against the shipped binary, `portal __complete open /po` returns zero candidates and `ShellCompDirectiveNoFileComp`, while `portal __complete open ""` returns the live names. Tab is therefore silently inert on exactly the shape this feature exists for: the user types `/po`, presses Tab, and has to type the rest of a `{project}-{nanoid}` name they cannot recall — which is the thing the search form was built to stop them doing.

**Solution**: A search-form branch on `open`'s positional completer that completes the term after — and excluding — the `/` against live session names, and offers each candidate with the slash still on the front.

**Outcome**: `portal __complete open /po` offers `/portal-a1b2` rather than `portal-a1b2`, `portal __complete open /` offers every live session name each carrying the slash, a term prefixing no name offers nothing, a session name containing a `/` is never offered, and every branch answers `ShellCompDirectiveNoFileComp`; a non-search word completes exactly as it does today.

**Do**:
- Add `func completeSearchTerm(toComplete string) ([]string, cobra.ShellCompDirective)` to `cmd/completion.go`: take `term := resolver.SearchTerm(toComplete)`, walk `completionSessionNames()`, skip any name containing `/`, keep the names satisfying `strings.HasPrefix(name, term)`, and return each as `"/" + name` with `cobra.ShellCompDirectiveNoFileComp`.
- Add `func completeOpenPositional(toComplete string) ([]string, cobra.ShellCompDirective)` beside it: `completeSearchTerm(toComplete)` when `resolver.IsSearchSigil(toComplete)` (task 1-4), otherwise `completeSessionNames(toComplete)`.
- Point `openCmd.ValidArgsFunction` (`cmd/open.go:730`) at `completeOpenPositional`, leaving `completeSessionNames` itself, `killCmd`'s completer, the `--session` and `--alias` flag completers, and the deliberately-absent `--path` / `--zoxide` completers untouched.
- Read the names through the existing `completionSessionNames` seam rather than a second client: it is what keeps the completer off the context-injected client on the bootstrap-exempt `__complete` path and what turns a failed read into nil.
- Extend `cmd/completion_test.go`: a `TestCompleteSearchTerm` table staging names through `withCompletionSessionNames`, plus wiring subtests driving `openCmd.ValidArgsFunction` directly and the end-to-end `completionCandidates(t, "__complete", "open", "/po")` helper already in that file.

**Acceptance Criteria**:
- [ ] `/po` offers the live session names prefixed by `po`, each returned with the leading `/` — `/portal-a1b2`, never `portal-a1b2`
- [ ] `/` offers every live session name, each carrying the slash
- [ ] `/ort` offers nothing (the offer is prefix-shaped) while the term itself still matches `portal-a1b2` by containment at run time — the phase-2 rule is untouched
- [ ] A live session whose name contains a `/` is never offered, for any term including the empty one
- [ ] `_portal-saver` and `_portal-bootstrap` are never offered, with no filter of their own added here
- [ ] No directory, path fragment or filesystem entry is ever offered on this branch
- [ ] A non-search word completes byte-identically to today: `/tmp/`, `~/Code/pro`, `.`, a bare word, and the empty word all route to `completeSessionNames` unchanged
- [ ] Every branch — including the zero-candidate ones — returns `cobra.ShellCompDirectiveNoFileComp`
- [ ] A tmux read failure yields no candidates and no error (the seam's nil), with the directive still `NoFileComp`
- [ ] The completer builds its own client through the existing seam and needs no context-injected client, so the bootstrap-exempt `__complete` path still works
- [ ] Prefix matching stays a byte prefix, as `completeSessionNames` already does — no case folding and no containment is introduced on the offer side
- [ ] `go test ./...` passes

**Tests**:
- `"it completes the term after the slash and keeps the slash on the candidate"` — names `portal-a1b2`, `web-9`; term `/po` → `["/portal-a1b2"]`
- `"it offers every live session name for a bare slash"` — `/` → all names, each slash-prefixed
- `"it offers nothing for a term that prefixes no name"` — `/ort`, empty result, `NoFileComp`
- `"it never offers a session name containing a slash"` — names `foo/bar` and `foo-1`, term `/foo` → only `/foo-1`
- `"it offers no candidates when the session read fails"` — seam returning nil
- `"it leaves a non-search word on today's completer"` — table over `/tmp/`, `~/Code/pro`, `.`, `work`, `""`, asserting the plain session-name behaviour and `NoFileComp`
- `"it returns NoFileComp on every branch"` — search hit, search miss, non-search word, read failure
- `"it routes open's positional completer through the search branch"` — `openCmd.ValidArgsFunction(openCmd, nil, "/po")`
- `"it answers a search word end to end through __complete"` — `completionCandidates(t, "__complete", "open", "/po")`
- `"it leaves kill's positional completer unchanged"` — `/po` against `killCmd` offers nothing new

**Edge Cases**:
- The shell discards any candidate that is not an extension of the word being completed, which is why the slash must ride on every offered word — driven against the emitted bash script with a stub answering `/portal-a1b2`, the candidate survives into `COMPREPLY`; the same stub answering `portal-a1b2` yields an empty reply
- `IsSearchSigil` is false for the empty string, so an empty word still takes the plain session-name branch and offers everything — the two branches agree there rather than competing
- A word carrying a second slash (`/Users/lee`, `/tmp/`) is not the shape and must not be answered with session names carrying slashes stitched back on; it falls to the existing completer, which offers nothing for it
- A completed directory would read as a path and trip the no-second-slash rule straight back into path territory, which is why directories are excluded from what is offered even though they count for matching
- Completion for `-s <TAB>` is a flag value rather than a positional and keeps the plain name completer: `-s /po` offers nothing, exactly as today
- What `open` should offer for a *second* positional after a search form (`open /term <TAB>`) is not settled by the specification — the composition rule refuses such a line at run time, but the completer is not the refuser. This task changes nothing there: the second word takes today's behaviour
- `ListSessionNames` inherits `ListSessions`' underscore-prefix filter at source, so Portal's own internal sessions are absent from the offered set already; restating that filter here would be a second home for a Portal-wide invariant

**Context**:
> `/po<TAB>` completes the term after — and excluding — the `/`, against live session names, leaving the sigil in place. `/po` completing to `/portal-a1b2` normally leaves that session as the only match, which attaches outright, so `/po<TAB><Enter>` becomes the whole interaction; where a second session's name or recorded directory also contains the completed name, the same keystrokes land in the picker on those two.
>
> Offered words carry the sigil — `/po` completes to `/portal-a1b2`, never to `portal-a1b2`, which would replace the whole word and drop the slash the user typed. The words offered are the live session names the typed term prefixes: the shell discards any candidate that is not an extension of the word being completed, so completion is prefix-shaped even though the form itself matches by containment. `/ort<TAB>` therefore offers nothing, while `/ort` still finds `portal-a1b2` on Enter.
>
> `/<TAB>` — the sigil with nothing after it — offers every live session name, since every name is prefixed by the empty term. Left as typed it opens the picker on an empty filter; accepting a completion instead narrows the term to one session and attaches it, which is what makes completing the bare slash worth doing.
>
> Directories are deliberately excluded from what is offered, even though they count for matching: a completed `/Users/leeovery/Code/portal` reads as a path and trips the no-second-slash rule straight back into path territory. A live session name carrying a `/` is held back for the same reason — tmux permits one, and completing `/foo` to `/foo/bar` would produce a word with a second slash. Such a session stays reachable: a term stopping short of the slash matches it by containment like any other. It is only never offered.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.2, §8.1

## open-with-forced-filter-5-2

### Task 5-2: Ask Portal for `open`'s completions after the session-opening function

**Problem**: Task 5-1's branch is unreachable from the shell function the feature is typed through. Cobra's emitted script asks the **typed word** for its completions: bash builds `requestComp="${words[0]} __complete ${args[*]}"` (`portal init bash`, line 29) and zsh `requestComp="${words[1]} __complete ${words[2,-1]}"` (line 52). The typed word is `x`, and `x` expands to `portal open` — so Tab after `x` runs `portal open __complete …`, a real `open` invocation carrying `__complete` as a positional target against the live tmux server, which returns no completion output, which is why the shell falls through to filenames. Driven against the emitted bash script with a recording stub on `PATH`, `x /po<TAB>` records `portal open __complete /po` while `xctl <TAB>` records `portal __complete` — the pair differ despite carrying identical registrations, because `xctl` expands to bare `portal`. Fish has its own version of the same fault: `complete -c x -w portal` wraps the line to bare `portal`, so it completes `portal`'s own arguments and never `open`'s. This is a pre-existing defect, folded in here because without it the completion of the search form is reachable only by typing `portal open /term` in full.

**Solution**: Correct what `portal init` emits so that the completion request for the session-opening function resolves to `portal __complete open …` — a word-rewriting shim delegating to the generated entry point in bash and zsh, and the wrap target in fish — leaving the control function's registration alone.

**Outcome**: Tab after the session-opening function reaches `open`'s completer in bash, zsh and fish, honouring whatever name `portal init --cmd <name>` chose; `x /po<TAB>` offers live session names, `x <TAB>` offers them too, the filename fallback stays off on that word, and `xctl` and the emitted function bodies are untouched.

**Do**:
- In `emitBashInit` (`cmd/init.go:51`), after the `GenBashCompletionV2` output and in place of the `complete -o default -F __start_portal %s` line for `cmdName` (`:71`), emit a fixed-name shim and register it for `cmdName` only:
  ```
  __start_portal_open() {
      COMP_WORDS=(portal open "${COMP_WORDS[@]:1}")
      (( COMP_CWORD += 1 ))
      COMP_LINE="${COMP_WORDS[*]}"
      COMP_POINT=${#COMP_LINE}
      __start_portal "$@"
  }
  complete -o default -F __start_portal_open <cmdName>
  ```
  Keep `-o default` (cobra's own `compopt +o default` at runtime is what switches the filename fallback off), and leave the `ctlName` registration line exactly as it is.
- In `emitZshInit` (`cmd/init.go:111`), in place of `compdef _portal %s` for `cmdName` (`:131`), emit the zsh shim and its `compdef`:
  ```
  _portal_open() {
      words=(portal open "${(@)words[2,-1]}")
      (( CURRENT += 1 ))
      _portal "$@"
  }
  compdef _portal_open <cmdName>
  ```
  Leave `compdef _portal portal` and the `ctlName` line untouched.
- In `emitFishInit` (`cmd/init.go:81`), replace the `cmdName` registration `complete -c %s -w portal` (`:101`) with two lines for `cmdName` — `complete -c %s -f` followed by `complete -c %s -w 'portal open'` — leaving the `ctlName` wrap at bare `portal` with no `-f` line of its own. The `-f` is unconditional rather than contingent on a live fish check: it is what holds the no-filename contract on that word whether or not the wrap carries the property across, and it is inert where the wrap already does.
- Add one line at the bash shim naming the constraint the code cannot state: the generated script asks the typed word for its completions, so the registration must point at the shim rather than at `__start_portal`.
- Extend `cmd/init_test.go`'s emitted-text tables — `TestInitBash`, `TestInitZsh`, `TestInitFish` and the three `_CmdFlag` siblings — with the new registration lines for the configured name and the unchanged control-function lines.
- Add `cmd/init_completion_shell_test.go` (unit lane, no build tag): for bash and zsh, execute `init <shell>` in-process into a buffer, write it to `t.TempDir()`, stage a recording `portal` stub on `PATH` that appends its argv to a file and prints a canned candidate list plus `:4`, and run `bash --noprofile --norc` / `zsh -f` over a driver that sources the script, sets the completion-context variables and invokes the registered completion function — reading the function name out of `complete -p <name>` in bash — then assert the recorded request line. Skip the shell's subtest when `exec.LookPath` cannot find it; drive fish the same way through `complete -C '<line>'`.

**Acceptance Criteria**:
- [ ] Driven through the emitted bash script, Tab after the session-opening function records the request `portal __complete open …` — for `x /po`, `x ""` and `x -s ""` alike — never `portal open __complete …`
- [ ] The same holds through the emitted zsh script
- [ ] In fish, completion after the function asks for `open`'s completions and offers live session names
- [ ] The control function's request is unchanged (`portal __complete …`) in all three shells
- [ ] The correction follows `portal init --cmd <name>`: with `--cmd p`, `p` gets the corrected registration and `pctl` the unchanged one, and the literal `x` appears nowhere in the emitted registrations
- [ ] Candidates carrying a leading `/` survive the shell's own prefix filter into the completion reply, so task 5-1's offers are usable as typed
- [ ] Filename fallback stays off on that word in every shell: a partial single-segment absolute directory (`x /tm<TAB>`) never gains a trailing slash and never turns into `/tmp/`
- [ ] A path argument after the function (`x ~/Code/pro<TAB>`) no longer completes filenames — the deliberate loss, and the contract `portal open` already has
- [ ] The emitted function bodies are unchanged, so every non-completion invocation of the function opens sessions exactly as before
- [ ] No portal binary is built or exec'd by the suite and no tmux server is contacted: the shells talk to a recording stub, and the suite runs in `go test ./...`
- [ ] A shell binary absent from the machine skips its own subtest rather than failing the suite
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it asks portal for open's completions after the session-opening function in bash"` — recorded request is `__complete open /po`
- `"it asks portal for open's completions after the session-opening function in zsh"`
- `"it asks portal for open's completions after the session-opening function in fish"`
- `"it leaves the control function's request unchanged"` — `__complete` with no `open`, all three shells
- `"it carries a slash-prefixed candidate into the completion reply"` — stub answering `/portal-a1b2`, asserted present in the reply
- `"it registers the corrected completion for a --cmd name"` — `--cmd p`: emitted registration for `p`, unchanged one for `pctl`, no literal `x`
- `"it offers no filename for a partial absolute directory"` — `x /tm`, reply carries no `/tmp/`
- `"it emits the unchanged function bodies"` — the existing body assertions re-run
- `"it skips a shell that is not installed"` — the skip path itself
- `"it emits consistently for a --cmd name that shadows a real command"` — `--cmd portal`: the same substitution, no special case

**Edge Cases**:
- The shim's name is fixed and independent of `--cmd`, so a chosen name that is not a legal function-name word still gets a working registration; a `--cmd portal` user's registration lands after cobra's own `complete … portal` line and wins, which is what naming the function `portal` asks for
- `_init_completion` is absent on a machine without the bash-completion package, and the generated script falls back to `_get_comp_words_by_ref`, which is absent too — the driver supplies a minimal `_init_completion` stand-in filling `cur/prev/words/cword` from `COMP_WORDS`/`COMP_CWORD`, which is the branch the generated script prefers
- `compopt` outside a live completion context prints a harmless diagnostic on stderr; the assertion is on the recorded request and the reply, not on a clean stderr
- Rewriting `COMP_WORDS` alone is not enough for every bash-completion helper — `COMP_LINE` and `COMP_POINT` are rebuilt from the rewritten words so the two cannot disagree
- In zsh `words` and `CURRENT` are the completion state itself, so the shim assigns them without `local`; the arithmetic on `CURRENT` is what keeps the cursor on the same word after two words are prepended
- The emitted bodies hardcode `portal`, so the shim's literal `portal` head is consistent with what the function itself runs — no alias resolution is owed, and cobra's `${words[0]}` indirection is deliberately given up on this one registration
- Fish is not installed on the development machine, so its leg is verified by the skipping subtest plus a manual `fish -c` run wherever fish is available; the emitted-text assertions run unconditionally in both lanes. The emitted output does not depend on that run — the `-f` line is unconditional precisely so nothing is left resting on a check nobody here can perform
- The fix lands in the output of `portal init`, which users evaluate in a shell startup file: an existing install does not pick it up until the shell is restarted or `portal init` is re-evaluated, even though the binary is new. That affects completion only — the `/term` form itself works the moment the binary is in place (documented by task 5-4)

**Context**:
> Cobra's emitted script asks the typed word for its completions, not a registered command name. The typed word is `x`, and `x` expands to `portal open` — so Tab after `x` runs `portal open __complete …`, a real `open` invocation carrying `__complete` as a positional target, and Portal's completer is never consulted. All three emitted shells share the mistake for the session-opening function, while `xctl` expands to bare `portal`, so its request resolves correctly.
>
> `portal init` is corrected so that Tab after the session-opening function asks Portal for `open`'s completions, reaching the completer that already answers `portal __complete open …` correctly — across bash, zsh and fish. The correction changes what command the emitted script asks for completions; registering the completer against a different name would not help, because the script does not consult a registered name. It applies to the configured function name, not the literal `x`.
>
> The correction brings the session-opening function onto `open`'s existing completion contract: live session names, and no filename fallback. Path arguments complete to filenames today only because Portal is never asked and the shell has nothing else to offer; once Portal answers, it switches that fallback off — and it must, since a filename fallback on the same word is what would turn `x /tm<TAB>` into `/tmp/` and a search into a mint. The loss is taken deliberately.
>
> It is folded into this feature rather than spun out because the deliverable is not a parsing rule, it is `x /term` becoming muscle memory: a completion decision that does not reach that form has not been delivered.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §8.2, §8.3, §8.4, §8.5

## open-with-forced-filter-5-3

### Task 5-3: Describe the search form in `portal open --help`

**Problem**: `openCmd.Long` (`cmd/open.go:116`) is the only place the surface is described at the terminal, and it enumerates the bare-positional chain, the four domain pins, `-f`, command scoping and the multi-target burst — every route except the one this feature adds. Two readers are misled by the omission. The first types `x /tmp` expecting the mint the help still implies and gets a search, with nothing on the page accounting for it. The second knows `-f` and, told only that a new form "opens the picker pre-filtered", assumes `/term` is shorthand for it — but they differ exactly where it matters: with one match `/term` attaches and `-f` shows a list of one. Described by input shape the pair read as duplicates; only the outcome separates them.

**Solution**: A search-form block in `Long` stating the form, its recognition rule, its three outcomes, the term-less form, that it composes with nothing, and the `-f` / `/term` split by outcome and by which to reach for — with the `-f` flag's own registered usage string left as the one-liner it is.

**Outcome**: `portal open --help` describes the search form completely enough that a reader can predict what `x /port`, `x /` and `x /tmp` each do and knows which of `-f` and `/term` to reach for, while every existing help-metadata assertion and the `Use` line's multi-target admission stay green.

**Do**:
- Insert a search-form block into `openCmd.Long` (`cmd/open.go:116`) immediately after the `-f, --filter` line, so the two forms are read together. This wording is the task's own and is what to review against the specification's requirements:
  ```
  A positional beginning with / and containing no further / searches your live
  sessions instead of resolving:
    open /term      search live sessions for "term"
    open /          open the picker with the filter empty and ready to type

  The term is matched case-folded, as a contiguous run, against each live session's
  name and its recorded directory. Exactly one match attaches that session outright;
  no match or several open the picker pre-filtered by the term — never an error.
  A single-segment absolute directory is read as a search, not a path: mint there
  with -p /tmp.

  A search composes with nothing — another target, a command, -f, a domain pin or a
  second search on the same line is a usage error.

  -f always opens the picker, which makes it the form for a script or a keybinding;
  /term takes you straight to the session when only one matches, which makes it the
  interactive form.
  ```
- Leave the registered flag usage at `cmd/open.go:720` (`open the picker pre-filtered by <text> (skips resolution)`) untouched, and add nothing to any other flag's usage string.
- Leave `openCmd.Use` and `openCmd.Short` unchanged.
- Extend the keyword slice in `TestOpenHelpMetadata_DescribesRedesignedVerb` (`cmd/retired_surface_test.go:110`) with `"/term"` and `"search"`, leaving its existing entries and its `Use`/`Short` assertions as they are.
- Add `cmd/open_help_test.go` with `TestOpenHelpMetadata_DescribesSearchForm`: assert `Long` carries the literals `open /term` and `open /`, names the three outcomes (`attach`, `picker`, and the not-an-error statement), states the composition refusal, mentions both `-f` and `/term` in one block, and assert the `filter` flag's `Usage` carries no newline and names neither the search form nor a match count.

**Acceptance Criteria**:
- [ ] `Long` states the recognition rule — a positional beginning with `/` and containing no further `/` — in terms a reader can apply to their own argument
- [ ] `Long` gives all three outcomes by match count: one match attaches, none or several open the pre-filtered picker, and neither of the latter is an error
- [ ] `Long` names the term-less form and says it opens the picker with the filter ready to type, not that it is an error and not that it mints at root
- [ ] `Long` states that the form composes with nothing, naming the colliding kinds rather than restating the whole refusal table
- [ ] `Long` distinguishes `-f` from `/term` by outcome and says which to reach for — `-f` for a script or a keybinding, `/term` interactively
- [ ] `Long` names the single-segment absolute-directory cost and the `-p` escape
- [ ] The `-f/--filter` registered usage string is unchanged and remains a single line
- [ ] `TestOpenHelpMetadata_DescribesRedesignedVerb` passes with its existing keywords plus the two new ones, and its `Args` admission of two positionals is untouched
- [ ] No behaviour changes: no flag is added, renamed or re-defaulted, and `RunE`, `Args` and the completer are untouched
- [ ] `go test ./...` passes

**Tests**:
- `"it describes the search form's recognition rule"` — `Long` carries the literal `open /term`
- `"it describes the term-less form"` — `Long` carries the literal `open /` and does not call it an error
- `"it gives the outcomes by match count"` — attach / picker / not-an-error keywords present
- `"it states that the search form composes with nothing"`
- `"it names the single-segment directory cost and the -p escape"`
- `"it distinguishes -f from the search form by outcome"` — both tokens present in one block, with the script-or-keybinding and interactive readings
- `"it keeps the -f flag usage a one-liner"` — no newline, no mention of the search form
- `"it keeps the existing help metadata assertions green"` — the redesign keyword table re-run with the two additions

**Edge Cases**:
- The `-f` one-liner is the flag's registered usage, printed in the flags block; none of the new material fits there, which is why it all lands in `Long`
- The distinction must be stated by outcome rather than by input shape: two forms that both "open the picker pre-filtered" read as duplicates, and the reader picks the wrong one
- The composition refusal is stated as a rule with its colliding kinds, not as the full table — the runtime error already names the specific element that collided
- `Use` keeps `[targets…]`, so the existing assertion that it no longer implies a single `[destination]` and the `Args` admission of two positionals both stay green; the search form is a positional and needs no new usage token
- Keyword assertions rather than a golden string, matching the existing test's stated reason: accurate copy edits must not churn the suite
- This is wording work — no behaviour changes here, and no other command's help is touched

**Context**:
> `portal open --help` (`cmd/open.go`'s `Long`) carries the points about `open`'s own surface: the form and its recognition rule, the outcomes by match count, the term-less form, that it composes with nothing, and the `-f` / `/term` distinction. The `-f` flag description stays a one-liner — none of these fits there.
>
> After this feature the surface reaches a live session by a bare exact session name, a quoted bare glob, `-s <name>`, `-s <glob>`, and `/term`, and opens the picker pre-filtered by `-f <text>` and `/term`. None of them is a duplicate, but the closest pair reads like one when described by input shape. `-f` documented as "open the picker pre-filtered" invites a reader to assume `/term` is shorthand for it. They differ exactly where it matters: with one match, `/term` attaches and `-f` shows a list of one. The pair must be described by outcome — one always shows the list, one takes you there when there is only one place to go.
>
> The outcome carries a use with it, and the documentation says which to reach for: `-f` is the form for a script or a keybinding, which needs to land in the same place every time; `/term` is the interactive form.
>
> `x /` — the sigil with no term — opens the picker with the filter open and empty, the cursor in it, ready to type. It is not an error, and it does not mean "mint at root".
>
> This is wording work, not behaviour change.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §9.1, §9.2

## open-with-forced-filter-5-4

### Task 5-4: Document the search form and the completion correction in the README

**Problem**: The README's `x (open)` section (lines 120–154) is the surface's public description — the example block, the resolution paragraph, the pin table and the multi-window paragraph — and after this feature it is wrong by omission in three ways. The form is absent, so a user who never runs `--help` never learns `x /port` exists, and the feature's own premise (that names are what they cannot recognise sessions by) goes unanswered. `x /tmp` silently stops minting, with no note of the cost or of the `-p` escape that covers it. And the completion correction has no home at all: it belongs to `portal init` rather than to `open`, so it cannot live in a command's help, and neither can its rollout consequence — an existing install picks it up only once the output of `portal init` is re-evaluated, which is the difference between "Tab does nothing for me" and "restart your shell".

**Solution**: A search-form block, two example lines, a resolution-table row that reads as a positional rather than a domain pin, and a tab-completion paragraph carrying the correction and its rollout consequence — all inside the `x (open)` section, with a token-based guard so the examples and the table row cannot silently vanish.

**Outcome**: The README documents `x /port` and `x /` in its examples, explains the form and what it matches, carries it in the resolution table without it reading as a pin, distinguishes `-f` from `/term` by outcome, and tells the reader why Tab only starts working after their shell is restarted — with no CHANGELOG entry written.

**Do**:
- Add two lines to the `x (open)` example block (README:125–133), keeping the existing comment alignment: `x /port` (search live sessions for "port") and `x /` (open the picker with the filter ready to type).
- Add a **Session search** block after the resolution paragraph and before the **Domain pins** paragraph (README:141) carrying: the recognition rule (a positional beginning with `/` and containing no further `/`); that the term is matched case-folded as a contiguous run against each live session's **name** and its **recorded directory**, home-abbreviated as the row displays it; the three outcomes by match count with a zero count named as not an error; the term-less `x /`; that the picker's own hand-typed filter stays fuzzy while this form is stricter because it can attach a lone match without showing it; the single-segment absolute-directory cost with the `x -p /tmp` escape; and that the form composes with nothing.
- Add a **`-f` or `/term`?** sentence to that block stating the pair by outcome and which to reach for — `-f` always opens the picker, so it is the form for a script or a keybinding; `/term` goes straight to the session when only one matches, so it is the interactive form.
- Amend the table intro (README:141) so the non-pin rows no longer sit under "never pops the picker", change the header's first cell (README:143) from `Flag` to `Flag / form`, and add a row after the `-f` row (README:149) whose first cell is the literal `` `/<term>` `` marked as a positional, whose second cell reads `session search` rather than naming a pin, and whose behaviour cell gives the pre-filtered picker plus the attach-on-exactly-one outcome.
- Add a **Tab completion** paragraph at the end of the `x (open)` section (after the multi-window paragraph, before the git-root line at README:154): this release corrects `portal init` so Tab after `x` reaches Portal's completer at all, and `x <TAB>` now offers your live session names; `x /po<TAB>` completes the term after the slash against those same names and leaves the slash in place; an existing install picks the correction up only when the output of `portal init` is re-evaluated — a new shell, or re-running the eval in the profile — while the `/term` form itself works the moment the new binary is in place; and `x <path><TAB>` no longer falls through to filenames, which is `portal open`'s existing completion contract.
- Add `cmd/open_docs_test.go` with `TestReadmeDocumentsSearchForm`: read `README.md` from `sourceguardtest.ProjectRoot(t)`, slice the `### \`x\` (open)` section up to the next `### ` heading, and assert that section carries the literals `x /port`, `x /`, `` `/<term>` ``, `-p /tmp`, `portal init`, and the pre-existing `-f, --filter` row.
- Write no CHANGELOG entry and touch no file other than `README.md` and the new test.

**Acceptance Criteria**:
- [ ] The example block carries `x /port` and `x /`, each with a one-line comment
- [ ] The section states the recognition rule as a test of the argument's shape, with no suggestion that the filesystem is consulted
- [ ] The section states what is matched — session name and recorded directory, home-abbreviated — and that the match is case-folded containment while the picker's own filter stays fuzzy
- [ ] The section gives all three outcomes by match count and says a zero count is not an error
- [ ] The section documents `x /` as opening the picker with an empty, ready-to-type filter
- [ ] The section names the single-segment absolute-directory cost and the `x -p /tmp` escape
- [ ] The section states that the form composes with nothing
- [ ] The section distinguishes `-f` from `/term` by outcome and says which to reach for
- [ ] The resolution table carries a row for the form that reads as a positional and not as a domain pin, and the table intro no longer claims every row hard-fails without popping the picker
- [ ] The section states that Tab after the session-opening function now offers live session names, and that `x /po<TAB>` completes the term after the slash against those same names
- [ ] The section documents the `portal init` correction and that an existing install picks it up only on a new shell or a re-run of `portal init` while the form itself works with the new binary
- [ ] The section notes that a path argument after the function no longer completes filenames — the deliberate loss beside that gain
- [ ] No CHANGELOG entry is written, and no file outside `README.md` and the new test is modified
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it documents the search form's examples"` — `x /port` and `x /` present in the open section
- `"it carries a resolution-table row for the search form"` — the `` `/<term>` `` cell present
- `"it keeps the -f row beside it"` — the `-f, --filter` row still present in the same section
- `"it documents the -p escape for a single-segment absolute directory"` — `-p /tmp` present
- `"it documents the completion correction in the open section"` — `portal init` named there
- `"it finds the open section at all"` — the slice is non-empty, so a heading rename fails loudly rather than passing vacuously

**Edge Cases**:
- The table's first column is otherwise all flags, so the new row must state on its face that it is a positional — a reader who takes it for a pin will expect the hard-fail-on-miss behaviour the pins have, which is the one thing this form does not do
- The table intro's "each hard-fails on a miss, never pops the picker" already reads oddly over the `-f` and `-e` rows and must not be left standing over a row whose whole purpose is to open the picker
- The completion correction belongs to `portal init`, not to `open`, which is why it lives in the README and not in the command's help — there is nowhere in a command's help for it
- The rollout consequence is the part a user will otherwise report as a bug: the binary is new, the form works, and Tab still does nothing until the shell is restarted
- The guard asserts literal tokens a user types, not prose, so accurate copy edits do not churn it — and the section slice is asserted non-empty so a renamed heading is a failure rather than a silent pass
- No CHANGELOG entry is written as part of this work: the release process owns that file, and a task asking for one is the thing to refuse

**Context**:
> The README's `x (open)` section carries the form and its recognition rule, the outcomes by match count, the term-less form, that it composes with nothing, and the `-f` / `/term` distinction — and, additionally, the completion correction and its rollout consequence, which belong to `portal init` rather than to `open` and have nowhere to live in a command's help. The README's resolution table is where the sigil row belongs, alongside the domain pins and `-f`.
>
> The documentation must carry: the single-segment absolute-directory cost and the `-p` escape; what the search matches, and that it matches by containment while the picker's own filter stays fuzzy; that the form completes against live session names after the slash; that the corrected completion offers live session names after the session-opening function and no longer falls through to filenames for a path argument; and that the corrected completion reaches an existing install only once the output of `portal init` is re-evaluated — a new shell, or re-running `portal init`.
>
> The sigil is not a domain pin: reaching the picker is its purpose rather than a fallback from a failure, and it has no unresolvable case to fall back from — a term matching nothing is a filter result that opens the picker on an empty list.
>
> No CHANGELOG entry is written as part of this work — the release process owns that file.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §2.4, §4.1, §4.4, §8.1, §8.3, §8.5, §9.1, §9.2, §9.3
