TASK: resume-hooks-silently-lost-6-8 — Give The Restoring-Marker Posture One Named Home (tick-53ed90)

ACCEPTANCE CRITERIA:
- The "a failed read counts as set" rule appears in exactly one place.
- Each of the four call sites is one line naming its reporting policy, not a restatement of the rule.
- Every existing test of daemon stand-down, commit-now suppression and sweep stand-down passes unchanged.

STATUS: complete

SPEC CONTEXT:
The specification's relevant constraint is that the daemon's `tick` reads `@portal-restoring` and returns
before reaching `maybeRunHookCleanup` — "that early return protected only the capture path before this
change; it is now load-bearing for hook retention and must not be relaxed or reordered"
(specification.md:308). Corrigendum 2026-09-04 (specification.md:568) establishes that a failed marker read
takes the same *posture* as a set marker (stand down; report not-evaluable) but under its own reason and
phrase. Both hold after this task: the return still precedes the idle branch, and the posture — not the
reporting — is what got consolidated. Phase 6 is an implementation-analysis phase, so the task body is the
governing authority; the spec is context only.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/state/markers.go:108` — `RestoreWindowActive(restoring bool, err error) (bool, error)`,
    the single home of `restoring || err != nil` (markers.go:109).
  - `cmd/state_daemon.go:175` (tick) — WARN on a failed read, then return via the folded bool.
  - `cmd/state_daemon.go:340` (`defaultShutdownFlush`) — WARN on a failed read / DEBUG on a set marker,
    both inside the one stand-down arm, with the shutdown INFO emitted once either way.
  - `cmd/state_commit_now.go:107` — the fold applied to the existing `deps.IsRestoring()` seam;
    WARN on read failure, INFO on a set marker, then the touch + exit 0.
  - `internal/hooksweep/sweep.go:72` (`StalenessStandDown`) — the sweep/diagnosis gate; this is the
    task's `restoreWindowActive` call site after phase 9 moved it out of `cmd/run_hook_stale_cleanup.go`.
- Notes:
  - AC1 holds and is verifiable: `grep -rn "restoring || err"` over the tree matches only
    `internal/state/markers.go:109`, and the four production readers of the marker
    (`grep -rn "RestoreWindowActive"`) are exactly the four the task names — no fifth reader restates it.
  - AC2 holds: each site now expresses only its reporting policy (WARN / INFO / DEBUG / decline-reason);
    none re-derives "a failed read counts as set". The removed local helper
    `cmd/run_hook_stale_cleanup.go:restoreWindowActive` and its 7-line rationale comment are gone, and
    the orphaned `errAttr` helper it fed is gone too (`grep -rn errAttr` finds no `cmd`/`hooksweep`
    declaration).
  - Behaviour is preserved at all four sites, which is what the task asked for. `IsRestoringSet` returns
    `(false, err)` on failure, so the fold yields `(true, err)`: the daemon tick's WARN-then-return, the
    shutdown flush's WARN + `shutdown flush_completed=false`, commit-now's WARN + touch + exit 0, and the
    sweep's silent decline all reach the same terminal state by the same route as before.
  - No drift from the task body. The one departure from the literal Do list is item 3's "collapse the two
    rationale comments into one reference to the shared home": `tick`'s surviving comment
    (`cmd/state_daemon.go:170-173`) is about tick ordering, not the failed-read rule, so there was nothing
    there to collapse — the substance (no site restating the rule) is delivered.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/state/markers_test.go:366` `TestRestoreWindowActive` — the three-way table the task asked
    for (set / absent / read error), each subtest composing `RestoreWindowActive(IsRestoringSet(...))`,
    the read-error case asserting both the presumed-active bool and `errors.Is` against the sentinel.
  - `cmd/state_daemon_run_test.go:320` `TestDaemonTick_SkipsEntireTickAndWarnsOnRestoringReadError` (new)
    — asserts no `list-sessions` and exactly one `read @portal-restoring failed` WARN. The fake's
    `optionErr` is scoped to `show-option` only (`cmd/state_daemon_run_test.go:91-95`) and `sessionsOut`
    is seeded, so a leak through the guard really would invoke `list-sessions` and trip the assertion —
    the test fails if the behaviour breaks.
  - Set-marker counterpart: `cmd/state_daemon_run_test.go:302`.
  - `cmd/state_daemon_lifecycle_log_test.go:281` / `:152` — the shutdown flush's read-error and
    set-marker arms, each asserting exactly one shutdown INFO with `flush_completed=false`, the
    read-error arm additionally pinning the WARN count at 1.
  - `cmd/state_commit_now_test.go:640` (read error → WARN + touch + untouched `sessions.json`) and
    `:561` `TestStateCommitNow_ShortCircuits_LogsInfoSkipEvent` (set marker → INFO). Both branches of
    the WARN/INFO split this task introduced are therefore observed; the read-error test runs at
    `PORTAL_LOG_LEVEL=warn`, so an inverted split would drop the record and fail it.
  - Sweep stand-down: `internal/hooksweep/sweep_test.go:280,300` and `sweep_move_test.go:57,62` cover
    both `ReasonMarkerReadFailed` and `ReasonRestoring`, with the `cmd` renderers pinned in
    `cmd/doctor_stand_down_copy_test.go:60,71`.
- Notes:
  - AC3 is satisfied structurally: the commit changed no existing test — it only added
    `TestDaemonTick_SkipsEntireTickAndWarnsOnRestoringReadError` and `TestRestoreWindowActive`.
  - Not over-tested. The mild redundancy in `TestRestoreWindowActive`'s set/absent subtests (they
    re-exercise `IsRestoringSet`, already tabled at `markers_test.go:168`) is the task's own requested
    shape and documents the intended composition; two extra subtests is not bloat.

CODE QUALITY:
- Project conventions: Followed. `internal/state` stays a leaf (no new imports); the daemon's WARN
  messages and the `daemon` component binding are unchanged, so the closed log vocabulary is untouched;
  the new unit tests sit in the fast lane and touch no tmux server, binary or daemon.
- SOLID principles: Good. The rule now has one owner; reporting policy stays with the caller that has the
  context to choose it — which is precisely the separation the task set out to make.
- Complexity: Low. The predicate is one expression; each call site lost a branch.
- Modern idioms: Yes. `RestoreWindowActive(IsRestoringSet(c))` is the idiomatic Go multi-value-into-call
  composition, and it also accepts the `commit-now` seam's `func() (bool, error)` unchanged — which is
  what let the fourth site be re-pointed without touching its DI shape.
- Readability: Good. `cmd/state_daemon.go:175-181` reads as `warn-if-err` then `return-if-active` with no
  early return in the error arm; that is only correct because the fold guarantees `err != nil ⇒ active`,
  and the doc comment at `internal/state/markers.go:102-107` states exactly that, so the invariant a
  reader needs is one jump away and named.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
