TASK: resume-hooks-silently-lost-9-49 (tick-7ae640) — "The Read-Lock Bound Pin Asserts A Claim Its Own Probe Falsifies, And Its Half-Relation Is Integer-Division-Fragile"

ACCEPTANCE CRITERIA:
- [x] The floor assertion's message names the floor.
- [x] The half-relation holds at a `lockTimeout` of exactly 10ms, which the current form fails.
- [x] The three existing sampled values still pass, and the crossover value is among the sampled set.
- [x] No production bound value changes.

STATUS: complete

SPEC CONTEXT:
The specification's Corrigendum 2026-09-01 (specification.md:554-556) is the governing text for the value under test:
the clean's advisory pre-read is bounded at a hundredth of `lockTimeout` **floored at one poll interval**, and the
floor is called out as load-bearing rather than a rounding detail ("the acquire re-tests its deadline only after a
poll sleep, so every bound shorter than one interval costs that same one interval"). The second bound exists so a
contended clean costs the daemon's 1s tick one mutation bound rather than two. The subject of this task is the *test*
that pins that derivation (`TestSnapshotLockBoundDerivation`), not the derivation itself — a phase-9
implementation-analysis task whose authority is its own body.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/hooks/read_lock_test.go:360-441 (commit 0b817c97, single file, +71/-7)
  - `halfRelationCrossoverBound = 10 * time.Millisecond` (:360-364)
  - `sampledMutationBounds()` → 2s / 300ms / 60ms (:366-370)
  - `pollIntervalFloor(t)` (:372-380) — drives `lockTimeout` to 1ns so the fraction divides away and the returned
    bound is the floor alone
  - `assertHalfRelation(t, mutation)` (:382-394) — `if 2*preRead > mutation { t.Errorf(...) }`
  - `TestSnapshotLockBoundDerivation` subtests at :399, :409, :431, :437
- Notes:
  - Criterion 1: the replaced message ("it must still grant an uncontended lock", falsified by the probe) is gone.
    Both surviving messages in the floor subtest name the floor — the guard at :401-403 ("the floor is %v — a bound
    at or below zero is waited out before it is ever tested") and the loop at :406-408 ("it must not fall below the
    %v floor, under which the deadline is re-tested only after a poll sleep and the figure named stops being the
    figure waited"), which restates the corrigendum's own justification. Both hold against `acquireLock`
    (internal/hooks/lock.go:51-73): the flock is attempted before any deadline test, and a bound ≤ 0 leaves the
    deadline already elapsed at its first test.
  - Criterion 2: at `lockTimeout = 10ms`, `snapshotLockBound()` = max(100µs, 5ms) = 5ms; the new form evaluates
    `2*5ms > 10ms` → false → passes, where the removed `preRead >= mutation/2` read 5ms >= 5ms and tripped. The
    relaxation is the correct one: the helper's doc says "no more than half", and equality is exactly the crossover.
  - Criterion 3: the three sampled values pass under the new form (2s→20ms, 300ms→5ms, 60ms→5ms; `2*preRead` is
    40ms/10ms/10ms, all ≤ the mutation bound). The crossover is covered twice — appended to the floor subtest's
    sampled set at :404 and driven by its own named subtest at :437-439, which is the split the task's own Tests
    section prescribes. `sampledMutationBounds()` returns a fresh slice per call, so the `append` cannot alias into
    the other subtest.
  - Criterion 4: `git show 0b817c97 --stat` is one test file. `lockTimeout = 2s`, `lockPollInterval = 5ms` and
    `snapshotLockFraction = 100` (internal/hooks/lock.go:20-27) are untouched.
  - The identifiers introduced are unique across the repo (no shadowing of an existing helper), and the file is
    `package hooks_test` with no `t.Parallel` anywhere in `internal/hooks` — required, since these subtests mutate
    the package-level `lockTimeout` through `SetLockTimeoutForTest`. The LIFO `t.Cleanup` chain in the floor subtest
    restores the original 2s.

TESTS:
- Status: Adequate
- Coverage: All four tests named in the task body exist and each pins a distinct property.
  - "it pins the pre-read bound at the poll-interval floor" (:399) — would fail if the `max(..., lockPollInterval)`
    floor were dropped from `snapshotLockBound`: `pollIntervalFloor` would return 0 and trip the guard, and the
    300ms/60ms/10ms samples would fall to 3ms/600µs/100µs, below the floor.
  - "it grants an uncontended acquire whatever the bound" (:409) — drives a real `store.Load(hooks.ViaCLI)` at 2s, 0
    and −1s over a staged sidecar (`Staging.SidecarAbsent` defaults off, so the sidecar exists — hookstest/staging.go:85-89)
    and asserts no `load-unlocked` record. `Load → loadShared → loadSharedBounded(via, lockTimeout)`
    (internal/hooks/store.go:45-79), so the bound under test is genuinely the one the acquire runs at; the test would
    fail if `acquireLock` ever tested its deadline before attempting the flock. This is the empirical probe from the
    task's Problem statement turned into a standing assertion rather than left as prose.
  - "it holds the half-relation at the production bound and the lowered test bounds" (:431) and "… at the
    integer-division crossover bound" (:437) — both route through the one `assertHalfRelation` helper, so the two
    subtests differ only in the bounds they sample.
- Notes: Not over-tested — the crossover appears in the floor loop and in its own subtest, but for two different
  properties (floor vs half-relation). The assertions are not tautological: a `snapshotLockFraction` of 1 or a
  `lockPollInterval` raised above half a sampled bound both fail the half-relation test.

CODE QUALITY:
- Project conventions: Followed. Unit lane, no build tag needed (no binary, no daemon, no tmux); no `t.Parallel`;
  fixtures staged through `hookstest.StageStore` and records read through `hookstest.UnlockedRecords` over a
  `logtest.Install` sink, which is the repo's single route for both.
- SOLID principles: Good — three small named helpers, each with one job; the half-relation is asserted in one place
  so the two subtests cannot drift.
- Complexity: Low.
- Modern idioms: Yes (`max` builtin in the production derivation is untouched; helpers take `*testing.T` and call
  `t.Helper()`).
- Readability: Good. The failure messages carry the reason, not just the numbers, and the helper doc explains why the
  pre-read is multiplied rather than the mutation bound halved.
- Issues: None rising to a finding. The one loose phrase is `halfRelationCrossoverBound`'s "the fraction is already
  exhausted there" (:361) — at 10ms the fraction yields 100µs rather than nothing, so "exhausted" carries a weaker
  sense here than in `pollIntervalFloor`'s comment; the operative claim beside it ("the pre-read stands at exactly
  half the mutation bound") is exact, so nothing is misled about the value or the assertion.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
