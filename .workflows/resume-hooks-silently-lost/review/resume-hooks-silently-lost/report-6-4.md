TASK: resume-hooks-silently-lost-6-4 — Emit The Reaper's Per-Key Deletion Lines Only After The Write That Performs The Deletion (tick-9e66ca)

ACCEPTANCE CRITERIA:
- A failed save produces zero `op=clean-stale` INFO lines and exactly one WARN summary carrying the error.
- A successful save produces one INFO line per removed key, each carrying the removed `on-resume` command, plus the INFO summary.
- The line's attrs and ordering relative to the summary are otherwise unchanged.

STATUS: complete

SPEC CONTEXT:
The specification (`.workflows/resume-hooks-silently-lost/specification/resume-hooks-silently-lost/specification.md:285-291`) promotes the hooks store's per-key `clean-stale` breadcrumb from DEBUG to INFO and requires it to carry the reaped command in the existing `value` attr, so an operator can answer "what did I lose?" from the production-default log without correlating a registration breadcrumb against a bare count. The batch summary is retained alongside, and the attr vocabulary (`op`, `hook_key`, `value`, `via`, `entries`) is unchanged. The spec does not itself pin the emission's position relative to the save — this task supplies that: a line promoted to the forensic level must not claim a deletion the file did not receive.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/hooks/store.go:343-355` (`deleteStale`). The failure branch at 343-346 emits only `storelog.EmitCleanStaleSummary(logger, len(removed), start, err)` and returns the wrapped save error; the per-key INFO loop sits at 350-353, after the successful `s.save(kept)`, followed by the success summary at 355. Commit `cc60f8ed` is the move itself; the loop's body has since been refined by later tasks (`removedValue(h[key])` for the `value` attr, `ViaInternal.String()` for `via`) without disturbing the ordering.
- Notes: The values still come from `h`, the pre-delete map loaded under the exclusive hold (`internal/hooks/store.go:328`), so the `value` attr carries the reaped command — the deletions are applied to the `kept` clone at 338-341, leaving `h` intact. The in-source comment at 348-349 states both facts and holds against the code. Ordering relative to the summary is unchanged (per-key lines, then summary). No second site claims a per-key deletion ahead of the write: `internal/hooksweep/sweep.go` emits only counts and stand-downs, and `cmd/doctor.go`'s `Pruned stale hook: <key>` output is printed from `CleanStale`'s returned slice. `internal/project/store.go:173-175` keeps the pre-save ordering, but its line is DEBUG and outside this task's stated scope — the task's own reasoning is that the ordering is harmless at DEBUG.

TESTS:
- Status: Adequate
- Coverage: `internal/hooks/store_test.go:1010` ("it emits no per-key lines and warns in the summary when the save fails") stages a seeded store with `WritesDenied: true`, asserts `CleanStale` errors, partitions the captured records and requires zero per-key lines and exactly one summary — asserted WARN with `component=hooks`, `op=clean-stale`, `via=internal` plus `error_class=write-failed-temp-create` and an `errors.Is` against `fileutil.ErrWriteTempCreate` — and finishes with `hookstest.AssertHooksFileUnchanged`, which is what ties "no line" to "no deletion" rather than merely to "no log". `internal/hooks/store_test.go:897` covers the success side: two reapable keys plus one unjudgeable, two per-key INFO records compared as a `map[hook_key]value` set against `{ReapableSeedA: "cmd1", ReapableSeedB: "cmd2"}`, each asserted INFO with `via=internal`, plus the INFO summary with `entries=2` and a `took` duration. `internal/hooks/store_shape_test.go:82` pins the zero-removed case (no file write, no `clean-stale` record at all).
- Notes: The tests would fail if the loop moved back above the save — the failure-path subtest is a direct negative on the per-key partition, not an incidental attr check. Some overlap exists between that subtest and `store_test.go:1085` ("emits WARN with write-failed-* error_class …") on the WARN summary's attrs, but the latter's subject is the returned error's classification and `entries` count rather than the per-key partition, so it is not redundant coverage of this task. `partitionCleanStaleRecords` (`store_test.go:865`) skips the `load-unlocked` DEBUG the sidecar-less fixtures legitimately produce, so the partition cannot pass by mislabelling a degraded-read breadcrumb.

CODE QUALITY:
- Project conventions: Followed. The emission stays on the store method (the chokepoint), the component/attr vocabulary is untouched, and the `hooks` component's single binding at `internal/hooks/store.go:20` is unchanged. Both tests are unit-lane and hermetic (temp dirs, `logtest.Install`), consistent with the lane rule.
- SOLID principles: Good — a statement reordering within one method, no surface change.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. The comment at 348-349 explains why the loop sits where it does and where the values come from, which is the non-obvious part of the arrangement.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
