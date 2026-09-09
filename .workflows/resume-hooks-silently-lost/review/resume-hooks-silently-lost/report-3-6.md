TASK: resume-hooks-silently-lost-3-6 — "One Hydrate Log-Record Filter, Parameterised By Message" (tick-d6f5a9)

ACCEPTANCE CRITERIA:
- One record-filter helper remains in package `cmd` for hydrate records; the other two are gone
- Its name makes no claim about log level
- Every call site asserts on exactly the record it asserted on before — no assertion moves, no message changes
- The exactly-one-record cardinality check is preserved at every site
- `go test ./cmd/` passes; `go test -tags integration -p 1 ./cmd/...` passes

STATUS: complete

SPEC CONTEXT:
The task is a test-helper consolidation inside phase 3, whose specified subject is the empty-hook-key hydrate
contract: spec §"collectArmInfos must not bake an empty key" (specification.md:170) requires that a saved pane with
an empty `PortalPaneID` is armed with no hook, and that `portal state hydrate` treats an absent `--hook-key` and an
empty one alike. Task 3-3 wrote the guard for that (`cmd/state_hydrate_empty_hookkey_test.go`) and, with it, the
third copy of a hydrate log-record filter — which is what 3-6 exists to collapse. The spec has nothing to say about
the shape of a test helper; the task body is the authority here, and the three filtered messages
(`timeout waiting for hydrate signal`, `signal timeout`, `scrollback replayed`) are three genuinely distinct
production emissions from `cmd/state_hydrate.go`.

IMPLEMENTATION:
- Status: Implemented, then legitimately superseded by later in-plan consolidation
- Location:
  - Task commit 58960c80 did exactly what the task asked: it deleted `signalTimeoutRecord`
    (`cmd/state_hydrate_timeout_log_test.go`) and `scrollbackReplayedRecord`
    (`cmd/state_hydrate_replayed_log_test.go`), renamed the survivor `hydrateWarnRecord` → `hydrateRecord`
    (no level claim), moved it into `cmd/state_hydrate_test.go` beside the other shared `cmd` hydrate helpers,
    and re-pointed all four call sites with the message each already filtered on. The stale `logtest` imports
    were dropped from the two emptied files in the same commit.
  - Commit 88191dcd (task 6-19, "logtest owns the sink, the filters and the record assertion") then removed
    `hydrateRecord` itself, and 8-42 finished the job: the same rule is now the shared repo-wide vocabulary,
    `sink.Records().Matching(component, msg).Only(t, description)` — `Matching` at
    `internal/logtest/capture.go:149` (lifting `Record.Matches`, `capture.go:113`) and the exactly-one terminal
    `Only` at `internal/logtest/capture.go:157`.
  - Delivered call sites, all four on that one route: `cmd/state_hydrate_empty_hookkey_test.go:152`,
    `cmd/state_hydrate_timeout_log_test.go:44`, `cmd/state_hydrate_replayed_log_test.go:68` and `:142`.
- Notes:
  - The literal acceptance criterion "one record-filter helper remains in package `cmd`" is no longer true of the
    delivered tree — zero remain, because a later task in this same plan lifted the rule into `internal/logtest`,
    which CLAUDE.md documents as the canonical route ("Records are read through one base query — `Sink.Records()`
    — narrowed by chaining the exported filters"). This is the code moving past the plan's wording, not a loss:
    the task's stated outcome — one named way to pull a single hydrate record out of a sink — holds more strongly
    now than the task asked for, since the one way is shared across every package rather than per-package.
    Not reported as a finding.
  - No cmd-local twin survives to compete with it: grep for `hydrateWarnRecord|signalTimeoutRecord|
    scrollbackReplayedRecord|hydrateRecord` across the repo returns nothing, and no other `cmd` helper returning
    a `logtest.Record` filters hydrate records (the ones that exist — `assertStandDown`, `failedCaptureWarn`,
    `assertHooksRecord`, `componentOf` — are other components' assertions).
  - The two rendered-text hydrate helpers `execLogLine` (`cmd/state_hydrate_exec_log_test.go:15`) and
    `countLogLines` (`cmd/state_hydrate_file_missing_log_test.go:18`) still live in the files that first needed
    them rather than in `cmd/state_hydrate_test.go`. They are a different subject (substring assertions on
    `Sink.Body()`), each has exactly one declaration, and they were outside this task's stated scope.

TESTS:
- Status: Adequate
- Coverage: The task correctly earns no test of its own — the helper is a lookup whose failure mode is exercised
  every time a message is wrong. The existing suites are the proof, and they still assert what they asserted:
  - `cmd/state_hydrate_empty_hookkey_test.go:152-155` — the timeout WARN's `hook_key` renders empty. The attr's
    presence is still fatal (`AttrString`, `internal/logtest/capture.go:35-42`), as the old inline
    `rec.Attrs["hook_key"]`/`ok` pair was.
  - `cmd/state_hydrate_timeout_log_test.go:44-47` — `took` is a `slog.KindDuration` equal to `hydrateTimeout`;
    the kind check (not the rendering) is preserved through `DurationAttr` (`capture.go:85-95`), which is the
    exact rationale the deleted helpers' comments carried.
  - `cmd/state_hydrate_replayed_log_test.go:68-71` — `bytes` read as a structured `IntAttr`; `:142-147` — `took`
    is a Duration under the settle sleep.
  - Cardinality: all four sites terminate in `.Only(t, …)`, which fatals unless exactly one record survives —
    the exactly-one check the three deleted helpers each hand-rolled.
- Notes: Tests were not executed (reading only, per this review's remit). Nothing in the delivered tree references
  a deleted symbol, and the `logtest` import was removed from precisely the files that stopped using it, so the
  package compiles as read.

CODE QUALITY:
- Project conventions: Followed. The delivered route is the one CLAUDE.md names for `logtest` consumers
  ("a suite declaring a plain package-local helper over `*logtest.Sink` where it wants shorthand"), and the
  filters compose orthogonally rather than adding a combination method.
- SOLID principles: Good — `Record.Matches` is the single home of the component+message predicate, and
  `Records.Matching` lifts it instead of restating either half.
- Complexity: Low. Three near-identical 12-line loops became one chained expression per site.
- Modern idioms: Yes — method chaining over a named slice type, `t.Helper()` on the fatal terminal.
- Readability: Good. Each site names the record it wants in its own `Only` description, so a cardinality failure
  says which set was empty or doubled.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
