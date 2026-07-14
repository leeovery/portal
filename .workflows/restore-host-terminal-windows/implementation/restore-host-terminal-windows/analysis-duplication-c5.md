AGENT: duplication
FINDINGS: none
SUMMARY: No actionable cross-file duplication remains. A full fresh pass over the
  spawn/multi-select implementation confirms every prior-cycle finding is resolved and
  no new copy-paste-drift risk was introduced. The four earlier high/medium findings
  are all consolidated: the CLI/picker production spawn seams now build from one shared
  cmd.buildProductionSpawnSeams (c4); the section-header line-0 splice is single-homed in
  replaceHeaderLine and used by all eight applySectionHeader/applyProjectsSectionHeader
  branches (c3); the left-bar single-glyph column is single-homed in
  renderLeftBarGlyphColumn with the selector/marked/gone renderers delegating (c3); and
  the footer narrow-degrade loop is single-homed in fitClusterToWidth with both
  fitLeftCluster and fitFilterCluster delegating (c3). The internal/spawn shared vocabulary
  (SplitNetN, PreflightMissing, PartitionResults, FirstPermission, QuoteJoin/GoneMessage/
  UnsupportedNoopMessage/PartialFailureMessage, runArgvCombined/combineOutput/
  execFailureDetail, and the LogGone/LogUnsupported/LogPermission/LogBatchSummary emission
  shapes) is single-sourced, and both the CLI (cmd/spawn.go runSpawn) and the async picker
  (internal/tui decideBurst/dispatchBurst/burstRunner/handleBurstPartialFailure) derive
  every net-N split, pre-flight, count-semantics, message, and log decision through it — so
  the two orchestrations cannot drift. The residual repeats observed are non-actionable and
  intentional: (1) the four one-line `if m.multiSelectMode { return m, nil }` row-action
  suppression guards in model.go were flagged low in c4 and deliberately kept per-arm to
  preserve the per-key rationale comments and keymap_dispatch_guard_test's default-mode
  parity probe — single-statement guards well below the block-extraction threshold; (2) the
  two byte-identical `Run` one-liners on execOsascriptRunner/execRecipeRunner are a
  documented driver-quarantine type separation whose shared plumbing already lives in
  runArgvCombined; and (3) the two "keep line 0, replace body" splices
  (replaceListBodyWithNoMatches, replaceListBodyWithEmptyState) are pre-existing code
  outside this work unit's plan scope. Nothing new to consolidate.
