---
phase: 4
phase_name: Cold-Boot Classification
total: 5
---

# Phase 4: Cold-Boot Classification — 5 tasks

## open-with-forced-filter-4-1

### Task 4-1: Hold the loading page until the search decision resolves

**Problem**: On the concurrent route the model parks on the loading page and leaves it only when both gates close — `bootstrapComplete` and `minElapsed`, the two arms at `internal/tui/model.go:1533` and `:1550`, each calling `transitionFromLoading` (`:1418`). A search form on a cold boot needs a third outcome at exactly that instant: its match count cannot be taken before the tmux server is answering and every saved session is back, so the decision the count drives — attach, picker, or a failed read — has to be issued at the gate rather than before the TUI ran. The model has no seam through which that decision can arrive, and its failure shape must not be confused with the bootstrap fatal: `FatalError()` (`:435`) is what drives the in-TUI error frame and parks the model on the failed loading page, which a failed session-list read on this path must not do.

**Solution**: A nil-tolerant decision closure carried on the Phase 1 `tui.SearchForm` value, invoked exactly once at the loading→picker gate, whose three answers are a session name (set `selected`, quit without ever painting the picker), an error (retained on its own accessor, quit) and nothing (transition to the picker exactly as today).

**Outcome**: A model built with a decision closure holds the loading page until both gates close and then either quits with `Selected()` set and its buffered warnings still unsurfaced, quits with `SearchError()` set while `FatalError()` stays nil, or transitions to the pre-filtered picker; a model built without one behaves exactly as it does today.

**Do**:
- Add `Decide func() (string, error)` to `SearchForm` (`internal/tui/build.go`, the struct task 1-3 added), documented as: a non-empty name is the session to attach, `("", nil)` opens the picker, a non-nil error is a failed session-list read. Inside `Build`'s existing `if deps.Search != nil` block append `WithSearchDecision(deps.Search.Decide)` beside `WithSearchForm(deps.Search.Term)`; the option is nil-tolerant.
- Add `searchDecide func() (string, error)`, `searchAttached bool` and `searchErr error` to `Model` (`internal/tui/model.go`), beside task 1-3's `searchForm` / `searchTerm`.
- Add `internal/tui/search_decision.go` holding `func WithSearchDecision(fn func() (string, error)) Option`, `func (m *Model) resolveSearchDecision() tea.Cmd`, and the accessors `func (m Model) SearchAttached() bool` and `func (m Model) SearchError() error`.
- `resolveSearchDecision` returns nil when `m.searchDecide == nil`; otherwise it clears the field before calling the closure, then returns `tea.Quit` after setting `m.selected` and `m.searchAttached` for a non-empty name, returns `tea.Quit` after setting `m.searchErr` for a non-nil error, and returns nil when the closure answers `("", nil)`. It must set none of `fatalActive`, `fatalStep`, `fatalMessage`, `fatalErr`, and must call the closure synchronously rather than wrapping it in a `tea.Cmd`.
- Call it from both loading-gate arms ahead of `transitionFromLoading()`: in the `LoadingMinElapsedMsg` arm (`:1533`) inside the existing `m.bootstrapComplete && m.activePage == PageLoading` branch, and in the `BootstrapCompleteMsg` arm (`:1550`) inside the existing `m.minElapsed && m.activePage == PageLoading` branch — `if quit := (&m).resolveSearchDecision(); quit != nil { return m, quit }`. In the second arm it sits after the existing `m.bufferedWarnings = msg.Warnings` assignment (`:1559`). Comment at one of the two call sites that the decision must precede `surfaceBufferedWarnings`.
- Leave the `fatalActive` guards, the `BootstrapProgressMsg` arm (`:1545`), the loading page's key handling and `transitionFromLoading` itself untouched.
- Add `internal/tui/search_decision_test.go` (`package tui`), driving models through the `coldTUIModel` / `transitionWithWarnings` idiom already in `internal/tui/sessions_postload_warning_test.go`.

**Acceptance Criteria**:
- [ ] A decision returning a session name leaves the model on `PageLoading`, sets `Selected()` to that name, sets `SearchAttached()`, and returns a quit command — no picker frame is ever composed
- [ ] On that attach `BufferedWarnings()` is still populated and no flash/notice band was set — `surfaceBufferedWarnings` did not run
- [ ] A decision returning `("", nil)` transitions to the picker exactly as today, warnings surfacing through the existing path
- [ ] A decision returning an error sets `SearchError()`, leaves `FatalError()` nil and the fatal state unset, stays on `PageLoading` and returns a quit command
- [ ] The decision fires only when both gates have closed: complete-then-min and min-then-complete each invoke it exactly once, and neither gate alone invokes it
- [ ] No `BootstrapProgressMsg` — including a restore-counter event — invokes the decision
- [ ] A `BootstrapFatalMsg` followed by `LoadingMinElapsedMsg` never invokes the decision and keeps the error frame (`FatalError()` non-nil, page still `PageLoading`)
- [ ] The closure is invoked at most once across any message sequence, including a duplicate `BootstrapCompleteMsg`
- [ ] `Ctrl-C` on the loading page before the gates close quits with `Selected()` empty and the decision uninvoked
- [ ] With no decision closure supplied, every existing loading-page transition, warning surfacing and refetch behaviour is byte-identical (existing suites green unmodified)
- [ ] `go test ./...` passes

**Tests**:
- `"it attaches the single match without painting the picker"` — page, `Selected()`, `SearchAttached()`, quit command
- `"it leaves the buffered warnings unsurfaced on an attach"` — `BufferedWarnings()` non-empty, no notice band
- `"it opens the picker when the decision returns no session"` — page `PageSessions`, warnings surfaced as today
- `"it records a read failure without setting a fatal"` — `SearchError()` non-nil, `FatalError()` nil, page `PageLoading`
- `"it holds the decision until the minimum display span elapses"` — complete first: zero calls; then `LoadingMinElapsedMsg`: one call
- `"it holds the decision until the bootstrap completes"` — min first: zero calls; then `BootstrapCompleteMsg`: one call
- `"it issues no decision on a progress event"` — several `BootstrapProgressMsg` including one carrying restore counters
- `"it issues no decision after a bootstrap fatal"` — `BootstrapFatalMsg`, then both gates
- `"it issues the decision exactly once"` — both arms plus a duplicate complete
- `"it selects nothing when Ctrl-C arrives before the decision resolves"`
- `"it leaves a picker with no decision closure unchanged"` — the existing transition expectations re-asserted with `Decide` nil

**Edge Cases**:
- A fatal step never produces a `BootstrapCompleteMsg` (the pipe converts it to `BootstrapFatalMsg`), so `bootstrapComplete` stays false and the gate the decision hangs off never opens — the `fatalActive` guards already in both arms are what keep a stray complete from re-opening it
- The decision must sit after the `bufferedWarnings` assignment and before `surfaceBufferedWarnings`: that helper clears the buffer on every transition, so an attach that ran it would have nothing left for the caller to write (task 4-3)
- The closure is called synchronously inside `Update`. The loading page is already painted, the read behind it is a single `list-sessions` round trip, and the alternative — bouncing through a `tea.Cmd` — needs a second gate to stop the transition firing first
- `Model` is a value type Bubble Tea copies on every `Update`; the single-shot guard is the field clear on the copy that runs the decision, and that copy is the one returned, so no later copy can re-enter it
- A term-less form supplies no closure at all (task 4-2), so the seam is inert for `portal open /` on every route
- `transitionFromLoading` deliberately leaves `sessionsLoaded` false on the concurrent route, so the picker branch still lands its filter through the post-restore refetch — the decision arm must not pre-empt that by transitioning itself

**Context**:
> K is evaluated against the live session set once the tmux server is ready to answer for it. On a cold server the sigil takes the picker's concurrent-bootstrap path, so the count is taken once that bootstrap has run to completion — every step of it, not merely the restore that reconstructs the saved sessions. Nothing the sigil decides fires earlier: the loading page stands until the count can be taken, and is then replaced by the attach when K turns out to be 1, or by the picker at any other count.
>
> The loading page's own minimum display span is untouched by this: where the count can be taken before that span has elapsed, the page stands for the remainder of it, and the attach or the picker follows when it lifts.
>
> A session list that could not be read is not a zero match. A tmux read failure is reported in tmux's own terms and exits non-zero — the one failure path on this form. On a cold boot it reaches the user after teardown; it is not a bootstrap fatal and takes no in-TUI error frame.
>
> On K = 1 the TUI tears down before the connector runs, so the notice band never surfaces; the accumulated soft warnings are written to the terminal at that point instead — which is why this task leaves them buffered rather than surfacing them.
>
> When K turns out to be 1 the loading page appears and is replaced by the attach, so the user sees a brief screen they did not need. That is the accepted cost of classifying on the form rather than on the outcome.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.4, §3.7, §7.4, §7.5

## open-with-forced-filter-4-2

### Task 4-2: Defer the search count to the loading page when a bootstrap is in flight

**Problem**: `runSearchForm` (`cmd/open_search.go`, task 2-2) enumerates the live sessions and counts the matches synchronously, before `openTUIFunc` is ever called. On the concurrent route that read has nothing to answer it: `PersistentPreRunE` deliberately does not run the bootstrap there, stashing the orchestrator on the context instead (`cmd/root.go:122-133`) for `openTUI` to run in a goroutine behind the loading page (`cmd/open.go:575`). A count taken up front on a cold boot therefore runs against a server that has not been started, with no saved session restored — every term would answer K = 0 and open an empty picker over a machine full of sessions. The count must be handed to the loading page rather than taken before it.

**Solution**: When the invocation carries a deferred bootstrap, `runSearchForm` takes no count and hands the picker a closure that takes the identical count later, which task 4-1's seam invokes once the whole bootstrap has completed.

**Outcome**: `portal open /port` on a cold server reaches `openTUI` having issued no `list-sessions`, and the count is taken from inside the TUI at the loading gate — attaching the lone match through the connector built before the TUI ran, opening the pre-filtered picker at any other count, and surfacing a failed read as a non-zero exit that is neither a usage error nor a bootstrap fatal; a warm or latched invocation still counts up front with no closure supplied.

**Do**:
- Add `decide func() (string, error)` to `pickerLanding` (`cmd/open_search.go`, task 1-5) and set `Deps.Search.Decide = landing.decide` in `buildTUIModel` (`cmd/open.go:521`) beside the `Term` it already sets from the landing.
- Add `func searchDecision(src SearchSessionSource, term string) func() (string, error)` to `cmd/open_search.go`: the closure runs `searchCandidates(src)` and `searchMatches(term, …)` — task 2-2's own two helpers, not a second implementation — and returns `(matches[0].Name, nil)` for exactly one match, `("", nil)` for any other count, and `("", err)` for an enumeration error.
- In `runSearchForm`, after the term-less early return and before the synchronous count, branch on `deferredBootstrapFromContext(cmd) != nil` (`cmd/bootstrap_context.go:23`): resolve the source once with `buildSearchSessionSource(cmd)` — which issues no tmux read — and return `openTUIFunc(cmd, pickerLanding{filter: term, search: true, decide: searchDecision(src, term)}, nil, serverWasStarted(cmd))`. With no deferred bootstrap, leave task 2-2's synchronous count exactly as it is.
- Extend `processTUIResult` (`cmd/open.go:558`) with a `model.SearchError()` branch between the existing `FatalError()` branch and the `Selected()` branch, returning it unwrapped so `main.classify` prints it and exits 1.
- Leave the `-f` branch, the no-target branch and every other `openTUIFunc` call site passing a landing whose `decide` is nil.
- Add `cmd/open_search_deferred_test.go`, staging the deferred context with `context.WithValue(ctx, deferredBootstrapKey, &deferredBootstrap{})`, capturing the landing through `withFuncSeam(t, &openTUIFunc, …)` and injecting a call-recording source through `withOpenDeps(t, OpenDeps{SearchSessions: …})`.

**Acceptance Criteria**:
- [ ] With a deferred bootstrap on the context, `runSearchForm` calls neither source method before `openTUIFunc` runs, and the captured landing carries a non-nil `decide` alongside `{filter: term, search: true}`
- [ ] Invoking the captured closure returns the single matching session's name; zero matches and two-plus matches both return `("", nil)`; an enumeration error is returned unchanged
- [ ] The closure's candidate set is identical to the up-front count's — the same discriminating probe read, and inside tmux the current session excluded
- [ ] Without a deferred bootstrap the count is taken up front exactly as task 2-2 leaves it, and the landing's `decide` is nil
- [ ] The term-less form supplies no closure on either route and calls neither source method
- [ ] `-f <text>` and the no-argument picker pass a nil `decide`
- [ ] `processTUIResult` returns the model's search error without connecting anything, ahead of `Selected()` and behind `FatalError()`; the returned error is neither a `*UsageError` nor a `*bootstrap.FatalError`
- [ ] A single match found by the closure is connected by the connector `openTUI` built before `p.Run()` — the closure constructs no connector and performs no connect of its own
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it takes no count when a bootstrap is in flight"` — recording source, zero calls before the picker seam ran
- `"it hands the picker a decision closure on the deferred route"` — landing fields asserted
- `"it returns the single matching session from the deferred closure"`
- `"it returns no session for zero matches and for two or more"`
- `"it returns the enumeration error from the deferred closure"`
- `"it excludes the current session from the deferred closure's candidates"` — inside tmux (`t.Setenv("TMUX", …)`), the sole match being the current session
- `"it counts up front with no closure on a warm invocation"` — no deferred value on the context
- `"it supplies no closure for the term-less form on either route"`
- `"it supplies no closure for -f or for the no-argument picker"`
- `"it returns the model's search error without connecting"` — a stub connector recording zero calls
- `"it prefers the bootstrap fatal over the search error"` — both set, the fatal is returned

**Edge Cases**:
- A warm or latched invocation reaches `RunE` with no deferred value on the context, so the branch is a pure addition: nothing about the synchronous count's behaviour, ordering or error handling moves
- `buildSearchSessionSource` resolves the `*tmux.Client` from the context rather than reading tmux, so building the closure on a cold server is safe; the concurrent route always has a client on the context, because `shouldRunConcurrentBootstrap` refuses a nil one (`cmd/root.go:181`)
- The closure runs on Bubble Tea's goroutine after the orchestrator goroutine has sent its terminal event, so the two never issue tmux commands concurrently
- A failed read is an ordinary error, not a `*UsageError` and not a bootstrap fatal: the loading page is torn down first and no in-TUI error frame is painted, so the user sees tmux's own words on stderr and a non-zero exit
- The count the closure takes and the picker's own session read remain two separate reads, so a session can still vanish between them — the connector reports that in tmux's words, exactly as the warm route already does
- Any composition beside the form is refused by the `Args` validator before `PersistentPreRunE` (task 1-7), so no deferred search landing can carry a command or a pin

**Context**:
> K is evaluated against the live session set once the tmux server is ready to answer for it. On a warm server that is immediately. On a cold server the sigil takes the picker's concurrent-bootstrap path, so the count is taken once that bootstrap has run to completion — every step of it, not merely the restore that reconstructs the saved sessions. Acting at the end of restore would replace the process mid-bootstrap and abandon the steps that follow it — among them the clearing of the `@portal-restoring` marker, which must not outlive bootstrap.
>
> An attach under K = 1 uses the connector the invocation already selects — `syscall.Exec` into `tmux attach-session` outside tmux, `switch-client` inside it. The sigil introduces no third connection mode.
>
> A session list that could not be read is not a zero match: K = 0 says the search ran and found nothing, which is a filter result and opens the picker, while a failed read has searched nothing. It is reported in tmux's own terms and exits non-zero. The usage errors of the composition rule are refusals of a malformed command line, not outcomes of a search — which is why this failure is not one of them.
>
> Nothing about how the concurrent bootstrap behaves changes; this phase adds the sigil to the set of invocations that take it.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.4, §3.7, §7.1, §7.6

## open-with-forced-filter-4-3

### Task 4-3: Deliver the accumulated soft bootstrap warnings when a search ends without a picker

**Problem**: Soft bootstrap warnings reach the user by exactly three routes — `bootstrapWarnings.EmitTo(cmd.ErrOrStderr())` in `PersistentPreRunE` for a non-picker line (`cmd/root.go:112`, `:145`), `stageBootstrapWarningsOnModel` for the warm TUI path (`cmd/bootstrap_warnings.go:44`), and the progress channel into the model's `bufferedWarnings` for the concurrent one (`internal/tui/model.go:1559`). Once task 4-4 classifies a search form as a picker invocation, none of the three fires for a single-match attach: `PersistentPreRunE` stops writing because the line is now a picker line; the warm route never builds a model, because the count attaches before the TUI runs; and on the cold route the model quits at the loading gate, so the notice band never appears. A saver-down warning would be dropped silently on exactly the invocation that is about to hand the terminal over to tmux.

**Solution**: Write the accumulated warnings to stderr wherever a search form ends without a picker — the up-front count's attach and its failed read in `cmd`, and the TUI teardown that follows either outcome — all through the shared `warning.WriteLines` so the lines are byte-identical to the CLI path's.

**Outcome**: A `/term` invocation resolving to one session prints its soft bootstrap warnings and then attaches, on a warm or latched server and on a cold boot alike, and one whose session-list read fails prints them ahead of tmux's own error; a zero- or two-plus-match picker still routes them to the notice band and writes nothing after teardown; a cancelled loading page and an early-quit picker write nothing.

**Do**:
- In `runSearchForm` (`cmd/open_search.go`), on the up-front single-match branch, call `bootstrapWarnings.EmitTo(cmd.ErrOrStderr())` immediately before `openSessionFunc(cmd, matches[0].Name)`.
- Make the same call on the up-front branch that returns an enumeration error, immediately before returning it, so a failed session-list read surrenders its warnings ahead of tmux's own message.
- Add `func emitSearchTeardownWarnings(w io.Writer, model tui.Model)` to `cmd/open_search.go`: a no-op unless `model.SearchAttached()` or `model.SearchError() != nil`, otherwise `tui.WriteBootstrapWarnings(w, model.BufferedWarnings())`.
- Call it from `openTUI` (`cmd/open.go:569`) between `tui.RestoreTerminalBackground(os.Stdout, model)` (`:697`) and `processTUIResult(model, connector)`, passing `cmd.ErrOrStderr()`.
- Gate that call on those two search outcomes alone — never on `len(model.BufferedWarnings()) > 0`, which also holds after a `Ctrl-C` from the loading page, where today's behaviour is to drop them. A torn-down picker surrenders its warnings whichever way it tore down; a cancelled one does not.
- Add nothing to the picker branches: `surfaceBufferedWarnings` already owns the notice band and empties the buffer on every transition.
- Add `cmd/open_search_warnings_test.go`: drive the warm route by calling `runSearchForm` directly — `resetBootstrapWarnings(t)`, one warning added to the sink after that reset, a `*cobra.Command` whose `SetErr` is a buffer, the session source injected through `withOpenDeps` and a stubbed `openSessionFunc` recording call order — so the only thing that can put a line in that buffer is the write under test, whichever way the invocation classifies at this point in the phase; drive the teardown route by building a model with `tui.Build` carrying a decision closure, stepping it through `LoadingMinElapsedMsg` and `BootstrapCompleteMsg{Warnings: …}`, and calling `emitSearchTeardownWarnings` over a buffer.

**Acceptance Criteria**:
- [ ] A warm single-match attach writes every accumulated warning line to stderr, in order, before `openSessionFunc` is called
- [ ] Those lines are byte-identical to what `warning.WriteLines` produces for the same warnings — the CLI path's output for the same bootstrap
- [ ] The sink is empty afterwards, so no later drain writes the same warning twice
- [ ] A warm search whose session-list read fails writes every accumulated warning line to stderr before tmux's own error reaches the user
- [ ] On the concurrent route a recorded search error writes the model's buffered warnings after teardown, exactly as an attach does
- [ ] With no warnings accumulated, neither route writes a byte
- [ ] `emitSearchTeardownWarnings` writes exactly `model.BufferedWarnings()` when the model records an attach or a search error, and nothing otherwise
- [ ] On the concurrent route the write follows the terminal-background restore and precedes the connect, so the exec'd attach (which never returns) cannot pre-empt it
- [ ] A zero- or two-plus-match search picker surfaces its warnings in the notice band and writes nothing after teardown
- [ ] A loading page cancelled with `Ctrl-C` while warnings are buffered writes nothing
- [ ] Warning routing for every non-search invocation — CLI lines, `-f`, the no-argument picker, the domain pins — is unchanged (existing suites green unmodified)
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it writes the accumulated warnings before a warm single-match attach"` — stderr buffer plus recorded call order
- `"it drains the sink so a later emit writes nothing"` — a second `EmitTo` over the same buffer adds nothing
- `"it writes nothing when no warnings accumulated"` — both routes
- `"it writes the buffered warnings on a decision attach"` — model driven to `SearchAttached()`
- `"it writes nothing after teardown when the picker opened"` — decision returning `("", nil)`, warnings surfaced as a band
- `"it writes the accumulated warnings before a warm failed read"` — erroring source; stderr carries the lines, then the tmux error surfaces
- `"it writes the buffered warnings on a decision read failure"` — model driven to a non-nil `SearchError()`
- `"it writes nothing when the loading page was cancelled"` — `Ctrl-C` before the gates close, buffer non-empty, nothing written
- `"it writes the same lines as the CLI path"` — compared against `warning.WriteLines` over the same warnings
- `"it writes before connecting"` — the stub connector records that the buffer was already non-empty when it ran
- `"it leaves a -f picker's warning routing unchanged"`

**Edge Cases**:
- The sink drains itself on `EmitTo`, so the up-front write is idempotent against any later drain and an empty sink writes nothing at all
- On the concurrent route the model's buffer, not the sink, is the carrier: the sink was already drained into `pendingBootstrapWarnings` before the program started, and that field is folded into a `BootstrapCompleteMsg` only on the warm route, where the orchestrator had already run
- The write is gated on the recorded search outcome rather than on a non-empty buffer, because `surfaceBufferedWarnings` clears the buffer on every transition — leaving a non-empty buffer at teardown reachable only by an attach, a search error and an early quit, and the early quit must keep dropping them
- A failed session-list read delivers its warnings too: the picker is never painted on that path either, so the reasoning that puts them on the terminal for an attach — the band never surfaces, the alternate screen is gone — applies unchanged. The warning and the tmux error are the pairing where the first explains the second (a saver that is down is why the list could not be read), so they are written together, warnings first
- A cancelled loading page is the one torn-down picker that still drops them, because the user asked for nothing and is owed no report
- The outside-tmux connector execs and never returns, which is why the write cannot be deferred past `processTUIResult` on either route
- On a warm search attach the model never exists, so the two routes are genuinely disjoint — no invocation can take both
- This task lands before the classification flip (task 4-4), so a `/term` line still reads as a CLI line and `PersistentPreRunE` writes the warnings and drains the sink before `RunE`. An end-to-end `rootCmd.Execute()` assertion would therefore pass over output this task did not produce — which is why the warm route is driven through `runSearchForm` directly. The end-to-end stderr behaviour is pinned by task 4-4's own `"it holds warnings out of stderr for a search-form line"`

**Context**:
> On K = 1 the TUI tears down before the connector runs, so the notice band never surfaces. The accumulated soft warnings are written to the terminal at that point instead — after teardown, before the attach — which is where a warm-server sigil attach already puts them. The alternate screen is gone by then, so the corruption this classification prevents cannot occur.
>
> The same verdict decides where soft bootstrap warnings go: on the non-picker classification they are written straight to the terminal the picker is about to take over with its alternate screen, which is the corruption the classification exists to prevent.
>
> A sigil invocation is classified as a picker invocation: it takes the concurrent bootstrap and the honest loading page, and its soft bootstrap warnings take the in-TUI route rather than stderr.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §7.1, §7.2, §7.5

## open-with-forced-filter-4-4

### Task 4-4: Classify a search form as a picker invocation

**Problem**: `isTUIPath` (`cmd/root.go:168`) answers `cmd.Name() == "open" && len(args) == 0 && !anyOpenDomainPin(cmd)`, so anything positional classifies as not-heading-for-the-picker. A search form is a positional. Until that changes, `x /port` on a cold boot runs the ten-step bootstrap synchronously in `PersistentPreRunE`: a blank terminal for the whole multi-second restore, which reads as a hang and is the first impression the feature makes after every reboot, while `x -f port` on the same boot shows the loading page throughout. The same verdict decides where soft warnings go, so a saver-down line would be written straight into the terminal the picker is about to claim with its alternate screen. Tasks 4-1 to 4-3 have the re-timing and the warning delivery in place; this is the flip that puts them to work.

**Solution**: Widen the predicate by exactly one shape — a line whose target positionals carry a search form is a picker invocation — reusing the Phase 1 scan so the shape rule and the `--` rule are stated once.

**Outcome**: `portal open /port` and `portal open /` on a cold server take the concurrent bootstrap and the honest loading page exactly as `portal open -f port` does on the same boot, with their soft warnings held for the in-TUI route; a line refused for composition still starts no server, a domain pin still reads as non-picker, and every other invocation classifies exactly as before.

**Do**:
- Change `isTUIPath` (`cmd/root.go:168`) to `cmd.Name() == "open" && (len(args) == 0 || len(searchFormPositionals(cmd, args)) > 0) && !anyOpenDomainPin(cmd)`, taking the scan from `cmd/open_search.go` (task 1-5) rather than restating the shape test.
- Leave `anyOpenDomainPin` (`:176`), `shouldRunConcurrentBootstrap` (`:180`), the orchestrator's step set, `cmd/bootstrap/progress_emitter.go` and `internal/tui/loading_progress.go` untouched — this decides which invocations take the existing route, never how that route behaves.
- Extend `TestIsTUIPath` (`cmd/concurrent_bootstrap_gate_test.go:28`) with the search-form rows, reusing its `openProbeCmd` / `openProbeCmdWithFlags` helpers, including a parity row asserting the same verdict for `-f port` and `/port`.
- Add a cold-route test beside `TestPersistentPreRunE_ColdTUI_DefersBootstrap` (`cmd/concurrent_bootstrap_route_test.go:17`) driving `open /port` through `rootCmd.Execute()`: the orchestrator must record zero synchronous `Run` calls and `openTUIFunc` must observe a deferred bootstrap on the context.
- Leave `TestPersistentPreRunE_EmitsWarningsForOpenWithPositionalArg` (`cmd/bootstrap_warnings_test.go:255`) green on its multi-segment path fixture — a path positional is still a CLI line — and add its search-form counterpart asserting an empty stderr with the warning still in the sink.

**Acceptance Criteria**:
- [ ] `isTUIPath` is true for `open /port` and `open /`, and for a search form sitting at any positional index
- [ ] It is false for `open /Users/x/y`, `open /tmp/`, `open ./port`, `open ~/dir`, a bare word, two positional targets, every domain pin, and for any command other than `open`
- [ ] Words after a `--` separator are never inspected: `open ~/Code/api -- ls /tmp` is false
- [ ] A search form on the same line as a domain pin still reads as non-picker — a shape the validator refuses before the classification is ever consulted
- [ ] On a cold server `open /port` stashes a deferred bootstrap and the orchestrator runs zero times synchronously — the same verdict `open -f port` gets on the same boot
- [ ] On a latched server `open /port` takes the abridged path with `serverStarted=false` and no deferred bootstrap, exactly as a `-f` invocation does
- [ ] `PersistentPreRunE` writes no warnings to stderr for a search-form line, and leaves them in the sink; a path positional still writes them
- [ ] A line refused by `validateOpenArgs` never reaches the classification: no server is started, no loading page is painted, the exit is 2
- [ ] The warm path, every CLI path and the concurrent route's step sequence and labels are unchanged (existing suites green unmodified)
- [ ] `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

**Tests**:
- `"it classifies a search form as a picker invocation"` — `/port` and `/`
- `"it classifies a search form at any positional index as a picker invocation"`
- `"it still classifies a path positional as a CLI invocation"` — table over `/Users/x/y`, `/tmp/`, `./port`, `~/dir`, a bare word
- `"it never inspects words after a -- separator"` — `~/Code/api -- ls /tmp`
- `"it still classifies a domain pin as a CLI invocation"` — the existing pin table, plus a pin beside a search form
- `"it classifies a search form exactly as -f"` — parity row over the same probe command
- `"it defers the bootstrap for a cold search form"` — zero synchronous orchestrator calls, deferred value observed by the picker seam
- `"it takes the abridged path for a latched search form"` — no deferred bootstrap, `serverStarted=false`
- `"it holds warnings out of stderr for a search-form line"` — empty stderr, one warning still in the sink
- `"it starts no bootstrap for a refused composition"` — re-pinning task 1-7's property through the flipped classification

**Edge Cases**:
- `validateOpenArgs` runs before `PersistentPreRunE` (cobra validates args, then walks the pre-run hooks), so a refused composition can never reach this predicate — which is why the classification only has to be right for a lone search form
- The scan stops at `cmd.ArgsLenAtDash()`, so a command's own `/word` argument cannot flip an ordinary mint line onto the picker route
- `anyOpenDomainPin` keeps its veto ahead of the new arm: a pin dispatches one resolved target directly, and that is unchanged for every line the validator admits
- The probe commands in the gate suite register no `ack` flag and no positional parsing beyond cobra's defaults; both helpers the predicate calls tolerate a flag set that does not declare the flag
- The term-less form classifies identically to a term-carrying one — the scan matches the shape, never the term
- Nothing about the concurrent route itself moves: the same ten steps, the same five friendly labels, the same progress channel

**Context**:
> A sigil invocation is classified as a picker invocation. It takes the concurrent bootstrap and the honest loading page, and its soft bootstrap warnings take the in-TUI route rather than stderr.
>
> `/term` does not know whether it is heading for the picker — that depends on K, and on a cold server there are no sessions to match until restore has finished. The classification is therefore taken on the form rather than on the outcome. A brief loading page on the way to a direct attach costs a flicker; a silent multi-second blank terminal on every cold boot reads as a hang. The warnings argument points the same way — they need somewhere safe to land, and the picker path already has one.
>
> A refused line starts nothing: the refusal is decided from the arguments alone, so on a cold machine as on a warm one it prints its usage error and exits without starting the server, restoring a session or painting a frame. The picker classification applies to a complete sigil invocation only.
>
> Nothing about how the concurrent bootstrap behaves changes. This section adds the sigil to the set of invocations that take it.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §5.1, §7.1, §7.2, §7.3, §7.6

## open-with-forced-filter-4-5

### Task 4-5: Pin the search decision behind the whole bootstrap against a real cold server

**Problem**: Everything tasks 4-1 to 4-4 pin is asserted against synthetic messages: a hand-fed `BootstrapCompleteMsg` proves the decision waits for *a* complete event, not that the event arrived after all ten real steps. The property that matters is an ordering against a real orchestrator. Acting at the end of restore — the first moment a session list has anything in it — would replace the process mid-bootstrap and abandon the four steps that follow, among them step 8's clearing of `@portal-restoring`, whose leak suppresses the daemon's `captureAndCommit` indefinitely. That failure mode is invisible to a fake pipe, because a fake emits whatever the test tells it to.

**Solution**: An integration-tagged fixture that drives the real ten-step orchestrator through the real progress pipe into a real search-form model, with the decision closure recording what had already happened at the instant it was called.

**Outcome**: On a real cold boot over an isolated socket and state dir, the decision closure is invoked exactly once, after all ten step events and the terminal complete event, with `@portal-restoring` clear and the restored sessions visible to its own live read; the model stands on the loading page for every step before that, and never paints the picker when the decision attaches.

**Do**:
- Add `cmd/concurrent_search_decision_integration_test.go` under `//go:build integration` in `package cmd`, reusing `setupConcurrentColdBootEnv`, `buildConcurrentColdBootOrchestrator` and `concurrentBootDrainBudget` from `cmd/concurrent_coldboot_integration_test.go`.
- Seed saved sessions with `restoretest.SeedSessionsJSON(t, stateDir, …)` — two sharing one term fragment and one that does not — so the same fixture drives a K = 1 attach on a term matching a single name and a K ≥ 2 picker on the shared fragment.
- Start the real pipe (`pipe := newBootstrapProgressPipe(); pipe.start(context.Background(), orch)`) and build the model with `tui.Build(tui.Deps{Lister: client, ServerStarted: true, ProgressReceiver: pipe.receiver(), Search: &tui.SearchForm{Term: …, Decide: probe}})`.
- The `probe` closure records, at the moment it runs: how many real step events the drive loop has already delivered, `state.IsRestoringSet(client)`, and the session names its own `client.ListSessionsProbe()` returns; it then answers with the single match's name or `("", nil)`.
- Drive the model by hand: feed `tea.WindowSizeMsg{Width: 80, Height: 24}` and `tui.LoadingMinElapsedMsg{}`, then pump a single `receiver := pipe.receiver()` in a loop — each message into `model.Update`, counting a real step for every `tui.BootstrapProgressMsg` whose restore counters are both zero — until the channel-closed reply arrives or the model reports the decision ran, bounded by `concurrentBootDrainBudget`. Do not execute the model's own returned commands: the model carries the same receiver closure, and running both would consume the channel twice.
- Assert after each pumped step event that the model is still on `PageLoading` and the probe has not run.
- Isolation is `setupConcurrentColdBootEnv`'s: `portaltest.IsolateStateForTest` + `PORTAL_STATE_DIR`, `portaltest.RegisterStateDirTeardownGuard` before `tmuxtest.New`, a disposable `-S` socket, and the saver-daemon reap in cleanup. Enumerate no process the fixture did not spawn and signal nothing outside that socket.

**Acceptance Criteria**:
- [ ] The decision closure is invoked exactly once across the whole boot
- [ ] All ten real step events, and the terminal complete event, were delivered before it ran — it is never invoked at the end of restore (step 6)
- [ ] `state.IsRestoringSet(client)` is false at the instant it runs, so the marker is cleared before anything the decision drives fires
- [ ] The closure's own live read returns the restored sessions, so the count it takes is over post-restore reality
- [ ] The model is on `PageLoading` after every step event and until the decision resolves
- [ ] On the single-match term the model quits with `Selected()` set to that session, `SearchAttached()` true, and `ActivePage()` still `PageLoading` — no picker frame is composed
- [ ] On the shared-fragment term the closure answers with no session and the model transitions to `PageSessions`
- [ ] `FatalError()` is nil on both runs and no `BootstrapFatalMsg` is observed
- [ ] The fixture touches only its own socket, state dir and spawned daemon; the suite carries `//go:build integration`
- [ ] `go test -tags integration -p 1 ./...` passes, and `go test ./...` is unaffected

**Tests**:
- `"it takes the search decision only after every bootstrap step"` — step count at probe time equals ten, complete event already delivered
- `"it takes the search decision with the restoring marker cleared"`
- `"it sees the restored sessions in the decision's own read"`
- `"it stands on the loading page for every step"` — page asserted after each pumped step event, probe uninvoked
- `"it attaches the single match without painting the picker"` — `Selected()`, `SearchAttached()`, page `PageLoading`
- `"it opens the picker when two sessions match"` — page `PageSessions`, `Selected()` empty
- `"it invokes the decision exactly once"`

**Edge Cases**:
- The harness pumps the receiver itself and discards the model's returned commands, because the model holds the same closure — pumping both would race two consumers on one channel and lose events
- `transitionFromLoading` leaves `sessionsLoaded` false on the concurrent route and `evaluateDefaultPage` additionally waits on `projectsLoaded`, so the search landing's committed filter is not observable from this fixture; the page flip is what it asserts, with the narrowed list covered by the Phase 2 unit suites
- The drive loop needs the same bounded budget the existing cold-boot suite uses: a pipe that never closes must fail with the portal log attached rather than hang the lane
- The orchestrator here is the production step set minus hook registration, exactly as `buildConcurrentColdBootOrchestrator` assembles it, so the fixture never mutates the host's global tmux hook table
- `EnsureSaver` spawns a real daemon inside the fixture's own socket; the registered reap is what keeps the state dir's fds released before `t.TempDir` cleanup on macOS
- The integration lane runs `-p 1`: this fixture is daemon-timing-sensitive and must not be run in parallel with the other cold-boot suites

**Context**:
> On a cold server the sigil takes the picker's concurrent-bootstrap path, so the count is taken once that bootstrap has run to completion — every step of it, not merely the restore that reconstructs the saved sessions. Acting at the end of restore would replace the process mid-bootstrap and abandon the steps that follow it — among them the clearing of the `@portal-restoring` marker, which must not outlive bootstrap.
>
> The loading page stands until the count can be taken, and is then replaced by the attach when K turns out to be 1, or by the picker at any other count.
>
> A test must never mutate or affect the real system: not the filesystem outside its temp dirs, not the default tmux server, and not any OS process the test did not spawn. Every test that spawns a `portal state daemon` carries the integration tag, calls `portaltest.IsolateStateForTest`, and applies the returned env to every subprocess.

**Spec Reference**: `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md` §3.4, §7.1, §7.6
