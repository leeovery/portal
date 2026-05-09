TASK: Fix step-number docstring drift in cmd/bootstrap_production.go and add missing adapter to inventory (4-1)

ACCEPTANCE CRITERIA:
- `cleanStaleAdapter` docstring (line ~51): "Step 8" → "Step 9".
- Inventory comment at lines 16-17 missing `StaleMarkerCleaner` — add it (later updated by 4-4).
- Wiring unchanged (docs-only).
- Existing tests pass without modification.

STATUS: Complete

SPEC CONTEXT: Phase 4 corrects docs/wiring drift after Phase 2 inserted CleanStaleMarkers as step 7. Task 4-4 explicitly coordinated by deleting the StaleMarkerCleaner pass-through, so final-state inventory should be four-adapter list.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap_production.go`
  - Line 55: docstring reads "Step 9 of the bootstrap sequence" — correct.
  - Lines 16-17 inventory: "HookRegistrar, RestoringMarker, RestoreAdapter, FIFOSweeper" — exactly matches the four exported types in `internal/bootstrapadapter/adapters.go`. `StaleMarkerCleaner` correctly absent (4-4 removed adapter).
  - Lines 22-28: documents `bootstrap.MarkerCleanupCore` constructed inline.
  - Lines 89-138 wiring: `StaleMarkers: &bootstrap.MarkerCleanupCore{...}` inline at 123-128.
- Notes: No drift remaining. `grep StaleMarkerCleaner` against production source surfaces only workflow artefacts.

TESTS:
- Status: Adequate (docs-only).
- Coverage: Underlying wiring exercised by `cmd/bootstrap/...` and `internal/bootstrapadapter/...` suites.

CODE QUALITY:
- Project conventions: Followed. Two-home adapter split documented and matched.
- SOLID: N/A.
- Complexity: Low.
- Readability: Good. Inventory paragraph names four exported adapters.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] Package comment dense; if a fifth exported adapter is added later, bullet list at that point would help.
