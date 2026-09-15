TASK: open-with-forced-filter-11-2 (tick-61e334) — Give the four staging methods the value-in/value-out shape the package's handlers already use

ACCEPTANCE CRITERIA:
1. All four declare a value receiver and return `(Model, tea.Cmd)`, and no `(&m).` prefix remains on any of the four at any call site, production or test (the package's other pointer-receiver helpers keep theirs).
2. `return m, m.createSession(dir)` — and the same shape for the other three — no longer compiles, two results being unable to fill one operand.
3. The warning comment above `createSession` is gone, and nothing — comment, guard test or convention note — is added in its place.
4. Behaviour unchanged on every route: the search decision still sets `selected`/`searchAttached`/`searchErr` on the model the program returns, a failed session read still quits with `searchErr` and no fatal frame, and a project pick taken while the bootstrap is in flight still stages `stagedMint`/`stagedMintDir` for the terminal `BootstrapCompleteMsg` arm.
5. `go build ./...`, `go test ./...` and `golangci-lint run` are clean, and no test's semantics change beyond the three respelled calls.

STATUS: complete

SPEC CONTEXT: The specification has no surface for this task — it is an architecture finding from analysis cycle 5, not a behavioural requirement. What it protects is spec behaviour, though: §3.7 (an attach under K = 1 uses the connector the invocation already selects, run after the TUI tears down) and §7.5 (a sigil resolving to a direct attach still delivers its accumulated soft warnings, written after teardown and before the attach) both depend on `selected`/`searchAttached`/`searchErr` surviving on the `Model` the Bubble Tea program returns. The staged-mint route is the command-pending picker's counterpart: a project pick taken mid-bootstrap must still be minted by the terminal `BootstrapCompleteMsg` arm. In both cases the state is written by one step and read by a later one, so dropping the mutated copy is a silent loss — exit 0 having attached nothing, or a pick dropped with no band, no mint and no error.

IMPLEMENTATION:
- Status: Implemented (delivered in 157a53f7b; the shape survives at HEAD through five later commits that touch the same files)
- Location:
  - `internal/tui/model.go:1509` — `func (m Model) dismissLoadingGate() (Model, tea.Cmd)`
  - `internal/tui/model.go:1844` — `func (m Model) createSession(dir string) (Model, tea.Cmd)`
  - `internal/tui/model.go:2880` — `func (m Model) createSessionInCWD() (Model, tea.Cmd)`, still the one-line delegation `return m.createSession(m.cwd)`
  - `internal/tui/search_decision.go:51` — `func (m Model) applySearchDecision(msg searchDecisionMsg) (Model, tea.Cmd)`
  - Call sites: `internal/tui/model.go:1642` and `:1694` (`m, cmd = m.dismissLoadingGate()`), `:1963` (`m, cmd := m.createSession(pi.Project.Path)`), `:1652` (`return m.applySearchDecision(msg)`), `:2876` (`m, cmd := m.createSessionInCWD()`); tests at `internal/tui/staged_mint_band_test.go:32`, `:50`, `:69` (`m, _ = m.createSession("/tmp/alpha")`)
- Notes:
  - The fourth method named in the task, `resolveSearchDecision`, no longer exists under that name. Task 12-3 (tick-23141a, "Take the search decision off the render goroutine", b1b809fc4) split it into `searchDecisionCmd` (dispatch, value receiver, returns `tea.Cmd` and mutates nothing) and `applySearchDecision` (the mutating arm, value-in/value-out) plus the shared `completeLoadingDismissal` (`model.go:1522`, also value-in/value-out). This is a later approved task moving past 11-2's text, and it carries 11-2's property forward rather than losing it: every method that writes the decision state now returns the model it wrote.
  - A repo-wide grep for `(&m).` over `internal/tui` returns no hit on any of the four, in production or test. The remaining `(&m).` sites are the helpers the task explicitly excluded — `rebuildSessionList`, `resyncPageLayouts`, `setFlash`, `refreshSessionDelegate`, `resetBurstState`, `sessionBandHeight`, and the theme-panel family.
  - Criterion 2 holds by the language rather than by convention: a two-result call cannot fill the single second operand of `return m, …`, so the state-dropping spelling is now a build error at all four.
  - Criterion 3 holds: `createSession` at `model.go:1844` follows `bootstrapInFlight` with no doc comment at all, and no replacement warning, guard test or convention note appears anywhere in the package (grep for "unspecified", "order of the model read" and "staging would be lost" over `internal/tui` returns nothing).
  - The two converted helpers both call pointer-receiver mutators on the now-addressable local (`m.transitionFromLoading()` inside `completeLoadingDismissal`, `m.resyncPageLayouts()` inside `createSession`) and return that same local on every path, so the mutations survive. `completeLoadingDismissal` keeps the `cmd := tea.Batch(…); return m, cmd` two-statement shape, which is load-bearing: `surfaceBufferedWarnings` (`bootstrap_warnings.go:55`) is itself a pointer-receiver mutator, and the batch's three calls are ordered lexically by the spec while the model read is fixed by statement order.
  - The short variable declarations at `model.go:1963` and `:2876` sit in each function's outermost block alongside the receiver, so `m` is assigned rather than shadowed — the mutated model is what `return m, cmd` returns.

TESTS:
- Status: Adequate (no new test, per the task's explicit direction — the signature is the contract)
- Coverage: The named pins all exist and all read post-transition state off the returned model, so each would fail on a dropped copy rather than pass silently: `TestSearchDecision_AttachesSingleMatchWithoutPainting` (`search_decision_test.go:63` — asserts `Selected()`/`SearchAttached()` on `model.(Model)`), `TestSearchDecision_RecordsReadFailureWithoutFatal` (`:117` — asserts `SearchError()` and a nil `FatalError()`), `TestSearchDecision_IssuedExactlyOnce` (`:214`), `TestStagedMintProjectBand` (`staged_mint_band_test.go:24` — the three respelled calls, every assertion unchanged), `createSessionInCWD delegates to createSession with cwd` (`model_test.go:3085`), `TestLoading_TransitionDualGated` (`loading_view_test.go:443`), and the integration-lane `cmd/concurrent_search_decision_integration_test.go`.
- Notes: The diff confirms criterion 5's second half by reading — `staged_mint_band_test.go` changed on exactly three lines, each a call respelling, with no assertion touched. Adding a guard test here would be the deleted prose warning in another form, which the task rules out; declining one is correct.

CODE QUALITY:
- Project conventions: Followed. Value-in/value-out is the shape every other state-mutating handler in the package uses (`handleProjectEnter`, `handleNewInCWD`, `handleThemePanelKey`), so the four now match their neighbours instead of being the odd shape an edit would normalise back to the losing spelling.
- SOLID principles: Good. No responsibility moved; the change is purely how written state travels back to the caller.
- Complexity: Low. Same branches, same returns, one extra operand per return.
- Modern idioms: Yes. The conversion removes the `(&m).` idiom from these four entirely rather than leaving a mixed spelling at their call sites.
- Readability: Good. `m, cmd := m.createSession(dir); return m, cmd` reads as what it does, and the deleted three-line warning is no longer needed to explain it.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go build ./...`, `go test ./...` and `golangci-lint run` are clean, and no test's semantics change beyond the three respelled calls." — the second half is settled by reading the diff (three call-line changes in `staged_mint_band_test.go`, no assertion touched); the three commands themselves were not run. Settling the first half needs `go build ./...`, `go test ./...`, `golangci-lint run` from the project root, plus the integration-lane `go test -tags integration -p 1 ./cmd -run TestConcurrent` for `cmd/concurrent_search_decision_integration_test.go`.
