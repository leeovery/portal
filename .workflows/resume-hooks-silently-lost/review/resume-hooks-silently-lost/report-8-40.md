TASK: resume-hooks-silently-lost-8-40 (tick-247253) — "snapshotLockBound's Justification Lost Its Concrete Why And Gained A Vacuous Clause" (severity: comments; comment-only, no code change)

ACCEPTANCE CRITERIA:
- No clause in the comment is vacuous or false at any `lockTimeout` the code admits.
- The concrete figure justification is present.
- The derivation and the floor are unchanged.

STATUS: issues_found

SPEC CONTEXT: The sidecar lock and the clean's advisory pre-read are change **D** of the work unit ("the unlocked cross-process read-modify-write on `hooks.json` is closed", specification §1.2 / §6). Per the shared verifier context, this is a phase-8 implementation-analysis task, so its authority is its own body rather than the specification — it prescribes three edits to one doc comment and forbids any code change. CLAUDE.md states the same contract the comment describes: the pre-read is "bounded by `snapshotLockBound()` — a hundredth of `lockTimeout`, floored at one poll interval, so the two move together and a clean never spends two full `lockTimeout`s behind a wedged writer".

IMPLEMENTATION:
- Status: Partial
- Location: `internal/hooks/lock.go:25-40` (the `snapshotLockFraction` const comment and the `snapshotLockBound` doc comment); sole production consumer `internal/hooks/store.go:53-55` (`loadSnapshot`); test accessor `internal/hooks/locktest.go:18-25`.
- What is met:
  - The vacuous "cheapest figure that still grants an uncontended lock" clause is gone; nothing in the present comment restates it.
  - The false three-sample "bounds the clean pre-read below the mutation bound" claim is gone (Do 3, "drop it" arm taken).
  - Every remaining clause checks out against the code: `snapshotLockBound` really does bound the clean's pre-read alone (one caller, `store.go:54`); the degradation it may take really is a fall-through to an unlocked read costing one DEBUG record (`store.go:70-79`, the `load-unlocked` emission at `store.go:73`); the derivation really does track `lockTimeout` until the floor takes over (`lock.go:38-40`); the floor sentence is accurate — `acquireLock` tests its deadline only after a poll sleep (`lock.go:59-72`), so below one poll interval the named figure is not the figure waited.
  - "which is 20ms at the 2s bound above" is arithmetically correct: `lockTimeout` = 2s (`lock.go:21`), fraction 100 (`lock.go:27`), floor 5ms (`lock.go:23`) → `max(20ms, 5ms)` = 20ms.
  - Do 4 is honoured: the derivation (`max(lockTimeout/snapshotLockFraction, lockPollInterval)`), the floor, and `SnapshotLockBoundForTest` (`locktest.go:22-25`) are all intact, and no code changed.
- What is not met: two of the three prescribed edits did not land in the comment — the loop-ordering argument (Do 1 / the task's stated Outcome) and the concrete figure justification (Do 2 / acceptance criterion 2). See FINDINGS.
- Notes: I considered whether the surviving clause "so a clean held up by a stuck writer costs the daemon's tick one mutation bound rather than two" reintroduces the falsity acceptance criterion 1 guards against — it does degrade below a ~10ms mutation bound, where the 5ms floor makes the pre-read half or more of the mutation bound. I am not reporting it: no bound the product uses is anywhere near that range (production is 2s; the sub-10ms figures exist only inside `SetLockTimeoutForTest` probes), the comment already warns the relation stops scaling "until the floor below takes over", and the same qualitative sentence is stated identically at `store.go:49-52`, `read_lock_test.go:381-385` and in CLAUDE.md — so a "fix" here would either be inconsistent or spread across four sites for no reader benefit.

TESTS:
- Status: Adequate
- Coverage: A comment-only task needs no new test, and none was added — correct. The behaviour the comment describes is pinned by `internal/hooks/read_lock_test.go:398-481` (`TestSnapshotLockBoundDerivation`): the poll-interval floor (`:399-410`), the uncontended-acquire-at-any-bound property including 0 and negative bounds (`:412-429`), the half-relation at the production and lowered bounds plus the integer-division crossover (`:431-439`), the derivation tracking `lockTimeout` (`:441-451`), and the real degrade-to-unlocked pre-read under a held sidecar (`:453-480`). `SnapshotLockBoundForTest` pins are unchanged and still present (`read_lock_test.go:375-393`, `:400`, `:406`, `:443`, `:446`, `:455`). The surrounding mutation-lock suite (`internal/hooks/lock_test.go`) is untouched by a comment edit and still covers sidecar creation/inode identity, exclusion, release on every arm, and the bound.
- Notes: Not under-tested and not over-tested. Worth recording that the very argument Do 1 asked for in the comment is already encoded as an executable property — `read_lock_test.go:412-429`, whose failure message at `:426` reads "the lock is granted before any deadline is tested, so no figure of the bound is what grants it" — so the fact is guarded even though the comment does not carry it.

CODE QUALITY:
- Project conventions: Followed. No code changed; no new logging, no new component/attr, nothing that touches the lane rules or the isolation invariants.
- SOLID principles: N/A for a comment task; the surrounding shape is unchanged (one derivation, one caller, one test accessor).
- Complexity: Low.
- Modern idioms: Yes (`max` builtin at `lock.go:39` is retained).
- Readability: Good — the comment is accurate and readable as it stands; the gap is what it omits, not what it says.
- Issues: The comment answers "what the bound is" and "why it must not be lower than one poll interval", but not "why a hundredth" and not "why so short a bound is safe" — the two questions this task was raised to answer.

BLOCKING ISSUES:
- None. Both findings have a comment-text remedy, which per the review rules is never blocking, and no behaviour, contract or test is wrong.

FINDINGS:
- [in-scope] [contained] internal/hooks/lock.go:25-27 — the `snapshotLockFraction` comment names the derived value ("the hundredth of lockTimeout the clean's advisory pre-read waits, which is 20ms at the 2s bound above") but gives no justification for the fraction itself; add the sentence the task specified — 20ms is four poll intervals (`lockPollInterval` = 5ms, `lock.go:23`) above the sub-millisecond critical section, which is why a hundredth and not a thousandth (a thousandth is 2ms at the 2s bound, below the 5ms floor at `lock.go:39`, so it would resolve to the floor and the fraction would stop meaning anything). — FAILS: acceptance criterion "The concrete figure justification is present" is unmet in substance — the comment records the arithmetic result but not the reasoning, so the next engineer changing `snapshotLockFraction` has no record of what the figure was chosen against, which is precisely the knowledge this task existed to restore.
- [in-scope] [contained] internal/hooks/lock.go:29-37 — the `snapshotLockBound` comment carries no argument for why a bound this short is safe; add the loop-ordering argument the task specified — `acquireLock`'s first `Flock` attempt (`lock.go:60`) precedes any deadline test (`lock.go:68`), so an uncontended acquire cannot time out at any bound, which is why a short pre-read bound introduces no spurious-degradation surface at all. — FAILS: the task's stated Outcome ("`snapshotLockBound`'s comment carries the argument that actually makes the short bound safe") is unmet; the comment's nearest sentence is the floor argument, which answers a different question (why the bound must not fall *below* one poll interval), and the safety argument itself survives only as a test failure message at `internal/hooks/read_lock_test.go:426` — invisible to a reader of `lock.go` weighing a change to the bound.
