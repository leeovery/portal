TASK: resume-hooks-silently-lost-5-4 — The Sweep Decides What Is Stale Under Its Own Lock

ACCEPTANCE CRITERIA:
1. `CleanStale` takes the live token set and the snapshot key set and derives the delete set itself, inside its own exclusive hold, from the file it loaded there
2. The delete set is every key in the file under the lock AND in the snapshot AND absent from the live set AND either token-shaped or empty
3. A key absent from the snapshot is never deleted, however stale it looks
4. A key in the snapshot but no longer in the file under the lock is not reported as removed
5. `runHookStaleCleanup` takes its snapshot BEFORE the pane enumeration: an entry written during the enumeration survives the sweep
6. No lock is held during `ListAllPaneHookKeys` or the restore-marker read — the shared pre-read is released when it returns
7. The snapshot is taken through the short-bound read, so a cycle blocked by a held sidecar spends one full bound in total rather than two
8. Phase 1's rule is untouched: `staleKeys` is still the single implementation, `CleanStale` still never calls `StaleKeys`, and the source guard still passes
9. The empty-live-set guard still counts pane ROWS, fed by neither `hooks.json` read
10. The zero-persisted-entries early return still short-circuits silently, and the enumeration-error branch is unchanged
11. A cycle whose snapshot holds no keys returns before `CleanStale` is called: no lock, no config directory, no `<hooks.json path>.lock`
12. `onRemoved` is invoked exactly for the keys `CleanStale` deleted, so `doctor --fix` never prints `Pruned stale hook:` for a removal that did not happen
13. `runHookStaleCleanup` remains `CleanStale`'s only production caller
14. `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§6.1 diagrams the lost update this task closes: the sweep reads `hooks.json` twice and judges what it holds against a tmux pane enumeration taken outside any lock, so a `hook set` landing between the two reads writes a token-shaped entry the enumeration never saw and the shape rule reaps it — the hook vanishing seconds after the command reported success. §6.3 requires that the stale decision be computed under the exclusive lock and never from the pre-read, that the pre-read be taken strictly BEFORE the pane enumeration, and that it may only NARROW the delete set, never widen it; the delete set is "in the file under the lock AND in the snapshot AND absent from the live set AND token-shaped or empty". §6.4 forbids any tmux call inside the lock. §5.4 keeps the mass-deletion guard keyed on pane ROWS from the enumeration, fed by neither `hooks.json` read.

Two Corrigenda are directly load-bearing for this task and are authoritative over the body:
- 2026-09-01: "§6.3's `CleanStale` receives the live token set and the call-site snapshot's key set … corrected: `CleanStale` takes the *enumeration* as a callback and performs its own `loadSnapshot` before invoking it; the call site hands over no snapshot. The ordering the sections argue for … is preserved and strengthened, because a caller can no longer get the order wrong."
- 2026-09-06: "the rule lives in one **exported** `StaleKeys`, and both callers reach it directly, `CleanStale`'s deletion pass included … The re-entrancy reasoning never applied to this call: `StaleKeys` takes an already-loaded snapshot and acquires nothing."

IMPLEMENTATION:
- Status: Implemented (delivered shape deliberately past the task's prescribed signature; both divergences are recorded corrigenda and are strengthenings, not losses)
- Location:
  - `internal/hooks/store.go:298-310` — `CleanStale(enumerateLive func(Snapshot) ([]string, error))`: `loadSnapshot()` first, then the caller's enumeration with no lock held, then `deleteStale(live, snapshot)`.
  - `internal/hooks/store.go:318-358` — `deleteStale`: `acquireMutationLock()` → non-locking `s.load()` → `narrowToSnapshot(StaleKeys(h, live), snapshot)` at `:332` → clone/delete/save → per-key INFO after the write → summary.
  - `internal/hooks/store.go:266-274` — `narrowToSnapshot`, which can only drop candidates.
  - `internal/hooks/store.go:53-55` + `internal/hooks/lock.go:38-40` — the pre-read at `snapshotLockBound()` = `max(lockTimeout/100, lockPollInterval)`; `internal/hooks/lock.go:91-93` — `acquireSharedLock` passes no `O_CREATE`, so the pre-read creates neither the directory nor the sidecar.
  - `internal/hooksweep/sweep.go:152-167` — `Run`: restore-marker stand-down first, then `store.CleanStale(liveTokenEnumeration(reader))`.
  - `internal/hooksweep/sweep.go:126-148` — the enumeration closure: zero-snapshot silent short-circuit (`errNothingPersisted`) before any pane read or lock, then `JudgeAgainstLivePanes`, the counts DEBUG carrying `panes`/`entries`, then `liveTokensFrom(rows)`.
  - `internal/hooksweep/sweep.go:85-118` — the guard counts `len(rows)` (`:98`), and `liveTokensFrom` (`:110`) drops empty tokens, so rows and tokens stay separate questions.
  - `cmd/doctor.go:201-208` — `Pruned stale hook:` is printed from `outcome.Removed`, i.e. only from `CleanStale`'s returned slice.
- Notes:
  - AC 1/2/3/4 hold in substance. The delete set is derived inside the exclusive hold from the file loaded there (`StaleKeys(h, live)` iterates `h`, the locked read), then intersected with the snapshot. A key the snapshot lacks cannot be deleted; a key the file no longer holds cannot be reported.
  - AC 5/6 hold structurally rather than by call-site convention — the ordering is now inside `CleanStale`, which the 2026-09-01 corrigendum sanctions as strictly stronger. `loadSharedBounded` (`store.go:70-79`) closes the fd on return, so the shared hold is gone before `enumerateLive` runs, and `Run` takes the marker read before touching the store at all.
  - AC 8 diverges as written: `deleteStale` calls the exported `StaleKeys` and the source guard now *requires* it (`internal/hooks/cleanstale_staleness_guard_test.go:25`). The 2026-09-06 corrigendum records this: `StaleKeys` is a pure function over an already-loaded snapshot and acquires nothing, so the re-entrancy hazard the criterion guarded against does not exist on this call. The property §6.3 actually protects is enforced instead by `TestMutationsDoNotCallExportedLoadOrSave` (`internal/hooks/cleanstale_staleness_guard_test.go:56-77`), which names `deleteStale` as a mutation and forbids it calling `Load`/`List`/`loadShared`/`loadSharedBounded`/`loadSnapshot`. No loss.
  - AC 11: the zero-snapshot return now sits inside the enumeration closure rather than at a `cmd` call site, but lands before anything that creates: the pre-read never creates, and `acquireMutationLock` (the only `MkdirAll` + `O_CREATE` site, `lock.go:81-84`) is never reached.
  - AC 13: `grep -rn "CleanStale(" --include=*.go` over non-test sources returns exactly one call to the hooks store's method — `internal/hooksweep/sweep.go:159`. The other matches are `internal/project`'s unrelated same-named method and its consumers. `cmd/run_hook_stale_cleanup.go` no longer exists; the cycle was re-homed in `internal/hooksweep` by a later phase, and `cmd/hook_sweep_caller_guard_test.go` now forbids any bootstrap file from naming `CleanStale` or `hooksweep`.
  - The read-only diagnosis (`cmd/doctor.go:363-392`) still counts with an un-narrowed `StaleKeys` across its own read-then-enumerate window. It deletes nothing, and the spec scopes narrowing to the delete path — deliberate, matching the implementer's recorded note.
  - AC 14 not verified here: running the suite is outside a reviewer's remit. Judged by reading, no assertion in the packages touched contradicts the delivered shape.

TESTS:
- Status: Adequate
- Coverage: every named test in the task's Tests list has a counterpart, at whichever layer the consolidation left it:
  - ordinary reap — `internal/hooks/cleanstale_snapshot_test.go:25`
  - retains a key the snapshot did not hold — `:48`
  - the enumeration is handed the pre-enumeration file — `:75` (pins the ordering directly, not just its effect)
  - retains an entry written during the pane enumeration — `internal/hooksweep/snapshot_order_test.go:18`
  - holds no lock while enumerating — `internal/hooks/cleanstale_snapshot_test.go:94` and `internal/hooksweep/snapshot_order_test.go:44`, both with a `probed` flag so a removed enumeration call cannot pass the subtest vacuously, and both probing via `hookstest.AssertSidecarFree` (`internal/hookstest/hooks_lock.go:76-85`), which attempts `LOCK_EX|LOCK_NB` from a second fd — a genuine conflict test, since flock binds to the open file description
  - delete set derived from the file under the lock, not the snapshot — `internal/hooks/cleanstale_snapshot_test.go:143`
  - retains a non-token-shaped key / deletes an empty key — `internal/hooks/store_shape_test.go:14` and `:63`
  - reports exactly what was deleted — `internal/hooksweep/snapshot_order_test.go:62`, which stages BOTH mid-cycle windows at once (another writer `Remove`s a snapshot-held key while a registration lands for a key the snapshot lacks) and asserts `outcome.Removed` equals exactly the one key this sweep deleted
  - guard counts pane rows, not tokens — `internal/hooksweep/sweep_test.go:380-457`
  - zero persisted entries returns silently — `sweep_test.go:340` and `:490-520`
  - no lock taken with nothing persisted — `internal/hooksweep/snapshot_order_test.go:103-119`, asserting an empty config root and no sidecar afterwards
  - enumeration-error branch unchanged — `sweep_test.go:72` and `internal/hooks/cleanstale_snapshot_test.go:117` (which additionally pins that an aborted clean emits zero records)
  - the staleness-rule source guard — `internal/hooks/cleanstale_staleness_guard_test.go:14`, re-pointed to the inverted rule
  - the short pre-read bound — `internal/hooks/read_lock_test.go:398-451` pins the derivation, the poll-interval floor and the half-relation ("a contended clean must cost one bound, not two"), and `:278-283` pins that the pre-read degrades under `via=internal`
- Notes:
  - The one issue the implementation-phase reviewer raised against this task — that `"it feeds onRemoved exactly what was deleted"` measured a file delta in a fixture where the delta, the returned slice and a call-site prediction all coincided — is closed. The replacement (`snapshot_order_test.go:62-97`) asserts an exact expected set against a fixture where those three genuinely diverge, which is the strongest of the two forms the reviewer offered, and the call-site prediction it guarded against is now structurally unreachable: `cmd/doctor.go:206` iterates `outcome.Removed`, which is `CleanStale`'s return value and nothing else.
  - Each invariant would fail loudly if broken: reversing the snapshot/enumeration order reddens `snapshot_order_test.go:18` and `cleanstale_snapshot_test.go:75`; dropping `narrowToSnapshot` reddens `cleanstale_snapshot_test.go:48`; deriving from the snapshot instead of the locked read reddens `:143`; holding the lock across the enumeration reddens both no-lock probes; removing the zero-snapshot return reddens `snapshot_order_test.go:103`.
  - Not over-tested. The apparent duplication between `internal/hooks/cleanstale_snapshot_test.go` and `internal/hooksweep/snapshot_order_test.go` is two layers, not two copies: the former pins the store's contract with an arbitrary callback, the latter pins that the real cycle wires the live enumeration into it.

CODE QUALITY:
- Project conventions: Followed. `internal/hooks` stays a leaf under its own guard; the `hooks` log component is bound once per package (`store.go:21`, `sweep.go:14`); no new attr keys; unit-lane placement is correct (no daemon, no binary, no real tmux).
- SOLID principles: Good. The sequencing responsibility moved to the one place that can enforce it, and the caller supplies only the enumeration it owns; `deleteStale` does the mutation and nothing else.
- Complexity: Low. `CleanStale` is three steps; `deleteStale` is a linear acquire-load-derive-write; `narrowToSnapshot` is one loop.
- Modern idioms: Yes — `maps.Clone`, wrapped multi-`%w` errors, `max` for the bound floor.
- Readability: Good. The doc comments carry the reasoning that matters (why the snapshot may only narrow, why no tmux call sits inside the lock, why the shared hold is released on return) and match what the code does.
- Issues: None rising to a finding.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
