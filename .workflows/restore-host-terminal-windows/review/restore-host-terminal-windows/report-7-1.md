TASK: restore-host-terminal-windows-7-1 — Hoist result classification into internal/spawn and add the missing picker permission event

ACCEPTANCE CRITERIA:
- opened/failed/permission classification derived from the new spawn helpers at every site; no residual hand-rolled `r.Ack == spawn.AckConfirmed` / `Outcome == OutcomePermissionRequired` loop in cmd/spawn.go or internal/tui outside the per-window DEBUG-emit loops.
- A permission-required burst on the picker path emits the `spawn: permission required — nothing self-attached` INFO with the closed resolution/terminal/bundle_id/detail attrs and does NOT emit the generic `opened 0/N` summary — byte-identical to the CLI's logSpawnPermission output.
- The leave-what-opened selection mutation, guidance flash, and exit/quit behaviour of both callers are unchanged for all non-permission outcomes.
- Only the closed `spawn` attr keys appear on the new event; all existing spawn unit + integration tests remain green.

STATUS: Complete

SPEC CONTEXT: Spec §Observability (specification.md:440-450) designates the `spawn` component's closed event catalog — `permission-required` is a distinct catalogued entry (line 445) alongside per-window ack outcome and batch summary — and fixes the count semantics: `total` = N (incl. trigger self-attach target), `opened` = each acked spawn plus the trigger self-attach only when it occurs. The task problem statement: the "opened iff Ack==AckConfirmed" rule was re-implemented at ≥5 caller sites and the catalogued `permission-required` event was unreachable on the dominant picker path (it fell through to the generic `opened 0/N`). This refactor makes the rule a single chokepoint and makes the picker emit the catalogued permission event.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/classify.go:15-50 — `Confirmed()` (Ack==AckConfirmed), `PartitionResults` (confirmed/failed session names in list order, AckTimeout+AckFailed unified into failed, nil for absent class), `FirstPermission` (first Outcome==OutcomePermissionRequired, switches on generic Outcome only — driver-quarantine intact).
  - cmd/spawn.go:179 — `spawn.PartitionResults(results)`; :191 — `spawn.FirstPermission(results)`. The former `tallyWindowResults`/`firstPermission` were deleted and inlined; opened count is derived inside `spawn.LogBatchSummary` (logemit.go:56) from `PartitionResults`. Permission path routes through logSpawnPermission → spawn.LogPermission.
  - internal/tui/burst_observability.go:41-54 — `emitBurstSummary` delegates to `spawn.LogBatchSummary` (opened via PartitionResults); new `emitPermission` mirror delegates to `spawn.LogPermission`.
  - internal/tui/burst_partial_failure.go:43-47 — permission arm detected via `spawn.FirstPermission`, routed to `emitPermission` (else emitBurstSummary); :64 partition via `spawn.PartitionResults`; :108 flash via `spawn.FirstPermission`.
  - internal/tui/burst_progress.go:250-253 — `burstAllConfirmed` derives verdict from `spawn.PartitionResults` (len(failed)==0), keeping the msg.Err and len==burstExternal guards.
- Notes: grep across cmd/spawn.go + internal/tui (non-test) confirms zero residual `AckConfirmed` / `OutcomePermissionRequired` classification loops. The one remaining `result.Outcome == OutcomePermissionRequired` at internal/spawn/burst.go:177 is the Burster.Run early-stop inside internal/spawn itself (the source of the rule, not a caller re-implementation) — correctly out of scope. Routing is provably consistent with the CLI: a permission result has OK()==false → the burster assigns AckFailed (burst.go:164-167), so a permission window is always in the failed partition, meaning the picker's top-level FirstPermission check and the CLI's inside-`len(failed)>0` check fire on the same inputs.

TESTS:
- Status: Adequate
- Coverage:
  - spawn unit (classify_test.go): `TestWindowResult_Confirmed` truth table (confirmed/timeout/failed/zero); `TestPartitionResults` (empty→nil,nil; all-confirmed order preserved; timeout+failed both land in failed with order; all-failed); `TestFirstPermission` (empty→false; no-perm→false; first-in-order returned, asserts zero WindowResult on the false path).
  - tui unit (burst_observability_test.go): `TestBurstObservability_PermissionRequiredEmitsPermissionEvent` — picker permission burst emits exactly the emitPermission INFO with closed resolution/terminal/bundle_id/detail, asserts NO opened/total/batch attrs and NO generic `opened` INFO; `TestBurstObservability_PartialFailureNoPermissionEmitsSummary` — non-permission partial still emits `opened 1/3` and no permission event; `TestEmitPermission_ParityWithCLI` — picker body == golden literal.
  - cmd (spawn_test.go): `TestLogSpawnPermission_ParityBody` — CLI body == the same golden literal; `TestSpawnPermission_CLIEmitsPerWindowDebugsThenPermission` — CLI emits 2 per-window DEBUGs + 1 permission INFO + 0 summaries (preserves the deliberate CLI/picker asymmetry); `TestSpawnPermissionRequired` (behaviour) unchanged.
  - Cross-caller parity: both packages assert against an identical `wantPermissionBody` literal, and both emitters delegate to the single `spawn.LogPermission`, so byte-identical output is guaranteed structurally and pinned by two goldens.
- Notes: Test balance is right — no redundant happy-path duplication; the closed-attr-key guard (assertClosedSpawnKeys) enforces the "only closed spawn keys" criterion. Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Helpers live in internal/spawn alongside the other shared renderers; nil-logger tolerance via log.OrDiscard; closed attr vocabulary respected; test style matches (table tests, slices.Equal, logtest.Sink).
- SOLID principles: Good — the change is a textbook single-responsibility/DRY consolidation (one count-semantics chokepoint two callers derive from).
- Complexity: Low. Three small pure functions; caller bodies simplified.
- Modern idioms: Yes (append-nil partition, slices.Equal in tests).
- Readability: Good — intent is self-documenting and comments are precise about the CLI/picker asymmetry and the driver-quarantine boundary.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/burst_partial_failure.go:43,73,108 — `handleBurstPartialFailure` computes `spawn.FirstPermission(msg.Results)` at :43, then `burstPartialFailureFlash` re-scans the same slice via `FirstPermission` at :108. Negligible cost (slice is bounded by the marked set) and the current form keeps the flash builder a pure function of (results, failed). Decide whether to thread the already-detected `perm`/`ok` into the flash helper to avoid the second scan, or keep the purity. Low value.
- [idea] internal/tui/burst_partial_failure.go:43-59 — Residual CLI/picker log asymmetry on the pre-spawn-error arm (out of this task's scope, pre-existing): when `msg.Err != nil` with empty Results, the picker still emits `emitBurstSummary` → `opened 0/N`, whereas the CLI's runSpawn returns the error emitting nothing. This task only targeted the permission arm; decide whether aligning the pre-spawn-error arm (skip the 0/N summary when nothing was attempted) is worth a follow-up.
