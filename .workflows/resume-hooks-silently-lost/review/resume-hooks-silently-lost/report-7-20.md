TASK: resume-hooks-silently-lost-7-20 — Two Shape-Rule Subtests Are Duplicated Verbatim Across Two Files In internal/hooks (delete the two "it still …" copies from `cleanstale_snapshot_test.go`, leaving `store_shape_test.go` the single home for the shape rule)

ACCEPTANCE CRITERIA:
- The two "it still …" subtests are gone from `cleanstale_snapshot_test.go`.
- The snapshot file still covers its own subject: a key written after the snapshot is retained, and the delete set is derived from the file under the lock rather than from the snapshot.
- The empty-key deletion rule and the non-token-shaped retention rule are each still asserted exactly once, in `store_shape_test.go`.
- `go test ./internal/hooks` passes and the commit names the reduction.

STATUS: complete

SPEC CONTEXT: §5.2 of the specification defines the shape-aware deletion rule the deleted subtests were asserting — a token-shaped key absent from the live set is deleted, a non-token-shaped key is retained untouched on every run, and an empty key is deleted (it is neither token-shaped nor old-format, so the retention rule has nothing to protect in it). §3.2 supplies the shape predicate the rule reads (`nanoid.IsTokenShaped`), and §6.3 supplies the snapshot-narrowing property that is `cleanstale_snapshot_test.go`'s own subject: the call-site snapshot is taken before the pane enumeration and may only narrow the delete set, never widen it. The specification names no test files and prescribes no subtest names, so it constrains only that both rules keep an assertion somewhere — which they do. (This is a phase-7 implementation-analysis task; its authority is its own body.)

IMPLEMENTATION:
- Status: Implemented
- Location: commit `6a910b34` — `internal/hooks/cleanstale_snapshot_test.go`, 35 deletions, no other file touched.
- Notes: The commit deletes exactly the two named subtests ("it still retains a non-token-shaped key" and "it still deletes an empty key present in both the file and the snapshot") plus the `unjudgeableKey` local that only they used, so no unused declaration is left behind (`internal/hooks/cleanstale_staleness_guard_test.go:19` is now the sole remaining match for any of the old local key names across the package's tests). `store_shape_test.go` was not touched by this commit; its later movement (subtests now at `:14` and `:62` rather than the body's `:88-106` / `:31-62`) comes from commits `d8ead3e7` (task 7-23, seed-key vocabulary) and `8e3735ee` (task 9-8), not from this one.
- The commit message names the reduction as the Do list requires — "internal/hooks subtest pass verdicts 158 -> 156" — and records that the task body named the wrong twin for one of the two. That correction is accurate: the deleted "it still retains a non-token-shaped key" seeded an unjudgeable key alone and enumerated an *empty* live set, so its true twin is `internal/hooks/store_shape_test.go:82` ("it writes no file and emits no summary when every candidate is retained" — two unjudgeable keys, `enumerating()`, removed-empty plus file-unchanged), not `:14`, which enumerates a live key. Either way the claim survives, and the executor verified by mutation rather than by resemblance.
- No production source was touched, so there is no behavioural drift to assess.

TESTS:
- Status: Adequate
- Coverage: The shape rule keeps a failing-on-breakage assertion in each direction. `internal/hooks/store_shape_test.go:62` ("it deletes an empty key") asserts `slices.Equal(removed, []string{""})` and reloads to confirm the entry is gone, so retaining an empty key fails it. `internal/hooks/store_shape_test.go:14` ("it retains a non-token-shaped key absent from the live set") asserts nothing was removed, cross-checks the exported `hooks.StaleKeys` prediction, and asserts the file bytes are unchanged, so reaping an unjudgeable key fails it. The empty-live-set variant the deleted copy exercised is still covered at `internal/hooks/store_shape_test.go:82`.
- The snapshot file keeps its own subject intact: `internal/hooks/cleanstale_snapshot_test.go:48` ("it retains a key written after the snapshot") writes `ReapableSeedB` from inside the enumeration and asserts it survives, and `:143` ("it derives the delete set from the file under the lock, not from the snapshot") removes a snapshot-held key mid-enumeration and asserts it is not reported as removed. Both fail if narrowing regresses. `:25` remains the positive control that narrowing does not over-narrow.
- Neither rule is asserted twice anywhere else in `internal/hooks` — the package's other `CleanStale` suites cover the read sentinel, the lock, and the persisted key width. `internal/hooksweep/sweep_test.go:459` ("it reaps an empty key when a pane carries no token") is a different package asserting the sweep cycle end to end, not a second copy of the store rule.
- Notes: No stale reference to either deleted subtest survives in Go source; the only remaining mentions are workflow/planning documents (`phase-5-tasks.md:280-281`, the analysis records, `.tick/tasks.jsonl`), which are historical records rather than live contracts.

CODE QUALITY:
- Project conventions: Followed. Test-only change, unit lane, no `t.Parallel()`, no new helpers; the surviving files continue to reach `hooks.json` through `hookstest.StageStore` as CLAUDE.md requires rather than composing a path of their own.
- SOLID principles: N/A (deletion only)
- Complexity: Low
- Modern idioms: Yes
- Readability: Good — the file's remaining subtests all bear on the snapshot narrowing, matching its doc comment at `internal/hooks/cleanstale_snapshot_test.go:20-23`, which is now true of every subtest under it rather than most of them.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
