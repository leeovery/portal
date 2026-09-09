TASK: resume-hooks-silently-lost-2-12 — One Unresolvable-Pane Subtest (tick-1d8500)

Fold two subtests that shared their whole fixture — `cmd/hooks_pane_token_test.go`'s "it exits
non-zero from hook set on an unresolvable pane" (added by task 2-3) and `cmd/hooks_test.go`'s
pre-existing "it aborts hooks set when the hook-key read fails" — into one subtest in
`cmd/hooks_test.go` carrying the union of their assertions, and delete the redundant one.

ACCEPTANCE CRITERIA:
- One subtest covers the unresolvable-pane case
- Every assertion either currently makes still runs
- Both lanes pass

STATUS: complete

SPEC CONTEXT:
The specification's verification table (`specification.md:495`) requires "`hook set` on an
unresolvable `$TMUX_PANE`: exits non-zero and writes nothing to `hooks.json`", at unit lane.
`specification.md:190-192` is the mechanism behind it: the `show-options -p -t <pane>` existence
probe naming no option is what discriminates a gone pane from an unstamped one, so a target no
pane answers to fails on exit status before anything is minted, stamped or written. The subtest
under review is the unit-lane coverage of that table row's `hook set` half; the stamp/mint half
("nothing is minted or stamped") is covered separately and was deliberately left in place.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/hooks_test.go:431-454` (the surviving merged subtest, inside
  `TestHooksSetCommand`); `cmd/hooks_pane_token_test.go:182-207` (`TestHooksSetRefusesAnUnresolvablePane`,
  now holding only "it mints and stamps nothing when the probe fails").
- Notes:
  - The delivered change-set is exactly the two test files plus tick/manifest bookkeeping
    (commit `de682dcc`); no production code moved, which is right for a test-consolidation task.
  - Criterion 1 holds and is exclusive: grepping `cmd/*_test.go` for the unresolvable-pane
    fixture, the only `hook set` subtest asserting on the error's properties is
    `cmd/hooks_test.go:431`. The two other `&tmux.CommandError{Stderr: …}` fixtures in the
    package (`cmd/hooks_rm_exit_test.go:165` and `:306`) drive `hook rm`, a different command
    and a different contract, and `cmd/hooks_pane_token_test.go:192` is the retained
    mint/stamp-count subtest whose subject is not the error at all. That retention is correct —
    it asserts a property the merged subtest does not (zero mints, zero `set-option` calls), so
    it is not the duplicate the task named.
  - Criterion 2 holds as delivered. At `de682dcc` the merged subtest carried all five assertions
    from both originals: non-nil error, the pre-existing `"resolve"` substring, tmux's own words
    surviving in the message, `errors.As` recovering `*tmux.CommandError`, and `hooks.json`
    never created. The tree today shows four of those five: the `"resolve"` substring is gone and
    the words-survive check has been strengthened from `strings.Contains` to exact equality
    (`err.Error() != stderr`). That is a later task's deliberate change, not a loss here —
    `c9345df3` (task 6-10, "one seam resolver, one Portal clause") removed the Portal-authored
    "resolve" clause from the production path, so `resolveCurrentPaneKey` (`cmd/hooks.go:67-79`)
    now returns the resolver's error unaltered and an assertion for the word "resolve" could
    only fail. Exact equality is strictly stronger than the substring form it replaced.
  - The merged subtest injects `PaneStamper: &recordingPaneStamper{}` alongside the resolver,
    satisfying CLAUDE.md's rule that a test Executing a real command body inject every
    tmux-touching `*Deps` seam. `PaneLister` and `TokenMinter` are legitimately left to their
    production defaults: `hook set` never enumerates panes, and the mint is pure.

TESTS:
- Status: Adequate
- Coverage: The one subtest exercises the whole abort path — `TMUX_PANE=%999`, a resolver
  answering `&tmux.CommandError{Stderr: "no such pane: %999"}`, `hook set --on-resume` driven
  through the shared `runHookSet` helper — and asserts (1) a non-nil error, (2) the message is
  tmux's own words unaltered, (3) the value is still a recoverable `*tmux.CommandError`, and
  (4) `hooks.json` was never created. Each assertion observes a distinct way the feature could
  break: dropping the abort (nil error), re-wrapping the message (equality fails), flattening
  the error to a string (`errors.AsType` fails), or writing before the read is trusted (the
  stat succeeds). None is redundant with another, so the union is not over-tested.
- Notes:
  - The fixture is honest end-to-end: `CommandError.Error()` with a nil `Err` and nil `Args`
    returns the trimmed `Stderr` (`internal/tmux/command_error.go:24-39`), so the exact-equality
    assertion is a real statement about the command's behaviour rather than an artefact of the
    fake. `resolveCurrentPaneKey` returns the resolver error unaltered and cobra returns a
    `RunE` error as-is, so the typed value genuinely survives to the caller.
  - Unit lane, correctly: `cmd/hooks_test.go` carries no `//go:build integration` tag, and the
    subtest builds no binary, spawns no daemon and opens no tmux server.
  - Tests were not executed (reading only, per the review protocol); the third criterion "both
    lanes pass" is judged by reading. The merged subtest compiles against the current tree —
    `errors`, `tmux`, `os` and the `hookstest`/`logtest` helpers it uses are all imported and
    still referenced elsewhere in the file, and the deletion left `cmd/hooks_pane_token_test.go`
    with no orphaned imports (`bytes`, `fmt`, `os`, `hookstest`, `nanoid`, `state`, `tmux` all
    still used).

CODE QUALITY:
- Project conventions: Followed. Seam staging goes through `withHooksDeps` (`cmd/testhelpers_test.go:26`)
  rather than a direct `hooksDeps = &…` assignment, which is what `cmd/seam_guard_test.go`
  requires; the drive goes through the shared `runHookSet` helper rather than a hand-rolled
  `resetRootCmd`/`SetArgs` block, matching the file's newer cases. Seed keys come from the
  `hookstest` vocabulary where they appear.
- SOLID principles: Good — N/A in substance for a test-only change; the subtest asserts one
  contract (the abort) and delegates fixture construction to the shared helpers.
- Complexity: Low. Linear subtest, no branching, no shared mutable state with its neighbours.
- Modern idioms: Yes. `errors.AsType[*tmux.CommandError](err)` replaces the older
  `var cmdErr *tmux.CommandError; errors.As(err, &cmdErr)` two-step, consistent with the
  production code's own usage (`internal/tmux/command_error.go:45,72`) on Go 1.26.
- Readability: Good. The `const stderr` names the one literal both the fixture and the assertion
  depend on, so the "tmux's own words unaltered" claim cannot drift from the words the fake
  actually produced, and each failure message says what the assertion is protecting.
- Issues: None. Two things I considered and am deliberately not reporting: the `resolver` local
  is used once and could be inlined, and the injected `recordingPaneStamper` is plain rather than
  poisoned with an `err` naming the violation (as `paneKeyPathSeams` does for the `--pane-key`
  path). Neither fails anything — the stamp is structurally unreachable once the resolver errs
  (`cmd/hooks.go:195-198` returns before `stampPaneToken`), and the zero-stamp property is
  asserted directly by the retained subtest at `cmd/hooks_pane_token_test.go:185-206`.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
