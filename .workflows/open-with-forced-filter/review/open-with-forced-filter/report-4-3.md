TASK: Deliver the accumulated soft bootstrap warnings when a search ends without a picker (tick-3afec2 / open-with-forced-filter-4-3)

ACCEPTANCE CRITERIA:
- A warm single-match attach writes every accumulated warning line to stderr, in order, before `openSessionFunc` is called
- Those lines are byte-identical to what `warning.WriteLines` produces for the same warnings
- The sink is empty afterwards, so no later drain writes the same warning twice
- A warm search whose session-list read fails writes every accumulated warning line to stderr before tmux's own error reaches the user
- On the concurrent route a recorded search error writes the model's buffered warnings after teardown, exactly as an attach does
- With no warnings accumulated, neither route writes a byte
- `emitSearchTeardownWarnings` writes exactly `model.BufferedWarnings()` when the model records an attach or a search error, and nothing otherwise
- On the concurrent route the write follows the terminal-background restore and precedes the connect
- A zero- or two-plus-match search picker surfaces its warnings in the notice band and writes nothing after teardown
- A loading page cancelled with `Ctrl-C` while warnings are buffered writes nothing
- Warning routing for every non-search invocation is unchanged
- `go test ./...` and `go test -tags integration -p 1 ./...` pass

STATUS: issues_found

SPEC CONTEXT:
§7.1 classifies a sigil invocation as a picker invocation, so its soft bootstrap warnings take the in-TUI route rather than stderr. §7.2 makes that same verdict decide where warnings go — the point of the classification is that nothing sprays into the frame the picker is about to claim. §7.5 is this task's own requirement: on K = 1 the TUI tears down before the connector runs, the notice band never surfaces, so the accumulated soft warnings are written to the terminal after teardown and before the attach — "which is where a warm-server sigil attach already puts them". The spec states no rule for a cancelled loading page, and carries no corrigendum touching warning delivery.

IMPLEMENTATION:
- Status: Implemented; drifted from the task's prescribed shape by four later, recorded plan tasks, and the drift is sound
- Location:
  - `cmd/open_search.go:234-247` — `runSearchForm`'s two non-picker branches each call `bootstrapWarnings.EmitTo(cmd.ErrOrStderr())`: the failed-read branch (`:238`) before returning the error, and the single-match branch (`:245`) before `openSessionFunc(cmd, name)`. The picker branch (`:248`) deliberately leaves the sink full for `stageBootstrapWarningsOnModel`.
  - `cmd/bootstrap_warnings.go:36-38` — `EmitTo` drains the sink and delegates to `warning.WriteLines`, which is the same call `cmd/root.go:113` and `cmd/root.go:147` make on the CLI path, so byte-identity is structural rather than coincidental.
  - `cmd/open.go:619-629` — `finishTUI` runs the teardown tail: `tui.RestoreTerminalBackground` (`:624`), then `tui.WriteBootstrapWarnings(warnings, model.WarningsOwedAtTeardown())` (`:626`), then `processTUIResult` (`:628`), which is what performs the connect. `openTUI` calls it as `finishTUI(model, connector, os.Stdout, cmd.ErrOrStderr())` (`cmd/open.go:754`).
  - `internal/tui/model.go:481-488` — `WarningsOwedAtTeardown` is the teardown's decider.
- Notes on the drift (all four superseding changes are plan tasks of this same topic, each with its own recorded rationale — none is an unconsidered loss):
  - The prescribed `emitSearchTeardownWarnings(w, model)` helper in `cmd` no longer exists. Task 9-3 (tick-b8464e) moved the question into the model as `Model.WarningsOwedAtTeardown()` and collapsed `finishTUI` to one write, on the reasoning that "what does this teardown still owe the terminal" is not a search-shaped question. The substance the criterion names — exactly the buffered set written on an attach or a search error, nothing otherwise — is preserved for those two outcomes.
  - Task 6-4 (tick-fa4ede) added the warm-picker teardown write, closing a hole where a warm sigil/`-f`/bare picker dropped a saver-down warning entirely. This is why a zero-/two-plus-match *warm* search picker now writes its warnings at teardown rather than "writing nothing after teardown": on that route no notice band ever exists to surface them. The concurrent route still routes them to the band and writes nothing (`internal/tui/bootstrap_warnings.go:55-66`; pinned by "it owes nothing once a loading gate has surfaced the buffer", `cmd/open_search_warnings_test.go:255`).
  - Task 12-5 (tick-cb4c82) deliberately inverted the "a cancelled loading page writes nothing" criterion: since task 12-3 made Ctrl-C on the loading page the designed escape from a decision waiting on a slow tmux server, the cancel window became unbounded, and the user most likely to hit it is exactly the one a saver-down warning is for. `WarningsOwedAtTeardown` is now `slices.Concat(bufferedWarnings, pendingBootstrapWarnings)` (`internal/tui/model.go:487`) — "what nobody surfaced" — and the suite's cancel subtest was inverted with it. Judged: this is a gain, not a loss, and the spec states no rule it contradicts.
  - Task 10-3 (tick-4c0f85) briefly made `m.searchAttached` the teardown's discriminator; task 12-5 removed that branch again, leaving the flag with no production reader (see FINDINGS).
- No double-write exists on any route: the sink is drained by `EmitTo` on the two non-picker branches and by `stageBootstrapWarningsOnModel` on the picker branch; on the model, `surfaceBufferedWarnings` (`internal/tui/bootstrap_warnings.go:55`) nils the buffer when a band or flush takes it, and the `BootstrapCompleteMsg` arm (`internal/tui/model.go:1658-1672`) nils the staged set when the gate folds it into the buffer — so the union at teardown can only hold sets nobody consumed.
- Ordering holds where it is load-bearing: both `EmitTo` calls precede the exec'd attach handoff, and `finishTUI`'s write precedes `processTUIResult`'s `connector.Connect` (`cmd/open.go:616`).

TESTS:
- Status: Adequate
- Coverage: `cmd/open_search_warnings_test.go` carries the whole surface.
  - Warm route (`TestSearchForm_WarmRoute_WritesAccumulatedWarnings`, `:75`): warnings written before `openSessionFunc` runs (`:76`, captured inside a stubbed `openSessionFunc` so the ordering, not just the end state, is the assertion); CLI-path byte parity against `warning.WriteLines` (`:102`); sink drained so a second `EmitTo` adds nothing (`:118`); nothing written with an empty sink (`:135`); the failed-read branch writing before the error surfaces (`:150`); and the picker branch writing nothing while leaving the sink loaded for the model (`:168`). Each would fail if the corresponding `EmitTo` were removed.
  - Teardown decider (`TestWarningsOwedAtTeardown`, `:193`): attach (`:204`), attach with a set staged before the concurrent launch (`:213`), failed read (`:239`), warm picker (`:249`), surfaced band (`:255`), empty (`:264`), and the two cancel cases (`:270`, `:293`). Assertions compare through `warning.WriteLines` rendering, so ordering and content are both pinned.
  - `finishTUI` (`TestFinishTUI`, `:321`; `TestFinishTUI_StagedBootstrapWarnings`, `:420`): the owed set written exactly once and observed as already-written from inside the connector stub (`:322`) — which is the criterion about the exec'd attach that never returns — plus CLI parity (`:350`) and the `-f`/bare-picker parity cases (`:542`).
  - Cold route end-to-end: `cmd/concurrent_search_decision_integration_test.go:213-222` pins the attach-without-a-picker-frame outcome the teardown write depends on.
- Notes: the warm route is driven through `runSearchForm` directly rather than `rootCmd.Execute()`, which the task itself reasons through (at this point in the phase a `/term` line still classified as a CLI line, so an end-to-end assertion would have passed over output this task did not produce). Not over-tested: no subtest duplicates another's property, and the only near-repetition — "it writes the same lines as the CLI path" appearing in three suites — covers three different writers (the sink, the search teardown, the staged teardown).

CODE QUALITY:
- Project conventions: Followed. Seams are staged through `withOpenDeps`/`withFuncSeam` per `cmd/testhelpers_test.go`, the package-level sink is reset through `resetBootstrapWarnings(t)` (`cmd/abridged_saver_test.go:19`) with its own cleanup, no `t.Parallel()`, and no test touches a real tmux server or the developer's state dir.
- SOLID principles: Good. The teardown's decision lives beside the state it reads (`internal/tui/model.go:486`) and `cmd` makes one unconditional write; `finishTUI` takes its two writers as parameters, which is what makes the ordering testable without touching `os.Stdout`/`os.Stderr`.
- Complexity: Low. `runSearchForm` is four guarded returns; `finishTUI` is three statements.
- Modern idioms: Yes — `slices.Concat` for the union, value-receiver accessors consistent with the rest of `Model`.
- Readability: Good. Each `EmitTo` carries a comment stating why that branch writes; `finishTUI`'s comments state why the order is load-bearing rather than restating the calls.
- Issues: `cmd/open_search.go:236` says "the buffered warnings", borrowing the model's `bufferedWarnings` vocabulary for the `cmd`-side sink, which is a different container. Comprehensible in place and not reported as a finding.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] internal/tui/search_decision.go:60 — `m.searchAttached` is written here and exposed by `SearchAttached()` (`internal/tui/search_decision.go:19`), but nothing in production reads either: `WarningsOwedAtTeardown` (`internal/tui/model.go:486-488`) was its one production consumer and task 12-5 replaced that branch with the union, while explicitly leaving the accessors untouched. All 21 `SearchAttached()` call sites are in `_test.go` files (4 in `cmd/concurrent_search_decision_integration_test.go`, 2 in `cmd/open_picker_landing_test.go`, 5 in `cmd/open_search_warnings_test.go`, 10 in `internal/tui/search_decision_test.go`); the sole non-test occurrence is the declaration. Either delete the field and the accessor and re-point each assertion at the state production does act on (`Selected()` for the attach, `SearchError()` for the read failure) — a per-site judgement, not a mechanical rename — or give the flag a production reader again. — FAILS: `Model` carries an exported accessor and a field maintained as production surface that only tests read, and the suites that guard "the decision attached" with it (`cmd/open_search_warnings_test.go:206`, `:231`; `cmd/concurrent_search_decision_integration_test.go:217`) pin a fact no production path consults — so those pre-checks read as though they guard the branch asserted below them when they no longer guard anything, which is the same defect task 10-3 named before it was reintroduced.

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — both lanes must be executed; reading cannot settle them. The integration-lane half matters specifically here because `cmd/concurrent_search_decision_integration_test.go` drives a real cold boot through the search decision and the teardown.
- "A warm single-match attach writes every accumulated warning line to stderr, in order, before `openSessionFunc` is called" — the code path and the suite's recorded ordering settle it by reading; the end-to-end claim (a real `x /term` on a warm latched install with `_portal-saver` down printing the saver-down line and then attaching) would need a live run against a tmux server with the saver killed.
