TASK: resume-hooks-silently-lost-5-1 — Every Mutation Takes The File Under One Exclusive Hold

ACCEPTANCE CRITERIA:
- `Set`, `Remove` and `CleanStale` each hold one exclusive `flock` on `<hooks.json path>.lock` across their whole load-mutate-save, acquired once and released on every return path
- The lock is never taken on `hooks.json` itself, and the sidecar's inode is unchanged across a mutation that replaces `hooks.json`'s inode
- The sidecar path is derived from the resolved store path, so a `PORTAL_HOOKS_FILE` override carries it wherever it points
- The lock file is opened `O_CREAT` by writers and is never unlinked by any code path
- `acquireLock` takes its bound as a parameter, and two package-level bounds exist: `lockTimeout` for mutations and ordinary reads, and a near-zero bound for the sweep's advisory pre-read
- The config directory is created before acquisition, so the first `hook set` on a fresh machine succeeds rather than timing out
- A mutation whose config directory cannot be created fails through `acquireLock` rather than through a bare `MkdirAll` return
- No exported method is reached from inside another: `Set`, `Remove` and `CleanStale` use only the unexported `load`/`save`, and an uncontended mutation completes far inside a lowered bound
- Interleaved writers across the `Load`→`AtomicWrite` window lose no entry
- A mutation blocked by a held sidecar performs its load after the release, not before
- `Remove`'s `(bool, error)` answer still comes from the map loaded inside this hold; its silence on a no-removal is unchanged
- Neither `Save` nor `SaveAudited` exists on the exported surface, and no production code writes `hooks.json` outside a held lock
- `error_class=write-failed-temp-create` is still exercised by the amended write-failure fixtures
- CLAUDE.md's `hooks` row and README's Configuration table both name the `hooks.json.lock` sidecar
- Both lanes pass; no `t.Parallel()`

STATUS: complete

SPEC CONTEXT: §6.1 diagrams the lost update — two processes each `Load()` → mutate → `AtomicWrite`, with nothing guarding the read-modify-write window, so a `hook set` landing inside the daemon sweep's window is silently overwritten by the sweep's older snapshot; the end state is indistinguishable from the drift symptom the work unit exists to remove. §6.2 requires the lock on a dedicated `<hooks.json path>.lock` (never the target, whose inode `AtomicWrite`'s rename replaces on every write), opened `O_CREAT`, never unlinked, with the config directory created *before* acquisition so a fresh machine's first `hook set` cannot fail permanently. §6.3 requires the exclusive hold to span the whole mutation and never to nest — hence unexported non-locking `load`/`save`. §6.5 fixes the 2s bound and its rationale (an unbounded acquire would park the daemon's 1s tick behind a held-but-alive lock). Two corrigenda are load-bearing here: 2026-09-01 sanctions **two** bounds with the pre-read written as a *fraction* of the mutation bound floored at one poll interval rather than as a value of its own, and 2026-09-01 (CleanStale) records that `CleanStale` takes the enumeration as a callback and performs its own `loadSnapshot` before invoking it.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/lock.go:15` (`ErrLockHeld`), `:21` (`lockTimeout = 2 * time.Second`), `:23` (`lockPollInterval`), `:27`/`:38` (`snapshotLockFraction` + `snapshotLockBound()`), `:45` (`lockPath` = `s.path + ".lock"`), `:52` (`acquireLock(path, openFlags, flockMode, bound)` — poll on `unix.Flock(...|LOCK_NB)`, wrapped `ErrLockHeld` on expiry, fd closed on every failure path), `:81` (`acquireMutationLock` — `MkdirAll` with its error deliberately discarded, then `O_RDWR|O_CREATE` + `LOCK_EX` at `lockTimeout`), `:91` (`acquireSharedLock`, no `O_CREAT`)
  - `internal/hooks/store.go:84` (`load`, non-locking), `:103` (`save`, non-locking), `:116`/`:125` (Set: acquire then `defer Close`), `:174`/`:181` (Remove), `:298`/`:321`/`:325` (CleanStale → `deleteStale` acquire + `defer Close`)
  - `internal/hooks/locktest.go:11` (`SetLockTimeoutForTest`, `*testing.T`-first with `t.Cleanup` restore, on the shape of `log.SetTestHandler` in `internal/log/testhandler.go:10`), `:22` (`SnapshotLockBoundForTest` getter)
  - Docs: `CLAUDE.md:72` (hooks row names the sidecar and the whole-mutation hold), `CLAUDE.md:194` (the absent-sidecar-in-the-wild passage), `README.md:391` (Configuration table row)
- Notes:
  - Every criterion holds against the code as it stands. `save` (`store.go:103`) is the only writer of `hooks.json` in production — verified by enumerating `fileutil.AtomicWrite` call sites across non-test sources (`internal/prefs/store.go:316`, `internal/state/scrollback.go:81`, `internal/state/commit.go:35`, `internal/state/daemon_state.go:27,80`, `internal/project/store.go:67`, `internal/hooks/store.go:109`) — and its three callers (`store.go:143,201,343`) all sit inside a held exclusive hold. `Save`/`SaveAudited` are gone from the exported surface (`store.go` now exports only `Load`, `Set`, `Remove`, `List`, `CleanStale`, plus `LookupOnResume` in `lookup.go`).
  - Nothing unlinks the sidecar: the only `os.Remove` anywhere under `internal/hooks` is in a test (`cleanstale_read_sentinel_test.go:16`, removing the hooks.json).
  - Two divergences from the task's literal wording, both sound and both on the record. (1) The pre-read bound is a derived `snapshotLockBound()` (`lock.go:38`) rather than a package-level `snapshotLockTimeout` var with a setter seam — exactly what the 2026-09-01 corrigendum prescribes, and the property the seam existed for (a deliberately contended pre-read) is still reachable by holding the sidecar, which `internal/hooks/read_lock_test.go:453` does. (2) `CleanStale`'s exclusive hold covers `deleteStale`'s load-mutate-save while its advisory snapshot read sits outside it — the shape the CleanStale corrigendum and §6.3/§6.4 require, since a tmux enumeration must never run under the lock.
  - Self-blocking is foreclosed structurally as well as by convention: `loadSharedBounded` (`store.go:70`) releases before returning, and `cleanstale_staleness_guard_test.go:56` fails any of `Set`/`Remove`/`deleteStale` that calls a locking front door on `s`.

TESTS:
- Status: Adequate
- Coverage: All thirteen named tests exist and assert the property they name.
  - sidecar creation / overridden path / sidecar-vs-target inode / never-unlinked / directory-created-first: `internal/hooks/lock_test.go:35,51,64,89,116`. The inode test (`:64`) is the one that matters against a broken lock — it asserts `hooks.json`'s inode *changed* on each mutation while the sidecar's did not, so a lock mistakenly taken on the target could not pass it.
  - lost-update: `lock_test.go:134` — 20 goroutines, each with its own `*Store` over one path (separate open file descriptions, a faithful model of separate processes), asserting all 20 keys present and the map length exact.
  - load-after-release: `lock_test.go:167` — holds the sidecar from an independent fd, launches `Set(k2)`, writes `k1` directly under the hold, releases, then asserts **both** keys survive. A hold spanning only the write would lose `k1`; this is the assertion that distinguishes the two designs.
  - release on every arm: `lock_test.go:200` (set-noop), `:217` (no-removal), `:238` (failed save), each followed by `hookstest.AssertSidecarFree` plus a subsequent successful mutation.
  - acquire-once: `lock_test.go:254` — 500ms bound, fails at half of it, so a nested acquire (which costs the full bound) is an order of magnitude away from the threshold.
  - uncreatable-directory routing: `lock_test.go:291` asserts the error came through the sidecar acquire and explicitly *not* through `fileutil.ErrWriteTempCreate`, with `hooks.json` absent; `lock_write_test.go:187` pins the same condition's WARN.
  - `write-failed-temp-create` still exercised: `store_test.go:1231` (Set), `:1369` (Remove), `:1069` (CleanStale batch), `:1010` (clean-stale summary) — all now routed through `hookstest.StageStore{WritesDenied:true}`, which pre-creates the sidecar *before* the `chmod 0500` (`internal/hookstest/staging.go:85-95`, `hooks_lock.go:26`), so the fixtures still fail at `os.CreateTemp` rather than earlier at the sidecar's `O_CREAT`. The hand-rolled read-only-parent fixture the task named inside `TestCleanStaleLogging` is gone, replaced by the same stager (`store_test.go:1012`).
  - `TestSave`'s `AtomicWrite` properties re-pointed onto `Set` (`store_test.go:97-172`), including a no-stray-temp-file assertion that now allows exactly `hooks.json` and `hooks.json.lock`.
- Notes: No `t.Parallel()` anywhere in `internal/hooks` or `internal/hookstest`. The two "uncreatable directory" subtests (`lock_test.go:291`, `lock_write_test.go:187`) overlap on the fixture but assert different subjects — error routing versus the emission — so this is not redundancy. Timing-sensitive assertions are stated with generous margins and carry their own justification in-source (`lock_test.go:255-260`).

CODE QUALITY:
- Project conventions: Followed. The bound seam takes `*testing.T` first and restores through `t.Cleanup`, matching `internal/log`'s established non-test-file seam precedent. `internal/hooks`' leaf status is preserved — `lock.go` adds only `golang.org/x/sys/unix` (already a module dependency, `go.mod:14`), which `leaf_guard_test.go` admits as a non-module dep. Test staging routes through `hookstest` rather than hand-rolled fixtures, as the architecture table requires.
- SOLID principles: Good. `acquireLock` is parameterised over flags/mode/bound and knows nothing about the store; `acquireMutationLock`/`acquireSharedLock` are the two policies over it, so the shared read path (5-2) and the pre-read bound land on one implementation.
- Complexity: Low. `acquireLock` is a single poll loop with three exits; each mutation gains two lines and a `defer`.
- Modern idioms: Yes — `max()` in `snapshotLockBound`, `errors.Is(err, unix.EWOULDBLOCK)` rather than a raw errno compare, `range writers` over an int in the concurrency test.
- Readability: Good. Every non-obvious choice carries its reason at the declaration: why the lock is on a sidecar (`lock.go:42-44`), why `MkdirAll`'s error is discarded (`:77-80`), why the shared acquire passes no `O_CREAT` (`:86-90`), why the pre-read bound is a fraction with a floor (`:29-37`), and why `load`/`save` are non-locking (`store.go:82-83`, `:102`). I checked each of those comments against the code it sits on; none makes a claim the code falsifies, and none references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
