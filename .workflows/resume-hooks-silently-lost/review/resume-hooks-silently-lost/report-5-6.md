TASK: resume-hooks-silently-lost-5-6 — One Home For The Lock Fixture And The Degraded-Read Assertion (tick-adcd7f)

ACCEPTANCE CRITERIA:
- One sidecar hold, one shared hold, one sidecar create and one degraded-read assertion exist, in one package, consumed by both `internal/hooks` and `cmd`
- The three inline `os.WriteFile(path+".lock", …)` copies are gone
- No assertion is dropped: the degraded-read helper still checks exactly-one-record, DEBUG level, `op`, `via` and a non-empty `error`
- No test changes its verdict
- `go test ./...`, `go test -race ./internal/hooks/` and `go test -tags integration -p 1 ./...` all pass

STATUS: complete

SPEC CONTEXT:
The specification's §6.2/§6.3/§6.5 introduce the `hooks.json.lock` sidecar, the shared-for-reads / exclusive-for-mutations flock split, the bounded acquire, and the degrade-by-side contract: a mutation that cannot take the lock exits non-zero and writes nothing, while a read proceeds unlocked and emits one DEBUG breadcrumb — `op=load-unlocked`, the lock error in `error`, and `via` naming the caller (`hydrate` / `doctor` / `cli` / `internal`) (specification.md:347, :387, :505, :556). Phase 5 built that behaviour and, as the task records, wrote its test scaffolding for it twice — once in `internal/hooks`, once in `cmd`. This task is a scaffolding consolidation, not a behaviour change; its authority is its own body, and its subject is the fixture and the assertion that hold the spec's degraded-read contract in place.

IMPLEMENTATION:
- Status: Implemented (delivered in 8c8efc8a; later re-homed by phase-6/8 tasks, current tree verified)
- Location:
  - `internal/hookstest/hooks_lock.go:26` `CreateHooksSidecar`, `:39` `HoldHooksSidecar`, `:60` `HoldHooksSidecarShared`, `:98` `UnlockedRecords`, `:135` `AssertDegradedRead` — the single home
  - `internal/hookstest/doc.go:1-6` — the package sentence names the sidecar lock fixture and its breadcrumb assertions
  - Consumers in `internal/hooks`: `read_lock_test.go:31,131,147,229,299,320,457,479`, `lock_write_test.go:21,38,56,72,91,110,126,168`, `lock_test.go:169,280`, `lookup_test.go:155,162`
  - Consumers in `cmd`: `hooks_read_lock_test.go:30,48,81,85,91,119,133,143,164`, `hooks_write_lock_test.go:70,85,103,119,172,229`, `hook_prune_locked_test.go:16`, `hook_prune_single_report_test.go:51`, `hooks_rm_exit_test.go:88`, `doctor_stand_down_copy_test.go:224`
  - Further consumers the later re-home picked up: `internal/hooksweep/lock_timeout_test.go:25`, `sweep_move_test.go:89`; `internal/hookstest/staging.go:88` (StageStore stages the sidecar through the same helper)
- Notes:
  - The delivered commit took the task's primary route (`internal/transienttest`, doc sentence widened). A later phase-6 task (36265b17, `6-18`) moved the fixture to the alternative the task named, `internal/hookstest`, and restored `internal/transienttest/doc.go:1-3` to its narrow `list-panes -a` sentence — so exactly one of the two homes the task offered holds it, and no residue is left in the other. Both routes were sanctioned by the task body.
  - Criterion 2 verified by search: no `os.WriteFile(…".lock"…)` remains anywhere in the tree (the only surviving matches of that call shape seed unrelated blocker files, e.g. `internal/fileutil/atomic_test.go:171`). The only `unix.Flock` calls outside production (`internal/hooks/lock.go:60`, `internal/state/daemon_lock.go:16`) are the five in `internal/hookstest/hooks_lock.go:42,48,63,67,80` — one hold fixture, not two.
  - Doc claims on the moved helpers hold against the code they describe: `CreateHooksSidecar`'s "a read … takes its shared lock rather than degrading on a missing sidecar" is true because `Store.acquireSharedLock` passes no `O_CREATE` (`internal/hooks/lock.go:91-93`), and `HoldHooksSidecarShared`'s "an exclusive acquire blocks against it to the bound; a shared one is granted immediately" matches `acquireLock`'s `LOCK_NB` poll loop (`internal/hooks/lock.go:52-74`).

TESTS:
- Status: Adequate
- Coverage: The consolidation is pure movement — the commit diff is substitution-only across all eight touched test files, with no assertion, bound or flock mode altered. The moved `AssertDegradedRead` (`internal/hookstest/hooks_lock.go:135-150`) makes the same five checks both originals made: exactly one `load-unlocked` record (via `logtest.Records.Only`, which `t.Fatalf`s on any count but one — `internal/logtest/capture.go:157-163`, matching both originals' `t.Fatalf`), DEBUG level, `op=load-unlocked`, `via` equal to the caller, and a non-empty `error` attr. `UnlockedRecords` reproduces the originals' message filter exactly (`sink.Records().WithMessage("load-unlocked")`).
- Notes:
  - The task correctly earned no new test for the movement itself, and the helper is nonetheless exercised on both sides of its discrimination: positively at nine call sites and negatively (`len == 0`) at `internal/hooks/read_lock_test.go:47,122,215,339,425` and `cmd/hooks_read_lock_test.go:81`. `internal/hookstest/staging_test.go:34,54` drives both arms in one suite — a staged sidecar must produce no record, an absent one must produce the degraded line.
  - Not over-tested: no case was duplicated into the shared package, and the fixture package's own suite (`internal/hookstest/hooks_lock_test.go`) covers only `AssertLockWarn`, a later task's helper.
  - Test execution was not attempted (verification is by reading, per the review contract); the acceptance criterion naming three `go test` invocations is therefore unverified by me rather than failed.

CODE QUALITY:
- Project conventions: Followed. `internal/hookstest` is a test-only package taking `*testing.T` first on every fatal-on-failure helper, matching the tree's structural test-only enforcement; it stays out of `_test.go` so both packages can import it, and no production file imports it. The unit-lane rule is unaffected — the package builds nothing and spawns nothing.
- SOLID principles: Good. The fixture (hold/create) and the assertion (breadcrumb shape) stay separate helpers; `openSidecar` is the one private open the three flock helpers share.
- Complexity: Low.
- Modern idioms: Yes. The `sync.Once`-guarded idempotent release with a `t.Cleanup` registration is preserved verbatim from the originals, so a caller may release mid-test and still rely on cleanup.
- Readability: Good. Each exported helper's comment states what the fixture models and why a test would want it, rather than restating the call.
- Issues: None found.

BLOCKING ISSUES:
- None

FINDINGS:
- None
