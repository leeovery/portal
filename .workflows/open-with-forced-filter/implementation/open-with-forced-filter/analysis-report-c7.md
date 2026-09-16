# Analysis Report: open-with-forced-filter (Cycle 7)

## Stats

- Total findings: 0
- Deduplicated findings: 0
- Proposed tasks: 0

## Summary

All three analysis agents — duplication, standards and architecture — returned `FINDINGS: none` from a full fresh pass over the whole change set. Every rule the feature introduces was found to have exactly one home (the sigil's shape, its match domain and rule, the searched set, the home abbreviation, the `--` bound, the session-list read), the specification's decision points were confirmed implemented as decided with the build, `go vet`, the unit lane for the touched packages and `golangci-lint run` all clean, and no architectural candidate cleared the finding floor. No spec defect was raised and no comment correction was collected; nothing is staged for this cycle.

## Carried Verification Notes

Recorded from the standards and architecture passes so a later cycle need not re-derive them. None is a finding; none is actionable.

### Confirmed against the specification (standards)

- **Recognition**: `resolver.IsSearchSigil` is a pure shape test (leading `/`, exactly one `/`), reached from `IsPathArgument` as the narrowing the spec frames it as, and never consults the filesystem. `-p` bypasses `IsPathArgument` entirely via `ResolvePathPin` → `ResolvePath`, so the pinned-path escape works.
- **Positional independence and the `--` bound**: `searchFormPositionals` scans `preDashPositionals` only; `orderedOpenTargets` independently breaks at a bare `--`. `portal open -- /term` runs `/term` as the trailing command, unaffected.
- **Outcomes**: `_`-prefixed internal sessions are filtered in `parseSessionList`; the caller's own session is dropped by `tui.PickerSessions`, and both the count (`searchCandidates`) and the picker (`filteredSessions`) route through that helper with the same `currentPickerSession` degrade-to-`""` rule, so count and list cannot disagree. A term-less form takes no count, and `evaluateDefaultPage` pins `PageSessions` for a search form even at zero live sessions.
- **Landing**: `applySearchLanding` sets the text before the state flip; `list.SetFilterText` runs the pass synchronously and `GoToStart`s, matching `-f`'s landing exactly. The term-less form lands `list.Filtering` plus empty text.
- **Count timing**: `dismissLoadingGate` is reachable only once both `LoadingMinElapsedMsg` and `BootstrapCompleteMsg` have landed, and `searchDecideInFlight` holds the page against a repeat of either — so the decision is taken after every bootstrap step, never at end-of-restore.
- **Failure path**: `ListSessionsProbe` wraps the `*CommandError` (tmux's argv, exit status and stderr); the warm route emits buffered warnings then returns the error, and the cold route returns it through `Model.SearchError()` → `processTUIResult` ahead of `Selected()`, with no in-TUI error frame. Exit is non-zero via `main.classify`.
- **Match rule**: `MatchesSearchTerm` tests `SearchFields` separately and case-folded, never joined, with a literal term; `SessionItem.FilterValue()` joins name + one space + home-abbreviated recorded dir for the two fuzzy routes; `containmentFilter` falls through to `list.DefaultFilter` the moment the query diverges from the term by a single character, and emits ranks in target order with no re-rank. `searchEntry` carries `Session.Dir` only — the grouping-derived `m.derivedDirs` is a separate map that never lands in `Session.Dir`.
- **Directory column**: `ShowDir` is `m.searchForm`, set for the term-less form too and never cleared, so it survives a hand edit, a regroup, a `Space` round-trip and the Projects detour. `fitSessionDir` abbreviates first, left-truncates on a separator, and returns `""` below the one-whole-segment floor; the dir takes `countTok` (`TextMuted` / `TextSecondary` when selected) — no new token, no glyph under `NO_COLOR`.
- **Composition refusals**: `validateOpenArgs` runs as `Args`, which cobra 1.10.2 evaluates before `PersistentPreRunE`, so a refused line starts nothing. Each named arm names what collided; `firstSetLocalFlag`'s catch-all reads the registration (`LocalFlags().VisitAll` filtered on the shared `Changed`, accurate where `Visit` over that second set reports nothing) so a future `open` flag is refused the day it is registered. `--help` short-circuits ahead of `ValidateArgs`, and there are no root persistent flags today.
- **Completion**: `completeSearchTerm` offers from `tui.PickerSessions`, keeps the slash, and drops a name that would compose a second one; `completingPreDashPositional`'s `<=` is correct against cobra's probe-parse (`ParseFlags(append(finalArgs, "--"))` leaves pflag's dash index at `len(args)` for a separator-free line), and the post-separator arm answers `ShellCompDirectiveDefault` so the user keeps their filenames. The `init` shims rewrite `COMP_WORDS`/`COMP_CWORD`/`COMP_LINE`/`COMP_POINT` (bash) and `words`/`CURRENT` (zsh) and wrap via `-w` (fish), all keyed off the configured `--cmd` name, and the emitted scripts are driven through the real shells by `cmd/init_completion_shell_test.go`.
- **Cold path**: `isTUIPath` classifies a search form on its shape; warnings stay in the sink on that classification and reach the terminal through `WarningsOwedAtTeardown` before the connector runs on K = 1. `Init` subscribes `m.progressReceiver` from a single tail append on every branch, which brings the command-pending picker onto the terminal events; `createSession` stages while `bootstrapInFlight()` with first-stage-wins, `BootstrapCompleteMsg` replays through `mintSession`, `activeProjectNoticeBand` displaces the pick-a-project banner, and `BootstrapFatalMsg` quits a command-pending model — the closed defect, per the 2026-09-15 corrigendum.
- **Documentation**: `openCmd.Long` carries the form, the recognition rule, the three outcomes, the term-less form, the non-composition rule and the `-f` / `/term` outcome distinction; `-f`'s flag description stays a one-liner. The README's `x (open)` section adds the completion correction and its rollout consequence, and the pin table carries the `/<term>` row. No CHANGELOG entry was written.

### Examined and deliberately not written (architecture)

- `tmux.ListSessions` / `ListSessionsProbe` look like a derivation candidate (same query, different error policy), but the argv (`listSessionsArgs`) and the parse (`parseSessionList`) are already shared and the residue is one `c.cmd.Run` line; the naive derivation would additionally swallow parse errors `ListSessions` currently propagates, so it is not a clean composition.
- `resolver.IsPathArgument`'s sigil carve-out means `QueryResolver.Resolve("/term")` falls through to alias/zoxide rather than the path domain. Unreachable today — `open`'s RunE intercepts every search-form positional ahead of resolution and `validateSearchFormCollisions` refuses a sigil beside another target — so the risk is hypothetical for a caller that does not exist.
- `Model` gained seven loose search-related fields rather than a `searchState` sub-struct (the `themeState` precedent). No failure attaches to it; the model already carries many such field groups.
- `SessionDelegate.renderSessionRow`'s unsized branch (`total <= 0`) re-derives `resolver.AbbreviateHome` instead of routing through `fitSessionDir`. Reachable only before the first `WindowSizeMsg`; a divergence would change one transient frame.
- The cold-boot integration harness (`cmd/concurrent_search_decision_integration_test.go`) composes its own decision closure rather than driving `searchDecision`, so it exercises the timing seam but not the candidate-set composition. Its stated subject is the timing, which it pins hard (steps-at-call, marker cleared, exactly-once, no early dismissal), so it is not a guard passing while checking nothing.

### Near-duplicates confirmed below the floor (duplication)

`applySearchLanding` vs `applyInitialFilter` and `completeSearchTerm` vs `completeSessionNames` are each justified by a stated difference in rule; the unsized directory branch beside `fitSessionDir`, the capture fixture's home fallback beside `AbbreviateHome`'s, and the emitted completion shims' function names beside their registrations are each covered by a test that would fail loudly on drift. None clears the floor's silent-and-consequential test.

## Discarded Findings

None — no agent wrote a finding this cycle.
