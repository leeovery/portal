TASK: resume-hooks-silently-lost-1-10 — One Deny-The-Write Fixture In internal/hooks (tick-0ba9fe, severity: near-miss)

ACCEPTANCE CRITERIA:
- [ ] One implementation of the seed-then-deny sequence in `internal/hooks/store_test.go`
- [ ] Both subtests still assert the same failure, error class and file state
- [ ] The cleanup restores the directory mode on every path
- [ ] `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
This is an implementation-analysis task, not a specified behaviour: its authority is its own body. It serves the
two `CleanStale` save-failure subtests that cover the specification's §5.2 (shape-aware deletion) and §5.3 (the
reaper names what it deleted) — the cases proving that when the batched `Save` fails, nothing is deleted from the
file, the failure is classified as a write-phase failure rather than "unexpected", and the summary is a WARN. The
fixture those cases need is a hooks.json that *already holds content* whose directory then refuses new files, so
the failure lands at the temp create rather than earlier — which is precisely why it cannot reuse the pre-existing
`readOnlyDirPath` (that one denies before anything is seeded).

IMPLEMENTATION:
- Status: Implemented, then superseded compatibly by a later task in the same plan.
- Location:
  - Task commit `00c4e633` added `seedThenDenyWrites(t, body) (*hooks.Store, string)` to
    `internal/hooks/store_test.go` and re-pointed both hand-rolled blocks (the one task 1-2 added and the
    pre-existing one) at it, moving the two-line explanatory comment onto the helper and deleting both copies.
  - Current head: the helper's role is carried by `internal/hookstest/staging.go:90-95` (`Staging.WritesDenied`),
    introduced by task 7-16's staging consolidation. `internal/hooks/store_test.go` now holds neither
    `seedThenDenyWrites` nor `readOnlyDirPath`; its deny-the-write call sites are
    `internal/hooks/store_test.go:480`, `:1012`, `:1070`, `:1232`, `:1370`.
- Notes:
  - The criterion asks for one implementation "in `internal/hooks/store_test.go`". At head there are zero there
    and one in `internal/hookstest`, shared across `internal/hooks`, `internal/hooksweep` and `cmd`. That is the
    criterion's substance met more broadly, not a loss — a later planned task moved the home, and the sequence
    still exists exactly once.
  - The ordering the case depends on is preserved and now stated where it happens: `staging.go:85-89` creates the
    sidecar *before* the denial, with the comment naming why ("so a denied write fails at the temp create rather
    than earlier at the sidecar's own open"). Without that, the mutation would fail at `acquireMutationLock`'s
    `O_CREATE` and carry no `error_class` at all, silently turning both subtests into a different case.
  - The whole-repo scan for the sequence finds no third copy: the only other `chmod 0500` sites under
    `internal/hooks` are `lock_test.go:293` and `lock_write_test.go:189`, which `os.Mkdir` an *uncreatable-parent*
    fixture (no seed, no sidecar, path nested one level deeper) to exercise the sidecar-acquire failure. That is a
    genuinely different case, and the task explicitly permitted leaving such a case alone.
  - The seed mode moved from the pre-existing block's `0o644` to `0o600` in the task commit and back to `0o644`
    in `staging.go:120`. No assertion on either path reads the mode, and the denial is on the directory, so
    neither subtest observes it.

TESTS:
- Status: Adequate.
- Coverage:
  - The two subtests the task names are the proof and both keep their assertions:
    `internal/hooks/store_test.go:1010-1035` (error returned, zero per-key records, one WARN summary with
    component/op/via pinned, `write-failed-temp-create` + `fileutil.ErrWriteTempCreate` via
    `logtest.AssertWriteFailure`, and the file byte-unchanged via `hookstest.AssertHooksFileUnchanged` at `:1034`)
    and `internal/hooks/store_test.go:1069-1094` (error classified `ErrWriteTempCreate`, one WARN, `entries=2`,
    the same write-failure tail, and a `took` duration).
  - The first subtest's name and its per-key expectation changed after this task (from "keeps the per-key lines"
    to "emits no per-key lines") — that is task 6-4's deliberate behaviour change, not drift introduced here.
  - The fixture itself is now directly covered: `internal/hookstest/staging_test.go:57-76` asserts the sidecar
    exists after staging and that a subsequent `Set` fails with `fileutil.ErrWriteTempCreate` — so a staging change
    that moved the failure to the lock open would fail there rather than silently weakening the two subtests.
    `internal/hooksweep/lock_timeout_test.go:122-127` makes the same discrimination explicit at a consumer.
- Notes: No over-testing. The fixture's own test asserts one property per case; the two subtests assert distinct
  things (no-per-key + file state vs. classification + entry count).

CODE QUALITY:
- Project conventions: Followed. Test-only scaffolding lives in a test-only package (`internal/hookstest`), reached
  by name; the `WritesDenied` axis is documented as a field on a described-staging struct rather than a positional
  parameter, matching the package's stated one-stager shape. No production import of test helpers.
- SOLID principles: Good. The staging struct's axes are orthogonal and mutual exclusivity is enforced at the seam
  (`staging.go:57-62`) rather than left to a caller's discipline.
- Complexity: Low. The deny arm is three statements.
- Modern idioms: Yes.
- Readability: Good. The two comments that matter — why the sidecar precedes the denial, and what `WritesDenied`
  buys the caller — sit on the helper rather than being restated at each of the ten call sites, which is exactly
  what the task asked for.
- Issues: None.

Cleanup-on-every-path check (criterion 3): `staging.go:91-94` registers the restoring `t.Cleanup` immediately after
a successful `os.Chmod`; a failed chmod is `t.Fatalf` with the mode unchanged, so there is nothing to restore. Every
current `WritesDenied` call site's directory is `t.TempDir()` or a `filepath.Join(t.TempDir(), …)` child
(`cmd/state_daemon_hook_cleanup_test.go:125`, `cmd/hook_prune_one_component_test.go:28`,
`internal/hooksweep/lock_timeout_test.go:109` and `:142`, `internal/hooksweep/sweep_move_test.go:157`, plus the
`internal/hooks` sites), so the restore's cleanup is registered after the TempDir's and — cleanups running LIFO —
runs before the `RemoveAll` that needs the mode back.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
