TASK: resume-hooks-silently-lost-5-3 — "A Write That Cannot Take The Lock Writes Nothing" (tick-a017d3)

ACCEPTANCE CRITERIA:
1. A timed-out `Set` returns the error without loading, classifying or writing; a timed-out `Remove` returns `(false, err)` on the same terms
2. Exactly one WARN per timed-out mutation, under the `hooks` component, with `op=set` or `op=rm`, `hook_key`, `via` and the lock error in `error`
3. The WARN carries no `error_class` and no `value`, and no new `op` value is introduced anywhere in this task
4. `Remove`'s no-removal path is unchanged: no write, no record, `(false, nil)`
5. `hook set` / `hook rm` exit non-zero on a timeout with the reason reaching stderr, no usage text printed
6. `hooks.json` byte-identical after every timed-out mutation; an absent file stays absent
7. No `save.requested` created by a timed-out `hook set`
8. A timed-out `hook set` leaves the pane's stamped token in place; a retry after the lock frees reuses that token and writes one entry
9. A sidecar that cannot be opened or created at all takes the same write-side branch
10. `errors.Is(err, hooks.ErrLockHeld)` holds for a timeout through every wrap
11. The command returns at the bound rather than hanging
12. `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§6.5 ("Acquisition is bounded, and a timeout degrades rather than wedges") splits the two sides: a write that cannot take the lock does not write — `hook set` / `hook rm` "exit non-zero with the reason, rather than hanging a shell the user is sitting in" — while a read that cannot take the lock reads anyway, unlocked, at DEBUG. The emission paragraph says `hook set` and `hook rm` "emit the same WARN under their own `op` and `hook_key`, and the error is returned up through cobra … The log line and the stderr line are both present; neither stands in for the other", and that the sidecar failing to open or be created at all takes the same split. §6.2 requires the config directory be created before acquisition (so a fresh machine's first `hook set` is not a permanent timeout); §2.2 puts the `save.requested` touch after a successful write; §4.1 forbids rolling back a stamp on a failed write. The task's own planning decisions — `op=set` never `op=modify` (classifySet needs a load the timeout prevented), and no `error_class` (its floor would attribute a lock failure to a write phase that never ran) — are recorded in the task body and are the discriminating details.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hooks/store.go:115-124` — `Store.Set` acquires first; on failure emits `logger.Warn("set", "op", "set", "hook_key", key, "via", via.String(), "error", err)` and returns the error unchanged, before `s.load()` at :127, so nothing is read, classified or written.
  - `internal/hooks/store.go:173-180` — `Store.Remove` mirrors it under `op=rm`, returning `(false, err)`. The no-removal path below (`:188-194`) is untouched: `(false, nil)`, no write, no record.
  - `internal/hooks/store.go:318-325` — `deleteStale`'s acquire failure deliberately emits no store-side WARN and returns the error raw, leaving the single stood-down line to the sweep's call site.
  - `internal/hooks/lock.go:52-74` — `acquireLock` returns `fmt.Errorf("%w: %s", ErrLockHeld, path)` at :70 on expiry, so the sentinel survives every wrap; `internal/hooksweep/sweep.go:181` matches it with `errors.Is` (never on text).
  - `internal/hooks/lock.go:81-84` — `acquireMutationLock` runs `MkdirAll` first and discards its error, so an uncreatable directory falls through to the same acquire branch that reports a lock failure (criterion 9, §6.2).
  - `cmd/hooks.go:218-222` — `hooksSetCmd` returns the store's error from `RunE` before `touchSaveRequestedForHook` (criterion 7); `cmd/hooks.go:292-295` does the same for `rm`. `cmd/hooks.go` was not otherwise touched by this task, as the Do list required (commit 15b5e4fa's stat shows only `internal/hooks/store.go` plus tests).
  - `cmd/root.go:162-163` (`SilenceUsage`/`SilenceErrors`) plus `main.go:62-77` (`classify` prints a non-silent, non-`UsageError` error to `errOut` and exits 1) carry the reason to stderr with no usage dump (criterion 5).
- Notes: No drift from the task or the spec. The vocabulary is unchanged — `set` and `rm` were already the component's ops, and no attr key is added. The two WARN branches are near-identical three-line emissions in `Set` and `Remove`; folding them would be over-abstraction against the file's existing per-op explicit style, and each carries its own accurate rationale comment.

TESTS:
- Status: Adequate
- Coverage: `internal/hooks/lock_write_test.go` covers write-nothing for both methods (absent file staying absent, existing file byte-identical), the sentinel through the wrap, one WARN under `op=set` and under `op=rm`, the `op=set`-not-`modify` discrimination (`:101-118`, seeded with the *same* key and event under a different command so a load-then-classify implementation would file `modify` and fail), the re-asserted no-removal silence (`:140-160`), the `CleanStale` store-side silence (`:162-185`), and the uncreatable-sidecar branch (`:187-204`). `cmd/hooks_write_lock_test.go` covers the CLI half: non-zero exit with the reason and no usage output for `set` and `rm` (including `--pane-key`, which is additionally guarded to issue no tmux call), absent-file-stays-absent, no `save.requested`, token retention plus a retry that mints once, stamps once and writes one entry, and the returns-at-the-bound assertion for both verbs with a lower bound as well as a ceiling.
- Notes: The `error_class`/`value` absence assertions the task listed as their own case now live inside `internal/hookstest/hooks_lock.go:108-130` (`AssertLockWarn`), which every call site runs — so the criterion is enforced at four sites rather than two, and the helper's own failing paths are pinned by `internal/hookstest/hooks_lock_test.go:42-68`. That is a consolidation, not a coverage loss: the eight cmd subtests and all fourteen store subtests from the delivering commit are still present or subsumed. `AssertLockWarn` asserts exactly one record at or above WARN, which is what pins "exactly one WARN"; the lowered bound is driven through `hooks.SetLockTimeoutForTest` with the sidecar held from a second fd in-process, as the task specified, and no test asserts the production 2s figure as a timing measurement. Nothing here is redundant — the two `op` fixtures (fresh file, same-key file) test different implementations apart.

CODE QUALITY:
- Project conventions: Followed. The emission stays inside the closed `hooks` component with existing `op`/`hook_key`/`via`/`error` attrs; no new component binding, no call-site vocabulary invention. Both suites are unit-lane (no binary build, no daemon, no tmux), correctly untagged. `cmd` tests inject `HooksDeps` through `withHooksDeps` rather than assigning the seam, satisfying `cmd/seam_guard_test.go`.
- SOLID principles: Good — the store keeps sole ownership of its own breadcrumbs, and the sweep's stand-down stays at its call site so one event produces one line.
- Complexity: Low — two guard clauses.
- Modern idioms: Yes (`%w` wrapping, `errors.Is` discrimination, no text matching).
- Readability: Good. Both comments state the non-obvious decisions (why `set` rather than `classifySet`'s verdict; why neither `error_class` nor `value`; why this is not the silent no-removal) and both hold true against the code.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
