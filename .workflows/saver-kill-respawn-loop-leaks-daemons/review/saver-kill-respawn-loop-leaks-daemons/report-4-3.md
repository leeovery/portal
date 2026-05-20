TASK: Collapse eight install* seam helpers into a single generic helper (4-3 / tick-8dedac)

ACCEPTANCE CRITERIA:
- Eight install* seam helpers in internal/tmux/portal_saver_test.go collapse to a single generic helper
- t.Cleanup LIFO ordering preserved so seam-restore order is unchanged across the eight call sites

STATUS: Complete

SPEC CONTEXT: Cycle-2 duplication analysis flagged eight near-identical seam-install helpers (installBarrierReadPID, installBarrierIsAlive, installBarrierPollInterval, installBarrierTimeout, installBarrierLogger, installKillSaverFn, installReadVersionFile, installWriteVersionFile) sharing the same 4-line save/install/restore-via-cleanup skeleton. Recommendation: introduce `func swapSeam[T any](t *testing.T, ptr *T, v T)` and reduce each install* helper to a single-line wrapper.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/portal_saver_test.go:969-978 — generic swapSeam[T any] helper with doc comment explicitly noting LIFO cleanup ordering preservation.
  - internal/tmux/portal_saver_test.go:980-1009 — five barrier helpers collapsed.
  - internal/tmux/portal_saver_test.go:1323-1328 — installKillSaverFn collapsed.
  - internal/tmux/portal_saver_test.go:1648-1654 — installReadVersionFile collapsed.
  - internal/tmux/portal_saver_test.go:2020-2025 — installWriteVersionFile collapsed.
- All eight helpers now share a single implementation; existing seam accessors in export_test.go reused — no production-side change.

TESTS:
- Status: Adequate (no new tests required; behaviour-preserving refactor of test-only helpers)
- Coverage: All existing call sites (~30+ invocations) exercise the new generic helper transitively. The two SetBarrierLogger tests at :1588 and :1626 still capture/restore the seam directly (not via the helper) because they intentionally exercise the SetBarrierLogger production path.
- LIFO preservation: swapSeam registers exactly one t.Cleanup per call with no change to call ordering at call sites → LIFO restore order is byte-identical to the prior implementation.

CODE QUALITY:
- Project conventions: Followed. Standard Go test helper pattern; t.Helper() propagated.
- SOLID: Good — single responsibility; type-safe pointer indirection via generics.
- Complexity: Low.
- Modern idioms: Yes — uses Go 1.18+ generics appropriately.
- Readability: Good. Doc comment at :969-972 explicitly calls out the LIFO ordering invariant.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] The eight install* wrappers themselves could be removed in a follow-up by inlining swapSeam(t, tmux.<X>Seam(), v) at call sites. Keeping the wrappers preserves call-site readability — current state is the better trade-off; noted for completeness only.
