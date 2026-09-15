TASK: Route `portal open /term` to the search-form landing (open-with-forced-filter-1-5, tick-f275ce)

ACCEPTANCE CRITERIA:
- `portal open /port` reaches the picker with the landing `{filter: "port", search: true}` and no other branch of `RunE` runs
- No query resolver is constructed and no resolution is attempted on this path — an injected session lister / alias lookup / zoxide querier records zero calls
- `portal open '/po*rt'` opens the picker on the literal term `po*rt`: no glob expansion, no burst dispatch, no window spawned
- A term equal to a live session's name still opens the picker rather than attaching
- No record is emitted under the `resolve` component for a search-form invocation
- `searchFormPositionals` finds the shape wherever it sits among the positionals, and never inspects a word after a `--` separator
- `-f <text>`, `portal open` with no arguments, all four domain pins, the bare-positional chain and the multi-target burst behave exactly as before
- `go test ./...` passes and `go test -tags integration -p 1 ./...` passes

STATUS: issues_found

SPEC CONTEXT: §2.3 makes recognition positional-independent and stops the scan at a `--` separator (post-separator words belong to the trailing command). §3.1 puts the sigil in the session domain by declaration, so it never enters the bare-positional resolution chain and can never mint. §3.2 sets outcomes by match count — K=1 attaches directly, K=0 and K>=2 open the picker on the committed-filter landing; the form never fails. §3.6 forbids a `resolve` component line, because a sigil declares its domain the way a pin does. §4.3 makes the term literal text: `*`, `?` and `[` are characters to find, not wildcards.

IMPLEMENTATION:
- Status: Implemented (drifted, soundly — see notes)
- Location:
  - `cmd/open_search.go:17-24` (`searchFormPositionals`), `:28-33` (`preDashPositionals`), `:52-58` (`pickerLanding`)
  - `cmd/open.go:175-180` (the search branch in `RunE`, sitting ahead of the `-f` block, the multi-target gate, the pin dispatch and the resolution chain)
  - `cmd/open.go:559,594-598` (`buildTUIModel` threading the landing into `tui.Deps.Search` / `tui.Deps.InitialFilter`), `cmd/open.go:631` (`openTUI`'s landing parameter)
  - `internal/tui/build.go:53,71-79,132-133,177-179` (the `SearchForm` landing the seam carries)
- Notes:
  - Three deliberate drifts from the task's literal wording, all authored by later tasks in the same plan and all spec-conformant:
    1. `pickerLanding` carries `search *tui.SearchForm` rather than `search bool` + a reused `filter` string (phase 9, tick-251123). The criterion's `{filter: "port", search: true}` is held in substance — `cmd/open_search_test.go:198` asserts the reduced shape `{term: "port", search: true}`.
    2. The branch calls `runSearchForm` (`cmd/open_search.go:216-248`) rather than `openTUIFunc` directly, so K=1 attaches (phase 2, tick-ad2390). That is spec §3.2, which this task's criterion "a term equal to a live session's name still opens the picker rather than attaching" predates and which the plan's own phase 2 reverses. The code is right; the criterion is superseded, not unmet.
    3. The collision refusals moved out of `RunE` into `open`'s `Args` validator (`cmd/open_search.go:67-71`, phase 9 tick-f66f75), so a malformed search line is refused before any bootstrap runs.
  - `parseCommandArgs` (`cmd/open.go:162`) now runs ahead of the search branch. It is a pure argv parse with no tmux, no resolution and no dispatch, and every error shape it can return for a sigil line (`-e` with `--`, empty `-e`, a bare trailing `--`) is already refused by `validateSearchFormCollisions`, so "no other branch of `RunE` runs" still holds.
  - The scan's separator bound matches the burst's: `orderedOpenTargets` breaks at `--` (`cmd/open_targets.go:42-44`), so the two readers of the same argv cannot disagree about which words are targets.
  - `-f` (`cmd/open.go:184-187`), the no-argument picker (`:206-208`), the pin dispatch (`:200-204`), the burst (`:193-196`) and the bare chain (`:210-227`) all sit behind the branch and are otherwise untouched; `isTUIPath` / `shouldRunConcurrentBootstrap` were left to phase 4 as the task required.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/open_search_test.go:41-64` — `searchFormPositionals` table: sole/second/third positional, several in argv order, bare slash, and the three non-forms (multi-segment path, trailing slash, bare word). Positional independence pinned.
  - `cmd/open_search_test.go:67-81` — the `--` bound, both directions: a `/tmp` after the separator is not found, a `/port` before it still is.
  - `cmd/open_search_test.go:190-207` — the landing and that no other branch (session/path/burst) ran.
  - `:209-217` — the resolution seams (`SessionLister`/`AliasLookup`/`Zoxide`/`DirValidator`) record zero calls.
  - `:219-231` — `/po*` treated as literal text; `runOpenBurstFunc` never called.
  - `:247-271` — no `resolve`/`resolved` record over a `logtest.Install` sink, across no-match / one-match / two-match.
  - `:273-300` — regression pin: a multi-segment path positional still mints and never opens the picker.
  - `cmd/open_test.go:2185` (`-f` → `{filter: "blog"}`, resolver unconsulted), `:2390` (no-arg → zero landing), `:2526`/`:2552` (`-f` + `-e`/`--` command threading) — the untouched routes re-pinned on the new seam shape.
  - `internal/resolver/path_test.go:109-164` and `internal/resolver/search_match_test.go:95-116` carry the shape recogniser and the glob-metacharacter-as-literal rule at the library level.
- Notes:
  - The composed case "glob-metacharacter term with K>=2 lands on the picker carrying the literal term" is not pinned as one test; it is covered as two (literalness in the matcher table, term threading in `:190`/`:545`/`:564`), and the term reaches the landing through a single unbranched expression (`resolver.SearchTerm(forms[0])`), so nothing plausible escapes both.
  - Not over-tested: each assertion in the search suite names a distinct outcome, and the shared `installSearchFormSeams` keeps setup to one call per test.

CODE QUALITY:
- Project conventions: Followed — the seam is staged through `withFuncSeam`/`withOpenDeps` per `cmd/testhelpers_test.go`, no `t.Parallel()`, the tmux-touching dependency is injected for every body that Executes, and the unit-lane tests build no binary and spawn no daemon.
- SOLID principles: Good — `searchFormPositionals` scans, `preDashPositionals` bounds, `pickerLanding` carries, `buildTUIModel` translates; each has one reason to change, and the separator bound has one home reached by both the scanner and the completer.
- Complexity: Low — the branch is one guard, the scan one loop.
- Modern idioms: Yes.
- Readability: Good — `pickerLanding` names the three ways the picker is reached in one seam, which is what the task set out to buy.
- Issues: One dead test-helper parameter, below.

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] cmd/open_search_test.go:137 — `installSearchFormSeams(t *testing.T, lister resolver.SessionLister)` takes a `lister` override that no caller supplies: all 43 call sites across `cmd/open_search_test.go` (27), `cmd/open_search_deferred_test.go` (10) and `cmd/open_search_warnings_test.go` (6) pass a literal `nil`, so the `if lister != nil { deps.SessionLister = lister }` branch at `cmd/open_search_test.go:151-153` never executes. Drop the parameter and the branch and delete the `, nil` from the 43 calls; the `resolver` import stays live on lines 110 and 281, so the file still compiles. — FAILS: an unreachable override branch in the suite's shared fixture, plus 43 call sites carrying an argument that means nothing — a reader is told the helper can swap the recording `SessionLister` that `TestOpenCommand_SearchForm_ResolvesNothing` depends on, when nothing does and the swap has never been exercised.

UNSETTLED:
- "`go test ./...` passes and `go test -tags integration -p 1 ./...` passes" — both suites would have to be executed; reading cannot settle a suite result. The unit lane covers this task's tests (`cmd`, `internal/resolver`); the integration lane's `cmd/abridged_integration_test.go`, `cmd/reattach_integration_test.go` and `cmd/concurrent_coldboot_integration_test.go` carry the mechanical seam-signature updates and need a real run to confirm.
