TASK: resume-hooks-silently-lost-5-2 — "A Read Shares The Lock, And Reads Anyway When It Cannot"

ACCEPTANCE CRITERIA:
- `Load`, `List`, `Get` and `LookupOnResume` take a shared `flock` on the sidecar and release it before returning; none hands the fd to its caller
- Two concurrent reads both complete without either blocking to the bound
- A read while the sidecar is held exclusively elsewhere returns its data anyway, after its bound, rather than failing
- The sweep's advisory pre-read waits at the short bound, not `lockTimeout`, so a contended sweep cycle spends one full bound in total rather than two; every other read waits at `lockTimeout`
- The short bound is selected by an explicit parameter, never from the `via` value
- A degraded read emits exactly one DEBUG record per read under the `hooks` component with `op=load-unlocked`, the lock error in `error` and `via` naming the caller
- `via` is `hydrate` / `doctor` / `cli` / `internal` for the four callers
- `op=load-unlocked` is the only new `op`; no new attr key, no new `via`, no new component binding
- A read creates nothing (directory, `hooks.json`, sidecar all still absent afterwards)
- `portal doctor` (read-only) and `hook list` leave the config directory as they found it on a fresh install
- A read with the sidecar absent degrades and still returns the file's contents
- The non-locking `load()` inside a mutation's hold emits no `load-unlocked` record
- `LookupOnResume` returns the registered command under a concurrently held exclusive lock
- `checkStaleHooks`'s status and `portal doctor`'s exit code are unchanged by a degraded read
- `hook list` still takes no tmux read with zero entries and still exits 0 with empty fourth fields on a failed enumeration
- Both lanes pass

STATUS: complete

SPEC CONTEXT:
§6.3 (readers take `LOCK_SH`, writers `LOCK_EX`; a lock is acquired once per operation and never nested; a read's shared lock is released when the read returns so the clean's advisory pre-read is not still held when the deletion takes the exclusive one — otherwise a sweep waits on itself and stalls the daemon's 1s tick), §6.5 (a read that cannot take the lock reads anyway, unlocked, at DEBUG: `op=load-unlocked`, the lock error in `error`, `via` naming the caller; failing a read would forfeit a hook for nothing, with `LookupOnResume` under the 40-helper restore burst as the motivating case), §9.2 (a lock timeout degrades by side — writes fail, reads return their data). Two corrigenda govern later movement: 2026-09-01 replaced the single figure with two bounds, the pre-read derived as a hundredth of `lockTimeout` floored at one poll interval; 2026-09-01 (second) records that `CleanStale` now takes the enumeration as a callback and performs its own pre-read, so the call site hands over no snapshot.

IMPLEMENTATION:
- Status: Implemented (drifted from the task's literal wording by later, sound phase 6–10 work)
- Location:
  - `internal/hooks/lock.go:91-93` — `acquireSharedLock(bound)`: `os.O_RDONLY` + `unix.LOCK_SH` at the caller's bound, with the omitted `O_CREAT` explained at `:86-90`.
  - `internal/hooks/store.go:70-79` — `loadSharedBounded(via, bound)`: acquire, `defer f.Close()`, `load()`; on any acquisition error exactly one `logger.Debug("load-unlocked", "op", "load-unlocked", "via", via.String(), "error", err)` and fall through to the non-locking `load()`.
  - `internal/hooks/store.go:58-60` (`loadShared` → `lockTimeout`), `:53-55` (`loadSnapshot` → `snapshotLockBound()`), `:45-47` (`Load`), `:212-213` (`List`), `internal/hooks/lookup.go:14-21` (`LookupOnResume`, empty-key early return ahead of the acquire).
  - Call sites: `cmd/doctor.go:370` (`ViaDoctor`), `cmd/hooks.go:141` (`ViaCLI`), `cmd/state_hydrate.go:179` (`ViaHydrate`), `internal/hooks/store.go:54` (`ViaInternal`, the clean's pre-read).
- Notes:
  - Three divergences from the task text, all later and all defensible. (a) `Store.Get` no longer exists — it had no production caller and was removed by a later consolidation; nothing gained a second unlocked read path in its place (`load()` is unexported and its only callers are `loadSharedBounded`, `Set`, `Remove` and `deleteStale`, the last three from inside their own exclusive hold). (b) The exported `LoadSnapshot(via)` is now the unexported `loadSnapshot()` because `CleanStale` absorbed the pre-read (Corrigendum 2026-09-01) — the ordering property is strengthened, not lost, since a caller can no longer get snapshot-before-enumeration wrong. (c) `via` is the closed `hooks.Via` type rather than a string, and `LookupOnResume` therefore takes it as a parameter rather than naming `hydrate` in-package; the four values still render `cli`/`internal`/`hydrate`/`doctor` (`internal/hooks/via.go:22-27`) and the invented-literal risk the task's string parameter carried is now a compile error.
  - The bound-never-from-`via` property holds by enumeration: `loadSharedBounded` has exactly two callers, `loadShared` (line 59, `lockTimeout`) and `loadSnapshot` (line 54, `snapshotLockBound()`), and no branch in `internal/hooks` tests `via` for anything.
  - `checkStaleHooks` keeps the order the criteria require (`cmd/doctor.go:365-381`): nil-store guard, `store.Load`, `StalenessStandDown`, then `JudgeAgainstLivePanes`.
  - The prior review round's single issue — the doctor baseline being itself a degraded read, so the test compared degraded against degraded — is resolved: `hookstest.StageStore` stages the sidecar by default (`internal/hookstest/staging.go:85-89`), so the baseline at `cmd/hooks_read_lock_test.go:21-22` is a genuinely locked read and the comparison discriminates.

TESTS:
- Status: Adequate
- Coverage: `internal/hooks/read_lock_test.go` covers the shared acquire against a third-fd `LOCK_SH` holder (`:28`), release-before-return proven by a following exclusive acquire (`:52`), two overlapping reads inside a lowered bound (`:82`), read-anyway under a held `LOCK_EX` with the bound waited out (`:127`), one DEBUG for a 42-entry file (`:150`), absent-sidecar degradation with the sidecar still absent afterwards (`:166`), creates-nothing under a non-existent directory (`:187`), no record from inside a mutation (`:207`), bound-from-parameter with `Load(ViaInternal)` still waiting the full bound (`:222`), the per-read `via` table including the clean's pre-read (`:268`), and `LookupOnResume` under a held lock plus the empty-key no-acquire path (`:304`). `cmd/hooks_read_lock_test.go` covers the doctor status/detail/exit-code parity (`:17`), the untouched config directory across a read-only diagnosis (`:51`), `hook list` output parity under a degraded read and no tmux read on a fresh install (`:69`, `:94`), the hydrate exec still reached under a held lock (`:113`), and the pre-read's short bound with `via=internal` (`:137`).
- Notes: `hookstest.AssertDegradedRead` (`internal/hookstest/hooks_lock.go:135-150`) is the shared pin — `Only` enforces exactly one record, plus DEBUG, `op`, `via` and a non-empty `error`. Discrimination is real rather than nominal: `HoldHooksSidecarShared` is the discriminator that would fail if the read acquired exclusively, and the release test would fail if the fd were handed back. Timing assertions all sit well inside their margins (the tightest is a 10ms derived bound asserted under 500ms), so they should not manufacture flakes. Not over-tested: the bound tests and the `via` tests overlap in fixture but each names a different failure.

CODE QUALITY:
- Project conventions: Followed. One `log.For("hooks")` binding per package, op-as-message with a matching `op` attr, the closed attr vocabulary unchanged (`op=load-unlocked` is the only addition, `error`/`via` are existing keys). Both new suites are unit-lane and touch no real tmux, daemon or built binary; `cmd` fixtures inject `HooksDeps` and the doctor/hydrate deps rather than Executing production wiring.
- SOLID principles: Good — one bounded acquire, one degradation policy, one shared read helper; the bound is a parameter and the caller identity is a separate closed type, so neither can be derived from the other.
- Complexity: Low.
- Modern idioms: Yes (`for i := range n`, `wg.Go`, `max` for the derived floor).
- Readability: Good. The comments state the conclusions that cost something to rediscover — why no `O_CREAT`, why the shared hold is released before return, why correctness never depends on the lock.
- Issues: None rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
