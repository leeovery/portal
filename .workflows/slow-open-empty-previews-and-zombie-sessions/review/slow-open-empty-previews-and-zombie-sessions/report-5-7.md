TASK: 5-7 — Integration test bytes-identical scrollback dir snapshot across self-eject

STATUS: Complete

SPEC CONTEXT: Component D "No final flush on self-eject" — daemon must NOT execute one more captureAndCommit/gcOrphanScrollback on its way out. Snapshot at first failing tick must equal snapshot immediately after `os.Exit(0)`. Mirrors Phase 4 Task 4-2 pattern but applied to `os.Exit(0)` path.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_self_supervision_integration_test.go:709-862` `TestSelfEject_NoScrollbackDeltaAcrossEject`; supporting const `firstFailingTickObservationWindow` at 642-652
- `//go:build integration` at file level
- Uses `portaltest.IsolateStateForTest` + `StagePortalBinary` (functional equivalents of plan names)
- snapBefore at startInstant + 1200ms — lands AFTER first probe-1 failing tick (~1s), BEFORE counter reaches N=3 (~3s)
- snapAfter immediately after `daemon.Wait()` returns (kernel finalized teardown)
- Equality via `portaltest.DiffFingerprints`; failure dumps both key sets, delta list, portal.log, stderr
- Exit-code assertion present (824-840)
- Absent-saver trigger (mirrors 5-5)
- Wait reaper goroutine launched BEFORE snapBefore window for deterministic reap

TESTS:
- Status: Adequate
- Asserts no-final-flush via byte-level snapshot equality across eject window
- Diff covers added/removed/size/mtime/ctime/sha256/symlink-target
- Plan listed two named tests collapsed legitimately into one: snapBefore IS empty under chosen setup, exercising both subcases. Header (696-708) justifies collapse
- Snapshot timing uses fixed-delay sleep (1.2s) rather than log polling; rationale documented; spec allows either

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; absolute paths in errors
- SOLID: Good; snapshot/diff/format via portaltest helpers
- Complexity: Low; linear stage → spawn → snap → wait → snap → diff
- Modern idioms: `maps.Keys`, `slices.Sorted` (Go 1.21+)
- Readability: Excellent; heavy header explains spec linkage, choreography, timing rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Consider mirroring 2s exit-latency floor from `TestSelfEject_PortalSaverAbsent_ExitsCleanly` as belt-and-braces; cheap given `startInstant` already captured
- [idea] Plan specifies two verbatim test names; implementation uses one combined; grep for plan-specified names would no-match
- [quickfix] Plan AC references `NewIsolatedStateEnv`/`BuildPortalBinary`; impl uses `IsolateStateForTest`/`StagePortalBinary`; sync plan text or add one-line equivalence comment
