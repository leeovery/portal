AGENT: duplication
FINDINGS:
- FINDING: burstPartialFailureFlash re-partitions Results the caller already partitioned
  SEVERITY: low
  FILES: internal/tui/burst_partial_failure.go:64, internal/tui/burst_partial_failure.go:120
  DESCRIPTION: Fix 3 (honest total-failure copy) added a second `spawn.PartitionResults(...)`
    call inside `burstPartialFailureFlash` (line 120: `confirmed, _ := spawn.PartitionResults(results)`)
    to derive `othersOpened = len(confirmed) > 0`. But its only caller,
    `handleBurstPartialFailure`, has already partitioned the identical slice one call
    earlier (line 64: `confirmed, failed := spawn.PartitionResults(msg.Results)`) and used
    `confirmed` for the selection mutation. So within a single partial-failure handling
    pass the same `msg.Results` is partitioned twice, and the `confirmed` value the caller
    holds is discarded and re-computed by the callee. It is a redundant recomputation of a
    value the caller already owns rather than a correctness bug (PartitionResults is pure and
    cheap), but it is a small copy-of-a-computation smell introduced by this plan and the kind
    of parallel derivation the code-quality "Compose, don't duplicate" guidance discourages.
  RECOMMENDATION: Thread the already-computed signal down instead of re-deriving it. Change
    `burstPartialFailureFlash(results, failed)` to accept the derived `othersOpened bool`
    (or the already-computed `confirmed` slice) as a parameter, and have `handleBurstPartialFailure`
    pass `len(confirmed) > 0` from its existing line-64 partition. This removes the duplicate
    `PartitionResults` call and makes the single line-64 partition the one source for both the
    selection mutation and the flash's othersOpened signal. Keep the shared
    `spawn.PartialFailureMessage(failed, othersOpened)` renderer as-is (it is correctly single-sourced).

SUMMARY: The bugfix largely reuses the already-extracted shared spawn seams
  (PartitionResults, PartialFailureMessage, LogWindowResults, QuoteJoin/GoneMessage), so
  there is no significant new cross-file duplication. The one genuinely plan-introduced,
  actionable item is a low-severity redundant double `PartitionResults` call inside the
  TUI partial-failure path. The `othersOpened = len(confirmed) > 0` derivation appearing in
  both the CLI (cmd/spawn.go:179/210) and picker callers, and the `wantPermissionBody` /
  closedSpawnAttrKeys test helpers duplicated across the spawn/tui/cmd packages, are the
  spec's intended cross-caller parity design (single-sourced renderer, per-site golden
  anchors) and/or pre-existing code, so they are not flagged.
