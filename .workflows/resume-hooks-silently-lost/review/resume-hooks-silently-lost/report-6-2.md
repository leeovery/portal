TASK: resume-hooks-silently-lost-6-2 (tick-4f59f4) — Make The Hooks Store Own CleanStale's Ordering Invariant Instead Of Documenting It

ACCEPTANCE CRITERIA:
- The two same-typed `[]string` parameters no longer sit adjacent on `CleanStale`'s signature; a transposed call does not compile.
- No tmux read happens while the store holds either lock.
- The existing anti-reap behaviour is unchanged for every fixture in the current suite.

STATUS: complete

SPEC CONTEXT:
Specification §6.3/§6.4 (`.workflows/resume-hooks-silently-lost/specification/.../specification.md:355,357,365`) fix the anti-reap ordering: the snapshot read must strictly precede the live pane enumeration, the delete set must be derived under the exclusive hold, and the snapshot may only narrow that set — never widen it. §6.4:365 additionally requires that the `ListAllPaneHookKeys` read and the empty-live-set guard both resolve with no lock held. The spec body describes the *old* two-slice signature, and Corrigendum 2026-09-01 (specification.md:566) records this task's change as authoritative: "`CleanStale` takes the *enumeration* as a callback and performs its own `loadSnapshot` before invoking it; the call site hands over no snapshot. The ordering the sections argue for … is preserved and strengthened, because a caller can no longer get the order wrong." Code, corrigendum and CLAUDE.md's `hooks` architecture row all agree.

IMPLEMENTATION:
- Status: Implemented (preferred form — Do item 1, the callback)
- Location:
  - `internal/hooks/store.go:298` — `func (s *Store) CleanStale(enumerateLive func(Snapshot) ([]string, error)) ([]string, error)`. One parameter, a closure; the transposition the task describes is no longer expressible.
  - `internal/hooks/store.go:299-309` — the sequence the store now owns: `loadSnapshot()` → `enumerateLive(snapshot)` → `deleteStale(live, snapshot)`.
  - `internal/hooks/store.go:53-55` — `loadSnapshot` is unexported again (the exported `LoadSnapshot` that existed only so the cmd-layer pre-read could reach it is gone; no reference to the old name survives anywhere in the tree).
  - `internal/hooks/store.go:32` — `Snapshot` named type; `narrowToSnapshot` at :266 now takes it directly, and `deleteStale` at :332 intersects `StaleKeys(h, live)` with it.
  - `internal/hooks/store.go:70-79` — `loadSharedBounded` closes its fd (releasing the flock) via `defer` before returning, so the shared hold is gone before `enumerateLive` is reached.
  - `internal/hooksweep/sweep.go:126-148,159` — the single production caller. `liveTokenEnumeration` is the closure; the ordering comment the task asked to delete is gone, replaced by the *why* note on `CleanStale`'s doc (`store.go:288-297`). The sweep's empty-store guard rides through as the `errNothingPersisted` sentinel (`sweep.go:122,134-136`), classified back at `sweep.go:177-178`.
  - `cmd/doctor.go:363-392` — the read-only diagnosis takes the same two gates (`hooksweep.StalenessStandDown`, `JudgeAgainstLivePanes`) after its `store.Load` has returned, so it too performs no tmux read under a hold.
- Notes:
  - Criterion 1 holds structurally: there is exactly one parameter.
  - Criterion 2 holds structurally, not by convention: the tmux read (`reader.ListAllPaneHookKeys`, `sweep.go:86`) can only run from inside `enumerateLive`, which `CleanStale` invokes between a released shared hold and an unacquired exclusive one. The restore-marker read (`StalenessStandDown`, `sweep.go:155`) runs before the store is touched at all.
  - Criterion 3: `narrowToSnapshot`'s semantics are preserved exactly — the old form built a set from `snapshotKeys []string` and tested membership; the new form tests membership in the snapshot map. Same predicate, one fewer allocation.
  - Later phases moved the cycle from `cmd/run_hook_stale_cleanup.go` into `internal/hooksweep` and renamed `ErrSnapshotRead` → `ErrStoreRead` (now covering both of the clean's reads). That is downstream work, not drift from this task; this task's shape survives it intact.
  - No stale prose: a repo-wide search for `snapshotKeys`, `LoadSnapshot`, `ErrSnapshotRead` and the old call shape returns nothing outside `.workflows/`.

TESTS:
- Status: Adequate
- Coverage (all four requested fixtures exist, at both layers):
  - Snapshot-read-precedes-the-closure: `internal/hooks/cleanstale_snapshot_test.go:75` ("it hands the enumeration the file as it stood before it ran") — proves the ordering by observation rather than by a call-order recorder: the closure writes a new key and asserts the snapshot it was handed does not contain it. Stronger than the "record call order" the task sketched.
  - No lock held while the closure runs: `internal/hooks/cleanstale_snapshot_test.go:94` and, at the cycle level, `internal/hooksweep/snapshot_order_test.go:44` — both call `hookstest.AssertSidecarFree` from *inside* the closure. That helper (`internal/hookstest/hooks_lock.go:74-84`) takes `LOCK_EX|LOCK_NB` from a fresh open file description, which flock's per-fd semantics make a real intra-process probe, and both fixtures assert the closure actually ran so the probe cannot pass vacuously.
  - Closure error aborts with nothing written and nothing logged: `internal/hooks/cleanstale_snapshot_test.go:117` — asserts `errors.Is` the caller's own sentinel (the unwrapped pass-through), an unchanged file byte-for-byte, and zero records on an installed `logtest` sink.
  - The old `cmd/hook_sweep_snapshot_order_test.go` behaviour survives: re-homed as `internal/hooksweep/snapshot_order_test.go:17` with all three of its cases (retain-a-registration-landing-in-the-gap, no-lock-while-enumerating, report-exactly-what-was-deleted) plus the nothing-persisted-creates-nothing case at :103.
- Beyond the ask, and warranted: `cleanstale_snapshot_test.go:143` (delete set derived from the file under the lock, not from the snapshot), `:170` (an unreadable file aborts *before* the enumeration runs), `cleanstale_read_sentinel_test.go:28` (both reads share `ErrStoreRead`, a failed save carries neither), and `read_lock_test.go:453` (the pre-read degrades to unlocked at the derived bound rather than the mutation bound, with the snapshot still read).
- Notes: the two "holds no lock" fixtures overlap in subject but not in layer — one pins the store API's contract, the other pins the sweep cycle's. The task asked for both. Not redundant.

CODE QUALITY:
- Project conventions: Followed. Unit lane only (no binary build, no daemon, no real tmux); `hooks` log component and its `op`/`via`/`hook_key`/`value`/`error`/`error_class` attrs are unchanged; the staleness rule stays behind the single exported `StaleKeys`, which `internal/hooks/cleanstale_staleness_guard_test.go:14` guards, and the same file's second guard (:56) forbids the mutation paths — `deleteStale` included — from re-entering a locking front door.
- SOLID principles: Good. The change is a straight inversion of control: the store owns the sequence it is the only party able to enforce, and the caller supplies only the part it owns (the tmux read). The `Reader`/`PaneHookLister` seams are unchanged.
- Complexity: Low. `CleanStale` is 12 lines and linear; `deleteStale` carries the mutation it always carried.
- Modern idioms: Yes. `%w: %w` double-wrap for the read sentinel, `maps.Clone`, closure-typed parameter.
- Readability: Good. The doc comments state *why* the order matters (`store.go:288-297`, `:312-317`) rather than instructing a caller to keep it, which is exactly what Do item 3 asked for. Every comment I checked holds against the code: the snapshot is read first, the shared hold is released before the closure, the enumeration's error is returned unwrapped, a clean that removes nothing writes no file and emits no summary (`store.go:334-336`).
- Issues: None reaching the bar.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
