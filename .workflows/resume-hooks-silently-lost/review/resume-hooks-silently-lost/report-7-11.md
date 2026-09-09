TASK: resume-hooks-silently-lost-7-11 — CLAUDE.md Drift Left By This Work Unit (tick-e9cfa9, severity: drift)

ACCEPTANCE CRITERIA:
- The `logtest` row's accessor list and consumption sentence match `internal/logtest`'s exported surface.
- No line in CLAUDE.md attributes the nanoid alphabet to `internal/spawn`.
- The snapshot narrowing appears in the file with its invariant stated (narrow only, never widen).
- The sidecar-absence window and the migration's sidecar-less move are both recorded.
- No claim is added that the tree does not support — each of the four edits is checkable against a named file.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context). It is a documentation-drift repair: four passages of CLAUDE.md that
this work unit's code changes had falsified or left silent — the `logtest` package row, the multi-window-spawn
single-sourcing list, and two invariants of the hooks store's clean-stale path (snapshot narrowing, sidecar absence).
The underlying behaviour it documents is the hook-key/staleness machinery the work unit built: `hooks.StaleKeys`,
`CleanStale`'s snapshot-then-enumerate-then-delete sequencing, and the `hooks.json.lock` sidecar introduced by the
locking work.

IMPLEMENTATION:
- Status: Implemented
- Location: commit 0ef57876 (`CLAUDE.md`, +7/-3). Current state of the four passages:
  - `CLAUDE.md:84` — the `logtest` row (rewritten again by later phase-8/9 tasks; verified against today's tree).
  - `CLAUDE.md:220` — the spawn "single-sourced in `internal/spawn`" list, with "the nanoid alphabet" removed.
  - `CLAUDE.md:72` — the `hooks` row, carrying the snapshot pre-read / `narrowToSnapshot` / narrow-never-widen text.
  - `CLAUDE.md:192` — "A registration landing in the enumeration gap is never reaped."
  - `CLAUDE.md:194` — "The `hooks.json.lock` sidecar is absent on every install in the wild."
- Notes: Each claim was read back against the file it describes, as the task's Verification section prescribes:
  - `internal/logtest/capture.go:27-257` + `install.go:11` + `assert.go:14-50` — every exported symbol the row names
    exists with the stated shape (`Install(t) *Sink`; `NewCaptureLogger(t) (*slog.Logger, *Sink)`; `Sink.Records()`
    as the single base query; the four `Records` filters `AtExactLevel`/`AtOrAboveLevel`/`WithMessage`/`Matching`;
    the one terminal `Records.Only(t, description)`; `Body`/`Lines`; the eight `Record` accessors; `RecordWant`/
    `AssertRecord`/`AssertWriteFailure(t, rec, wantClass, sentinel)`). No exported symbol is omitted beyond the
    `slog.Handler` methods the row covers by describing `Sink` as a capture-`slog.Handler`, and the accessors the row
    says are gone (`Sink.RecordsAtExactLevel`, `OnlyRecord`, `OnlyRecordWith`, …) are absent from the package.
  - The consumption sentence holds: no embedded-field `*logtest.Sink` wrapper survives in `cmd`, `cmd/bootstrap`,
    `internal/state` or `internal/restore` — the only package-local helper types over a `Sink` are named-field
    (`internal/tmux/portal_saver_test.go:1144`, `internal/tmux/hooks_register_test.go:707`), exactly the "plain
    package-local helper" the row describes. The row's other named symbols also exist
    (`cmd/open_test.go:2859` `warnBypassHandler`, `cmd/bootstrap/latch_test.go:35` `orchestrationSeqHandler`,
    `internal/log/log_test.go:42` `recordingHandler`, `internal/log/rotate_test.go:15` `componentCapture`,
    `internal/hooks/store_test.go:1255` `TestSetEmitsOpAsJSONField`, `internal/logtest/leaf_guard_test.go:26-32`
    pinning the dep set to `harnesstest` + `log` across both lanes via `sourceguardtest.Lanes()`).
  - `internal/nanoid/nanoid.go` is the alphabet's home, as `CLAUDE.md:79` already states; a repo-wide grep for
    "alphabet" in CLAUDE.md returns only `:79` (the `nanoid` row) and `:190` (the staleness paragraph's "neither in
    the id alphabet"), so no line points at `internal/spawn` for the id vocabulary.
  - `internal/hooks/store.go:298-310` — `CleanStale` takes `loadSnapshot()` first, then calls `enumerateLive`, then
    `deleteStale`; `store.go:318-332` derives the delete set under its own exclusive hold and applies
    `narrowToSnapshot` (`store.go:266-274`, which only keeps candidates the snapshot holds). The narrow-never-widen
    invariant in CLAUDE.md matches the code and its own comment verbatim in substance.
  - `internal/hooks/lock.go:38-39` — `snapshotLockBound()` is `max(lockTimeout/snapshotLockFraction,
    lockPollInterval)` with `snapshotLockFraction = 100` (`lock.go:27`), so the row's "a hundredth of `lockTimeout`,
    floored at one poll interval" is exact. `lock.go:81-83` `acquireMutationLock` passes `O_CREATE`; `lock.go:91-92`
    `acquireSharedLock` does not — both as the file states. `store.go:70-79` degrades to an unlocked read after one
    `load-unlocked` DEBUG (`store.go:73`), also as stated.
  - `cmd/config.go:16-48` — `migrateConfigFile` stats and `os.Rename`s a single path, so the "moves `hooks.json`
    alone, without its sidecar" claim holds.
  - The store's error sentinel drifted after this commit (`ErrSnapshotRead` → `ErrStoreRead`, and
    `snapshotLockTimeout` → `snapshotLockBound()`); the current row names the current spellings
    (`internal/hooks/store.go:280`, `lock.go:38`), so later tasks carried the passage forward correctly and nothing
    stale is left behind.

TESTS:
- Status: Adequate (none required)
- Coverage: The task is documentation-only and declares no test; its Verification section prescribes a read-back of
  each edited passage against a named file, which is what I performed above. No production or test code changed in
  the commit (`CLAUDE.md` is the sole file), so no suite could observe it and no test baseline is at risk.
- Notes: Nothing in the tree asserts on CLAUDE.md's prose, and adding such an assertion would be wrong — the file is
  guidance, not a contract.

CODE QUALITY:
- Project conventions: Followed. The added passages match the file's established voice (bolded invariant statement
  followed by the mechanism and the consequence) and sit in the sections a reader would look in — the `hooks` table
  row for the machinery, the "Resume hooks" section for the data-safety invariant, beside the existing
  "A key the staleness rule cannot judge is retained forever" paragraph.
- SOLID principles: N/A (documentation).
- Complexity: N/A.
- Modern idioms: N/A.
- Readability: Good. Each new claim names the symbol or file that backs it (`loadSnapshot`, `narrowToSnapshot`,
  `acquireMutationLock`, `migrateConfigFile`), so a future reader can re-verify it the way I did.
- Issues: None. No process artifacts leaked into the file — the two `§` references at `CLAUDE.md:203` and `:209` and
  the "v1" at `:209` predate this work unit (theming / grouping features) and are untouched by this commit.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
