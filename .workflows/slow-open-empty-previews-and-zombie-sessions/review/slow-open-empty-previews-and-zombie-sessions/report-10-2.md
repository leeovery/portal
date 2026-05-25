TASK: 10-2 — Encode tri-state contract at the OrphanSweepCore.SaverPanePID seam boundary

STATUS: Complete

SPEC CONTEXT: c4 architecture (analysis-tasks-c4, Task 2). Pre-refactor seam `func() (int, error)` overloaded `(0, nil)` to mean "absent". Adapter collapsed `!present → (0, nil)`, erasing distinction. Widen seam (not inline) — preserves adapter as documentable wrapper.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/bootstrap/orphan_sweep.go:77` — seam `SaverPanePID func() (pid int, present bool, err error)` with godoc 60-77 documenting all three shapes
  - `cmd/bootstrap/orphan_sweep.go:152-162` — consumer reads `saverPID, saverPresent, saverErr`; switch dispatches `case saverErr != nil`, `case !saverPresent`, `default`. No `pid == 0` branch
  - `internal/bootstrapadapter/orphan_sweep.go:41-43` — adapter wraps `tmux.SaverPanePIDOrAbsent` returning all three values verbatim
  - Package godoc at `internal/bootstrapadapter/orphan_sweep.go:3-14` documents tri-state preservation contract

TESTS:
- Status: Adequate
- 19 stub sites in `cmd/bootstrap/orphan_sweep_test.go` updated to three-value shape
- All three arms exercised:
  - `(legitPID, true, nil)` legitimate-present (61, 277)
  - `(0, true, nil)` present-with-pid-0 (LOAD-BEARING — line 482, validates tri-state distinction)
  - `(0, false, nil)` absent (91, 111, 177, 195, 215, 246, 315, 345, 371, 399, 430, 453, 544, 562, 571)
  - `(0, false, err)` error (145, 509, 553)
- No over-testing; mechanical signature widenings

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; `present` bool avoids sentinel overloading — textbook ISP/contract clarity
- Complexity: Low; three-arm switch
- Modern idioms: named returns document intent at type site
- Readability: Good; godoc exceptionally clear about three shapes

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
