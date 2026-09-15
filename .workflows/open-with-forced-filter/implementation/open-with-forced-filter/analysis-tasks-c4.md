# Analysis Tasks: Open With Forced Filter (Cycle 4)

## Task 1: Give Init one place the progress receiver enters the batch
severity: duplication
sources: duplication

**Problem**: `Model.Init` (`internal/tui/model.go`) dispatches into three shapes — command-pending, loading page, and the plain warm picker — and the rule "if a bootstrap is running behind this model, subscribe to its channel" is authored separately inside two of them (model.go:1542, model.go:1555). The third pair of returns (model.go:1571-1574) carries no subscription at all; it is correct today only because `openTUI` never sets `cfg.progressReceiver` on that route, which is a fact about a different package. A branch that omits the append never receives `BootstrapCompleteMsg`, `BootstrapProgressMsg` or `BootstrapFatalMsg`: the orchestrator's soft warnings (a down saver daemon, corrupt state) are silently dropped and a bootstrap fatal never ends the run, so the picker keeps acting against a half-bootstrapped server and mints into it. The divergence is silent — a missing subscription changes nothing observable wherever the receiver is nil, which is every route the unit lane exercises — and this exact shape already failed once in this work unit: the command-pending arm shipped without the receiver and needed a dedicated task to add it.

**Solution**: Build the branch-specific `cmds` as now, then append `m.progressReceiver` once on a common tail before `tea.Batch`, so the rule has one site and a newly added branch cannot omit it. No guard is needed on the warm routes — `tea.Batch` routes through `compactCmds`, which drops nil commands (verified against bubbletea v2.0.7, `commands.go:15`). The loading branch's warm-route `BootstrapCompleteMsg` synthesis stays where it is, keyed on the same `m.progressReceiver == nil` test: that arm is a genuine per-branch decision about satisfying the `bootstrapComplete` gate, not a restatement of the subscription rule. Behaviour-preserving on every route that exists today — the two branches that append the receiver still get it, and the third never has one to append. This extends the ad-hoc pass's approved direction (issue the receiver on the command-pending branch) rather than reversing it: the receiver still reaches that branch, from one place instead of three.

**Outcome**: `Init` carries exactly one site where `m.progressReceiver` enters the batch, reached by every branch including the plain warm one, so a fourth branch added later subscribes without its author having to know the rule exists.

**Do**:
- In `internal/tui/model.go`'s `Init` (model.go:1525-1575), restructure the three arms so each builds the branch-specific `cmds` on top of the shared `requestBg` and `detectTimeout` and then falls through to one tail — `cmds = append(cmds, m.progressReceiver)` followed by a single `return tea.Batch(cmds...)`. The command-pending arm contributes `m.loadProjects()`; the loading arm contributes `fetchSessions`, `loadingPadTick` and `loadProjects`; the plain warm arm contributes `fetchSessions` and `loadProjects`.
- Delete both per-branch subscription sites (the `if m.progressReceiver != nil { cmds = append(cmds, m.progressReceiver) }` at model.go:1542-1544 and the then-arm at model.go:1555-1558). The tail append takes no nil guard.
- Keep the loading arm's warm-route synthesis inside that arm, re-expressed as a standalone `if m.progressReceiver == nil { cmds = append(cmds, func() tea.Msg { return BootstrapCompleteMsg{Warnings: pending} }) }` — it decides whether the `bootstrapComplete` gate is satisfied from the first tick, not whether to subscribe.
- Drop the `if loadProjects != nil` guards that only gate an append into the batch (model.go:1565-1567, 1571-1573), keeping `loadProjects` computed once as now.

**Acceptance Criteria**:
- [ ] `m.progressReceiver` is appended to the batch at exactly one site in `Init`, and every path out of `Init` returns through the single `tea.Batch(cmds...)` tail
- [ ] The only other mention of `m.progressReceiver` inside `Init` is the loading arm's `== nil` test gating the `BootstrapCompleteMsg` synthesis
- [ ] The command-pending route still issues the receiver when one is wired and issues no bootstrap message when it is nil
- [ ] The loading route still lets the channel own the terminal `BootstrapCompleteMsg` on the cold path and still synthesizes one on the warm path
- [ ] `go test ./internal/tui/... ./cmd/...` passes with no edit to any test file

**Tests**: behaviour-preserving refactor — no test semantics change and no test file is edited. The existing suites that must stay green:
- `TestCommandPendingBootstrapDelivery` — `"it issues the progress receiver for a command-pending model on the cold route"` and `"it issues no bootstrap message for a command-pending model on the warm route"` (`internal/tui/command_pending_bootstrap_test.go`)
- `TestConcurrentInit_IncludesProgressReceiver` and `TestConcurrentInit_DoesNotSynthesizeBootstrapComplete` (`internal/tui/bootstrap_progress_test.go`)
- `"Init emits BootstrapCompleteMsg from PageLoading first event-loop tick"`, `"Init does not emit BootstrapCompleteMsg when not on PageLoading"` and `"Init schedules a single LoadingMinElapsedMsg via tea.Tick when on PageLoading"` (`internal/tui/model_test.go`)

## Task 2: Derive the search completer's offered set from the rules that own it
severity: duplication
sources: duplication, architecture

**Problem**: `completeSearchTerm` (`cmd/completion.go:70-87`) re-authors two rules that each have a declared home elsewhere. It decides which candidates compose a legal search word with its own `strings.Contains(name, "/")` test rather than through `resolver.IsSearchSigil` (`internal/resolver/path.go:25`), which is the single declaration of "a leading slash and no further slash" and which both the parser (`IsPathArgument`) and the completer's own dispatch (`completeOpenPositional`, completion.go:92) route through; and it restates the searched set inline as `if current != "" && name == current { continue }` rather than through `tui.PickerSessions` (`internal/tui/picker_sessions.go:13`), which the other two consumers of that set — `searchCandidates` for the count and `Model.filteredSessions` for the list — both take. If either rule moves, Tab keeps offering the old answer: the user accepts a completed word that the parser then reads as a path and Portal mints a session at a directory instead of searching, or accepts a session name the search cannot reach and lands on a picker filtered to zero rows with nothing on screen accounting for it — the exact defect the attached-session corrigendum already recorded once. Nothing fails: `cmd/completion_test.go` pins the completer's behaviour against completion's own seams, so it stays green while the rules it copies have moved. The shape of the seam is what forces the second copy — `completionSessionNames()` returns `[]string` and so cannot be handed to a function defined over `tmux.Session` values.

**Solution**: Put both hold-backs on the declaring functions. Replace the `strings.Contains` test with a question put to the recogniser (`resolver.IsSearchSigil("/" + name)`), so the offered set is shaped by the same function that parses the accepted word. Change the completion seam to enumerate sessions rather than names (`completionSessions() []tmux.Session`, with the existing name-based completer mapping to names at its own call site) and build the candidate set from `tui.PickerSessions(completionSessions(), completionCurrentSession())`, so the offered set is derived from the searched set rather than re-authored beside it. `cmd` already imports both packages, so no new edge appears. The slash-bearing-name exclusion's *reason* stays completion's own — a completed name carrying a slash composes a word the parser reads as a path — it is only the shape test that moves. Byte-identical offered set on every input today; the doc comment on `completeSearchTerm` then describes rules it asks for rather than rules it holds.

**Outcome**: `completeSearchTerm` holds neither rule: its candidate set comes out of `tui.PickerSessions` and its shape hold-back out of `resolver.IsSearchSigil`, so narrowing or widening either one moves what Tab offers with it, and the offered set is unchanged on every input today.

**Do**:
- In `cmd/completion.go`, replace the `completionSessionNames` seam with `var completionSessions = func() []tmux.Session` over `tmux.DefaultClient().ListSessions()`, returning nil on error exactly as the name-based seam does, and keep the seam's stated reason for building its own client (the bootstrap-exempt `__complete` path carries no context client).
- Have `completeSessionNames` read `s.Name` off `completionSessions()` at its own call site, so the plain session-name completer's offered set is unchanged — it still offers the attached session, and internal `_`-prefixed sessions are still filtered by `ListSessions` itself.
- Build `completeSearchTerm`'s loop over `tui.PickerSessions(completionSessions(), completionCurrentSession())` and delete the inline `if current != "" && name == current { continue }` (completion.go:79-81) along with the now-unused `current` local.
- Replace the `strings.Contains(name, "/")` hold-back (completion.go:76-78) with `if !resolver.IsSearchSigil("/" + s.Name) { continue }`.
- Re-voice the doc comment on `completeSearchTerm` so it names the functions the two hold-backs come from instead of restating the rules — one line, wording left to the executor.
- In `cmd/completion_test.go`, rename `withCompletionSessionNames` to `withCompletionSessions` over `func() []tmux.Session` and reseed every existing case with `tmux.Session` values, leaving each case's assertions and failure text exactly as they are.

**Acceptance Criteria**:
- [ ] `cmd/completion.go` carries no `strings.Contains` test and no `name == current` test — both hold-backs are function calls into `resolver` and `tui`
- [ ] `completeSearchTerm` reaches its candidates only through `tui.PickerSessions`
- [ ] The offered set is unchanged on every input the suite covers: `/po`, `/`, `/ort`, a slash-bearing name, the attached session, an empty current-session read, and a failed session read
- [ ] `completeSessionNames` still offers the attached session, so the bare positional and `-s` completions are untouched
- [ ] `TestCompletionExcludesInternalSessions` still excludes `_portal-x` from both the plain and the search completers
- [ ] No new package edge — `cmd` already imports `internal/tmux` and `internal/tui`

**Tests**: behaviour-preserving refactor — the seam's type changes, so each existing case is reseeded with `tmux.Session` values, and no assertion changes. The suites that must stay green, unchanged in substance:
- `TestCompleteSearchTerm` — all nine subtests (`"it completes the term after the slash and keeps the slash on the candidate"`, `"it offers every live session name for a bare slash"`, `"it offers nothing for a term that prefixes no name"`, `"it never offers a session name containing a slash"`, `"it holds back a slash-bearing name for the empty term too"`, `"it does not offer the session the caller is attached to"`, `"it holds back the attached session for a bare slash too"`, `"it offers every live name when the current-session read answers nothing"`, `"it offers no candidates when the session read fails"`)
- `TestCompleteSessionNames` — including `"it still offers the attached session on the plain session-name completer"`
- `TestCompleteOpenPositional`, `TestSearchCompletionWiring` and `TestCompletionExcludesInternalSessions`

## Task 3: Let the recorded search decision drive the teardown's warning branch
severity: duplication
sources: architecture

**Problem**: `resolveSearchDecision` (`internal/tui/search_decision.go:37-56`) writes three pieces of state for one outcome — `m.selected`, `m.searchAttached` and `m.searchErr`. Production reads `selected` and `searchErr`; `m.searchAttached` and its exported `SearchAttached()` have no production consumer anywhere in the tree (verified — the nine call sites are all `_test.go`). `WarningsOwedAtTeardown` (`internal/tui/model.go:484-489`), the one place that must ask "did the gate decide without painting a picker frame?", re-derives the answer as `activePage == PageLoading && (selected != "" || searchErr != nil)` rather than reading the flag that records it — and `m.selected` has five other writers in the model. So the branch and the assertions that claim to cover it are keyed on different facts and the assertions cannot fail when the branch does: if the decision ever signals the attach by another route, `SearchAttached()` stays true and every test pinning "the decision attached" stays green while the teardown falls through to the staged branch and the user's accumulated soft bootstrap warnings are silently dropped on a cold-boot `x /term` attach — noticed only as a missing "saver down" line nobody can reproduce.

**Solution**: Make the recorded flag load-bearing rather than deleting it: `WarningsOwedAtTeardown` branches on `m.searchAttached || m.searchErr != nil`, and `SearchAttached()` keeps its accessor, now covering a production branch. Settled this way rather than the delete-the-flag alternative on the record: cycle 3's approved task settled that the teardown's answer belongs in the model "beside the state it reads", and the recorded decision is exactly that state — deleting the flag would leave the teardown re-deriving a decision from `m.selected`, a field five other arms also write, which is the coupling this finding names. The equivalence rests on `searchAttached`/`searchErr` being written only by the loading-gate resolution, which returns `tea.Quit` before any page transition, so the dropped `activePage == PageLoading` conjunct is implied rather than lost — the one property the change turns on. Behaviour-preserving on every route: attach, read-failure, cancelled loading page, warm picker and command-pending all keep the set they are owed today.

**Outcome**: One fact has one representation — the teardown's warning branch and the tests that assert "the decision attached" read the same two fields, so an assertion can no longer stay green while the branch it claims to cover falls through.

**Do**:
- In `internal/tui/model.go`, change `WarningsOwedAtTeardown` (model.go:484-489) to return `m.bufferedWarnings` when `m.searchAttached || m.searchErr != nil` and the staged set otherwise, dropping both the `m.activePage == PageLoading` conjunct and the `m.selected != ""` test.
- Re-voice that function's doc comment so it describes the recorded decision it now reads rather than the page-and-selection derivation it replaces, keeping the cancelled-loading-page paragraph (which stays true — a cancelled page records no decision) — wording left to the executor.
- Leave `resolveSearchDecision` (`internal/tui/search_decision.go:38-56`) writing all three fields exactly as it does, and leave `SearchAttached()` in place.

**Acceptance Criteria**:
- [ ] `WarningsOwedAtTeardown` reads `m.searchAttached` and `m.searchErr` and nothing else — no `activePage` test and no `m.selected` test
- [ ] `m.searchAttached` has a production consumer: written in `resolveSearchDecision`, read by the teardown branch, exposed by `SearchAttached()`
- [ ] A search attach and a failed search read each still owe the buffered set
- [ ] A warm picker, a picker-decision search, a cancelled loading page and a command-pending run each still owe the staged set
- [ ] A session selected by Enter on the picker still owes the staged set — that arm writes `m.selected` without writing `m.searchAttached`
- [ ] `go test ./internal/tui/... ./cmd/...` passes with no edit to any test file

**Tests**: behaviour-preserving refactor — no test semantics change and no test file is edited. The existing suites that must stay green:
- `TestWarningsOwedAtTeardown` — all five subtests (`"it owes the buffered set when the loading gate quit on a named session"`, `"…quit on a failed session-list read"`, `"it owes the pending set when a picker frame was painted"`, `"it owes nothing once a loading gate has surfaced the buffer"`, `"it owes nothing when the loading page was cancelled"`), whose `SearchAttached()` / `SearchError()` pre-checks now guard the very branch the assertion below them exercises (`cmd/open_search_warnings_test.go`)
- `internal/tui/search_decision_test.go` — the attach, picker-decision and no-closure cases
- `TestFinishTUI` (`cmd/open_search_warnings_test.go`) and the integration-lane `cmd/concurrent_search_decision_integration_test.go`

## Task 4: Make the constructor enforce the picker landing's exclusivity
severity: low
sources: architecture

**Problem**: `tui.Deps` admits both a search form and an initial filter (`internal/tui/build.go:52-56`), and the "exactly one applies" rule lives only in the `cmd` caller. `Build` applies `WithSearchForm` when `deps.Search != nil` (build.go:132-134) and `WithInitialFilter` when `deps.InitialFilter != ""` (build.go:177-179) with no relation between them; `applyInitialFilter` then commits the `-f` text and `applySearchLanding` immediately overwrites it (model.go:1313-1314). A caller that populates both — a capture fixture, or a second `cmd` route added later — gets neither a refusal nor a diagnostic, only a landing decided by statement order inside the model, noticed as a wrong filter with nothing to explain it. `cmd` already models the landing as a union (`pickerLanding` carrying `search *tui.SearchForm` beside `filter string`, settled in cycle 3) and then flattens it into two independent `Deps` fields, so the layer that owns the invariant discards the type that expressed it and leaves it as a comment on `applySearchLanding` stating something the type does not enforce.

**Solution**: Have `Build` state the precedence itself — apply `InitialFilter` only when `deps.Search == nil`, one `else if` — so the search form's landing wins by construction rather than by the order two `apply*` calls happen to run in. The comment on `applySearchLanding` then describes what the constructor guarantees rather than what a caller happens to do. The union stays in `cmd`, where it already forecloses the combination on the only production route; this closes the same hole at the boundary that union is flattened across. No behaviour change on any route that exists today — no current caller populates both fields.

**Outcome**: A `Deps` carrying both a search form and an initial filter lands on the search term because `Build` decided it, not because `applySearchLanding` happens to run after `applyInitialFilter`, and a test pins that precedence at the constructor.

**Do**:
- In `internal/tui/build.go`, gate the initial-filter application on the search form's absence — `if deps.Search == nil && deps.InitialFilter != ""` at build.go:177 — which is the `else` half of the decision the `deps.Search != nil` arm at build.go:132-134 already takes.
- Re-voice the comment above `applySearchLanding` (`internal/tui/model.go:1318`) so it states the guarantee the constructor now makes rather than the discipline a caller is trusted with — one line, wording left to the executor.
- Add the precedence case to `internal/tui/search_landing_test.go` (`package tui`, beside `searchLanding`/`ingestLanding`): build a `Deps` carrying both `Search: &SearchForm{Term: "myapp"}` and `InitialFilter: "other"`, ingest the standard session set, and assert the committed filter is the search term.
- Leave `cmd`'s `pickerLanding` union and `buildTUIModel`'s if/else exactly as they are — the union is what forecloses the combination on the production route, and this closes the boundary it is flattened across.

**Acceptance Criteria**:
- [ ] `Build` applies `WithInitialFilter` only when `deps.Search == nil`
- [ ] A `Deps` with both fields populated produces a model whose session-list filter value is the search term, in `FilterApplied` state, landed on the Sessions page
- [ ] A `Deps` with `InitialFilter` alone is unchanged — `-f` still commits its text and still decides its own landing page
- [ ] A `Deps` with `Search` alone is unchanged — term committed, or focused-and-empty for a term-less form
- [ ] `cmd/open.go`'s `buildTUIModel` is untouched

**Tests**:
- `"it lands on the search term when a Deps carries both a search form and an initial filter"` — both populated; the filter value is the search term, never the `-f` text
- `"it commits the initial filter when no search form is present"` — the existing `-f` landing, unchanged
- `"it lands a search term on the Sessions page with the filter committed"` — the existing `TestSearchFormLanding` case, unchanged

## Task 5: Build the search-results fixture's home paths from the running process's home
severity: medium
sources: standards

**Problem**: `sessionsSearchResultsFixture` (`internal/capture/fixtures.go:462-470`) hardcodes its session directories under a literal `/home/user`, while `fitSessionDir` renders them through `resolver.AbbreviateHome`, which folds a path only when it sits under the *process's* `$HOME`. So a reviewer running the one live-view route the project documents — `go run ./cmd/capturetool --fixture sessions-search-results` — sees `/home/user/code/portal` beside every row instead of `~/code/portal`: the home-abbreviation rule this feature introduces, and the rule the frame exists to show, is invisible in the frame being signed off, and the widths the truncation ladder is judged at are wrong by the length of the home prefix. Pinning `HOME` on that command is not an escape — `HOME=/home/user go run …` fails outright at the Go build cache. The consequence has been worked around three times instead: `testdata/vhs/sessions-search-results.tape:29-30` splits build from run so it can prefix `HOME=/home/user`, `internal/capture/capture_test.go:981` pins `HOME` before the frame read, and the swap-and-diff completeness guard's own entry (`internal/capture/swap_harness_test.go:66-68`) records that it cannot judge the abbreviated forms at all and asserts only `/opt/portal-tools`, the one path absolute by design. The fixture's doc comment claims the frame carries "a home path shown abbreviated", which holds under the pin and not under the documented command.

**Solution**: Derive the fixture's home-relative directories from `os.UserHomeDir()`, degrading to the current literal when it errors, and apply the same derivation to the matching `project.Project` paths so the index still pairs with them. `AbbreviateHome` then folds them to `~/code/portal` under any `$HOME`, and `/opt/portal-tools` stays absolute by construction, so the frame carries every branch of the column the fixture was written to show — abbreviated, absolute, dir-matched, empty-dir and deep-truncated — on whatever machine renders it. Determinism is preserved in the sense the harness needs: the *rendered* text is fixed (`~/code/portal`) even though the underlying path is not, which is the opposite of today, where the underlying path is fixed and the rendered text moves with the environment.

**Outcome**: The `HOME` pin comes out of the tape and out of `capture_test.go`, and the swap-and-diff entry asserts the abbreviated forms it currently has to sit out.

**Do**:
- In `internal/capture/fixtures.go`, add an unexported helper answering the home directory the fixture's paths are built under: `os.UserHomeDir()`, degrading to the literal `/home/user` when it errors or answers empty — the same degrade `resolver.AbbreviateHome` takes, so the fixture and the renderer agree on every machine.
- Build `sessionsSearchResultsFixture`'s four home-relative session `Dir`s from it with `filepath.Join` — `code/portal`, `code/portal-gateway`, `code/portal/internal/capture/testdata/reference/design-exports/frames` and `code/evvi` — leaving `portal-notes`'s `/opt/portal-tools` absolute and `legacy-port-shim`'s absent `Dir` absent.
- Build the three home-relative `project.Project` paths (`code/portal`, `code/portal-gateway`, `code/evvi`) from the same helper, leaving `/opt/portal-tools` absolute, so `project.Index` still pairs with the sessions' directories and the tags still group.
- Leave every other fixture's `/home/user` literal untouched: the directory column renders only for a search-opened picker (`ShowDir: m.searchForm`), so no other frame is affected.
- Remove the `searchResultsHome` constant, its doc comment and the `t.Setenv("HOME", …)` from `internal/capture/capture_test.go:965-981`, and add a subtest to `TestSessionsSearchResultsFixture` that reads the frame under `t.Setenv("HOME", t.TempDir())` and still finds the abbreviated form — the property the pin used to manufacture.
- Collapse both tapes — `testdata/vhs/sessions-search-results.tape:27-32` and `testdata/vhs/sessions-search-results-nocolor.tape:28-33` — to one typed `go run ./cmd/capturetool --fixture sessions-search-results --theme tokyo-night` line (the nocolor tape keeping its `NO_COLOR=1` prefix) with a single sleep covering compile and render, and drop each tape's header paragraph explaining the `$HOME`/build-cache split.
- In `internal/capture/swap_harness_test.go:65-67`, extend the `sessions-search-results` entry's `present` set with the abbreviated forms it currently sits out — `~/code/portal` and `~/code/portal-gateway` — and delete the comment saying they cannot be judged there.

**Acceptance Criteria**:
- [ ] `go run ./cmd/capturetool --fixture sessions-search-results` on a machine whose home is not `/home/user` renders `~/code/portal` beside `portal-a1b2`, `~/code/portal-gateway` beside `api-work`, `/opt/portal-tools` beside `portal-notes`, an empty slot beside `legacy-port-shim` and a `…/`-prefixed tail on the deepest path
- [ ] No `HOME` pin for this fixture survives anywhere — `rg -n 'HOME=/home/user|searchResultsHome' testdata/vhs internal/capture` returns nothing
- [ ] Each tape runs `capturetool` in one typed line, with no separate build step
- [ ] The swap-and-diff entry for `sessions-search-results` asserts `~/code/portal` and `~/code/portal-gateway` alongside `/opt/portal-tools`, and the completeness assertion still covers every build-backed fixture
- [ ] The rendered text is identical to today's pinned frame, so the committed PNGs stay accurate and are not re-captured
- [ ] No other fixture's session directories or project paths change

**Tests**:
- `"it renders the fixture's home paths abbreviated under an arbitrary home directory"` — `t.Setenv("HOME", t.TempDir())`, then assert the frame carries `portal-a1b2 ~/code/portal`
- `"it renders a home-abbreviated directory beside the session name"` — the existing case, now passing with no `HOME` pin
- `"it leaves a directory outside the home directory unabbreviated"` — `/opt/portal-tools` is absolute by construction, so it is unaffected by the home the process runs under
- `"it left-truncates the longest path in the frame"` — the deepest path still reaches the `…/` rung at the harness width once abbreviated
- `"it renders an empty directory slot for a session carrying none"` — `legacy-port-shim` still carries no `Dir`
- `TestModelAt_ReachesCapturedState/sessions-search-results` — the swap-and-diff entry, now judging the abbreviated forms
