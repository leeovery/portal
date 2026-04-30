---
agent: standards
cycle: 8
findings_count: 0
status: clean
---
# Standards Analysis (Cycle 8)

## Summary

Phase 14 cleanup conforms to spec and project conventions; defer placement, seam normalisation, and unexport rename all match sibling patterns with no residual drift.

## Verification (no action)

**Spec Conformance**: Phase 14 is pure-cleanup — no spec semantics, tmux invariants, or resurrection state shape touched.

**Convention compliance**:
- T14-1 defer at cmd/state_cleanup.go:77-78 placed immediately after `buildStateCleanupDeps()` — matches relative position in state_signal_hydrate.go:150-151, state_hydrate.go:366-367, state_notify.go:47-48, state_migrate_rename.go:33-34, state_daemon.go:216. Nil-receiver-safe like the five siblings.
- T14-2 seams (cmd/open.go:22, 29) now use direct `var x = fn` matching signalHydrateRunFunc (state_signal_hydrate.go:127). init() at lines 428-431 retains only legitimate cobra wiring.
- T14-3 unexport at internal/restoretest/restoretest.go:258 — symbol now `openAndSignalFIFO`, single in-package caller at line 187 updated, signature `(path string, delay, budget time.Duration) error` preserved, build tag `//go:build integration` unchanged. Repo-wide grep for `OpenAndSignalFIFO` returns zero source hits.

**Documentation Drift**: None.
- openTUIFunc godoc accurately describes t.Cleanup-restored override; the fictitious "openTUIFunc → openTUI → openCmd → openTUIFunc cycle" wording is fully removed.
- openPathFunc godoc's syscall.Exec rationale is load-bearing — openPath's outside-tmux path calls `po.execer.Exec(...)`.
- openAndSignalFIFO godoc framing ("internal helper for DriveSignalHydrate") matches the unexported name.
- Compile-time seam-signature assertions at cmd/reattach_integration_test.go:862-863 still match unchanged.
