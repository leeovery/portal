---
agent: architecture
cycle: 8
findings_count: 0
status: clean
---
# Architecture Analysis (Cycle 8)

## Summary

No architectural issues. Phase 14 tightens API surface (`OpenAndSignalFIFO` → `openAndSignalFIFO`), removes a logger leak (`defer logger.Close()`), and simplifies test seams (`var x = fn`) without introducing structural concerns.

## Verification (no action)

**API Surface**: `internal/restoretest` exports exactly the surface its three integration consumers call. All exported symbols (`ProjectRoot`, `BuildPortalBinaryDir`, `BuildPortalBinaryStable`, `PrependPATH`, `DriveSignalHydrate`, `DriveSignalHydrateBinary`, `WaitForSkeletonMarkersCleared`, `SortedKeySet`) have verified external consumers. No over-exposure; no dead exports.

**Module Structure**:
- `cmd/state_cleanup.go:78` — `defer func() { _ = logger.Close() }()` is correctly scoped to RunE. Placed after `buildStateCleanupDeps()` so the logger exists; outlives all three actions (kill saver / unregister hooks / purge) so their log lines flush before close.
- `cmd/open.go:22,29` — `openTUIFunc = openTUI` / `openPathFunc = openPath`. Tests reassign these directly; a `*Deps` struct would be over-engineered for two single-function seams. Compile-time signature assertions pin the contract against silent drift.

**Documentation Coherence**:
- `openTUIFunc` / `openPathFunc` godoc correctly describes test override via `t.Cleanup`-restored assignment, including the `syscall.Exec` and Bubble Tea launch concerns that motivate each seam.
- `openAndSignalFIFO` godoc ("internal helper for DriveSignalHydrate") consistent with the now-unexported casing.
- No drift between code and docs in scope.
