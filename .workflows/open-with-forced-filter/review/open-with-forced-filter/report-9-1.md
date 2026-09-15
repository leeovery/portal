TASK: Refuse every malformed open line before any bootstrap runs (tick-f66f75 / open-with-forced-filter-9-1) — move open's argv-only refusals (`-f` beside a target or a pin, an empty `-f`, `-e` together with `--`, an empty `-e`, a bare `--`) out of `RunE` into `validateOpenArgs`, leaving `--ack`'s shape check in `RunE`.

ACCEPTANCE CRITERIA:
1. `portal open -f ""`, `-f blog api`, `-f blog -s api`, `-e ""`, `-e vim -- claude` and `--` are each refused by open's `Args` validator: bootstrap runs zero times, no resolver seam is consulted, `openTUIFunc` is never called.
2. Each refusal's message text is byte-identical to today's and each still arrives as a `*UsageError` (exit 2).
3. A line colliding several ways names the refusal it names today: search-form collision first, then the `-e`/`--` refusal, then `-f`'s.
4. `RunE` refuses none of the five; `parseCommandArgs` still returns them to `RunE`, restated nowhere.
5. `portal open -f "" --help` prints help rather than the refusal.
6. A malformed `--ack` is still refused from `RunE`, unchanged.

STATUS: complete

SPEC CONTEXT: §5.1 ("A refused line starts nothing") states that a malformed line is knowable from the argv alone, so it needs no tmux server and takes no loading page — on a cold machine as on a warm one it prints its usage error and exits without starting the server, restoring a session or painting a frame. The spec states that property for the search form's refusals; this task generalises it to open's remaining argv-only refusals, which is an extension consistent with the section rather than a divergence from it. §5.1 also fixes that flags answering before the command body (`--help`) are outside the rule, which criterion 5 pins.

IMPLEMENTATION:
- Status: Implemented (matches the plan's "Do" list step for step)
- Location:
  - `cmd/open_search.go:60-72` — `validateOpenArgs` now sequences `validateSearchFormCollisions` then `validateCommandScopeAndFilter`; its doc comment was rewritten to the set it now refuses (the old comment described the search form alone and would have been false).
  - `cmd/open_search.go:128-137` — `validateCommandScopeAndFilter` calls `parseCommandArgs(cmd, args)` for its error alone and hands the `destination` it returns to `validateFilterFlag`.
  - `cmd/open.go:231-246` — new `validateFilterFlag(cmd, destination)`, gated on `Changed("filter")`, carrying the two `-f` refusals verbatim.
  - `cmd/open.go:182-187` — `RunE`'s filter block is now the `Changed("filter")` gate, the value read and the `openTUIFunc` dispatch; both refusals are gone from it.
  - `cmd/open.go:355-394` — `parseCommandArgs` untouched (verified by `git show e36ecdbbc`: the commit touches only the three regions above plus tests).
  - `cmd/open.go:167-173` — `--ack`'s shape check left in `RunE`.
- Notes: the whole delivered change survives at HEAD unmodified — `git diff e36ecdbbc..HEAD -- cmd/open.go cmd/open_search.go` touches only the later `pickerLanding.search` reshape, the `firstSetLocalFlag` catch-all arm in `validateSearchFormCollisions`, and the teardown-warnings rework, none of which reach this task's arms.

  Criterion-by-criterion, read against the code and cobra v1.10.2's source:
  - (1) `Args` is `validateOpenArgs` (`cmd/open.go:160`). In cobra v1.10.2 `ValidateArgs` has exactly one call site — `command.go:968`, inside `execute()` — and it sits above the `PersistentPreRunE` call at `command.go:986`, so a refused line never reaches root's `PersistentPreRunE` and therefore never reaches `runBootstrap`.
  - (2) All five messages are single-sourced: the two `-f` strings live only at `cmd/open.go:240` and `cmd/open.go:243`, the three command-scope strings only at `cmd/open.go:363`, `368`, `380` (grep over non-test sources in `cmd` and `internal` returns those five lines and no others). `*UsageError` → exit 2 is mapped in `main.go:73-75`, independent of where the error was raised. Rendering is likewise home-independent: cobra returns an `Args` error and a `RunE` error from the same `execute()` return, so `ExecuteC`'s error/usage printing is identical.
  - (3) Order in `validateOpenArgs` is search-form collisions → `parseCommandArgs` → `validateFilterFlag`, which reproduces `RunE`'s old order (`parseCommandArgs` first, filter block after).
  - (4) `RunE` calls `parseCommandArgs` once for the command and destination it needs (`cmd/open.go:162`) and carries no mutual-exclusion refusal of its own; the only other `parseCommandArgs` call is the validator's.
  - (5) cobra answers `--help` at `command.go:934-936` (`return flag.ErrHelp`), above the `ValidateArgs` call, so a malformed `-f` line with `--help` prints help.
  - (6) `--ack` unchanged in `RunE`, downstream of bootstrap.
  - The validator's second `parseCommandArgs` call per invocation is a pure read of parsed flags plus `cmd.ArgsLenAtDash()` — no side effects, nothing cached, nothing to diverge.
  - `args` is the same slice in both homes: cobra passes `argWoFlags` to `ValidateArgs` (`command.go:968`) and to `RunE` (`command.go:1015`), so the `destination` the validator computes is the one `RunE` computes.
  - The plan's completion claim holds: `ValidateArgs` appears nowhere in cobra's `completions.go`, so a partially-typed malformed line still gets its candidates.
  - No admitted line is newly refused: `-f <text>` with no target/pin, `-f <text> -- cmd`, two positional targets, and `<target> -- ls /tmp` all fall through both arms (the last three are pinned by the pre-existing `TestValidateOpenArgs_AdmitsEveryNonSearchLine`).

TESTS:
- Status: Adequate
- Coverage (`cmd/open_search_test.go:794-940`): `executeOpenExpectingPreBootstrapUsage` is the shared assertion — message text, `*UsageError` type (via `executeOpenExpectingUsage`, `cmd/open_search_test.go:320-335`), `runner.calls == 0`, no resolver seam consulted, `openTUIFunc` never reached. Driven over all six argvs of criterion 1: `TestValidateOpenArgs_RefusesAMalformedFilterLineBeforeBootstrap` (empty `-f`, `-f` beside a positional), `TestValidateOpenArgs_RefusesFilterBesideEachDomainPinBeforeBootstrap` (all four pins), `TestValidateOpenArgs_RefusesAMalformedCommandScopeBeforeBootstrap` (empty `-e`, `-e` with `--`, bare `--`). Precedence is pinned by `TestValidateOpenArgs_KeepsTodaysRefusalPrecedence` (search form outranks empty `-f`; empty `-e` outranks empty `-f`). Criterion 5 by `TestValidateOpenArgs_StillAnswersHelpOnAMalformedFilterLine`, criterion 6 by `TestOpenCommand_MalformedAckStillRefusedFromTheCommandBody`.
- Notes: the bootstrap-count assertion is load-bearing rather than vacuous. `runBootstrap` (`cmd/root.go:69`) bypasses the `sync.Once` whenever `bootstrapDeps != nil && !ForceMemoise`, and these tests install `BootstrapDeps{Orchestrator: runner}` with `ForceMemoise` unset, so `runner.Run` is invoked on every invocation that reaches `PersistentPreRunE` — if a refusal moved back into `RunE`, `calls` would be 1 and the test would fail. The `--ack` test is the matching control in the opposite direction: it asserts `calls == 1`, which is what proves the count discriminates.
  Coverage overlaps the pre-existing `-f` refusal tests in `cmd/open_test.go` (`TestOpenCommand_Filter_WithPin_UsageError` at :2235, `TestOpenCommand_Filter_EmptyValue_UsageError` at :2342, and `TestOpenCommand_Filter_WithPositionalTarget_UsageError` at :2196) on argv and message; the new tests add the pre-bootstrap property those cannot see, and the old ones additionally assert that no attach/mint outcome fires. Both sets remain true under the change — neither asserts where the refusal was raised.

CODE QUALITY:
- Project conventions: Followed. The refusal helpers stay in `cmd` beside their dispatch; no new seam, no new `*Deps`, no direct seam assignment in tests (`withBootstrapDeps` / `withOpenDeps` / `withFuncSeam` are used throughout, so `cmd/seam_guard_test.go` stays satisfied); no `t.Parallel()`; no new log component or attr key.
- SOLID principles: Good. `validateFilterFlag` takes the already-parsed `destination` rather than re-deriving it, so the parse has one home; `parseCommandArgs` remains the single owner of the command-scoping rules and is consulted, not copied.
- Complexity: Low. Three small functions, one linear sequence, no new branching in `RunE` (the filter block lost two arms).
- Modern idioms: Yes.
- Readability: Good. Each helper's doc comment states what it refuses and why it lives where it does, and `RunE`'s filter-block comment was updated to name the validator as the refusing party rather than leaving the old claim behind.
- Comment accuracy: the four comments touched by this change all hold against the code and against cobra v1.10.2 — the ordering claim ("cobra validates args before PersistentPreRunE") is true at `command.go:968` vs `986`, and "the Args validator has already refused every line where it would not" is true of `validateFilterFlag`'s two arms.
- Issues: None.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- None
