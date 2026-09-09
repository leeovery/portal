TASK: resume-hooks-silently-lost-1-2 — The Reaper Names The Entry It Reaped (promote the per-removed-key `clean-stale` DEBUG line to INFO carrying the removed command in `value`)

ACCEPTANCE CRITERIA:
1. Each removed key produces exactly one INFO record under the `hooks` component with `op=clean-stale`, `via=internal`, `hook_key=<key>`, `value=<removed on-resume command>`
2. No DEBUG record is emitted for a removed key — promoted, not duplicated
3. The batch summary `op=clean-stale entries=N via=internal took=…` is still emitted once per cycle with removals
4. A removed key whose entry carries no `on-resume` event still produces its line, with `value` empty
5. A removed key carrying several events produces exactly one line, whose `value` is its `on-resume` command
6. A cycle removing nothing emits neither per-key lines nor a batch summary
7. When `Save` fails after the per-key lines were emitted, those lines stand and the summary's WARN arm fires alongside them, and `CleanStale` still returns the error with the file unchanged
8. `portal doctor --fix` stdout is byte-identical to before for the same removals

STATUS: complete

SPEC CONTEXT:
§5.3 ("The reaper names what it deleted") requires each removed key logged at INFO under the `hooks` component carrying the removed entry's command in the existing `value` attr alongside `hook_key`, with the existing DEBUG line promoted rather than duplicated, the batch summary retained, and no new component or attr key. §5.1/§5.2 keep the deletion behaviour itself unchanged. The Corrigenda hold nothing bearing on this task; the two behavioural deltas below were sanctioned by later tasks in the same plan.

IMPLEMENTATION:
- Status: Implemented (with two deliberate, later-sanctioned refinements)
- Location:
  - `internal/hooks/store.go:348-357` — the per-key `logger.Info("clean-stale", "op", "clean-stale", "hook_key", key, "value", removedValue(h[key]), "via", ViaInternal.String())` loop, followed by the unchanged `storelog.EmitCleanStaleSummary`
  - `internal/hooks/store.go:360-384` — `removedValue`, which renders the reaped key's value attr
  - `internal/hooks/store.go:334-336` — the zero-removals early return, untouched
  - `internal/storelog/clean_stale.go:18-26` — the batch summary, untouched (INFO on success, WARN carrying `error`/`error_class`/`took` on save failure)
  - `cmd/doctor.go:206-208` — `Pruned stale hook: %s` stdout, untouched; it prints from the keys `hooksweep.Run` returns
- Notes:
  - Criteria 1, 2, 3, 6 and 8 hold exactly as written. `internal/hooks/store.go:351` is the only per-key `clean-stale` emission in the `hooks` component (verified by scanning every non-test `"clean-stale"` literal in the tree: the other three are `internal/project/store.go:174`, which is the projects store's own line, and the two in `internal/storelog/clean_stale.go`). No DEBUG twin survives, and `internal/hooksweep/sweep.go` adds no second per-key line (its own emissions are `stale-hook cleanup counts` / `… removed` at DEBUG and the stand-down/failure WARNs).
  - Criterion 7's wording is superseded by task 10-1's predecessor 6-4 (`cc60f8ed`, "a deletion line means the key actually went"): the emission now sits **after** a successful `s.save(kept)` rather than ahead of it, so a failed save emits no per-key line at all. This is a gain, not a loss — a line now exists only for a key the file no longer holds, so the log never claims a removal that did not land. The WARN summary arm, the returned error and the untouched file are all still present (`store.go:343-346`), which is the substance criterion 7 was protecting.
  - Criteria 4 and 5's wording is superseded by task 9-5 (`6d812e19`, "the event is a closed type, the sweep reports what it removed"): `removedValue` renders the command for a single-event key whatever event it was filed under (so an entry with only `on-exit` logs `value=x` rather than empty), and renders every `event=command` pair in event order for a multi-event key. Both changes report strictly more of what the deletion destroyed than the criteria asked for, and the criteria's intent — one line per key naming what was lost — is preserved. The single-event branch deliberately renders the bare command (the copy-pasteable recoverable form), which the function's own doc comment states.
  - `h` is the pre-delete map and `kept := maps.Clone(h)` is what the deletions are applied to (`store.go:338-341`), so reading `h[key]` after the write returns the entry as it stood. Every key in `removed` came from `StaleKeys(h, …)` narrowed to the snapshot, so `h[key]` is never absent.

TESTS:
- Status: Adequate
- Coverage (`internal/hooks/store_test.go:896-1130`, `TestCleanStaleLogging`, plus `internal/hooks/store_shape_test.go:82-104`):
  - `:897-951` two removals → exactly two INFO per-key records compared as a **set** (`maps.Equal` over hook_key→value), each asserted INFO with `via=internal`, plus one INFO summary with `entries=2`, `via=internal` and a `took` duration. A surviving DEBUG twin would land in the `perKey` partition and fail both the count and the level check, so criterion 2 is observed rather than merely assumed.
  - `:953-971` value carries the removed `on-resume` command; `:973-991` value is the command of a key filed under another event; `:993-1008` a multi-event key produces one record whose value is `on-exit=x; on-resume=cmd1`.
  - `:1010-1035` save denied → zero per-key records, one WARN summary asserted through `logtest.AssertRecord` + `AssertWriteFailure` (`write-failed-temp-create`, `fileutil.ErrWriteTempCreate`), the error returned, and the file byte-unchanged.
  - `:1096-1120` zero removals → no records at all and the file untouched; `store_shape_test.go:82-104` covers the same for an all-retained candidate set.
  - `cmd/doctor_fix_hook_prune_report_test.go:18-30` executes `doctor --fix` and asserts exactly one line equal to `Pruned stale hook: <key>` and exactly one line with that prefix; `cmd/doctor_test.go:866-870` and `cmd/doctor_fix_hook_prune_move_test.go:27` pin the same wording.
- Notes:
  - `partitionCleanStaleRecords` (`:862-894`) partitions on `hook_key` vs `entries` and errors on any third shape, which is what keeps the per-key line and the summary distinguishable without a new `op` value — matching the task's explicit instruction not to add one.
  - Ordering is never asserted as a sequence; the two-removal case compares maps, honouring the map-iteration-order edge case.
  - Not over-tested: each subtest names a distinct branch of `removedValue` or of the save/zero-removal paths; none restates another's assertions.
  - The zero-event entry (`{"tok":{}}`) has no test, and needs none: `Set` always writes an event and `Remove` drops the key with its last event, so only a hand edit produces one, and `removedValue` returns `""` for it by construction.

CODE QUALITY:
- Project conventions: Followed. One emission per event from the store chokepoint, `hooks` component, `op`/`hook_key`/`value`/`via` all inside the existing closed vocabulary (no new attr key, no new `op` value), `via` taken from the `Via` type rather than a literal. Unit-lane tests, no `t.Parallel()`, capture through `logtest.Install`/`Sink` as the convention requires.
- SOLID principles: Good. `removedValue` is one rendering decision in one place; the store keeps the per-key detail and `storelog` keeps the shared summary.
- Complexity: Low.
- Modern idioms: Yes (`maps.Clone`, `maps.Equal` in the test, range-over-map with an early return).
- Readability: Good. The two comments on the changed code both hold against it: `store.go:348-349` ("After the write, so a line exists only for a key the file no longer holds. The commands come from `h`, the pre-delete map.") is true of the emission's position and of `h`; `store.go:360-365` accurately describes both branches of `removedValue`, including the event ordering.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
