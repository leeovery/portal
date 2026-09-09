TASK: resume-hooks-silently-lost-8-44 (tick-c41dfc) — "The Sweep Enumerates Every Pane On The Server Before Checking Whether There Is Anything To Sweep"

ACCEPTANCE CRITERIA:
- With an empty `hooks.json`, one cycle issues zero pane enumerations (assertable on the fake lister's call count).
- The counts DEBUG never carries a pane count for a cycle that did not enumerate.
- `liveTokenEnumeration(reader, nil)` does not panic.
- `declinedSweep`'s four decline sites route through one emit-and-return, with each branch's reason and level unchanged.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis task, so its authority is its own body rather than the specification. The
spec's relevant surrounding contract is §6.3/§6.4 (as amended by the 2026-09-01 corrigendum): `CleanStale` takes the
*enumeration as a callback* and performs its own `loadSnapshot` before invoking it, so the snapshot-strictly-before-
enumeration ordering is structural. The task's change lives inside that callback — the short-circuit is taken on the
snapshot `CleanStale` already handed in, so the ordering property is untouched and the narrowing rule (§6.3: the
snapshot may only narrow the delete set) is strictly safer, since with an empty snapshot no deletion phase runs at
all. Nothing in the spec requires a pane enumeration on a cycle with nothing persisted.

IMPLEMENTATION:
- Status: Implemented (and since relocated by a later phase)
- Location: `internal/hooksweep/sweep.go:126-148` (`liveTokenEnumeration`), `:134-137` (the empty-snapshot
  short-circuit + entry-count-only DEBUG), `:139-142` (enumeration + the panes-bearing counts line, gated on
  `view.Enumerated`), `:174-194` (`declinedSweep`), `:199-202` (`standDownOutcome`), `:14` (the package-level
  `logger = log.For("hooks")`). Driven from `cmd/state_daemon.go:214` (`maybeRunHookCleanup`) and
  `cmd/doctor.go:201` (`pruneDoctorStaleHooks`).
- Notes:
  - Do 1 (short-circuit before the enumeration): done. `len(snapshot) == 0` at `sweep.go:134` returns
    `errNothingPersisted` before `JudgeAgainstLivePanes` at `:139`. Because `hooks.Store.CleanStale`
    (`internal/hooks/store.go:298-310`) invokes the callback between `loadSnapshot` and `deleteStale`, the abort also
    skips the mutation lock, so all four costs the task names are avoided, not just the tmux read.
  - Do 2 (counts DEBUG for that case): done — `logger.Debug(countsMsg, "entries", 0)` carries no `panes` attr, and
    the panes-bearing line at `:141` is additionally gated on `view.Enumerated`, so a failed enumeration emits no
    counts line either.
  - Do 3 (default the logger inside the factory): the task's commit (8942e506) added `countsOrDefault` in
    `cmd/run_hook_stale_cleanup.go`. A later phase moved the cycle into `internal/hooksweep` and replaced the
    injected `*slog.Logger` with the package-level binding at `sweep.go:14` (`log.For` is documented safe before
    `log.Init`), so the nil-logger hazard the task was closing no longer has a parameter to arrive through. The
    substance — the factory cannot panic on a nil logger — holds by construction; `countsOrDefault` and
    `countsLogger` are fully gone from the tree with no orphans (grep: only the unrelated `cmd/state_common.go`
    `hooksLogger` remains, used by `cmd/hooks.go:241`). This is a sound supersession, not a loss.
  - Do 4 (collapse the emit-then-return sites): done, and stronger than asked. `standDownOutcome` (`:199-202`) is now
    the single `decline.emit()` call site in the whole repository (verified by grep for `.emit()` — one hit,
    `sweep.go:200`), reached from `Run`'s stand-down (`:156`) and all three declining branches of `declinedSweep`
    (`:180`, `:185`, `:190`). Each branch's reason and level are unchanged: `declineWarn` for lock-timeout and
    store-read-failed, the carried `StandDown` for a `declinedError`.
  - Behavioural change worth naming, and correct: with an empty `hooks.json` and a failing tmux read, the cycle no
    longer stands down under `pane-read-failed`, so `portal doctor --fix` prints no "Skipped stale hook prune" line
    for an install with nothing to prune. Nothing is lost — with zero entries no key can be stale — and the
    read-only diagnosis (`cmd/doctor.go:379`, untouched by this task) still reports its own reading. Its verdict
    parity test (`cmd/hook_prune_verdict_parity_test.go`) only enumerates cases with entries present, so no contract
    is broken by the divergence.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooksweep/sweep_test.go:490-537` `TestHookSweepWithNothingPersisted`:
    - `:491` "it enumerates no panes when nothing is persisted" — asserts `stubReader.calls == 0` after a full `Run`
      over an empty store. Fails if the branch moves back below the enumeration (calls would be 1). Covers AC 1.
    - `:504` "it reports the entry count alone for an empty snapshot" — `entries == 0` and `!rec.HasAttr("panes")`
      over the installed sink. Covers AC 2; fails on a revert, since the old code emitted no counts line for the
      empty case and `Only(…)` would find no record.
    - `:522` "it answers the live token set for a non-empty snapshot" — the former nil-logger case, re-pointed at the
      one-argument factory when the logger parameter was retired. Pins both closure arms.
  - `internal/hooksweep/snapshot_order_test.go:103-119` `TestHookSweepTakesNoLockWithNothingPersisted` — asserts the
    cycle creates nothing under a config root holding no `hooks.json` (no directory, no `.lock` sidecar), which is
    the other three costs the short-circuit avoids.
  - `internal/hooksweep/decline_error_test.go:58-75` — nothing-persisted returns no decline reason and emits no
    stand-down line.
  - AC 4's "reason and level unchanged" is carried by `internal/hooksweep/sweep_move_test.go:96-137`: a table driving
    every stand-down path, asserting reason + level per case, with an exhaustiveness loop over `Reasons` (`:133-137`)
    that fails if a declared reason has no case driving it. `internal/hooksweep/sweep_test.go:444-457` keeps the
    positive counts line (`panes == 4`), so the two halves of the counts contract are both pinned.
- Notes: No over-testing — three focused subtests plus reuse of the existing decline table; no redundant assertions
  and no mocking beyond the single `stubReader` seam fake the package already had.

CODE QUALITY:
- Project conventions: Followed. Unit-lane tests only (no binary build, no daemon, no real tmux), no `t.Parallel()`,
  logging bound once per package via `log.For`, `hooks` component and the existing `entries`/`panes` attr keys — no
  new vocabulary invented. Test seams and the `logtest.Sink`/`hookstest.StageStore` helpers are used by name as
  CLAUDE.md requires.
- SOLID principles: Good. `standDownOutcome` gives the decline-rendering one home; `liveTokenEnumeration` keeps the
  cheap decision at the boundary that owns the snapshot.
- Complexity: Low. The closure is a three-branch ladder ordered cheapest-and-most-decisive first.
- Modern idioms: Yes.
- Readability: Good. The comment at `sweep.go:128-133` names all four costs the branch avoids and says why the counts
  line carries no pane figure; it holds against the code (the fourth cost, the exclusive hold, is real — the abort
  propagates out of `CleanStale` before `deleteStale` opens one).
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
