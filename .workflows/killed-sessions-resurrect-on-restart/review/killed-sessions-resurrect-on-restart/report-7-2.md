TASK: killed-sessions-resurrect-on-restart-7-2 — Correct NewRestoreAdapter docstring to remove inaccurate production-site reuse claim

ACCEPTANCE CRITERIA:
- NewRestoreAdapter's docstring no longer claims production reuses the inner Orchestrator beyond the adapter.
- Replacement rationale accurately names the four siblings at cmd/bootstrap_production.go (HookRegistrar, RestoringMarker, EagerSignalCore, MarkerCleanupCore).
- grep "reuses the inner Orchestrator" returns zero hits.
- No production wiring change (doc-only).

STATUS: Complete

SPEC CONTEXT: Doc-correctness paydown from Phase 7 cycle 4. Task 6-2 introduced NewRestoreAdapter with structural-reuse claim unsupported by source — restoreInner in cmd/bootstrap_production.go is declared and used once. Real reason is parity with surrounding adapters.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/bootstrapadapter/adapters.go:93-106
- Notes: New docstring at lines 97-102 reads as prescribed: "Production wiring at cmd/bootstrap_production.go retains its open-coded form for parity with the surrounding inline-struct adapters at that site (HookRegistrar, RestoringMarker, EagerSignalCore, MarkerCleanupCore); migrating it is mechanical and out of scope for the constructor's introduction." Logger nil-safety paragraph preserved. Constructor body untouched.

TESTS:
- Status: Adequate (no new tests required — doc-only)

CODE QUALITY:
- Project conventions: Followed. cmd/bootstrap_production.go untouched.
- Readability: Improved — rationale verifiable against cmd/bootstrap_production.go:119, :120, :131, :137.
- Issues: None.

Verification:
- grep "reuses the inner Orchestrator" across *.go: zero hits.
- Four named siblings confirmed inline at cmd/bootstrap_production.go.
- restoreInner two-step preamble unchanged.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
