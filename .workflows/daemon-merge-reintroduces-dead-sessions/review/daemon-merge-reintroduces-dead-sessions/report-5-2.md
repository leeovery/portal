TASK: Fix stale step-numbering and forward-reference docstrings (5-2)

ACCEPTANCE CRITERIA:
- `internal/bootstrapadapter/adapters.go:77` "FIFOSweeper / step 7" → "step 8".
- Four FIFOSweeper-attributed "step 7" refs in `cmd/bootstrap/phase5_integration_test.go` (225, 231, 272, 317) → "step 8".
- `scrollback_resumption_test.go` "step 7" refs (CleanStaleMarkers context) untouched.
- `internal/state/capture.go` `buildLiveStructure` docstring rewritten — no forward-reference to "subsequent tasks".
- Docs-only.

STATUS: Complete

SPEC CONTEXT: Phase 5 task 2 addresses doc rot from earlier renumbering and stale forward-reference on `buildLiveStructure`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/bootstrapadapter/adapters.go:77` — reads "FIFOSweeper / step 8".
  - `cmd/bootstrap/phase5_integration_test.go:225,231,272,317` — all four FIFOSweeper-attributed refs say "step 8"; surrounding prose internally consistent.
  - `cmd/bootstrap/scrollback_resumption_test.go:17,21,178,223,224,232` — CleanStaleMarkers/"step 7" refs preserved.
  - `internal/state/capture.go:149-154` — `buildLiveStructure` docstring describes current responsibility (projecting Sessions/Windows/Panes into nested lookup map for three-level filter); no forward-reference language.
- Notes: All targeted edits land verbatim; no scope creep.

TESTS:
- Status: Adequate (docs-only).
- Coverage: Existing tests continue to compile and assert behaviour.

CODE QUALITY:
- Project conventions: Followed. Comments match nine-step terminology.
- SOLID: N/A.
- Complexity: N/A.
- Readability: Improved — `buildLiveStructure` docstring reads as self-contained.
- Issues: None.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
