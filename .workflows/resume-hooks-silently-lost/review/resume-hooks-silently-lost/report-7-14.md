TASK: resume-hooks-silently-lost-7-14 — "logtest.Record Has No Error Accessor, So Nine Sites Hand-Roll It" (tick-9ddf57)

ACCEPTANCE CRITERIA:
- `ErrorAttr` fatals on an absent attr and on a non-error value, and returns the error otherwise.
- None of the nine sites indexes `rec.Attrs["error"]` or type-asserts `.Any().(error)` directly.
- All six `took` presence checks read through `RequireDuration`, and each still passes.
- Every `errors.Is` assertion downstream of the nine sites is unchanged.
- `go test ./...` passes with no test renamed.

STATUS: complete

SPEC CONTEXT: This is a phase-7 implementation-analysis task, not specified work — the work unit's specification (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md`, 572 lines) never mentions `logtest`, and per the shared verifier context a phase 6–10 task's authority is its own body. Judged against that body: a duplication finding — nine test sites re-derive an error out of a captured `logtest.Record` (index the attr map, fatal on absent, type-assert `.Any().(error)`, fatal on the wrong kind), and six more restate a `took` presence check that `Record.RequireDuration` already expresses as a kind assertion. The remedy is one new member of the `Record` accessor family plus routing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/logtest/capture.go:68-80` — `func (r Record) ErrorAttr(t harnesstest.TestingT, key string) error`, sited directly after `IntAttr` (`:56-66`) and before `DurationAttr` (`:85`), exactly as the Do list prescribed.
  - Nine hand-rolled blocks routed through it in commit `ce5e7ecc`: `internal/hooks/lock_write_test.go` (the `assertLockWarn` helper), `internal/hooks/store_test.go` ×3, `internal/project/store_logging_test.go` ×4, `internal/storelog/clean_stale_test.go` ×1.
  - Six `took` presence checks replaced with `RequireDuration`, now at `internal/hooks/store_test.go:950,1093`, `internal/project/store_logging_test.go:398,473`, `internal/storelog/clean_stale_test.go:32,62`.
  - `CLAUDE.md`'s `logtest` architecture row amended to name `ErrorAttr` in the accessor family; the row's current text lists it and holds true.
- Notes:
  - Enumerated the before/after of the target pattern rather than trusting the counts in the task body. At `ce5e7ecc^`, `.Any().(error)` appeared at 14 sites; the nine the task names are exactly the nine the commit removed. The five it left were out of the task's declared scope (`cmd/bootstrap/eager_signal_hydrate_test.go`, `internal/state/signal_hydrate_test.go`, `internal/tmux/hooks_register_warn_test.go` ×2, and `cmd/state_daemon_capture_logging_test.go:34`, which walks raw `slog.Attr`s in a bespoke handler rather than a `Record`). In the tree as it stands the first four have since been converted by later tasks, and the only surviving `.Any().(error)` in the repo is `internal/logtest/capture.go:75` — the accessor itself.
  - `rec.Attrs["error"]` survives at two sites (`cmd/bootstrap/eager_signal_hydrate_test.go:116`, `internal/state/signal_hydrate_test.go:150`), both asserting the attr's `Kind()` is `slog.KindAny` before reading the value through `ErrorAttr` on the following lines. Neither is one of the nine, and neither re-derives the error.
  - `rec.Attrs["took"]` survives at exactly one site, `internal/logtest/capture_test.go:100` — `logtest`'s own test of the flattened attr map, whose subject is the map rather than the accessor.
  - Every `errors.Is` tail was preserved verbatim by this commit. Three of the hooks sites and four of the project sites have since moved onto `logtest.AssertWriteFailure`, which performs the same `errors.Is` against the same sentinel through `ErrorAttr` (`internal/logtest/assert.go:50-58`) — a later task's consolidation, downstream of this one, and semantically identical.
  - Symbol signature drifted after the commit: `ErrorAttr` was authored taking `logtest.TestingT` and now takes `harnesstest.TestingT`, the shared stand-in that a subsequent task made the single declaration of that subset. Sound and consistent with every sibling accessor.

TESTS:
- Status: Adequate
- Coverage: `TestRecord_ErrorAttr` (`internal/logtest/capture_test.go:254-288`) carries all four named subtests verbatim: the happy path (`errors.Is` against a sentinel logged as the `error` attr), the absent-attr fatal (`"cause"`), the non-error-value fatal (driven against the `"op"` attr, a string), and the diagnostic-content check that the failure message names both the missing key and the record's attrs. The failure paths run through `harnesstest.Recorder` via the local `expectFail`/`captureFailure` helpers (`:391-405`), so the accessor's own fatals are observed rather than aborting the harness — which is what Do item 4 asked for and how the sibling accessors (`AttrString:143`, `DurationAttr:180-190`, `IntAttr:249`, `RequireDuration:297`) are covered.
- Notes: The two missing-key subtests overlap on the same code path but assert different properties (that it fails vs. what it says when it does), mirroring `DurationAttr`'s existing split at `:215-235`. Not redundant. The six `RequireDuration` substitutions strengthen presence to kind, as the task body flagged, and each named attr is a real `time.Duration` at the emission site, so no assertion was weakened or silently broken. No test was renamed by the commit; the only helper churn is `expectFail` gaining the `captureFailure` sibling it delegates to.
- Not executed: per this reviewer's remit, adequacy was judged by reading. No suite was run.

CODE QUALITY:
- Project conventions: Followed. `internal/logtest` stays test-only and its leaf-guard dependency set is untouched (no new import — `harnesstest` was already there). The accessor takes the shared `TestingT` subset rather than `*testing.T`, matching the whole family and keeping its failure paths unit-testable. Unit lane, no build tag, no test execution or process/tmux surface involved.
- SOLID principles: Good. One method, one job; the value-kind decision stays inside `Record` where its siblings live, and no caller re-derives it.
- Complexity: Low. Twelve lines, two guard clauses, one return.
- Modern idioms: Yes. Comma-ok type assertion on `slog.Value.Any()` is the correct read for an error attr — `slog` boxes an error as `KindAny`, so the `v.Kind()` shape the `IntAttr`/`DurationAttr` siblings use would not discriminate here, and the assertion does.
- Readability: Good. The doc comment ("fails the test if the attr is absent or carries a non-error value") states exactly the two fatals below it and nothing the code falsifies; the two failure messages name the key, the offending value and the record's attrs, matching the register of the surrounding family.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
