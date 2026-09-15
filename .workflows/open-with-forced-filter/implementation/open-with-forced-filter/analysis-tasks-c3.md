# Analysis Tasks: Open With Forced Filter (Cycle 3)

## Task 1: Refuse every malformed open line before any bootstrap runs
severity: low
sources: architecture

**Problem**: `validateOpenArgs` was installed in open's general `Args` slot (replacing `cobra.ArbitraryArgs`) but implements only the search form's collisions and returns nil for every other malformed line. Open's remaining refusals — `-f` beside a target or a domain pin (`cmd/open.go:184-193`), an empty `-f`, `-e` together with `--`, an empty `-e`, and a bare `--` (all in `parseCommandArgs`, `cmd/open.go:344-383`) — stay in `RunE`, downstream of `PersistentPreRunE`. Every one of them is knowable from the argv alone, so the difference between the two homes is purely *when* the refusal fires. Today `portal open -f ""` on a cold machine starts the tmux server, registers hooks and restores every saved session before printing a usage error: the user waits through a full restore for a complaint about their own command line. The search form has the property that a refused line starts nothing; the command does not, nothing in either file signposts that the choice of home decides it, and the validator's name gives no hint that it is partial — so the next mutual-exclusion refusal a maintainer adds will land in `RunE` beside the ones already there and inherit the same wait.

**Solution**: Move the argv-only refusals out of `RunE` into `validateOpenArgs`, so every malformed open line is refused before any bootstrap runs and the validator's name covers what it validates.

- In scope, exactly: `-f` with a target or a pin, an empty `-f`, `-e` together with `--`, an empty `-e`, a bare `--`. Each needs nothing but the parsed flags and `cmd.ArgsLenAtDash()`, which the validator already reads.
- The search-form collisions keep their precedence and their fixed internal order, so a line colliding several ways still names the one it names today.
- Neither rule is restated. `parseCommandArgs` stays the single home of the command-scoping refusals and the validator reaches them by calling it for its error alone; `RunE` goes on calling it for the command and destination it returns. The `-f` refusals move to one helper the validator calls, leaving `RunE`'s filter block with the value read and the dispatch.
- `--ack`'s shape check stays in `RunE`. It is not a mutual-exclusion refusal, and `--ack` is a hidden internal flag Portal writes for its own spawned windows — never typed, so the cold-machine wait it would save is a wait nobody takes.
- Settled that the rendering is unaffected: both slots return `NewUsageError`, so the message text and exit code are identical wherever the refusal fires. Settled that completion is unaffected: cobra v1.10.2 calls `ValidateArgs` only from `execute()` (`command.go:968`) and not on the `__complete` path, so a partially-typed malformed line still gets its candidates.
- `validateOpenArgs` keeps its name; the alternative the finding offered — narrowing it to `validateSearchFormArgs` and noting the rest are `RunE`'s — is declined, because it documents the wait rather than removing it and leaves the same trap for the next refusal.

**Outcome**: A malformed `portal open` line starts no tmux server, registers no hooks and restores nothing — it prints its usage error and exits, on a cold machine as on a warm one. The behaviour the search form already had becomes the command's.

**Do**:
- Add a filter-refusal helper in `cmd/open.go`, beside the filter dispatch it is taken from — `validateFilterFlag(cmd *cobra.Command, destination string) error`, gated on `cmd.Flags().Changed("filter")` and carrying the two refusals verbatim: `cannot use -f/--filter with a target or a domain pin (-s/-p/-z/-a)` when `destination != "" || anyOpenDomainPin(cmd)`, then `-f/--filter value must not be empty` for an empty value.
- Extend `validateOpenArgs` (`cmd/open_search.go:54-75`): after the search-form switch falls through, call `parseCommandArgs(cmd, args)` and return its error alone (the command and destination it returns are `RunE`'s), then return `validateFilterFlag(cmd, destination)`. That order is today's — `RunE` reached `parseCommandArgs` before its filter block — so a line colliding both ways still names the `-e`/`--` refusal.
- Strip the two refusals from `RunE`'s filter block (`cmd/open.go:184-193`), leaving the `Changed("filter")` gate, the value read and the `openTUIFunc` dispatch.
- Leave `parseCommandArgs` (`cmd/open.go:344-383`) untouched — it stays the single home of the `-e`/`--` refusals — and leave the `--ack` shape check where it is in `RunE`.
- `validateOpenArgs`'s doc comment describes the search form alone and would be false after this; correct it to the set it now refuses, wording yours.

**Acceptance Criteria**:
- [ ] `portal open -f ""`, `portal open -f blog api`, `portal open -f blog -s api`, `portal open -e ""`, `portal open -e vim -- claude` and `portal open --` are each refused by open's `Args` validator: the bootstrap orchestrator runs zero times, no resolver seam is consulted and `openTUIFunc` is never called.
- [ ] Each refusal's message text is byte-identical to today's and each still arrives as a `*UsageError` (exit 2).
- [ ] A line colliding several ways names the refusal it names today: a search-form collision first, then the `-e`/`--` refusal, then `-f`'s.
- [ ] `RunE` refuses none of the five; `parseCommandArgs` still returns them to `RunE` as it does now, restated nowhere.
- [ ] `portal open -f "" --help` prints help rather than the refusal.
- [ ] A malformed `--ack` is still refused from `RunE`, unchanged.

**Tests**:
- `"it refuses an empty -f before the bootstrap runs"`
- `"it refuses -f beside a positional target before the bootstrap runs"`
- `"it refuses -f beside a domain pin before the bootstrap runs"`
- `"it refuses an empty -e before the bootstrap runs"`
- `"it refuses -e together with -- before the bootstrap runs"`
- `"it refuses a bare -- before the bootstrap runs"`
- `"it names the /term collision when a search form sits beside an empty -f"`
- `"it names the -e refusal when an empty -e sits beside an empty -f"`
- `"it still prints help for a malformed -f line"`
- `"it still refuses a malformed --ack from the command body"`

## Task 2: Make pickerLanding carry the search form itself
severity: near-miss
sources: architecture

**Problem**: `tui.SearchForm{Term, Decide}` already is the domain type for "the picker was opened by a search". `pickerLanding` (`cmd/open_search.go:38-45`) mirrors it through three loose fields — `filter` doubling as the term, `search` as a bool discriminator, `decide` as the closure — and `buildTUIModel` (`cmd/open.go:583-587`) unpacks and repacks them one layer down into the very type they came from. The struct can represent two states that mean nothing: `filter` set with `search` true, where the field name says "filter text" but the value is a search term; and `decide` set without `search`, which compiles and type-checks while silently dropping the classification — `buildTUIModel` takes the `else` arm, the closure never reaches the model, and the cold-boot single-match attach stops working, with no error and no log line. The user gets the picker where they used to get the session, and only an integration test exercising a cold boot at exactly one match would notice. Correctness rests today on every construction site remembering to set the flag alongside the field.

**Solution**: Give `pickerLanding` a `search *tui.SearchForm` field beside `filter string` and drop the `search` bool and the `decide` closure. The mode becomes `landing.search != nil`, term and decision travel as one value, and `buildTUIModel` assigns `deps.Search = landing.search` with no unpack/repack. The two invalid combinations stop being constructible. `cmd` already imports `internal/tui`, so no new edge appears. The two production construction sites (`cmd/open_search.go:161` and `:165-168`) and the test-side readers of the fields (`cmd/testhelpers_test.go:245`'s `landingShape`, `cmd/open_search_deferred_test.go:37-40`) convert with the type. Behaviour-preserving: the model receives exactly what it receives today on every route.

**Outcome**: A landing that carries a decision closure carries the classification with it, because they are one value — `pickerLanding{decide: fn}` without the flag no longer compiles — and the picker receives on every route exactly what it receives today.

**Do**:
- Redeclare `pickerLanding` (`cmd/open_search.go:38-45`) as `filter string` plus `search *tui.SearchForm`: the `search` bool and the `decide` field go, along with the field comment describing the closure — `tui.SearchForm.Decide` (`internal/tui/build.go:71-78`) already carries that contract.
- `runSearchForm` (`cmd/open_search.go:159-187`): the term-less form becomes `pickerLanding{search: &tui.SearchForm{}}`; the term form becomes `pickerLanding{search: &tui.SearchForm{Term: term}}`, and the deferred-bootstrap route sets `landing.search.Decide = decide` where it set `landing.decide`. A search landing leaves `filter` empty — the term now lives in `search.Term`.
- `buildTUIModel` (`cmd/open.go:583-587`): `if landing.search != nil { deps.Search = landing.search } else { deps.InitialFilter = landing.filter }` — no `tui.SearchForm` is constructed here any more.
- Convert `landingShape` (`cmd/testhelpers_test.go:235-246`): add a `term` field reading `l.search.Term` (empty when `search` is nil), `search` becomes `l.search != nil`, `decided` becomes `l.search != nil && l.search.Decide != nil`.
- Move the term from `filter` to `term` in the search-form assertions that carry one (`cmd/open_search_test.go:198`, `:520`, `:537`; `cmd/open_search_deferred_test.go:63`, `:183`), read the closure as `sc.landing.search.Decide` (`cmd/open_search_deferred_test.go:37-40`), and leave the `-f` assertions (`cmd/open_test.go:2185`, `:2526`, `:2552`) as they stand.

**Acceptance Criteria**:
- [ ] `pickerLanding` has exactly two fields — `filter string` and `search *tui.SearchForm` — with no bool discriminator and no free closure.
- [ ] `buildTUIModel` assigns `landing.search` straight through to `deps.Search` and constructs no `tui.SearchForm`.
- [ ] A search landing carries its term in `search.Term` with `filter` empty; a `-f` landing carries its text in `filter` with `search` nil.
- [ ] `deps.Search` and `deps.InitialFilter` receive what they receive today on all five routes: bare sigil, warm term, deferred term, `-f`, no-args picker.
- [ ] `cmd` takes no new import.
- [ ] Every converted assertion asserts the same fact it asserted before — the term changes field, not meaning; no case is dropped and none is added.

**Tests**:
- `"it hands the picker a search form carrying the term"`
- `"it hands the picker a term-less search form for a bare sigil"`
- `"it hands the picker the deferred decision closure on the cold route"`
- `"it hands the picker -f's text as the initial filter with no search form"`
- `"it hands the picker neither for a bare open"`

## Task 3: Let the model answer what the teardown still owes the terminal
severity: complexity
sources: architecture

**Problem**: `Model` exposes two warning buckets — `BufferedWarnings` (claimed by the loading gate) and `PendingBootstrapWarnings` (never claimed by a gate) — and `finishTUI` (`cmd/open.go:610-621`) writes both: one through `emitSearchTeardownWarnings` (`cmd/open_search.go:194-199`), gated on the search-specific `SearchAttached() || SearchError() != nil`, and one directly, under a comment asserting the two sets are disjoint. The question being answered is not search-specific — it is "what does this teardown still owe the terminal" — and the model already holds every fact needed to answer it (`surfaceBufferedWarnings` nils the buffer precisely when a frame consumed it; the model knows whether the exit was a cancelled loading page). Instead the answer is assembled in `cmd` from four accessors plus an invariant nothing checks. This cycle added the first exit path that leaves without painting a picker; the next one drops whatever the concurrent bootstrap accumulated unless its author remembers to widen a search-named predicate in a search-named file — and a saver-down or restore warning then never reaches the user at all, noticed only much later, when the thing it warned about fails.

**Solution**: Add one `Model.WarningsOwedAtTeardown() []BootstrapWarning` that returns the buffered set when the exit left no picker frame and the pending set otherwise, with the deliberate "a cancelled loading page is owed no report" carve-out expressed there, beside the state it reads. `finishTUI` then makes a single `tui.WriteBootstrapWarnings` call and `emitSearchTeardownWarnings` — with its search-named predicate — goes. Settled that the rule belongs in the model rather than in `cmd`: the model holds both buckets and both facts the answer turns on, while `cmd` today reconstructs the answer from four accessors and a prose invariant. This extends cycle 1's approved Task 4 rather than reversing it — both sets still reach the terminal on exactly the routes that owe them, nothing is written twice, and the disjointness that task asserted in a comment becomes a property of the one function that decides. The concurrent route's notice band is untouched. Behaviour-preserving on every route that exists today.

**Outcome**: A teardown that leaves no picker frame reports what the bootstrap accumulated because the model says it is owed, not because a search-shaped predicate happened to match — so the next such exit path inherits the behaviour instead of having to remember it.

**Do**:
- Add `func (m Model) WarningsOwedAtTeardown() []BootstrapWarning` in `internal/tui`, beside the two buckets it arbitrates (`model.go:468-476`): it answers `bufferedWarnings` when the exit left the loading gate without painting a picker frame — the model still on `PageLoading` with a decision recorded on it, a session named in `selected` or a failed read in `searchErr` — and `pendingBootstrapWarnings` on every other exit. Decide it from the model's own fields, not from `SearchAttached`/`SearchError`, so an exit path that is not a search inherits the answer.
- A cancelled loading page records no decision and so falls to the pending set, which the gate already emptied — that is the carve-out, and it is not evident from the expression; state it in one comment line beside the rule, wording yours.
- Collapse `finishTUI` (`cmd/open.go:610-621`) to a single `tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())`, in the slot the two writes occupy now: after `tui.RestoreTerminalBackground` and before `processTUIResult`, since the outside-tmux attach execs and never returns.
- Delete `emitSearchTeardownWarnings` (`cmd/open_search.go:189-199`) and the `io` import it was the only user of.
- Re-point the four cases `TestEmitSearchTeardownWarnings` (`cmd/open_search_warnings_test.go:194-278`) pins — decision attach, decision read failure, picker opened, cancelled loading page — at the new decider with their expectations unchanged, and add the warm-picker pending case beside them. `BufferedWarnings()` and `PendingBootstrapWarnings()` stay: what goes is `cmd`'s arbitration between them, not the model's own vocabulary.

**Acceptance Criteria**:
- [ ] `finishTUI` makes exactly one `tui.WriteBootstrapWarnings` call and reads exactly one model accessor to decide its argument.
- [ ] No predicate in `cmd` reads `SearchAttached()` or `SearchError()` to decide a warning write (`processTUIResult`'s `SearchError()` read, which returns the error, is untouched).
- [ ] Route parity, each verified: search attach → the buffered set written; search read failure → the buffered set written; warm picker painted → the pending set written; picker painted after a loading gate → nothing (both buckets empty); cancelled loading page → nothing.
- [ ] Nothing is written twice on any route, and the disjointness `finishTUI` asserted in prose is now a consequence of one function returning one set.
- [ ] `surfaceBufferedWarnings` and the concurrent route's notice band are unchanged.
- [ ] The teardown's lines still match the CLI path's byte for byte.

**Tests**:
- `"it owes the buffered set when the loading gate quit on a named session"`
- `"it owes the buffered set when the loading gate quit on a failed session-list read"`
- `"it owes the pending set when a picker frame was painted"`
- `"it owes nothing when the loading page was cancelled"`
- `"it owes nothing once a loading gate has surfaced the buffer"`
- `"it writes the owed warnings once, before the connect"`
- `"it writes the same lines as the CLI path"`

## Task 4: Declare the session-opening function's expansion once
severity: duplication
sources: duplication

**Problem**: The literal `portal open` appears seven times across `cmd/init.go` in two roles that must stay in lockstep. Three are what the emitted function *runs* — bash `%s() { portal open "$@"; }` (:79), fish `function %s\n    portal open $argv\nend` (:112), zsh `function %s() { portal open "$@" }` (:145). Four are what the emitted completion *asks for* — the bash shim's `expansion="portal open"` (:60) and `COMP_WORDS=(portal open …)` (:61), the zsh shim's `words=(portal open …)` (:70), and fish's `complete -c %s -w 'portal open'` (:132). Nothing ties the roles together: they are seven unrelated string literals, and the shell-driven tests in `cmd/init_completion_shell_test.go` assert the request is `__complete|open|…` as a constant of their own, so they pin the shim's rewrite rather than its agreement with the function body — a body that changed and a shim that did not leaves every assertion green. Tab after `x` would then ask Portal for the completions of a command line the shell no longer runs: the user gets candidates for a different invocation, or none, and notices only on a Tab press in a shell that has already re-evaluated `portal init`. That is the same disagreement-between-expansion-and-request class this feature was opened to fix.

**Solution**: Declare the expansion once as a package-level `const openFunctionExpansion = "portal open"` in `cmd/init.go` and interpolate it into all seven sites: the three function-body `Fprintf` formats, fish's `-w` target, and the two shim constants. Settled that the shims become format templates rendered through `fmt.Sprintf` at their emission site rather than being concatenated — they are multi-line raw strings whose readability is the reason they are named consts, and neither carries a `%` character (verified), so rendering them through a format introduces no escaping. Extraction only: the emitted text is byte-identical for every shell, so the existing shell-driven suites stay green unchanged and no new emitted text appears.

**Outcome**: What the emitted function runs and what its completion shim asks Portal for are one string in the source, so a change to the expansion lands on both roles at once and cannot leave Tab after `x` requesting completions for a command line the shell no longer runs.

**Do**:
- Declare `const openFunctionExpansion = "portal open"` in `cmd/init.go`, beside the shim consts that consume it.
- Interpolate it into the three function bodies — bash `%s() { portal open "$@"; }` (`:79`), fish `function %s\n    portal open $argv\nend` (`:112`), zsh `function %s() { portal open "$@" }` (`:145`) — each taking `cmdName` then the const as its two format arguments.
- Interpolate it into fish's `complete -c %s -w 'portal open'` (`:132`).
- Render the two shims through a format at their emission sites rather than concatenating: `bashOpenCompletionShim` carries two occurrences (`expansion="%s"` at `:60` and `COMP_WORDS=(%s …)` at `:61`) and `zshOpenCompletionShim` one (`words=(%s …)` at `:70`); `fmt.Fprint` at `:96` and `:162` becomes `fmt.Fprintf` with the const supplied once per occurrence.
- Leave the sites naming the binary alone — `%s() { portal "$@"; }`, `complete -c %s -w portal`, `compdef _portal %s` and the `__start_portal`/`_portal` function names are the control verb's role, not the session-opening function's expansion.

**Acceptance Criteria**:
- [ ] `portal init bash`, `portal init zsh` and `portal init fish` each emit output byte-identical to today's, for the default `x` and for `--cmd p`.
- [ ] The literal `portal open` appears exactly once in `cmd/init.go` — in the const declaration — and all seven sites derive from it.
- [ ] The bash shim's `expansion=` value and its `COMP_WORDS=` prefix come from the same const, so its `COMP_LINE`/`COMP_POINT` rewrite cannot disagree with itself.
- [ ] No emitted text is added or removed, and no shell suite is edited to accommodate the change.

**Tests**:
- `"portal init zsh emits the x function and the _portal_open shim unchanged"` — `TestInitZsh`, `TestInitZsh_CmdFlag`
- `"portal init bash emits the x function and the __start_portal_open shim unchanged"` — `TestInitBash`, `TestInitBash_CmdFlag`
- `"portal init fish emits the x function and its -w 'portal open' registration unchanged"` — `TestInitFish`, `TestInitFish_CmdFlag`
- `"a Tab after x still asks portal for the open completions"` — `TestInitCompletion_AsksPortalForOpenCompletions`, `TestInitCompletion_FollowsTheConfiguredFunctionName`
