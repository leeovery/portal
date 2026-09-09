TASK: resume-hooks-silently-lost-8-42 — logtest.Sink's query surface grows multiplicatively because the composable filters were closed off (tick-e03255)

ACCEPTANCE CRITERIA:
- `Sink` exposes one base query plus the exactly-one assertions; no combination method survives.
- The four filters are exported and composable, and no two routes reach the same set.
- Every caller composes its own route; none re-derives a filter.
- CLAUDE.md's `logtest` row matches the shipped surface.

STATUS: complete

SPEC CONTEXT: Phase 8 is an implementation-analysis consolidation task, so its authority is its own body rather than the
specification (per the shared verifier context). The subject is test infrastructure only: `internal/logtest` is the shared
capture-`slog.Handler` every suite outside `internal/log` reads records through. No production behaviour, no resume-hook
semantics and no lane/isolation invariant is touched by the change; the risk surface is (a) a mistranslated query changing
the record set a suite asserts on, and (b) CLAUDE.md's `logtest` row going stale against the shipped surface.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/logtest/capture.go:130,135,141,149` — the four orthogonal filters, now exported: `AtExactLevel`,
    `AtOrAboveLevel`, `WithMessage`, `Matching`.
  - `internal/logtest/capture.go:250` — `Sink.Records()`, the single base query, is the only `Records*` method left on
    `Sink` (the six derived forwarders `RecordsAtExactLevel` / `RecordsAtOrAboveLevel` / `RecordsWith` /
    `RecordsWithMessage` / `RecordsAtExactLevelWith` / `RecordsAtExactLevelWithMessage` are deleted in commit
    `abe3132a`).
  - `internal/logtest/capture.go:157` — `Records.Only`, the exactly-one terminal, survives unchanged (it had already moved
    off `Sink` in an earlier task, which is the "if task 23 landed it" branch of the task's Do step 2).
  - `internal/logtest/capture.go:119-125,149-152` — the doc comments restating the invariant ("No combination of them is
    itself a method") and justifying `Matching` as a lift of `Record.Matches` rather than a second component dimension.
  - `CLAUDE.md:84` — the `logtest` row rewritten to describe base-query-plus-chained-filters.
  - 44 caller files across `cmd`, `cmd/bootstrap`, `internal/hooks`, `internal/hookstest`, `internal/project`,
    `internal/restore`, `internal/spawn`, `internal/state`, `internal/theme`, `internal/tmux`, `internal/tui` and
    `main_panic_test.go` re-pointed onto composed chains.
- Notes:
  - I read every converted call line in the commit against its predecessor. All are set-preserving: the removed
    `RecordsAtExactLevelWith(l, c, m)` was `RecordsWith(c, m).atExactLevel(l)` and became
    `Records().Matching(c, m).AtExactLevel(l)`; `RecordsAtExactLevelWithMessage(l, m)` was
    `RecordsWithMessage(m).atExactLevel(l)` and became `Records().WithMessage(m).AtExactLevel(l)`. Both filters are
    order-preserving and independent, so composition order does not change the set.
  - No stale reference to a removed method survives anywhere in the tree: a repo-wide grep for
    `RecordsAtExactLevel|RecordsAtOrAboveLevel|RecordsWith|RecordsWithMessage|OnlyRecord` outside `.git`, `.workflows` and
    `.tick` returns only CLAUDE.md:84's deliberate "…and are gone" clause. Integration-tagged files are covered by the same
    grep, so the tagged lane cannot hold a dangling caller.
  - No import churn in the commit (`git show abe3132a -U3` shows no `log/slog` import added or removed), so no converted
    file is left with an unused import — every site that dropped a `slog.Level` argument passes the same constant into the
    chained filter instead.
  - Single-route property holds as claimed: there is no component-only filter, so `Matching(c, m)` cannot be composed out
    of the other three, and none of the four is expressible as a chain of the others. The comment at
    `internal/logtest/capture.go:149-152` states exactly that rather than leaving it implicit.
  - Package-local shorthand helpers over the exported filters survive and are sanctioned by CLAUDE.md's row
    (`cmd/bootstrap/bootstrap_test.go:819,823`, `internal/tmux/hooks_register_warn_test.go:15`,
    `internal/tmux/portal_saver_test.go:3623`, `internal/hookstest/hooks_lock.go:98`) — each is a one-line composition of
    the exported filters, not a re-derivation of a predicate.
  - The remaining hand-rolled `for _, r := range sink.Records()` loops I checked (e.g.
    `cmd/bootstrap/clean_sweep_summary_test.go:56`, `cmd/bootstrap/eager_signal_hydrate_test.go:237,267,274`,
    `internal/project/store_logging_test.go:344`, `internal/tui/burst_observability_test.go:40`) are whole-capture negative
    assertions, per-record shape checks or partitions on predicates no exported filter expresses (an attr-key closed-set
    walk, a `strings.HasPrefix` on the message). None re-derives one of the four filters.

TESTS:
- Status: Adequate
- Coverage: `internal/logtest/capture_test.go` covers each exported filter in isolation
  (`TestRecords_AtExactLevel:334`, `TestRecords_AtOrAboveLevel:315`, `TestRecords_WithMessage:407`,
  `TestRecords_Matching:355`), the empty-result nil contract on each, composition
  (`TestRecords_FilterComposition:374`), the `Only` terminal off both a component-filtered and a level-filtered chain
  (`TestRecords_Only:192`), and the whole composed matrix over one capture
  (`TestRecords_ComposedRoutesCoverEveryQuery:461`). The three test names the task's Tests section names all exist
  verbatim: "it filters at exactly the given level" (`:335`), "it composes a level filter with a component filter"
  (`:375`), "it filters on message alone across components" (`:408`) — and the two mis-descriptive names the task asked to
  correct (`TestRecords_FilterChainCombinesLevelAndComponent`, `TestRecords_MsgFiltersOnMessageAloneAcrossComponents`) are
  gone, folded into `TestRecords_FilterComposition` and `TestRecords_WithMessage`.
- Notes: The consumer suites are the real verification for a migration of this shape, and they keep their assertions —
  every converted site asserts on the same record set it did before, which I checked line by line rather than by name.
  `TestRecords_ComposedRoutesCoverEveryQuery` overlaps the single-filter tests, but it pins the whole route matrix against
  one capture (the parity check the deleted forwarders' individual tests used to provide), so it earns its place.

CODE QUALITY:
- Project conventions: Followed. `logtest` stays stdlib + `harnesstest` + `internal/log` (no new import — its
  `leaf_guard_test.go` dependency pin is unaffected); test-only package, no production importer; subtests use the project's
  "it …" naming.
- SOLID principles: Good. The change moves the query surface from a cross-product of named methods to one base query plus
  orthogonal, composable predicates — additive growth per dimension, and each filter has exactly one reason to change.
  `Matching` still delegates to `Record.Matches`, keeping that predicate single-homed.
- Complexity: Low. Every filter is a one-line `filter(keep)` call; the shared `filter` helper is unchanged.
- Modern idioms: Yes — value-receiver methods on a named slice type returning new slices, nil for the empty result.
- Readability: Good. The type-level comment on `Records` states the composition rule and the property it protects, and
  `Matching`'s comment pre-empts the obvious "why isn't this two filters?" question.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
