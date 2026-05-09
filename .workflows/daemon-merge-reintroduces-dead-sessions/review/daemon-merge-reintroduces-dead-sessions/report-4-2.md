TASK: Fix step-number drift in adapters_test.go FIFOSweeper docstring (4-2)

ACCEPTANCE CRITERIA:
- adapters_test.go FIFOSweeper docstring references step-8.
- Production docstrings in adapters.go remain correct.
- StaleMarkerCleaner / CleanStaleMarkers attribution preserved as step 7.
- `go test ./internal/bootstrapadapter/...` passes.

STATUS: Complete

SPEC CONTEXT: Phase 4 cleans documentation drift after orchestrator grew from 8 to 9 steps. Task 4-2 fixes FIFOSweeper test docstring to match nine-step framing.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/bootstrapadapter/adapters_test.go:35` — `TestFIFOSweeper_PropagatesListSkeletonMarkersError` docstring reads "step-8 Warn-and-swallow path".
  - `internal/bootstrapadapter/adapters.go:77, 106, 130` — production FIFOSweeper docstrings consistently say step 8.
  - `internal/bootstrapadapter/adapters.go:95` — correctly attributes "step 7" to CleanStaleMarkers.
- Notes: Task description mentioned `adapters_test.go:139` StaleMarkerCleaner reference — current test file is only 132 lines, no StaleMarkerCleaner reference remains (adapter removed in 4-4). CleanStaleMarkers/step-7 attribution now lives in production code (`adapters.go:95`, `cmd/bootstrap/bootstrap.go:113, 268, 270`).

TESTS:
- Status: Adequate
- Coverage: `TestFIFOSweeper_PropagatesListSkeletonMarkersError` exercises the `listerStub` error path.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: N/A.
- Complexity: Low.
- Readability: Good — test docstring mirrors production seam under nine-step framing.
- Issues: None.

Verification cross-checks:
- Grep `step-7|step 7` in `internal/bootstrapadapter/`: only `adapters.go:95`, attributed to CleanStaleMarkers.
- Grep `step-8|step 8` in `internal/bootstrapadapter/`: four matches at `adapters.go:77, 106, 130` and `adapters_test.go:35`, all FIFOSweeper.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
