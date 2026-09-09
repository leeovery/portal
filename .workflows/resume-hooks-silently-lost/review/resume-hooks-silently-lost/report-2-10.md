TASK: resume-hooks-silently-lost-2-10 — One Level Filter And One Hooks-Record Assertion (tick-c4a7e8)

ACCEPTANCE CRITERIA:
- [ ] `internal/logtest` owns the level filter; neither `cmd` site reimplements it
- [ ] One assertion carries the shared record shape, called from both helpers
- [ ] Each helper keeps the assertion that is its own
- [ ] No assertion is dropped
- [ ] `go test ./...` passes

STATUS: complete

SPEC CONTEXT:
The two record shapes these helpers assert are spec-fixed, so the consolidation must not blur them.
§2.2 (specification.md:85) pins the dirty-flag touch failure: one WARN under the `hooks` component with
message and `op` both `touch-save-requested`, alongside `hook_key`, `via=cli` and the existing `error`
attr, filed under its own `op` rather than under `set` (a `set` WARN "would name a loss that did not
happen"). §5.4 (specification.md:310) pins the stand-down: `op=clean-stale-skipped`, `via=internal`, and
`reason` naming which of six declines fired. Both lines are what an operator greps after a hook vanishes,
so the level, the message, the `op` and the `via` are each load-bearing and none may be silently dropped
by a shared helper.

IMPLEMENTATION:
- Status: Implemented (delivered at 8bbd4a3b, then carried forward by later-phase consolidation)
- Location:
  - Level filter, now in logtest: `internal/logtest/capture.go:135` (`Records.AtOrAboveLevel`), reached
    through `Sink.Records()` at `internal/logtest/capture.go:250`.
  - Shared record-shape assertion: `cmd/logging_capture_test.go:44` (`hooksRecordWant`) and
    `cmd/logging_capture_test.go:51` (`assertHooksRecord`), which pins `Component: "hooks"` and delegates
    the five shared properties to `logtest.AssertRecord` (`internal/logtest/assert.go:25`).
  - Call site 1 — stand-down: `cmd/hookkey_vocabulary_test.go:249` inside `assertStandDown`
    (`cmd/hookkey_vocabulary_test.go:236`), with `standDownWant` at `cmd/hookkey_vocabulary_test.go:259`.
  - Call site 2 — touch failure: `cmd/hooks_test.go:742` inside `assertTouchWarn`
    (`cmd/hooks_test.go:738`).
  - Remaining `cmd` consumers of the filter: `cmd/hooks_test.go:741`, `cmd/hooks_test.go:829`,
    `cmd/hooks_test.go:846`, `cmd/hookkey_vocabulary_test.go:241`, `cmd/hookkey_vocabulary_test.go:246`.
- Notes: The delivered names have moved since the task's own commit, legitimately and for the better.
  `Sink.RecordsAtLevel(min)` became the chained `Records().AtOrAboveLevel(min)` (with `AtExactLevel`,
  `WithMessage` and `Matching` as its orthogonal siblings and `Only` as the single terminal), and the
  `cmd`-local `assertHooksRecord` body was lifted into the tree-wide `logtest.AssertRecord` /
  `RecordWant`, which now serves the alias, project, hooks, storelog, hooksweep and hookstest suites too.
  The `cmd` wrapper survives only to bind `Component: "hooks"` for its two callers. `standDownRecord` and
  the `warnRecords` copy are gone from the tree (no occurrence of either name remains), and the file that
  held the former was later renamed into the `hook_prune_*` / `hook_sweep_*` set. This is the task's
  outcome reached by a wider route, not drift away from it.

TESTS:
- Status: Adequate
- Coverage:
  - The moved primitive carries its own test: `internal/logtest/capture_test.go:315`
    (`TestRecords_AtOrAboveLevel`) pins inclusion at the threshold, exclusion below it, capture order,
    and the nil return when nothing matches — the descendant of the `TestSink_RecordsAtLevel` the task
    added. `internal/logtest/assert_test.go:28` pins that `AssertRecord` reports all five mismatches
    separately rather than short-circuiting, which is what keeps a consolidated failure as diagnosable as
    the five inline checks it replaced.
  - The task's own proof is the two subtests that drive the helpers: `cmd/hooks_test.go:771` and
    `cmd/hooks_test.go:793` both end in `assertTouchWarn`, and `assertStandDown` is driven from
    `cmd/hook_prune_single_report_test.go:29,43,62` and `cmd/doctor_stand_down_copy_test.go:308`. The
    consolidation gained consumers rather than losing them, which is exactly the "a third `op` arrives
    later and adds a call" outcome the task named.
- Notes: No assertion was dropped, checked property by property against the pre-task bodies in the diff.
  Touch helper: WARN-count-of-one (now `AtOrAboveLevel(WARN).Only(t, …)`, which fatals on any count but
  one), level, message, `component`, `op`, `via`, `hook_key`, non-empty `error` — all eight still run.
  Stand-down helper: the "nothing at or above WARN" sweep, exactly-one-record, level, message,
  `component`, `op`, `via`, `reason` — all still run, with `reason` now parameterised over
  `hooksweep.Reason` instead of the literal `"restoring"`, which widened it to every decline. One
  deliberate relaxation arrived later, at `cmd/hookkey_vocabulary_test.go:246`: for a WARN stand-down the
  exactly-one count is taken over the at-or-above-WARN set rather than the whole sink, because the
  degraded pre-read emits its own DEBUG breadcrumb alongside. The comment at
  `cmd/hookkey_vocabulary_test.go:230` states that reason, and the DEBUG branch above it still holds the
  strict whole-sink count. Sound. No test was added for the extraction itself, correctly — the task said
  "no new test", and an extraction whose every assertion still runs at both call sites is proved by those
  call sites.
- Not run: per the review protocol I read the tests rather than executing them, so the
  `go test ./...` criterion is assessed by reading. Nothing in the changed code could fail to compile
  silently — `hooksRecordWant` and `assertHooksRecord` are package-level in `cmd` with two callers each,
  and a stale duplicate would be a redeclaration error rather than a passing build.

CODE QUALITY:
- Project conventions: Followed. `logtest` remains the single home of every typed record accessor, which
  is the convention the task's problem statement said was being violated by the two `cmd` copies.
  `AssertRecord` takes `harnesstest.TestingT` rather than `*testing.T` (`internal/logtest/assert.go:25`),
  keeping the failing helper's own failure path testable, per the `harnesstest` rule. `logtest`'s
  stdlib-plus-`harnesstest`-plus-`internal/log` dependency set is untouched by the change, so its leaf
  guard is unaffected. Both files stay in the unit lane, as a test-helper extraction should.
- SOLID principles: Good. The filter is one predicate on one type; the assertion carries only the five
  properties every audit-trail line shares, and each caller keeps the attr that is its own — the split
  the task's Do list asked for, and the split `internal/logtest/assert.go:22` documents.
- Complexity: Low. `AtOrAboveLevel` is a one-line `filter` call; `assertHooksRecord` is a five-field
  struct translation.
- Modern idioms: Yes. The `Records` slice type with chainable filters returning `Records` is the
  idiomatic shape for this, and it composes (`Matching(…).AtExactLevel(…).Only(…)`) without the
  cross-product of named methods the earlier `Sink.RecordsAtExactLevelWithMessage` family had grown.
- Readability: Good. Every helper carries a comment stating what it pins and, where it is not obvious,
  why: `cmd/hookkey_vocabulary_test.go:230` explains why the count's record set depends on the level,
  and `internal/logtest/capture.go:128` distinguishes the exact-level filter from the threshold one so a
  caller cannot reach for the wrong one by name.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
