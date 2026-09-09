TASK: resume-hooks-silently-lost-8-26 — "The Hooks-Store Lock WARN Is Asserted By Two Independently Written Helpers, Plus Three Inline Restatements Of Its Negative Half" (tick-a4c21b, phase 8, consolidation/duplication)

ACCEPTANCE CRITERIA:
- One helper asserts the lock WARN; both local helpers are gone.
- Both halves of the contract are covered for both suites — neither loses what the other's helper added.
- The negative half is stated once.
- `cmd` still requires exactly one WARN on its path.

STATUS: complete

SPEC CONTEXT:
The specification's §6.2/§6.5 govern the `hooks.json.lock` sidecar: a mutation that cannot take the sidecar within the bound exits non-zero, leaves `hooks.json` byte-identical and WARNs, while reads fall through to an unlocked read with a DEBUG `load-unlocked` breadcrumb (spec line 505 pins this as a unit-lane obligation). This phase-8 task is a test-consolidation task over the assertions for that WARN, not a behaviour change — its authority is its own body, and the production emission it asserts against is unchanged by it (`internal/hooks/store.go:122` for `set`, `:178` for `rm`: message and `op` both the verb, plus `hook_key`, `via` and `error`, and deliberately no `error_class`/`value` because no write phase ran).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/hookstest/hooks_lock.go:108-130` — new shared `AssertLockWarn(t harnesstest.TestingT, sink *logtest.Sink, op, key, via string)`, sited next to `AssertDegradedRead` (`:135`) as the task prescribed. It covers the whole line in one call: exactly one record at or above WARN (`Records().AtOrAboveLevel(slog.LevelWarn).Only`), then `logtest.AssertRecord` for level/message/`component=hooks`/`op`/`via`, then `hook_key`, the absence of `error_class` and `value`, and a non-empty `error`.
  - `internal/hooks/lock_write_test.go` — the local `assertLockWarn` is gone (the file now opens at `TestMutationLockTimeoutWritesNothing`, line 15); the four call sites are `:98`, `:117`, `:137`, `:200`.
  - `cmd/hooks_write_lock_test.go` — the local `assertOneLockWarn` is gone (`assertLockFailureReachesStderr` at `:19` is immediately followed by `assertReturnsAtLockBound` at `:43`); the two call sites are `:77` and `:204`.
  - `internal/hookstest/doc.go:1-11` and the `hookstest` row of `CLAUDE.md` are updated to name the second breadcrumb assertion.
- Notes:
  - Every acceptance criterion holds. The positive half (level/message/op/component/via/hook_key/non-empty error) and the negative half (no `error_class`, no `value`) are both asserted for both suites, and each suite gains what the other's helper had: `internal/hooks` gains the exactly-one-WARN count and the two absence checks at every call site; `cmd` gains the non-empty-`error` check.
  - The negative half is stated exactly once in the tree. Verified by enumeration: `HasAttr("value")` appears at `internal/hookstest/hooks_lock.go:124` and at five other sites, all with different subjects — `internal/hooks/store_test.go:1218` (a `set-noop` DEBUG) and `:1309`, `internal/alias/store_logging_test.go:183`, `internal/project/store_logging_test.go:254`, `internal/logtest/capture_test.go:310`. None restates the lock WARN's contract.
  - One deliberate divergence from the task's Do list, judged sound and not a loss: Do step 4 said to keep `cmd`'s exactly-one-WARN count "at its call site"; the implementation folded it into the shared helper instead. The acceptance criterion it serves ("`cmd` still requires exactly one WARN on its path") is met — `cmd/hooks_write_lock_test.go:77` and `:204` still fail on a second WARN — and the count is not in fact command-specific: the store's acquire-failure path returns immediately after its single `logger.Warn`, so the same count is true of `internal/hooks`. The change is recorded in the helper's doc comment (`internal/hookstest/hooks_lock.go:103-107`) and in the `hookstest` CLAUDE.md row, so the record matches the code.
  - No orphaned code: `cmd`'s `assertHooksRecord`/`hooksRecordWant` (`cmd/logging_capture_test.go:44,51`), which the deleted `assertOneLockWarn` used, are still consumed at `cmd/hookkey_vocabulary_test.go:249,260` and `cmd/hooks_test.go:742`.

TESTS:
- Status: Adequate
- Coverage: The two consuming suites are unchanged in subject and outcome — six call sites now route through the shared assertion. The shared assertion carries its own coverage in `internal/hookstest/hooks_lock_test.go`, driven through `harnesstest.Recorder` so the helper's failing paths are observable: the passing case (`:31`), both cases the task named verbatim — "it fails when the WARN carries an error_class" (`:42`) and "it fails when the hook_key differs" (`:70`) — plus "it fails when the WARN carries a value" (`:56`), "it fails when the error attr is empty" (`:84`) and "it fails when a second record joins the WARN at or above its level" (`:101`, asserting the `Only` fatal). Each covers a distinct branch of the helper; none is redundant.
- Notes:
  - The helper's expectations match the production emission read at `internal/hooks/store.go:122` and `:178` — same message, `op`, `hook_key`, `via` and `error`, and neither of the two attrs the write-phase WARNs at `:144` and `:202` add.
  - The `emitLockWarn` fixture (`internal/hookstest/hooks_lock_test.go:16`) varies one attr per case rather than restating the whole line, which is why the six cases stay small.
  - `slog.New(sink)` at `internal/hookstest/hooks_lock_test.go:103` is legal under the log guard: `internal/log/discard_guard_test.go:14` forbids only `slog.NewTextHandler(io.Discard`.

CODE QUALITY:
- Project conventions: Followed. The helper lives in `internal/hookstest`, the stated cross-package home for this shape, beside the `HoldHooksSidecar` fixture that produces the condition and the `AssertDegradedRead` sibling. It reports through `harnesstest.TestingT` — the single declaration of that subset — which is what makes its own failing paths testable. Both new files are unit-lane (untagged) and touch no real tmux, daemon or portal binary. The CLAUDE.md architecture row and the package doc were both updated in the same commit.
- SOLID principles: Good — one assertion, one contract; the `logtest.RecordWant` shared properties are delegated rather than restated.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The doc comment at `internal/hookstest/hooks_lock.go:103-107` states why the negative half is asserted ("the two attrs a write phase adds, whose absence is what says no write was ever attempted"), which is the part a reader could not recover from the code.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
