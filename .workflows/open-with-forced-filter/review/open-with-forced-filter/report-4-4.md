TASK: Classify a search form as a picker invocation (open-with-forced-filter-4-4, tick-9b4d04)

ACCEPTANCE CRITERIA:
- `isTUIPath` is true for `open /port` and `open /`, and for a search form sitting at any positional index
- It is false for `open /Users/x/y`, `open /tmp/`, `open ./port`, `open ~/dir`, a bare word, two positional targets, every domain pin, and for any command other than `open`
- Words after a `--` separator are never inspected: `open ~/Code/api -- ls /tmp` is false
- A search form on the same line as a domain pin still reads as non-picker
- On a cold server `open /port` stashes a deferred bootstrap and the orchestrator runs zero times synchronously
- On a latched server `open /port` takes the abridged path with `serverStarted=false` and no deferred bootstrap
- `PersistentPreRunE` writes no warnings to stderr for a search-form line, and leaves them in the sink; a path positional still writes them
- The probe-free decider guard still covers a CLI-classified direct-path line after the flip
- A line refused by `validateOpenArgs` never reaches the classification: no server is started, no loading page is painted, the exit is 2
- The warm path, every CLI path and the concurrent route's step sequence and labels are unchanged
- `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT: §7.1 requires a sigil invocation to be classified as a picker invocation — concurrent bootstrap, honest loading page, in-TUI warning route. §7.2 names the predicate verbatim (`return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`, after a domain-pin veto) and states that the same verdict decides where soft warnings go. §7.3 justifies classifying on form rather than on outcome (K is unknowable on a cold server). §7.6 requires the concurrent path itself to be unchanged. §5.1 requires a refused line to start nothing, and states the picker classification applies to a complete sigil invocation only.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/root.go:173-178` — `isTUIPath` restructured to an early-return veto (`cmd.Name() != "open" || anyOpenDomainPin(cmd)`) followed by `len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`.
  - Scan reused rather than restated: `cmd/open_search.go:17` (`searchFormPositionals`) and `cmd/open_search.go:29` (`preDashPositionals`), the Phase 1 helpers also driven by `validateSearchFormCollisions` (`cmd/open_search.go:78,86`) and `cmd/open.go:178`.
  - Call sites unchanged: `cmd/root.go:112` (abridged-path warning gate), `cmd/root.go:146` (full-bootstrap warning gate), `cmd/root.go:190` (`shouldRunConcurrentBootstrap`).
- Notes:
  - The delivered task commit (`dcdf31b91`) wrote `len(args) == 0 || …`; the current `len(preDashPositionals(cmd, args)) == 0` arm is the later task tick-3024a0's work (command-only line with no target). Spec §7.2 was updated to match, so the code, the spec and the plan's successor task agree. No drift.
  - The restructure is verdict-identical to the plan's prescribed single expression: `!(¬open ∨ pin) ∧ C` is `open ∧ ¬pin ∧ C`, and both short-circuited calls are pure (`anyOpenDomainPin` via pflag `Changed`, which is nil-safe for a flag the probe command never registered; `searchFormPositionals` via `ArgsLenAtDash` + `IsSearchSigil`). The `args[:dash]` slice at `cmd/open_search.go:30` is reached in strictly fewer cases than the prescribed form.
  - Slice safety confirmed: cobra 1.10.2 hands `PersistentPreRunE` the same `c.Flags().Args()` slice pflag's `argsLenAtDash` indexes into (`command.go:963,986`), so `dash <= len(args)` holds on every production call.
  - The plan's edge case that `validateOpenArgs` runs first is confirmed against the dependency: `ValidateArgs` at `command.go:968` precedes the `PersistentPreRunE` walk at `command.go:986`.
  - `anyOpenDomainPin` (`cmd/root.go:182`), `shouldRunConcurrentBootstrap` (`cmd/root.go:189`), the orchestrator's step set, `cmd/bootstrap/progress_emitter.go` and `internal/tui/loading_progress.go` were all left untouched by the task commit, as the plan required.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/concurrent_bootstrap_gate_test.go:104` — `/port` and `/` are the TUI path.
  - `:112` — a search form at a non-zero positional index.
  - `:118` — the negative table `/Users/x/y`, `/tmp/`, `./port`, `~/dir`, `api`.
  - `:126` — two positional targets are not the TUI path.
  - `:132` — the `--` separator row, built by `ParseFlags([]string{"~/Code/api", "--", "ls", "/tmp"})` then `isTUIPath(c, c.Flags().Args())`, exactly as the plan prescribed (a hand-written slice would leave `ArgsLenAtDash()` at pflag's `-1` and the row would pass for the wrong reason).
  - `:142` — a search form beside `-s`.
  - `:152` — the `-f` parity row.
  - `:254` — the probe-free decider guard's direct-path row re-pointed to `/dir/sub`, keeping it a CLI-classified line rather than a duplicate of the bare-picker row beside it.
  - `cmd/concurrent_bootstrap_route_test.go:105` — cold search form: zero synchronous orchestrator calls, deferred bootstrap observed on the context at `openTUIFunc`.
  - `cmd/concurrent_bootstrap_route_test.go:133` — latched search form: zero orchestrator calls, no deferred bootstrap, `serverStarted=false`, empty stderr, and the `SaverDownWarning` still in the sink at `openTUIFunc`. The warning reaches the sink through production code (`stubSaverAliveCheck(t, false)` + `satisfiedLatchSaverAbsentCommander()`), not by seeding — so this test now discriminates the `cmd/root.go:112` gate it is named for.
  - `cmd/bootstrap_warnings_test.go:277` — the full-bootstrap route's counterpart: empty stderr, one warning left in the sink.
  - `cmd/bootstrap_warnings_test.go:254` — the path-positional row still emits, on the multi-segment `/nonexistent/path-for-test` fixture task 1-4 re-pointed it to.
  - `cmd/open_search_test.go:439` — `TestValidateOpenArgs_RefusedLineStartsNoBootstrap`: a refused composition runs the orchestrator zero times and never reaches the picker. The `*UsageError` → exit 2 mapping it depends on is pre-existing (`main.go:73-75`).
- Notes:
  - I swept `cmd/*_test.go` for other single-segment absolute positionals feeding this classification; `/dir` was the only one and it is the row that was re-pointed. Every other classification fixture uses a multi-segment or `~/` path.
  - The `-f` parity row computes `want` rather than pinning `true`, so it alone would stay green if both forms regressed to false together. Its sibling subtests at `:104` and the `-f` row at `:60` pin each side absolutely inside the same function, so the suite still goes red; noted rather than reported.
  - `isTUIPath(openProbeCmd(), []string{"api", "/port"})` at `:112` is a shape the validator refuses before this predicate is ever consulted in production. It pins the predicate's positional-independence, which is what the criterion asks for.
  - Not over-tested: the two warning-silence tests cover two different routes (the full-bootstrap route and the abridged/latched route), not the same one twice.

CODE QUALITY:
- Project conventions: Followed. The predicate stays in `cmd/root.go` beside its two call sites; the shape rule and the `--` rule are stated once in `cmd/open_search.go` and reused, so the classifier and the validator cannot drift. No new seam, no new package-level state, no test executes a real command body without its `*Deps` injected (`withBootstrapDeps` / `withOpenDeps` / `withFuncSeam` throughout).
- SOLID principles: Good — one predicate, one reason to change; the pin veto is structural (early return) rather than positional.
- Complexity: Low — one guard clause and one disjunction.
- Modern idioms: Yes.
- Readability: Good. The added doc comment on `cmd/root.go:170-172` states why the classification is taken on shape rather than outcome, holds true against the code and the specification, and carries no task id, phase or spec-section reference.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — both lanes have to be executed; reading cannot settle a suite result. The integration lane additionally needs `-p 1` and a real tmux.
- "The warm path, every CLI path and the concurrent route's step sequence and labels are unchanged (existing suites green unmodified)" — the *unchanged* half is settled by reading (the task commit touched only `cmd/root.go` plus three test files; `cmd/bootstrap/progress_emitter.go`, `internal/tui/loading_progress.go` and the orchestrator step set are untouched, and the only edit to an existing assertion is the deliberate `/dir` → `/dir/sub` re-point at `cmd/concurrent_bootstrap_gate_test.go:254`). The *green* half needs the warm-path and concurrent-route suites run.
- "On a cold server `open /port` … the same verdict `open -f port` gets on the same boot" — the in-process route test settles the deferred-bootstrap verdict; whether the honest loading page actually paints for `/port` against a real cold tmux server is settled by the integration-tagged task 4-5 suite (`tick-781c51`), which has to be executed.
