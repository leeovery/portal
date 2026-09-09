TASK: resume-hooks-silently-lost-6-21 — Give The Deny-Writes Fixture A Guard That Detects Its Own Trap (tick-794873)

ACCEPTANCE CRITERIA:
- The write-phase error class is asserted at this level.
- Removing the sidecar staging from `writesDenied` makes the test fail.
- The lock-timeout-vs-failure discrimination assertions are unchanged.

STATUS: complete

SPEC CONTEXT: The specification's §6.5 (referenced at specification.md:260, 312, 382, 505) governs the sidecar-lock degradation contract: a lock timeout must be discriminable from a genuine save failure — `hook set`/`hook rm` exit non-zero leaving `hooks.json` byte-identical, the sweep deletes nothing and WARNs, and reads degrade to unlocked with `op=load-unlocked` at DEBUG. This task is not spec-derived behaviour work; it is a phase-6 implementation-analysis task whose authority is its own body: the fixture that produces the *save-failure* half of that discrimination had no assertion tying it to the failure it exists to produce, so a staging change could silently convert it into a lock-open failure and the test would still pass.

IMPLEMENTATION:
- Status: Implemented
- Location: commit 7b1eaa46 (`cmd/hook_sweep_lock_timeout_test.go`, +7 lines). The file was later moved wholesale to `internal/hooksweep/lock_timeout_test.go` by task 9-12 (commit a4898f41); the assertion survives the move at internal/hooksweep/lock_timeout_test.go:121-126.
- Notes: The Do list's option 1 was taken in its `errors.Is` form rather than the sink `error_class` form — both were sanctioned by the task, and they are equivalent (`fileutil.ClassifyWriteError` at internal/fileutil/atomic.go:26-38 derives the `error_class` token from the same sentinel).

  The trap is genuinely armed. Traced end to end: `hookstest.StageStore` creates the sidecar before applying the 0500 denial (internal/hookstest/staging.go:85-95, with the ordering stated in the comment at :86-87), so `deleteStale`'s `acquireMutationLock` succeeds (internal/hooks/lock.go:81-84 — the sidecar already exists, so the `O_CREATE` open needs no directory write), `load` succeeds, and `save` → `fileutil.AtomicWrite` fails at `os.CreateTemp` in the read-only directory, wrapped `%w` with `ErrWriteTempCreate` (internal/fileutil/atomic.go:61-63). That wrap is preserved through `deleteStale` (internal/hooks/store.go:343-346) and returned unclassified by `declinedSweep` (internal/hooksweep/sweep.go:174-194), so `errors.Is(err, fileutil.ErrWriteTempCreate)` holds at the `Run` boundary the test asserts on.

  The control criterion also holds against the *current* consolidated staging: adding `SidecarAbsent: true` to the `writesDenied` fixture makes `acquireMutationLock`'s `O_RDWR|O_CREATE` open fail EACCES in the 0500 directory, producing "open hooks lock: …" — which is neither `hooks.ErrLockHeld` nor `fileutil.ErrWriteTempCreate`, so the new assertion at :124 fires and the test fails. The fixture reaches the save at all because `StaleHookSeed` holds `ReapableSeedA` beside `LiveSeedA` (internal/hookstest/hooks.go:206-209) and the stub lister reports only `LiveSeedA`, so the delete set is non-empty and `save` is reached.

TESTS:
- Status: Adequate
- Coverage: The task's subject *is* a test, and the added line is the coverage. The first subtest now pins three separable properties of one cycle: an error is returned (:115-117), it is not the lock sentinel (:118-120), and it carries the write-phase class the fixture exists to produce (:124-126). The second subtest's text-vs-sentinel guard and its own self-guard (:151-153) are untouched.
- Notes: Not over-tested — one assertion, no new fixture, no new setup. Not redundant with `internal/fileutil/atomic_classify_test.go`, whose subject is the sentinel wrapping inside `AtomicWrite`; this one's subject is that the sweep-level fixture still reaches that phase. The `errors.Is`/`t.Errorf` shape matches the neighbouring assertions (non-fatal, so all three properties report in one run).

CODE QUALITY:
- Project conventions: Followed. Unit-lane test, no tmux, no daemon, no binary build — correctly untagged. No `t.Parallel()`. Logging captured through `logtest.Install` per the `logtest` convention; no hand-rolled handler.
- SOLID principles: N/A (a single assertion in an existing test).
- Complexity: Low.
- Modern idioms: Yes — `errors.Is` against an exported sentinel rather than a string match, which is exactly the failure mode the sibling subtest exists to police.
- Readability: Good. The comment at :121-123 states why the class is the fixture's subject and what a broken staging would produce instead.
- Comment accuracy: Verified true. "the mutation took its lock and read cleanly, then failed at the temp create" matches the traced path (lock.go:81-84 → store.go:327 → store.go:343 → atomic.go:61); "A staging change that failed at the lock open instead would carry a different class" matches the EACCES path above. No process artifacts (no task ids, phases or spec section numbers).
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
