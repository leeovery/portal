---
topic: ghostty-spawn-zero-windows
cycle: 1
total_findings: 3
deduplicated_findings: 2
proposed_tasks: 2
---
# Analysis Report: ghostty-spawn-zero-windows (Cycle 1)

## Summary
Three agent findings across duplication, standards, and architecture collapse to two
deduplicated items — the duplication and architecture agents independently flag the same
`burstPartialFailureFlash` double-partition recomputation, and the standards agent flags
an untested precondition gap in the Fix 4 `ghosttycompile` guard. Both are low severity
with no correctness or behaviour impact, but each is a plan-introduced or plan-adjacent
code-quality item with a concrete, self-contained remedy, so both are promoted to tasks.
No high-severity findings were raised.

## Deduplication Notes
- The duplication finding (`burstPartialFailureFlash re-partitions Results the caller
  already partitioned`) and the architecture finding (`Picker partial-failure path
  recomputes the shared partition twice`) describe the identical issue in
  `internal/tui/burst_partial_failure.go` — merged into Task 1 with both sources recorded.
  Architecture additionally notes the redundant second `FirstPermission(results)` scan
  (line 108) alongside the double `PartitionResults` (line 120); both are folded into the
  single "make the helper self-contained" remedy.
- The non-flagged parity items the duplication agent explicitly excluded (the
  `othersOpened = len(confirmed) > 0` derivation in both CLI and picker callers, and the
  `wantPermissionBody` / `closedSpawnAttrKeys` test helpers duplicated across
  spawn/tui/cmd) are spec-intended cross-caller parity / pre-existing code, not findings —
  they are not discarded findings and are not carried forward.

## Discarded Findings
- None — both deduplicated findings were promoted to tasks. No low-severity finding was
  dropped this cycle (each carried a concrete, independently executable remedy; the
  burst-partition item is corroborated by two agents), and no high-severity finding exists.
