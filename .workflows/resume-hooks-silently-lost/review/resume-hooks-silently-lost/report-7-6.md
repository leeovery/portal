TASK: resume-hooks-silently-lost-7-6 — CleanStale's Enumeration Callback Carries Its Decline Reason Out-Of-Band

ACCEPTANCE CRITERIA:
- `errCycleDeclined` and the captured `decline` variable are both gone from the file.
- A declined cycle still emits exactly one `clean-stale-skipped` line at the reason's own level with its own attrs.
- `sweepOutcome.DeclineReason` still carries the reason for every decline path, and remains empty for a cycle that ran.
- The `errNothingPersisted`, `ErrLockHeld` and `ErrSnapshotRead` branches are unchanged in behaviour.
- A decline carrying an empty reason is not constructible from the closure — the reason travels with the error rather than beside it.

STATUS: complete

SPEC CONTEXT:
The specification (§5.4, plus the 2026-08-30 and 2026-09-04 corrigenda) requires that every declined
hook-staleness cycle be identifiable from one line shape — `op=clean-stale-skipped`, `via=internal`, and a
`reason` attr drawn from a closed six-value vocabulary (`restoring`, `marker-read-failed`, `store-read-failed`,
`pane-read-failed`, `empty-pane-read`, `lock-timeout`) — because an operator whose hook vanished must answer
"did the prune stand down, and why" with one grep. This task is a phase-7 implementation-analysis hardening of
the *mechanism* behind that requirement rather than of the requirement itself: the reason and the error that
aborts the clean were two channels kept in step by hand, so a future decline path that returned the sentinel
without assigning the captured variable would have emitted an empty `reason=` — precisely the unidentifiable
stand-down §5.4 exists to prevent, and indistinguishable at `sweepOutcome.DeclineReason` from a cycle that ran
and found nothing.

IMPLEMENTATION:
- Status: Implemented (subsequently relocated by later tasks — see Notes)
- Location:
  - `internal/hooksweep/standdown.go:51-60` — `declinedError struct{ StandDown }` with an `Error()` naming the
    reason (`"hook staleness cycle declined: " + string(e.reason)`).
  - `internal/hooksweep/sweep.go:143-145` — the enumeration closure returns `declinedError{view.Decline}`,
    guarded by `view.Decline.Declined()`, with no captured decline variable anywhere in the file.
  - `internal/hooksweep/sweep.go:174-194` — `declinedSweep` recovers it with `errors.As(err, &declined)` and
    renders through `standDownOutcome(declined.StandDown)`.
  - `internal/hooksweep/sweep.go:122` — `errNothingPersisted` is the only bare sentinel left on the path.
  - Original delivery: commit `2c03b9e2` against `cmd/run_hook_stale_cleanup.go`.
- Notes:
  - Every criterion holds against the code as it stands. `errCycleDeclined` is gone repo-wide (grep over all
    `*.go` returns only workflow-artifact prose under `.workflows/`), the captured `decline standDown` is gone,
    and the only remaining construction of a decline is the guarded literal at `sweep.go:144`.
  - Drift, judged and accepted: later phase-8/9 tasks moved this code out of `cmd` into `internal/hooksweep`
    (`sweep.go` + `standdown.go`), renamed `sweepOutcome` → `hooksweep.Outcome`, promoted `standDown` →
    exported `StandDown`, replaced the string reason with the typed `hooksweep.Reason`, and renamed
    `hooks.ErrSnapshotRead` → `hooks.ErrStoreRead` (widening it to cover both of the clean's reads). The task's
    mechanism survives all of it intact and is arguably better placed — `declinedError` now sits beside the
    `StandDown` it transports. The criterion's `ErrSnapshotRead` wording is stale text, not a lost behaviour:
    the branch still classifies the same failure and still renders it as a stand-down.
  - The transport is sound at the type level: `CleanStale` returns the closure's error unwrapped
    (`internal/hooks/store.go:304-307`), so `errors.As` recovers it; the value receiver on `Error()` makes
    `&declined` a valid `errors.As` target; and `errors.Is(err, errNothingPersisted)` is ordered ahead of the
    `errors.As` branch with no overlap between the two.
  - The doc contract the task cites — "returned unwrapped, so a caller can carry its own reasons through"
    (`internal/hooks/store.go:294-297`) — is now the mechanism actually in use rather than an unexercised
    invitation.
  - Exactly-one-line behaviour is preserved by construction: `JudgeAgainstLivePanes` builds the `StandDown`
    without emitting, and emission happens once at `sweep.go:200` inside `standDownOutcome`. The read-only
    diagnosis (`cmd/doctor.go:375-379`) calls the same two exported gates and emits nothing, so the sweep
    remains the sole emitter.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooksweep/decline_error_test.go:16` — the closure's error carries the reason: `errors.As`
    recovers a `declinedError`, its `reason` is `ReasonEmptyPaneRead`, and `Error()` names it.
  - `internal/hooksweep/decline_error_test.go:36,58` — `DeclineReason` empty and no stand-down line for a cycle
    that ran and removed nothing, and for the nothing-persisted path.
  - `internal/hooksweep/decline_error_guard_test.go:14` — source guard failing any production `declinedError`
    composite literal with no elements, with a scanned-nothing tripwire (`literals == 0` is fatal). This is
    what makes criterion 5 structural rather than aspirational, and the guard's own comment is honest about
    why (Go cannot forbid the zero literal inside the declaring package).
  - `internal/hooksweep/sweep_move_test.go:47` — the every-reason table: for all six `Reasons` it asserts the
    outcome's `DeclineReason`, that nothing was removed, and (via `assertStandDown`,
    `internal/hooksweep/helpers_test.go:114`) exactly one record at the reason's own level carrying
    msg/op/component/via/reason. Its trailing loop over `Reasons` fails if a declared reason has no case, so
    the coverage cannot silently shrink.
  - `cmd/doctor_stand_down_copy_test.go:56-166` — per-reason attr pins beyond the reason itself: the `error`
    attr's exact or containing text for the four failure reasons, its *absence* for `restoring`, and the
    `entries` count for `empty-pane-read`. Dropping `s.attrs...` from `StandDown.emit()` would fail here, so
    "with its own attrs" is genuinely observed rather than assumed.
- Notes:
  - Three of the six test names in the plan's Tests list do not exist verbatim ("it emits one
    clean-stale-skipped line naming the restore reason", "…the empty-pane-read reason", "it reports
    DeclineReason on the outcome for every decline path"). They were never authored under those names by the
    task's own commit either; the behaviours they name are covered — and covered better — by the consolidated
    every-reason tables cited above, which drive all six reasons rather than two. No loss, so no finding.
  - Not over-tested: the three new subtests are distinct (error transport, ran-and-removed-nothing,
    nothing-persisted) and the guard is a source scan the task's own reasoning calls for.

CODE QUALITY:
- Project conventions: Followed. Unit-lane tests, no `t.Parallel()`, the whole cycle stays on the `hooks` log
  component from the package's single `log.For` binding (`internal/hooksweep/sweep.go:14`), and the source
  guard routes through `sourceguardtest.ParsePackageSources` rather than re-authoring the enumerate-parse loop.
- SOLID principles: Good. The error type has one job — transporting a `StandDown` — and the emit/render split
  (build in `JudgeAgainstLivePanes`, emit in `standDownOutcome`) keeps the one-line-per-decline invariant in one
  place.
- Complexity: Low. `declinedSweep` is a flat four-arm classification switch with a default fall-through that
  both logs and returns the error.
- Modern idioms: Yes. Typed error + `errors.As` is the idiomatic replacement for sentinel-plus-side-channel.
- Readability: Good. `declinedError` is a five-line type whose doc comment states the property it exists to
  hold ("so a decline names its reason at the site that returns it rather than in a variable beside it").
- Comment accuracy: The comments on `declinedError`, `liveTokenEnumeration`, `declinedSweep` and
  `standDownOutcome` all hold against the code; none references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
