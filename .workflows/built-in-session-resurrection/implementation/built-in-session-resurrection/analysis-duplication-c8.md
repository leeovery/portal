---
agent: duplication
cycle: 8
findings_count: 0
status: clean
---
# Duplication Analysis (Cycle 8)

## Summary

Phase 14 cleanup is duplication-clean — T14-1 closes cycle-7 D1 (logger lifecycle now uniform across five RunE bodies), T14-2 godoc rewrite removes obsolete wording without introducing parallel duplication, T14-3 unexport leaves no orphan exported symbols.

## Verification (no action)

- **V1 — T14-1 closes cycle-7 D1.** `cmd/state_cleanup.go:78` adds `defer func() { _ = logger.Close() }()` immediately after `buildStateCleanupDeps()`, byte-identical to the same pattern at `cmd/state_signal_hydrate.go:151`, `cmd/state_hydrate.go:367`, `cmd/state_notify.go:48`, `cmd/state_migrate_rename.go:34`, `cmd/state_daemon.go:216`. Five call sites, single-line idiom — no wrapper warranted.
- **V2 — T14-2 godocs no longer redundant or misleading.** The "break the cycle" / `init()`-assigned wording is gone. Both `openTUIFunc` and `openPathFunc` godocs now describe the t.Cleanup-restored override pattern with parallel-but-not-duplicated phrasing; openPath's godoc justifies the seam via the syscall.Exec process-replacement rationale specific to its branch. `init()` in `cmd/open.go:428-431` is now flag+AddCommand only.
- **V3 — T14-3 unexport leaves no orphan exported symbols.** `openAndSignalFIFO` (`internal/restoretest/restoretest.go:258`) has its single in-package caller at line 187. All other exported symbols in the package have verified external consumers in `cmd/`, `cmd/bootstrap/`, and `internal/restore/`. No follow-on unexport owed.
- **V4 — `openAndSignalFIFO` ↔ `cmd/state_signal_hydrate.writeFIFOSignal` byte-equivalence acknowledged, not eliminable.** The test helper's deadline-budget shape diverges from the production seam-driven retry; consolidating would require either widening `signalHydrateConfig` or layering production cmd code on top of test scaffolding (inversion). Godoc cross-reference at `internal/restoretest/restoretest.go:254` documents the relationship.
