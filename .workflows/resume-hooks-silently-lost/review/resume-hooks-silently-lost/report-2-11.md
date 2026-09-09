TASK: resume-hooks-silently-lost-2-11 — One Way To Reach The Hooks Seams (tick-404bd6, severity: drift)

ACCEPTANCE CRITERIA:
- All three seams are reached through named accessors following the package's `buildX` convention
- `resolveCurrentPaneKey` no longer holds the resolver branch
- No seam semantics change and no test is re-pointed
- Both lanes pass

STATUS: complete

SPEC CONTEXT:
The specification governs the hook-key rework (pane-durable `@portal-pane-id` tokens replacing volatile
`<session>:window.pane` keys). It speaks to the seam only in substance, not in shape: §"pane-option write is
added" requires the new pane-scoped option setter to be "reached through the `hook` command's existing `*Deps`
seam like every other tmux call the CLI makes", and the test-strategy rows require the injected-seam cases to be
drivable from the unit lane. It says nothing about accessor naming or the internal shape of the seam
resolution — that is this consolidation task's own authority. Both spec obligations hold in the delivered code:
every hook-command tmux call runs through `HooksDeps`, and the seams are injectable per-test.

IMPLEMENTATION:
- Status: Implemented, then deliberately superseded within the same plan (drift is sound, not a loss)
- Location:
  - Task commit: `ce4fa23f` — `cmd/hooks.go` only (plus `.tick/tasks.jsonl`, the plan manifest). It extracted
    `buildHookKeyResolver()` from the inline branch inside `resolveCurrentPaneKey` and renamed
    `hooksPaneStamper`/`hooksTokenMinter` to `buildPaneStamper`/`buildTokenMinter` — exactly the Do list, with
    the branch bodies carried over character-for-character (pure extraction + rename, no semantics moved).
  - Current source of truth: `cmd/hooks.go:83-104` (`hookSeams()`), consumed at `cmd/hooks.go:73`
    (`resolveCurrentPaneKey`), `cmd/hooks.go:109` (`stampPaneToken`), `cmd/hooks.go:151` (`hook list`).
    Commit `c9345df3` (task 6-10, "one seam resolver, one Portal clause") collapsed the four `buildX` accessors
    into the single `hookSeams()` merge that fills a production default per unset field.
- Notes:
  - The wording of criterion 1 ("named accessors following the `buildX` convention") no longer matches the code,
    because a later task in this same plan replaced four near-identical three-line accessors with one merge
    function. Judged against intent rather than wording, this is strictly stronger delivery of the task's stated
    Outcome — "all seams are reached the same way": there is now exactly one way, and it covers the fourth seam
    (`PaneLister`, added by task 4-3) that the `buildX` shape would have made a fifth copy of. No behaviour the
    task's intent needs is gone, so this is not recorded as a finding.
  - Criterion 2 holds in the current code: `resolveCurrentPaneKey` (`cmd/hooks.go:67-79`) is the two reads it is
    named for — `requireTmuxPane` then `hookSeams().KeyResolver.ResolveHookKey`.
  - Criterion 3 holds: `git show --stat ce4fa23f` touches no `*_test.go`; tests set `hooksDeps` fields through
    `withHooksDeps` (`cmd/testhelpers_test.go:26-33`) and never call the accessors.
  - No inline seam branch survives anywhere: the only production readers of `hooksDeps` are `hookSeams()` itself
    (`cmd/hooks.go:85-87`) and nothing else — verified by enumerating every non-test reference to `hooksDeps`,
    `hookSeams` and `buildHooksTmuxClient` across the tree (5 sites, all inside `cmd/hooks.go`).
  - No dangling references to any of the five removed names (`hooksPaneStamper`, `hooksTokenMinter`,
    `buildHookKeyResolver`, `buildPaneStamper`, `buildTokenMinter`) remain in `.go` or `.md` outside the
    `.workflows/` planning record, which is history rather than a live pointer.
  - Types line up on inspection: `HooksDeps.TokenMinter` is `nanoid.Generator` (`internal/nanoid/nanoid.go:26`)
    and `nanoid.NewPaneTokenGenerator()` returns that type (`:35`); `*tmux.Client` satisfies all three interface
    seams, asserted at compile time by the `var _ …` block at `cmd/hooks.go:33-37`.

TESTS:
- Status: Adequate (no new test was required and none was added, per the task's Tests section)
- Coverage:
  - `cmd/hooks_seams_test.go:12-89` drives the resolution directly across all three arms: every seam falling
    through to the production `*tmux.Client`/production minter; every seam resolving to its injected fake; and
    the mixed case where one seam is injected and the rest fill in. The production-minter case additionally
    asserts `nanoid.IsTokenShaped` on the minted token, so a minter swapped to the wrong width fails here rather
    than silently producing a key `hooks.json` can never judge.
  - `cmd/hooks_seams_test.go:91-136` (`TestGonePaneErrorCarriesOnePortalClause`) drives both `hook set` and
    `hook rm` through the real error chain with only the `KeyResolver` seam injected, which exercises the
    fill-in path for the remaining seams on the live command bodies.
  - `cmd/seam_staging_test.go:27,66-76` pins that `hooksDeps` is installed for the test and restored after it,
    so a seam leaking across tests fails loudly.
- Notes:
  - Not over-tested: the three sub-tests are one per resolution arm with no redundancy between them, and the
    `hook set`/`hook rm` suites are reused as the behavioural coverage rather than duplicated.
  - Would the tests fail if the change broke? Yes — dropping a fill-in branch makes the corresponding
    "production default" assertion fail on a nil/zero seam, and re-introducing an inline branch that bypassed
    `hookSeams` would break the injected-fake identity comparisons at `:53-66`.
  - Both lanes: judged by reading only (running tests is outside this review's remit). The change is
    compile-local to `cmd/hooks.go`, every symbol it introduces or consumes resolves, and no test file was
    re-pointed, so there is no reason visible in the source for either lane to break.

CODE QUALITY:
- Project conventions: Followed. The seam is a package-level `*Deps` pointer read by the command body and staged
  through the `withHooksDeps`/`withoutHooksDeps` helpers, which is exactly the DI pattern CLAUDE.md mandates for
  `cmd`; the seam-guard test derives the family from the production sources, so this seam is covered by it.
- SOLID principles: Good. Each seam is a 1-method interface (`HookKeyResolver`, `PaneHookLister`,
  `PaneOptionSetter`) plus a function type for the minter — interface segregation held, and the command bodies
  depend on the abstractions rather than `*tmux.Client`.
- Complexity: Low. `hookSeams()` is a flat sequence of four nil-checks with no branching depth; the accessors it
  replaced were four copies of the same two-condition branch.
- Modern idioms: Yes. `tmux.DefaultClient()` is a cheap struct construction (`internal/tmux/tmux.go:75-77`), so
  building it unconditionally in `hookSeams()` costs nothing even when every seam is injected — the simpler
  unconditional form is the right call over a lazy one here.
- Readability: Good. `hookSeams`'s doc comment states the whole contract in one sentence and holds true against
  the body; `stampPaneToken` (`:108-120`) takes one `hookSeams()` value and reads both its seams from it, so the
  minter and the stamper it pairs cannot come from two different resolutions.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
