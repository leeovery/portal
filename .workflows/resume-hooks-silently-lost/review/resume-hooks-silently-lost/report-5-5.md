TASK: resume-hooks-silently-lost-5-5 — "A Locked-Out Sweep Stands Down And Says So"

ACCEPTANCE CRITERIA (from the plan task):
- A `CleanStale` lock timeout stands the cycle down: nothing deleted, `hooks.json` byte-identical, the sweep returns nil
- Exactly one WARN per stood-down cycle, under the `hooks` component, `op=clean-stale-skipped`, `via=internal`, `reason=lock-timeout`, lock error in `error`; the daemon emits no second WARN
- The level is WARN, unlike the restore window's DEBUG
- `onSkipped` invoked once with `lock-timeout`; a nil `onSkipped` (the daemon's call shape) is safe on this branch
- `portal doctor --fix` prints `Skipped stale hook prune: hooks.json is locked` on the same writer and in the same repair block as its `Pruned stale hook:` lines
- `doctor --fix`'s exit code unaffected by the stand-down — driven solely by the post-repair diagnosis
- `checkStaleHooks` in the same window reads unlocked and reports the un-pruned entry as stale
- A save failure (or any other non-sentinel error) still returns as an error and is never a stand-down
- The stand-down is recognised by `errors.Is` on the exported sentinel, never by error text
- A fully contended cycle costs one `lockTimeout` in total, not two; no retry loop, backoff or escalation
- Both call sites behave identically — the daemon's throttled idle branch and `doctor --fix`
- `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§6.5 splits lock-failure behaviour by side: a write that cannot take the lock does not write, a read that
cannot take it reads anyway (unlocked, DEBUG `op=load-unlocked`). The sweep's skipped write is specified as
the existing `hooks` failure shape under `op=clean-stale-skipped`, `via=internal`, `reason=lock-timeout` and
the lock error in `error` (spec line 384). §5.4 fixes one skip-line shape over the reason set and pins the
levels — DEBUG for the restore window, WARN for a lock that will not yield. §5.1 fixes the `doctor --fix`
repair lines (`Skipped stale hook prune: hooks.json is locked`, spec line 256) and states none of them
affects the exit code, which stays driven by the post-repair diagnosis. §9.2's "a lock timeout degrades by
side" is the named verification. The 2026-09-01 corrigendum adds the `lock-timeout` entry to
`notEvaluableDetails` for vocabulary completeness (that path is unreachable, since a read degrades rather
than failing), and the second 2026-09-01 corrigendum establishes the two-bound derivation the "one bound
per contended cycle" criterion rests on.

IMPLEMENTATION:
- Status: Implemented (drifted from the plan's file/API names by later phases; the drift is sound)
- Location:
  - `internal/hooksweep/sweep.go:181` — the `errors.Is(err, hooks.ErrLockHeld)` arm of `declinedSweep`,
    returning `standDownOutcome(declineWarn(ReasonLockTimeout, "error", err))`, i.e. a WARN stand-down and
    a nil error, ordered after the `errNothingPersisted` and `declinedError` arms and before `ErrStoreRead`.
  - `internal/hooksweep/reason.go:24,41` — `ReasonLockTimeout Reason = "lock-timeout"`, declared and
    enumerated in `Reasons`.
  - `internal/hooksweep/standdown.go` — `declineWarn` / `standDownAttrs`, giving the line
    `op=clean-stale-skipped`, `via=internal`, `reason=<r>` at WARN under `log.For("hooks")`.
  - `cmd/doctor.go:225,242,253` — `lockStandDownPhrase = "hooks.json is locked"` composed into both
    `skippedPrunePhrases` and `notEvaluableDetails`.
  - `cmd/doctor.go:201-211` (`pruneDoctorStaleHooks`) — renders `outcome.DeclineReason` through
    `reportSkippedPrune(w, …)` on `cmd.OutOrStdout()`, the same writer and block as the `Pruned stale hook:`
    lines.
  - `cmd/state_daemon.go:214` (`maybeRunHookCleanup`) — `_, _ = hooksweep.Run(…)`, so no second WARN.
  - `internal/hooks/lock.go:15` (`ErrLockHeld`), `:71` (only the elapsed bound wraps it — an open failure
    and a non-EWOULDBLOCK flock failure do not), `:26-39` (`snapshotLockBound`), `:118-124`
    (`acquireMutationLock`); `internal/hooks/store.go:318-325` (`deleteStale` returns the acquire error
    unwrapped, emitting no WARN of its own — which is what makes the sweep's line the only one).
- Notes:
  - Two legitimate divergences from the plan's wording, both post-dating this task:
    1. The cycle moved from `cmd/run_hook_stale_cleanup.go` to `internal/hooksweep` (task 9-12), so the
       plan's `skipReasonLockTimeout` string const is now the typed `hooksweep.ReasonLockTimeout` and the
       WARN is emitted by the package that owns the cycle rather than by `cmd`. The property the task cared
       about — the logged value and the printed line cannot drift — is stronger for it: the value is a
       closed type, the phrase table is keyed on it, and two guards enforce the pairing
       (`internal/hooksweep/reason_enumeration_guard_test.go`, `cmd/doctor_stand_down_phrase_guard_test.go`).
    2. The `onSkipped` callback became the returned `Outcome.DeclineReason`, so the "nil `onSkipped` must be
       safe" criterion is met structurally rather than by a nil check — the hazard no longer exists. Not a
       loss.
  - The plan's third-value framing ("the third entry in the reason→phrase table") is now sixth of six; the
    two extra reasons are the 2026-08-30 corrigendum's `store-read-failed` / `pane-read-failed`. Consistent.
  - The planning decision that only the elapsed bound yields `reason=lock-timeout` is honoured exactly: an
    unopenable sidecar produces `open hooks lock: …` with no `ErrLockHeld` in the chain, so it falls to
    `declinedSweep`'s default — one `stale-hook cleanup failed` WARN and a returned error, which
    `doctor --fix` renders as `Skipped stale hook prune: the sweep could not complete`.
  - The one-bound property holds in code: `CleanStale` reads via `loadSnapshot` →
    `loadSharedBounded(ViaInternal, snapshotLockBound())` (`internal/hooks/store.go:53-55`), which degrades
    to an unlocked read after ~one poll interval, so only `deleteStale`'s exclusive acquire spends the full
    `lockTimeout`.
  - No retry, backoff or escalation was added inside the cycle.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooksweep/lock_timeout_test.go` — deletes nothing under a held sidecar with `hooks.json`
    byte-identical; exactly one WARN at the right level/component/op/via/reason; the one-bound timing case
    (asserts `elapsed >= bound` and `elapsed <= 1.5×bound`, which is what separates one bound from two);
    the next-cadence retry after release, asserting the stale key is reaped and the live one is not.
  - `internal/hooksweep/lock_timeout_test.go` (`TestHookSweepDiscriminatesLockTimeoutFromFailure`) — a save
    failure still returns an error, is not `errors.Is` the sentinel, carries no `DeclineReason` and emits no
    stand-down record; and the text case stages the fixture in a directory literally named
    `hooks.ErrLockHeld.Error()`, first asserting the produced error's text really does carry the sentinel's
    words (so the case cannot silently measure nothing) before asserting it is not treated as a stand-down.
    That is a genuine guard against text matching, not a restatement of the code.
  - `cmd/hook_prune_single_report_test.go:47-68` — the daemon's throttled branch under a held lock: file
    unchanged, one WARN with `reason=lock-timeout`, and zero WARNs on the daemon's own injected logger.
  - `cmd/doctor_stand_down_copy_test.go` — the `"hooks.json locked"` row drives both surfaces from one
    table: the exact `Skipped stale hook prune: hooks.json is locked` line, the `error` attr carrying
    `hooks.ErrLockHeld`, the file untouched by the repair, placement in the hook-prune block (via
    `assertSkippedPruneLine`, which also fails a duplicated or differently-worded line), and the exit-code
    subtest that runs the whole `doctor` / `doctor --fix` Execute three ways — nil over a healthy
    post-repair diagnosis, `ErrDoctorUnhealthy` with a genuinely failing check on both paths.
  - `cmd/hook_prune_locked_test.go` — `checkStaleHooks` under the same held lock returns `checkFail` with
    `1 stale hook entry`, pinning the read/write split.
  - Exhaustiveness is guarded rather than asserted: `TestReasonsEnumeratesEveryDeclaredConst` fails a
    `Reason` const left out of `Reasons` (and proves its own rule against a synthetic source), and
    `TestStandDownCopy`'s first subtest fails a declared reason with no copy case, in both directions.
- Notes:
  - Not over-tested: the lock row shares the six-reason table rather than duplicating the surface
    assertions, and `internal/hooksweep` covers the cycle while `cmd` covers the two renderings — no
    overlap I could find.
  - The plan's `"it logs the stand-down at WARN with reason=lock-timeout"` asks for a non-empty `error`
    attr; `internal/hooksweep`'s own case asserts level/component/op/via/reason but not `error`. The
    `error` attr is asserted on the same emission by `cmd/doctor_stand_down_copy_test.go`
    (`standDownErrorAttrCarrying(hooks.ErrLockHeld.Error())`), so the criterion is covered — it is covered
    from the table that owns the copy rather than from the package's own suite.
  - `go test` was not run (reading-only review). Judged by reading, nothing in the changed code would break
    a passing assertion elsewhere.

CODE QUALITY:
- Project conventions: Followed. The whole cycle emits under `log.For("hooks")` in `internal/hooksweep`,
  matching CLAUDE.md's "it binds `log.For("hooks")` itself and emits the whole cycle under it"; `reason` is
  an existing attr key and `clean-stale-skipped` an existing `op` per the spec's vocabulary amendment; both
  lock-timeout tests are unit-lane and drive the bound down via `hooks.SetLockTimeoutForTest` rather than
  waiting out the production figure; no test touches a real tmux server or the developer's state.
- SOLID principles: Good. `declinedSweep` is one classification point; the reason vocabulary is a closed
  type in the package that produces it, and `cmd` holds only the two rendering tables — a caller cannot
  reach for words of its own, and `phraseFor` deliberately renders nothing rather than leaking a slug.
- Complexity: Low. The addition is one `case` in an existing `switch` plus two map entries.
- Modern idioms: Yes. Sentinel + `errors.Is`/`errors.As`, a named string type for the closed vocabulary,
  table-driven tests.
- Readability: Good. Every non-obvious choice carries its reason in-source (why the arm returns nil, why
  the pre-read has its own bound, why `lock-timeout`'s not-evaluable phrase exists with no path to it).
- Issues: None found. Comments in the changed regions hold against the code: the arm's "nothing was
  written" is true (the acquire precedes both the load and the save in `deleteStale`), and `reason.go`'s
  claim that `lock-timeout` cannot reach the not-evaluable surface is true — `checkStaleHooks`'s only
  store read is `Load`, which degrades to an unlocked read rather than returning `ErrLockHeld`.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
