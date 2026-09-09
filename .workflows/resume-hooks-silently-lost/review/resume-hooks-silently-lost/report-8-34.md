TASK: resume-hooks-silently-lost-8-34 (tick-f57d4f) — "The hook rm Test Corpus Carries Cross-File Duplicates And One Staging Shape Written Three Ways"

ACCEPTANCE CRITERIA:
- Each duplicated behaviour is pinned once; the two unique assertions survive.
- Both lock-timeout subtests run through `runRmCase`.
- A row naming both a pane key and a resolver fails loudly.
- A row naming neither runs against the production default without panicking.

STATUS: complete

SPEC CONTEXT:
This is a phase-8 implementation-analysis task, so its authority is its own body rather than the
specification (per the shared verifier context: phases 6–10 are consolidation/quality work the
implementation phase generated). The behaviour the consolidated corpus pins is nonetheless
spec-governed and unchanged by this task: `hook rm` exits 0 iff it removed an entry, the answer
comes from the removal itself, the `--pane-key` pass-through reaches tmux for nothing, and removal
neither mints nor unstamps a token nor touches a dirty flag (CLAUDE.md "Resume hooks"). Nothing in
this change touches production code — the whole delivered change-set is three `cmd` test files.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/hooks_rm_exit_test.go:60` — new `rmPaneSeams(t harnesstest.TestingT, tt rmCase)` decides the
    pane pair a row is driven against and refuses a contradictory row.
  - `cmd/hooks_rm_exit_test.go:29-33` — new `rmCase.holdLock` field; `:39-46` — `rmOutcome` gains
    `out` and `err`.
  - `cmd/hooks_rm_exit_test.go:78-108` — `runRmCase` now owns the lock-hold staging and returns the
    captured output/error.
  - `cmd/hooks_rm_exit_test.go:110-156` — `TestRmCaseRows` with the two required subtest names.
  - `cmd/hooks_write_lock_test.go:187-223` — the shared `oneEntry` seed plus the two held-lock
    subtests routed through `runRmCase`.
  - `cmd/hooks_test.go` — four `TestHooksRmCommand` subtests deleted (commit a5732ee3).
- Notes:
  - AC1 verified by re-deriving every assertion of the four deleted subtests against
    `git show a5732ee3^:cmd/hooks_test.go`:
    * "removes hook for current pane" (old :478) — strict subset of
      `cmd/hooks_rm_exit_test.go:246` "it exits 0 and removes on the resolved-token path".
    * "reads pane ID from TMUX_PANE and resolves hook key" (old :501) — its unique
      raw-`%42`-not-used-as-key check now lives at `cmd/hooks_rm_exit_test.go:266-268`, with the
      drive re-pointed to `TMUX_PANE=%42` at `:254` so the pane id and the resolved key differ.
    * "it exits non-zero when no hook exists for pane" (old :567) — its message assertion is at
      `cmd/hooks_rm_exit_test.go:218-221` and its empty-output assertion was added at `:222-224`.
      The old subtest captured both streams into one buffer; `runHookRm`
      (`cmd/testhelpers_test.go:189-198`) does the same, so `out != ""` is the same assertion.
    * "removes correct JSON entry from hooks file" (old :593) — sibling-survives is at
      `cmd/hooks_rm_exit_test.go:269-271`; the old-format-sibling variant of the same claim
      survives independently at `cmd/hooks_test.go:594-596`.
  - The second unique assertion (last-event cleanup emptying the file) was not touched: it is still
    at `cmd/hooks_test.go:518-542`, including the `len(data) != 0` check.
  - The deleted subtests drove the plural `hooks` alias; the alias is still exercised on the rm path
    by the surviving `TestHooksRmCommand` subtests (`cmd/hooks_test.go:488, 508, 529, 556, 584, 611`)
    and by `TestHookCommandRename`, so no alias coverage was lost.
  - AC2: the two subtests the task's problem statement scopes ("differing only in holding the sidecar
    lock and needing the captured stdout") are routed through `runRmCase`
    (`cmd/hooks_write_lock_test.go:194`, `:211`) with their hand-rolled staging gone. The third
    subtest, "it returns at the bound rather than hanging" (`:225`), is deliberately left
    hand-rolled: `assertReturnsAtLockBound` (`cmd/hooks_write_lock_test.go:43-60`) asserts
    `elapsed >= lockBound` as a lower bound, so folding `runRmCase`'s staging into the timed interval
    would inflate it in the direction that makes the assertion easier to pass. It still consumes the
    extracted `oneEntry` seed at `:227`.
  - AC3: `cmd/hooks_rm_exit_test.go:64-66` fatals on the contradictory row, naming the row and the
    reason. The explicit `return nil, nil, nil` after the `Fatalf` is required by Go's
    terminating-statement analysis (the callee is an interface method, not `*testing.T.Fatalf`), and
    is unreachable under both a real `*testing.T` (Goexit) and `harnesstest.Recorder` (sentinel
    panic absorbed by `Run`) — so the helper's post-fatal statements stay unreached as the
    `Recorder` doc requires.
  - AC4: `cmd/hooks_rm_exit_test.go:72-74` returns a true nil interface, which
    `hookSeams` (`cmd/hooks.go:90-92`) fills with the production client. The doc comment's claim
    about the typed-nil hazard is correct: a nil `*mockKeyResolver` widened to `HookKeyResolver` is a
    non-nil interface and `ResolveHookKey` dereferences the receiver (`m.calls++`,
    `cmd/hookkey_vocabulary_test.go:126-129`).
  - Ordering inside `runRmCase` matches the hand-rolled sequence it replaced: bound lowered, seed
    staged, `TMUX_PANE` set, `before` bytes read, and only then the sidecar held — so the recorded
    bytes are the file the timed-out mutation must leave alone, as the field comment claims.
  - `logtest.Install(t)` now runs before staging in the first lock subtest rather than after. This is
    safe: `hookstest.StageStore` (`internal/hookstest/staging.go:55-97`), `CreateHooksSidecar` and
    `HoldHooksSidecar` all write with `os.WriteFile`/`flock` and emit no records, so
    `AssertLockWarn`'s "exactly one record at or above WARN"
    (`internal/hookstest/hooks_lock.go:108-110`) still holds.
  - The fix-round item recorded in
    `.workflows/.../fix-tracking-resume-hooks-silently-lost-8-34.md` (replace the third subtest's
    inline seed literal with `oneEntry`) is applied at `cmd/hooks_write_lock_test.go:227`, and the
    `oneEntry` comment at `:184-186` carries no call-site cardinality claim.
  - No dangling references to the deleted subtest names remain anywhere in the tree outside archived
    workflow documents.

TESTS:
- Status: Adequate
- Coverage: The task's own two required tests exist verbatim as
  `"it rejects a case naming both a pane key and a resolver"` (`cmd/hooks_rm_exit_test.go:111`) and
  `"it falls back to the production resolver for a case naming neither"` (`:132`), both driven
  through `harnesstest.Recorder` so the helper's own refusal path is asserted rather than aborting
  the test. The "Consolidation: no surviving assertion is new" constraint holds — outside
  `TestRmCaseRows`, the only added assertions are the two the task named as unique-and-must-survive
  (`out != ""` at `:222`, `data["%42"]` at `:266`), both re-homed from deleted subtests rather than
  invented.
- Notes:
  - Not under-tested: the contradictory-row refusal asserts the fatal count, the reason text and
    that the message names the offending row; the fallback case asserts all three return values.
  - Not over-tested: no row in the corpus currently leaves both fields unset, so the default branch
    is exercised only by its own guard test — that is the point of the AC (closing the combination
    before a future row lands on it), not redundancy.
  - The fallback subtest asserts the seam is nil rather than driving a full row through `runRmCase`.
    Driving one is not available: `cmd`'s `TestMain` poisons `TMUX` package-wide, so the production
    `tmux.DefaultClient()` the nil seam resolves to would dial a dead socket. Asserting the seam
    shape is the correct level.
  - Both new subtests would fail if the behaviour they name broke: removing the `paneKeyPath &&
    resolver != nil` guard drops `rec.Fatals` to zero, and returning `tt.resolver` in the default
    branch makes the `resolver != nil` check fire.
  - Lane rule respected: all three files are unit-lane `package cmd` tests with no build tag; they
    build no binary, spawn no daemon and open no tmux server. No `t.Parallel()` was introduced.
  - Seam-injection rule respected: every drive goes through `withHooksDeps`
    (`cmd/hooks_rm_exit_test.go:93`), so `cmd/deps_seam_guard_test.go` stays satisfied and no real
    command body reaches a live tmux client.

CODE QUALITY:
- Project conventions: Followed. Test-only change in the `cmd` unit lane; helpers reported through
  `harnesstest.TestingT` per the shared-stand-in convention; `hookstest` remains the single route to
  a staged `hooks.json`; no new logging, no production surface.
- SOLID principles: Good. `rmPaneSeams` is a single-responsibility extraction (decide the pane pair,
  refuse a contradictory row) split cleanly out of `runRmCase` (stage, drive, assert exit), which is
  what makes the refusal itself testable.
- Complexity: Low. The four-arm `switch` is exhaustive by return and each arm is one line.
- Modern idioms: Yes. Named results document the three-value return; `harnesstest.Recorder` is used
  as intended rather than a bespoke stub.
- Readability: Good. The `rmCase.resolver`, `rmCase.holdLock` and `rmPaneSeams` comments each state a
  reason (why nil, why the hold comes after the read, why a typed-nil is not handed back) rather than
  restating the code, and every claim in them holds against the code.
- Issues: None found.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
