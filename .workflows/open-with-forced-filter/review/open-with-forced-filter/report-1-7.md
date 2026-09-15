TASK: Refuse a search form that shares its command line (tick-507aea / open-with-forced-filter-1-7)

ACCEPTANCE CRITERIA:
- Each of these exits 2 with a message naming the search form and the collided element: `/term <other-target>` (at every arity), `<other-target> /term`, `/term /other`, `/term -e <cmd>`, `/term -- <cmd>`, `/term -f <text>`, `/term -s|-p|-a|-z <value>`, `/term --ack <batch>:<token>`
- `portal open /term --` with nothing after the separator is refused as a command collision rather than reaching `RunE`'s "no command specified after --"
- A refused line runs no bootstrap: an injected recording orchestrator records zero `Run` calls, and the picker seam is never invoked
- `portal open /term --help` prints help and exits 0; a root persistent flag on a search-form line still applies
- `portal open ~/Code/api -- ls /tmp` is unaffected — the post-dash `/tmp` is the command's own argument and no refusal fires
- `portal open /term` alone and `portal open /` alone validate (nil) and reach the picker
- Every pre-existing `open` line still validates, including two or more positional targets for the multi-target burst
- `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: complete

SPEC CONTEXT:
§5.1 makes the sigil a whole-invocation mode: "`/term` composes with nothing. Any other target, any trailing command, and any of `open`'s own flags on the same line is a usage error", with a refusal that "names what collided" rather than a generic complaint, and a table enumerating each composition. §5.1 also fixes the load-bearing property that "a refused line starts nothing" — no server, no restore, no frame — because the refusal is knowable from argv alone. §5.1 exempts flags that answer before the command body: `--help` and root persistent flags. §2.3 makes recognition positional-independent and stops the scan at `--`, so a `/word` among the trailing command's own words is never a sigil. §5.2/§5.3/§5.4 derive the command, second-target, domain-pin and `--ack` refusals.

IMPLEMENTATION:
- Status: Implemented (and legitimately extended past the task's text by later analysis tasks tick-333efc and tick-f66f75, which are in their own scope)
- Location:
  - `cmd/open.go:160` — `Args: validateOpenArgs` replaces `cobra.ArbitraryArgs`
  - `cmd/open_search.go:68` — `validateOpenArgs`
  - `cmd/open_search.go:77-107` — `validateSearchFormCollisions`, the fixed-order refusal ladder plus the fail-closed unnamed-flag arm
  - `cmd/open_search.go:118` — `firstSetLocalFlag`
  - `cmd/open_search.go:16,28` — `searchFormPositionals` / `preDashPositionals` (the `--`-bounded scan)
  - `cmd/errors.go:12` + `main.go:73` — `NewUsageError` → exit code 2
- Notes:
  - Ordering verified against the dependency, not assumed: cobra v1.10.2 `command.go` `execute()` returns `flag.ErrHelp` for `--help` *before* `ValidateArgs`, and calls `ValidateArgs` *before* walking parents for `PersistentPreRunE`. The package declares no `cobra.OnInitialize` initializer, no `TraverseChildren` and no `DisableFlagParsing`, so nothing that could touch tmux runs ahead of the validator. The "a refused line starts nothing" property therefore holds structurally, not only under the injected seams the test asserts it with.
  - The ladder's order matches the task's fixed order exactly (second form → other positional → command → `-f` → domain pin → `--ack`), so a multiply-colliding line always names the same element.
  - `/term --` is caught by the `cmd.ArgsLenAtDash() >= 0` arm before `parseCommandArgs` can report "no command specified after --" — the criterion's point, since that error lives downstream.
  - The `--`-bound is `args[:cmd.ArgsLenAtDash()]`, and `dash` can never exceed `len(args)` (pflag records `argsLenAtDash` as the arg count at the moment `--` was seen), so the slice is safe.
  - Root persistent flags: Portal registers none anywhere (`grep PersistentFlags` over the module hits only skill examples), so the criterion's second clause is vacuously true today. It also holds by construction going forward — `firstSetLocalFlag` walks `cmd.LocalFlags()`, and cobra's `LocalFlags` excludes any flag identical to a parent's persistent flag. Cobra's auto-added `help` flag is local rather than persistent, but is unreachable here because `--help` short-circuits ahead of `ValidateArgs`.
  - The later `validateCommandScopeAndFilter` arm (tick-f66f75) moved `parseCommandArgs`/`validateFilterFlag` refusals into the same validator, diverging from this task's "Keep `RunE`'s existing usage errors exactly as they are". That is a deliberate, recorded successor decision, not a loss: the same messages are produced, now earlier, and `parseCommandArgs` is pure so calling it in both the validator and `RunE` has no side effect.

TESTS:
- Status: Adequate
- Coverage (`cmd/open_search_test.go`):
  - `TestValidateOpenArgs_RefusesSearchFormBesideAnotherTarget:339` — table over `/port api`, `api /port`, `/port api blog` (both orders, arity 2 and 3)
  - `TestValidateOpenArgs_RefusesSecondSearchForm:356`
  - `TestValidateOpenArgs_RefusesSearchFormWithACommand:361` — `-e`, `--`, and the empty `--` separator
  - `TestValidateOpenArgs_RefusesSearchFormWithFilter:380`
  - `TestValidateOpenArgs_RefusesSearchFormWithEachDomainPin:387` — table over `-s`, `-p`, `-a`, `-z`
  - `TestValidateOpenArgs_RefusesSearchFormWithAck:396`
  - `TestValidateOpenArgs_RefusedLineStartsNoBootstrap:436` — recording orchestrator asserts zero `Run` calls, and the `openTUIFunc` seam fails the test if reached
  - `TestValidateOpenArgs_StillAnswersHelpOnASearchFormLine:449`
  - `TestValidateOpenArgs_AdmitsEveryNonSearchLine:461` — lone `/port`, lone `/`, `~/Code/api -- ls /tmp`, and the two-positional burst line
  - `TestSearchFormPositionals_StopsAtDashSeparator:67` — pins the `--` bound at the scanner
  - `TestValidateOpenArgs_KeepsTodaysRefusalPrecedence:888` — pins that a search form outranks an empty `-f`
  - Exit code 2 is pinned at the mapping rather than restated per case: `main_test.go:62` asserts `*UsageError` → 2.
- Notes:
  - The refusal tests assert the error *type* and the exact message, so a reordered ladder or a reworded message fails rather than passes — the tests would catch the feature breaking.
  - `installSearchFormSeams` (`cmd/open_search_test.go:137`) stages the picker, burst, attach and mint seams, so deleting the validator would not silently reach real tmux: the line would open the picker and the assertion would fail with "want `*UsageError`".
  - `TestValidateOpenArgs_AdmitsEveryNonSearchLine` ignores non-`*UsageError` failures from the downstream dispatch. That is the right narrowing for a validator test — its subject is admission, not what the admitted line then does — and the downstream behaviour is covered by the dispatch tests in the same file.
  - No redundancy found: the scanner's `--` bound and the validator's `--` bound are separate subjects (what counts as a form vs. what collides), not the same assertion twice.

CODE QUALITY:
- Project conventions: Followed. `NewUsageError` is the project's exit-2 vocabulary; the validator lives beside the search-form helpers it reads; no new log component or attr key invented; the test file uses the package's `withFuncSeam`/`withOpenDeps`/`withBootstrapDeps` staging helpers rather than assigning seams directly, which the `cmd/seam_guard_test.go` source guard requires.
- SOLID principles: Good. `validateOpenArgs` composes two single-purpose validators; `searchFormPositionals`/`preDashPositionals` are shared with `RunE` and `isTUIPath` rather than restated.
- Complexity: Low. The ladder is a flat `switch` with one arm per rule.
- Modern idioms: Yes. `slices.ContainsFunc` in `anyOpenDomainPin`, a tagless `switch` for the ordered ladder.
- Readability: Good. Each refusal reads as its own sentence and the fixed order is stated in the doc comment.
- Issues: None. Comment accuracy checked line by line against the dependency's behaviour: the `firstSetLocalFlag` comment's claim that "LocalFlags() is a second set holding the same `*pflag.Flag` values, so Visit over it reports nothing" is correct (pflag's `Visit` walks `actual`, which `AddFlag` never populates), as is "pflag visits lexically with no early exit" (`VisitAll` sorts when `SortFlags` is true, which cobra copies from `Flags()` and which defaults true). No comment references a task id, phase or spec section.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — settling this needs both suites executed; reading the test bodies establishes their assertions and their staging, not their outcome. The integration lane additionally needs `-p 1` and a real tmux server.
