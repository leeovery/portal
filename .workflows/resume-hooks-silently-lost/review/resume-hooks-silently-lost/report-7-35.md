TASK: resume-hooks-silently-lost-7-35 — "A Second Lock Bound The Specification Never Decided" (severity low, sources analysis-standards-c2 S3)

ACCEPTANCE CRITERIA:
- [x] `snapshotLockTimeout` is expressed in terms of `lockTimeout`, not as an independent literal.
- [x] Its comment names what it protects and why its value is what it is, matching the `lockTimeout` comment's register.
- [x] A clean's worst case is one `lockTimeout`, not two.
- [x] Lowering `lockTimeout` in a test lowers the pre-read bound with it.
- [x] The pre-read still degrades to an unlocked read on timeout, emitting `op=load-unlocked` at DEBUG with `via=internal`, and never fails the clean.
- [x] The specification records the second bound and the more frequent degraded read beside the 2s figure.

STATUS: complete

SPEC CONTEXT:
§6.5 (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:387-395`) declares one acquisition bound — "The bound is **2 seconds**" — and rules that a read that cannot take the lock reads anyway, unlocked, at DEBUG (`op=load-unlocked`, `via` naming the caller; `internal` for the sweep's advisory pre-read). §6.3 (`:349-359`) establishes that a clean takes the sidecar twice — shared for `loadSnapshot`, exclusive for `deleteStale` — and that the shared hold is released before the exclusive one, so a sweep never waits on itself and never parks the daemon's 1s tick (`cmd/state_daemon.go` `TickerPeriod`). The Corrigendum of 2026-09-01 (`specification.md:554-556`) is the authoritative record for this task: it corrects §6.5 to "**there are two bounds, not one**", fixes the pre-read at a hundredth of the 2s figure floored at one poll interval, states that the derivation is written as a fraction "so the relationship is visible where the value is", names the floor as load-bearing (below a 500ms mutation bound the fraction is exhausted and the pre-read stays at one poll interval), and records the accepted price: ordinary contention routinely degrades the pre-read and emits `op=load-unlocked` at DEBUG with `via=internal`, which "is expected and is not a signal of contention worth investigating".

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/lock.go:25-27` — `const snapshotLockFraction = 100`, documented as the hundredth of `lockTimeout` the pre-read waits, "20ms at the 2s bound above" (2s/100 = 20ms, and `max(20ms, 5ms)` = 20ms — the arithmetic holds).
  - `internal/hooks/lock.go:29-40` — `snapshotLockBound() = max(lockTimeout/snapshotLockFraction, lockPollInterval)`. Derived, not declared: the independent `snapshotLockTimeout = 20 * time.Millisecond` var is gone (`git show be326480 -- internal/hooks/lock.go`), and no reference to the old identifier survives anywhere in the repo's Go or Markdown outside the workflow archive (grepped `snapshotLockTimeout|SetSnapshotLockTimeoutForTest` across `*.go`/`*.md`: only historical analysis/review documents match).
  - `internal/hooks/lock.go:23` — `lockPollInterval` moved above the derivation so the floor it supplies is declared before use.
  - `internal/hooks/store.go:49-54` — `loadSnapshot` reads at `snapshotLockBound()` via `loadSharedBounded(ViaInternal, …)`; `loadShared` (`:57-59`) still reads at the full `lockTimeout`, so only the clean's pre-read takes the short bound.
  - `internal/hooks/store.go:318-332` — `deleteStale` still acquires through `acquireMutationLock`, which passes `lockTimeout` (`internal/hooks/lock.go:81-84`). The two-bound split is therefore real: worst case is one full mutation bound plus a hundredth of it, not two.
  - `internal/hooks/locktest.go:18-25` — the mutable `SetSnapshotLockTimeoutForTest` seam is replaced by the read-only `SnapshotLockBoundForTest` accessor, with the "there is no setter" rationale stated. This is the substantive part of the fix: the previous suites set an absolute figure and then measured what they had set, so criterion 4 was unfalsifiable in exactly the two places that appeared to test it.
  - `CLAUDE.md`'s `hooks` architecture row was updated in the same commit to name `snapshotLockBound()` — "a hundredth of `lockTimeout`, floored at one poll interval, so the two move together" — matching the code.
- Notes: The comment at `lock.go:29-37` names what it protects ("costs the daemon's tick one mutation bound rather than two"), why it is derived rather than declared ("so lowering the mutation bound lowers this one with it until the floor below takes over"), that the pre-read is advisory ("may degrade to an unlocked read at no cost to correctness, paying one DEBUG breadcrumb"), and why the floor is load-bearing ("acquireLock re-tests its deadline only after a poll sleep") — the same register as the `lockTimeout` comment at `:17-20`. Verified against `acquireLock` (`lock.go:52-74`): the deadline is tested only after a failed non-blocking flock and before a `lockPollInterval` sleep, so a bound under one interval would indeed be waited out at one interval. Later tasks (8-40, 9-11, 9-13, 9-49) compressed the wording; the head state still satisfies every criterion.

TESTS:
- Status: Adequate
- Coverage:
  - `internal/hooks/read_lock_test.go:398-481` (`TestSnapshotLockBoundDerivation`) — the derivation suite. "it holds the half-relation at the production bound and the lowered test bounds" (`:431-435`) and "…at the integer-division crossover bound" (`:437-439`) drive `assertHalfRelation` (`:386-393`), which pins `2*preRead <= mutation` across 2s/300ms/60ms and the 10ms crossover — criterion 3. "it pins the pre-read bound at the poll-interval floor" (`:399-410`) pins the floor and that it is positive. "it lowers the pre-read bound with the mutation bound under test" (`:441-451`) is criterion 4, and it is now falsifiable: `SnapshotLockBoundForTest` reports a derived value rather than one the test set. "it degrades the pre-read to an unlocked read when the sidecar is held" (`:453-480`) is criterion 5 end-to-end — the snapshot still held both entries, the clean proceeded to the enumeration (aborted there deliberately so the elapsed time measures the pre-read alone), the wait was at the derived bound and not the mutation bound, and `hookstest.AssertDegradedRead(t, sink, "internal")` (`internal/hookstest/hooks_lock.go:135-150`) pins exactly one record, DEBUG, `op=load-unlocked`, `via=internal`, non-empty `error`.
  - `cmd/hooks_read_lock_test.go:137-166` (`TestSweepPreReadBound`) — the same property through the real sweep call path, with the bound read from the accessor rather than set (`:139-140`), and both sides of the split asserted (`elapsed >= short`, `elapsed < 1s` against a 5s mutation bound).
  - `internal/hooksweep/lock_timeout_test.go:56-72` ("it costs one bound for a fully contended cycle") covers the plan's "it still takes the exclusive lock at the full bound for the deletion" and "it caps a clean's worst case at one lock timeout" in one measurement: at a 300ms bound the fully contended cycle must take at least one full bound (the mutation waits) and well under two (the pre-read must not).
  - Non-regression: `TestReadSharedLockBoundSelection` (`internal/hooks/read_lock_test.go:221-265`) still pins that the bound comes from the parameter and not from `via` — the case that would otherwise pass trivially now that `via=internal` uniquely identifies the pre-read.
  - The `SetSnapshotLockTimeoutForTest` calls the seam removal orphaned were all deleted rather than replaced with a weaker assertion (`git show be326480 -- internal/hooks/read_lock_test.go`); the one behavioural case among them was rewritten as the derived-bound degradation test above, so no coverage was lost.
- Notes: All of the above are unit-lane and hermetic (temp dirs, `logtest.Install`, no tmux, no daemon, no built binary) — correct for the lane rule. The timing assertions are one-sided against generous margins (`< 500ms` against a 1s bound, `< 1s` against 5s, `< 1.5×` against 300ms), so they are not the kind of tight wall-clock pin that manufactures flakes on a loaded machine. Not over-tested: each of the six subtests fixes a different property (floor, uncontended grant, half-relation, crossover, proportional lowering, degradation-and-breadcrumb), and none re-asserts another's subject.

CODE QUALITY:
- Project conventions: Followed. The `*testing.T`-first accessor keeps `locktest.go` on the established precedent CLAUDE.md sanctions for `internal/log`'s test seams. The DEBUG breadcrumb stays inside the closed `hooks` component vocabulary (`op=load-unlocked`, `via=internal`) with no new op, via or attr key. `max` is the Go 1.21+ builtin (module is `go 1.26.0`), which is what `modernize` wants here.
- SOLID principles: Good. The bound has one home; `loadSharedBounded` takes the bound as a parameter and never derives it from `via`, which is what keeps the log attr and the policy separate.
- Complexity: Low — one `max` expression.
- Modern idioms: Yes.
- Readability: Good. The relationship between the two bounds is now visible at the declaration, which was the whole point of the task.
- Issues: None rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
